//go:build windows

package cli

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/tyr-zhang/agent-password/internal/platform"
	"golang.org/x/sys/windows"
)

func detachDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := platform.DataDir()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(dir, "daemon.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "daemon", "--foreground")
	cmd.Stdout, cmd.Stderr = log, log
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		log.Close()
		return err
	}
	return log.Close()
}

func installSignals(ctx context.Context, stop context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-ch:
			stop()
		case <-ctx.Done():
			signal.Stop(ch)
		}
	}()
}
