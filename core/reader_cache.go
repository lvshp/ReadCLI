package core

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
)

const epubSnapshotCacheVersion = 1

type diskSnapshot struct {
	Version      int             `json:"version"`
	Key          string          `json:"key"`
	Path         string          `json:"path"`
	Size         int64           `json:"size"`
	ModifiedAt   int64           `json:"modified_at"`
	Snapshot     reader.Snapshot `json:"snapshot"`
	SnapshotKind string          `json:"snapshot_kind"`
}

func cachedReaderForPath(path string) (reader.Reader, bool, error) {
	path = normalizeBookPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, false, err
	}
	if r, cached, err := cachedReaderIfFresh(path, info); cached || err != nil {
		return r, cached, err
	}
	if r, cached, err := diskCachedReader(path, info); cached || err != nil {
		return r, cached, err
	}

	r, size, modTime, err := loadFreshReader(path)
	if err != nil {
		return nil, false, err
	}
	storeCachedReader(path, r, size, modTime)
	return r, false, nil
}

func cachedReaderIfFresh(path string, info os.FileInfo) (reader.Reader, bool, error) {
	path = normalizeBookPath(path)
	if app != nil && app.readerCache != nil {
		if cached, ok := app.readerCache[path]; ok && cached.reader != nil {
			if cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
				return cached.reader, true, nil
			}
			delete(app.readerCache, path)
		}
	}

	return nil, false, nil
}

func loadFreshReader(path string) (reader.Reader, int64, time.Time, error) {
	path = normalizeBookPath(path)
	r, err := newReaderForPath(path)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	if err := r.Load(path); err != nil {
		return nil, 0, time.Time{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	return r, info.Size(), info.ModTime(), nil
}

func storeCachedReader(path string, r reader.Reader, size int64, modTime time.Time) {
	path = normalizeBookPath(path)
	if app != nil {
		if app.readerCache == nil {
			app.readerCache = map[string]cachedReader{}
		}
		app.readerCache[path] = cachedReader{reader: r, size: size, modTime: modTime}
	}
	storeDiskSnapshot(path, r, size, modTime)
}

func normalizeBookPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func newReaderForPath(path string) (reader.Reader, error) {
	switch strings.ToUpper(filepath.Ext(path)) {
	case ".TXT":
		return reader.NewTxtReader(), nil
	case ".EPUB":
		return reader.NewEpubReader(), nil
	default:
		return nil, fmt.Errorf("unsupported file format: %s", filepath.Ext(path))
	}
}

func diskCachedReader(path string, info os.FileInfo) (reader.Reader, bool, error) {
	if !supportsDiskSnapshot(path) {
		return nil, false, nil
	}
	entry, err := loadDiskSnapshot(path)
	if err != nil || entry == nil {
		return nil, false, nil
	}
	if entry.Version != epubSnapshotCacheVersion {
		return nil, false, nil
	}
	expectedKey := diskSnapshotKey(path, info.Size(), info.ModTime())
	if entry.Key != expectedKey || entry.Path != path || entry.Size != info.Size() || entry.ModifiedAt != info.ModTime().UnixNano() {
		return nil, false, nil
	}

	width := 0
	if app != nil {
		width = app.readingState.contentWidth
	}
	r, err := reader.ReaderFromSnapshot(entry.Snapshot, width)
	if err != nil {
		return nil, false, nil
	}
	storeMemoryCachedReader(path, r, info.Size(), info.ModTime())
	return r, true, nil
}

func storeDiskSnapshot(path string, r reader.Reader, size int64, modTime time.Time) {
	if !supportsDiskSnapshot(path) {
		return
	}
	snapshot, ok := reader.SnapshotFromReader(r)
	if !ok || snapshot == nil || snapshot.Kind != "epub" {
		return
	}
	cachePath, err := diskSnapshotPath(path)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return
	}
	entry := diskSnapshot{
		Version:      epubSnapshotCacheVersion,
		Key:          diskSnapshotKey(path, size, modTime),
		Path:         path,
		Size:         size,
		ModifiedAt:   modTime.UnixNano(),
		Snapshot:     *snapshot,
		SnapshotKind: snapshot.Kind,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = os.WriteFile(cachePath, data, 0644)
}

func loadDiskSnapshot(path string) (*diskSnapshot, error) {
	cachePath, err := diskSnapshotPath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	var entry diskSnapshot
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func diskSnapshotPath(path string) (string, error) {
	dataDir, err := lib.DataDirPath()
	if err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(normalizeBookPath(path)))
	return filepath.Join(dataDir, "cache", hex.EncodeToString(sum[:])+".json"), nil
}

func diskSnapshotKey(path string, size int64, modTime time.Time) string {
	return fmt.Sprintf("v%d|%s|%d|%d", epubSnapshotCacheVersion, normalizeBookPath(path), size, modTime.UnixNano())
}

func supportsDiskSnapshot(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".epub")
}

func storeMemoryCachedReader(path string, r reader.Reader, size int64, modTime time.Time) {
	if app == nil {
		return
	}
	if app.readerCache == nil {
		app.readerCache = map[string]cachedReader{}
	}
	app.readerCache[path] = cachedReader{reader: r, size: size, modTime: modTime}
}
