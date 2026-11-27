//go:build darwin

package platform

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type darwinPlatform struct{}

var current Platform = darwinPlatform{}

func (darwinPlatform) Name() string { return "darwin" }

func (darwinPlatform) DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "Application Support", "agent-pwd")
	if err := ensurePrivateDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func (darwinPlatform) Listen() (net.Listener, error) { return listenUnix() }

func (darwinPlatform) Dial() (net.Conn, error) { return dialUnix() }

func (darwinPlatform) CheckPeer(c net.Conn) error { return checkPeerPath(c, peerUID) }

// HardenProcess is best effort: Go's heap cannot be reliably mlocked, so the
// mlock call is applied to a small guard buffer only.
func (darwinPlatform) HardenProcess() error {
	buf := make([]byte, 4096)
	_ = unix.Mlock(buf)
	return nil
}

func (darwinPlatform) SecurityNote() string {
	return "macOS: Touch ID confirmation via a bundled helper; same-user malware is outside the trust model."
}

func (darwinPlatform) ServiceStatus() (string, error) {
	out, err := exec.Command("launchctl", "list", "com.tyrzhang.agent-pwd").CombinedOutput()
	if err != nil {
		return "not installed", nil
	}
	return string(out), nil
}

func (darwinPlatform) InstallService(execPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir, err := DataDir()
	if err != nil {
		return err
	}
	label := "com.tyrzhang.agent-pwd"
	plist := launchAgentPlist(label, execPath, filepath.Join(dir, "daemon.log"))
	target := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(target, []byte(plist), 0644); err != nil {
		return err
	}
	return exec.Command("launchctl", "load", target).Run()
}

func (darwinPlatform) UninstallService() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	target := filepath.Join(home, "Library", "LaunchAgents", "com.tyrzhang.agent-pwd.plist")
	_ = exec.Command("launchctl", "unload", target).Run()
	return os.Remove(target)
}

// launchAgentPlist must run the daemon in the foreground. The non-detaching
// `agent-pwd daemon` form forks and exits immediately, which makes launchd's
// KeepAlive spawn a new daemon in an endless loop and lose session state.
func launchAgentPlist(label, execPath, logPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>daemon</string><string>--foreground</string></array>
  <key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, label, execPath, logPath, logPath)
}
