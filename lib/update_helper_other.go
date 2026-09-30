//go:build !windows

package lib

import "errors"

func RunUpdateHelper(args []string) (bool, int) { return false, 0 }

func startWindowsDeferredUpdate(binaryPath, executablePath, tempDir string) error {
	return errors.New("退出后更新助手仅适用于 Windows")
}
