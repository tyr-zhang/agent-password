//go:build linux

package platform

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type linuxPlatform struct{}

var current Platform = linuxPlatform{}

func (linuxPlatform) Name() string { return "linux" }

func (linuxPlatform) DataDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(base, "agent-pwd")
	if err := ensurePrivateDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func (linuxPlatform) Listen() (net.Listener, error) { return listenUnix() }

func (linuxPlatform) Dial() (net.Conn, error) { return dialUnix() }

func (linuxPlatform) CheckPeer(c net.Conn) error { return checkPeerPath(c, peerUID) }

// HardenProcess blocks core dumps for the daemon. mlockall is best effort:
// containers usually have a very small RLIMIT_MEMLOCK.
func (linuxPlatform) HardenProcess() error {
	_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
	_ = unix.Mlockall(unix.MCL_CURRENT | unix.MCL_FUTURE)
	dumpable, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	if err == nil && dumpable != 0 {
		return fmt.Errorf("platform: PR_SET_DUMPABLE failed (dumpable=%d)", dumpable)
	}
	return nil
}

func (linuxPlatform) SecurityNote() string {
	return "Linux: no biometric confirmation (master password is required); PR_SET_DUMPABLE blocks core dumps but same-user ptrace remains possible."
}

func (linuxPlatform) ServiceStatus() (string, error) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return "systemd not available; start the daemon manually", nil
	}
	out, err := exec.Command("systemctl", "--user", "is-active", "agent-pwd").CombinedOutput()
	if err != nil {
		return "inactive", nil
	}
	return string(out), nil
}

func (linuxPlatform) InstallService(execPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	unit := fmt.Sprintf(`[Unit]
Description=Agent password credential proxy
[Service]
ExecStart=%s daemon --foreground
Restart=on-failure
[Install]
WantedBy=default.target
`, execPath)
	if err := os.WriteFile(filepath.Join(dir, "agent-pwd.service"), []byte(unit), 0600); err != nil {
		return err
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("platform: systemd not found; unit written, start with: %s daemon", execPath)
	}
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return err
	}
	return exec.Command("systemctl", "--user", "enable", "--now", "agent-pwd").Run()
}

func (linuxPlatform) UninstallService() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command("systemctl", "--user", "disable", "--now", "agent-pwd").Run()
	}
	return os.Remove(filepath.Join(home, ".config", "systemd", "user", "agent-pwd.service"))
}
