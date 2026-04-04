//go:build windows

package core

import (
	"os"
	"os/exec"
	"os/signal"
)

func runBossCommand(command string) error {
	cmd := exec.Command("cmd.exe", "/c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-sigCh:
		_ = cmd.Process.Kill()
		<-done
		return nil
	}
}
