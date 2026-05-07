//go:build !windows

package core

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

func runBossCommand(command string) error {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-lc", command)
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

	interrupted := false
	for {
		select {
		case err := <-done:
			if interrupted && isInterruptExit(err) {
				return nil
			}
			return err
		case <-sigCh:
			interrupted = true
		}
	}
}

func isInterruptExit(err error) bool {
	if err == nil {
		return false
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return false
	}
	if status.Signaled() {
		return status.Signal() == syscall.SIGINT
	}
	return status.Exited() && status.ExitStatus() == 130
}
