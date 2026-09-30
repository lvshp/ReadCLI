package lib

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const readcliRepo = "lvshp/ReadCLI"

type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ReleaseInfo struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Body    string         `json:"body"`
	Assets  []ReleaseAsset `json:"assets"`
}

const (
	UpdateStageDownload = "download"
	UpdateStageExtract  = "extract"
	UpdateStagePrepare  = "prepare"
	UpdateStageReplace  = "replace"
)

type UpdateProgress struct {
	Stage      string
	Downloaded int64
	Total      int64
}

type UpdateProgressFunc func(UpdateProgress)

type UpdateInstallError struct {
	Message string
	TempDir string
}

func (e *UpdateInstallError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.TempDir) == "" {
		return e.Message
	}
	return fmt.Sprintf("%s。临时文件保留在：%s", e.Message, e.TempDir)
}

func FetchLatestRelease(version string) (*ReleaseInfo, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+readcliRepo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ReadCLI/"+strings.TrimSpace(version))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("检查更新失败: %s", resp.Status)
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func ShouldOfferUpdate(currentVersion, latestVersion string) bool {
	currentVersion = strings.TrimSpace(currentVersion)
	latestVersion = strings.TrimSpace(latestVersion)
	if currentVersion == "" || latestVersion == "" || currentVersion == latestVersion {
		return false
	}

	currentSemver, currentOK := parseSemverLike(currentVersion)
	latestSemver, latestOK := parseSemverLike(latestVersion)
	if currentOK && latestOK {
		for i := 0; i < 3; i++ {
			if latestSemver[i] > currentSemver[i] {
				return true
			}
			if latestSemver[i] < currentSemver[i] {
				return false
			}
		}
		return false
	}
	if latestOK && !currentOK {
		return false
	}

	return latestVersion != currentVersion
}

func SelectReleaseAsset(release *ReleaseInfo, goos, goarch string) *ReleaseAsset {
	if release == nil {
		return nil
	}
	prefix := fmt.Sprintf("readcli-%s-%s-", goos, goarch)
	for i := range release.Assets {
		asset := &release.Assets[i]
		if strings.HasPrefix(asset.Name, prefix) &&
			(strings.HasSuffix(asset.Name, ".tar.gz") || strings.HasSuffix(asset.Name, ".zip")) {
			return asset
		}
	}
	return nil
}

func InstallLatestReleaseAsset(version, url, executablePath string) error {
	return InstallLatestReleaseAssetWithProgress(version, url, executablePath, nil)
}

// On Windows, nil means the update helper has prepared the new executable and
// will install it after this process exits. Other platforms install immediately.
func InstallLatestReleaseAssetWithProgress(version, url, executablePath string, onProgress UpdateProgressFunc) error {
	var prepareWindows func(string, string, string) error
	if runtime.GOOS == "windows" {
		prepareWindows = startWindowsDeferredUpdate
	}
	return installReleaseAssetWithClient(version, url, executablePath, onProgress, &http.Client{Timeout: 90 * time.Second}, prepareWindows)
}

func installReleaseAssetWithClient(version, url, executablePath string, onProgress UpdateProgressFunc, client *http.Client, prepareWindows func(string, string, string) error) error {
	tempDir, err := os.MkdirTemp("", "readcli-update-*")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tempDir)
		}
	}()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "ReadCLI/"+strings.TrimSpace(version))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载更新失败: %s", resp.Status)
	}

	isZip := strings.HasSuffix(url, ".zip")
	archiveExt := ".tar.gz"
	if isZip {
		archiveExt = ".zip"
	}
	archivePath := filepath.Join(tempDir, "readcli-update"+archiveExt)
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	if _, err := copyWithProgress(archiveFile, resp.Body, resp.ContentLength, onProgress); err != nil {
		archiveFile.Close()
		cleanup = false
		return &UpdateInstallError{Message: "写入更新包失败: " + err.Error(), TempDir: tempDir}
	}
	if err := archiveFile.Close(); err != nil {
		cleanup = false
		return &UpdateInstallError{Message: "保存更新包失败: " + err.Error(), TempDir: tempDir}
	}

	info, statErr := os.Stat(executablePath)
	fileMode := os.FileMode(0755)
	if statErr == nil {
		fileMode = info.Mode()
	}

	binaryName := "readcli"
	if isZip {
		binaryName = "readcli.exe"
	}
	binaryPath := filepath.Join(tempDir, binaryName)
	binaryFile, err := os.Create(binaryPath)
	if err != nil {
		cleanup = false
		return &UpdateInstallError{Message: "创建临时二进制失败: " + err.Error(), TempDir: tempDir}
	}
	archiveReader, err := os.Open(archivePath)
	if err != nil {
		binaryFile.Close()
		cleanup = false
		return &UpdateInstallError{Message: "打开更新包失败: " + err.Error(), TempDir: tempDir}
	}

	reportUpdateProgress(onProgress, UpdateProgress{Stage: UpdateStageExtract})
	var extractErr error
	if isZip {
		extractErr = extractBinaryFromZip(archiveReader, binaryFile)
	} else {
		extractErr = extractBinaryFromTarGz(archiveReader, binaryFile)
	}
	archiveReader.Close()
	if extractErr != nil {
		binaryFile.Close()
		cleanup = false
		return &UpdateInstallError{Message: extractErr.Error(), TempDir: tempDir}
	}
	if err := binaryFile.Chmod(fileMode); err != nil {
		binaryFile.Close()
		cleanup = false
		return &UpdateInstallError{Message: "设置更新文件权限失败: " + err.Error(), TempDir: tempDir}
	}
	if err := binaryFile.Close(); err != nil {
		cleanup = false
		return &UpdateInstallError{Message: "关闭更新文件失败: " + err.Error(), TempDir: tempDir}
	}

	if prepareWindows != nil {
		reportUpdateProgress(onProgress, UpdateProgress{Stage: UpdateStagePrepare})
		if err := prepareWindows(binaryPath, executablePath, tempDir); err != nil {
			cleanup = false
			return &UpdateInstallError{Message: "准备退出后更新失败: " + err.Error(), TempDir: tempDir}
		}
		// The helper owns the download directory until the parent has exited.
		cleanup = false
		return nil
	}
	reportUpdateProgress(onProgress, UpdateProgress{Stage: UpdateStageReplace})
	if err := replaceExecutable(binaryPath, executablePath); err != nil {
		cleanup = false
		return &UpdateInstallError{Message: "覆盖当前二进制失败: " + err.Error(), TempDir: tempDir}
	}
	return nil
}

func copyWithProgress(dst io.Writer, src io.Reader, total int64, onProgress UpdateProgressFunc) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64
	var lastReport time.Time

	report := func(force bool) {
		if onProgress == nil {
			return
		}
		now := time.Now()
		if !force && now.Sub(lastReport) < 200*time.Millisecond {
			return
		}
		lastReport = now
		onProgress(UpdateProgress{
			Stage:      UpdateStageDownload,
			Downloaded: written,
			Total:      total,
		})
	}

	report(true)
	for {
		read, readErr := src.Read(buffer)
		if read > 0 {
			wrote, writeErr := dst.Write(buffer[:read])
			if wrote > 0 {
				written += int64(wrote)
				report(false)
			}
			if writeErr != nil {
				report(true)
				return written, writeErr
			}
			if wrote != read {
				report(true)
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				report(true)
				return written, nil
			}
			report(true)
			return written, readErr
		}
	}
}

func reportUpdateProgress(onProgress UpdateProgressFunc, progress UpdateProgress) {
	if onProgress != nil {
		onProgress(progress)
	}
}

func CurrentExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func CurrentPlatformSupported() bool {
	asset := SelectReleaseAsset(&ReleaseInfo{
		Assets: []ReleaseAsset{
			{Name: "readcli-darwin-amd64-v0.0.0.tar.gz"},
			{Name: "readcli-darwin-arm64-v0.0.0.tar.gz"},
			{Name: "readcli-linux-amd64-v0.0.0.tar.gz"},
			{Name: "readcli-windows-amd64-v0.0.0.zip"},
		},
	}, runtime.GOOS, runtime.GOARCH)
	return asset != nil
}

func extractBinaryFromTarGz(source io.Reader, target io.Writer) error {
	gzReader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("更新包中未找到 readcli 可执行文件")
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(header.Name) != "readcli" {
			continue
		}
		_, err = io.Copy(target, tarReader)
		return err
	}
}

func extractBinaryFromZip(source io.Reader, target io.Writer) error {
	reader, ok := source.(io.ReaderAt)
	var size int64
	if !ok {
		data, err := io.ReadAll(source)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
		size = int64(len(data))
	} else if seeker, seekOk := source.(io.Seeker); seekOk {
		current, _ := seeker.Seek(0, io.SeekCurrent)
		end, _ := seeker.Seek(0, io.SeekEnd)
		seeker.Seek(current, io.SeekStart)
		size = end - current
	}
	zipReader, err := zip.NewReader(reader, size)
	if err != nil {
		return err
	}
	for _, f := range zipReader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(f.Name) != "readcli.exe" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		_, err = io.Copy(target, rc)
		rc.Close()
		return err
	}
	return errors.New("更新包中未找到 readcli.exe 可执行文件")
}

func parseSemverLike(version string) ([3]int, bool) {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	version = strings.TrimSpace(strings.TrimPrefix(version, "V"))
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return [3]int{}, false
	}

	var out [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		value, err := strconv.Atoi(parts[i])
		if err != nil || value < 0 {
			return [3]int{}, false
		}
		out[i] = value
	}
	return out, true
}

// replaceExecutable stages a complete copy beside the installed executable so
// every rename stays on the same volume, even when the download is in TEMP.
// Keep the downloaded file intact for recovery if installation fails.
func replaceExecutable(newPath, oldPath string) error {
	source, err := os.Open(newPath)
	if err != nil {
		return fmt.Errorf("打开下载的程序文件失败: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("读取下载的程序文件属性失败: %w", err)
	}
	stagedPath, err := stageExecutable(source, oldPath, info.Mode())
	if err != nil {
		return err
	}
	defer os.Remove(stagedPath)
	return replaceStagedExecutable(stagedPath, oldPath, runtime.GOOS == "windows", os.Rename)
}

func stageExecutable(source io.Reader, executablePath string, mode os.FileMode) (path string, err error) {
	file, err := os.CreateTemp(filepath.Dir(executablePath), "."+filepath.Base(executablePath)+".update-*")
	if err != nil {
		return "", fmt.Errorf("在程序安装目录创建暂存文件失败: %w", err)
	}
	path = file.Name()
	defer func() {
		if err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
		}
	}()
	if _, err = io.Copy(file, source); err != nil {
		return "", fmt.Errorf("写入安装目录暂存文件失败: %w", err)
	}
	if err = file.Chmod(mode); err != nil {
		return "", fmt.Errorf("设置暂存文件权限失败: %w", err)
	}
	if err = file.Sync(); err != nil {
		return "", fmt.Errorf("同步暂存文件失败: %w", err)
	}
	if err = file.Close(); err != nil {
		return "", fmt.Errorf("关闭暂存文件失败: %w", err)
	}
	return path, nil
}

// The rename parameter allows fault tests to exercise the Windows recovery
// sequence on every platform without changing process-wide filesystem hooks.
func replaceStagedExecutable(stagedPath, oldPath string, windows bool, rename func(string, string) error) error {
	if !windows {
		if err := rename(stagedPath, oldPath); err != nil {
			return fmt.Errorf("替换程序文件失败: %w", err)
		}
		return nil
	}

	// A previous process may still be using its backup. A unique name avoids
	// making that process (or a leftover .old file) block this installation.
	backup, err := os.CreateTemp(filepath.Dir(oldPath), "."+filepath.Base(oldPath)+".backup-*")
	if err != nil {
		return fmt.Errorf("创建旧程序备份路径失败: %w", err)
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("关闭旧程序备份占位文件失败: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("准备旧程序备份路径失败: %w", err)
	}
	if err := rename(oldPath, backupPath); err != nil {
		return fmt.Errorf("备份当前程序失败: %w", err)
	}
	if err := rename(stagedPath, oldPath); err != nil {
		if rollbackErr := rename(backupPath, oldPath); rollbackErr != nil {
			return errors.Join(
				fmt.Errorf("替换程序文件失败: %w", err),
				fmt.Errorf("恢复旧程序失败，旧程序备份保留在：%s: %w", backupPath, rollbackErr),
			)
		}
		return fmt.Errorf("替换程序文件失败，已恢复旧程序: %w", err)
	}
	// The running Windows image can keep this backup locked until exit. Its
	// cleanup is best effort and must not turn a successful update into failure.
	_ = os.Remove(backupPath)
	return nil
}
