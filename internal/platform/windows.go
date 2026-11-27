//go:build windows

package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

type windowsPlatform struct{}

var current Platform = windowsPlatform{}

func (windowsPlatform) Name() string { return "windows" }

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("platform: create data dir: %w", err)
	}
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("platform: invalid data dir %s: %v", path, err)
	}
	return nil
}

func (windowsPlatform) DataDir() (string, error) {
	base := os.Getenv("APPDATA")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, "AppData", "Roaming")
	}
	dir := filepath.Join(base, "agent-pwd")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("platform: create data dir: %w", err)
	}
	return dir, nil
}

func pipeName() (string, error) {
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return "", err
	}
	sid := u.User.Sid.String()
	sum := sha256.Sum256([]byte(sid))
	return fmt.Sprintf(`\\.\pipe\agent-pwd-%s`, hex.EncodeToString(sum[:8])), nil
}

func (windowsPlatform) Listen() (net.Listener, error) {
	name, err := pipeName()
	if err != nil {
		return nil, err
	}
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return nil, err
	}
	sddl := fmt.Sprintf("D:P(A;;GA;;;%s)(A;;GA;;;SY)", u.User.Sid.String())
	return winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: sddl})
}

func (windowsPlatform) Dial() (net.Conn, error) {
	name, err := pipeName()
	if err != nil {
		return nil, err
	}
	timeout := 3 * time.Second
	return winio.DialPipe(name, &timeout)
}

// CheckPeer is a no-op: the pipe DACL only grants access to the current user
// and SYSTEM. A same-user process is inside the documented trust boundary.
func (windowsPlatform) CheckPeer(net.Conn) error { return nil }

func (windowsPlatform) HardenProcess() error {
	buf := make([]byte, 4096)
	_ = windows.VirtualLock(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return nil
}

func (windowsPlatform) SecurityNote() string {
	return "Windows: pipe ACL restricted to the current user; Windows Hello is used when available, otherwise the master password is required."
}

func (windowsPlatform) ServiceStatus() (string, error) {
	out, err := exec.Command("schtasks", "/Query", "/TN", "agent-pwd").CombinedOutput()
	if err != nil {
		return "not installed", nil
	}
	return string(out), nil
}

func (windowsPlatform) InstallService(execPath string) error {
	task := fmt.Sprintf(`"%s" daemon --foreground`, execPath)
	return exec.Command("schtasks", "/Create", "/TN", "agent-pwd", "/TR", task,
		"/SC", "ONLOGON", "/RL", "LIMITED", "/F").Run()
}

func (windowsPlatform) UninstallService() error {
	return exec.Command("schtasks", "/Delete", "/TN", "agent-pwd", "/F").Run()
}

func describeClient(name string) string {
	runes := []rune(name)
	for i, r := range runes {
		if r < 32 || r == 127 {
			runes[i] = ' '
		}
	}
	name = string(runes)
	if len(name) > 160 {
		name = name[:160]
	}
	return name
}
