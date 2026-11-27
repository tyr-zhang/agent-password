// Package platform contains the only OS-specific code. All other packages
// share the same business logic and build with CGO_ENABLED=0.
package platform

import (
	"errors"
	"net"
	"os"
	"path/filepath"
)

var ErrBiometricUnavailable = errors.New("platform: biometric confirmation unavailable")

type Platform interface {
	Name() string
	DataDir() (string, error)
	Listen() (net.Listener, error)
	Dial() (net.Conn, error)
	CheckPeer(net.Conn) error
	HardenProcess() error
	ServiceStatus() (string, error)
	InstallService(execPath string) error
	UninstallService() error
	BiometricAvailable() bool
	VerifyBiometric(challenge []byte, reason string) (bool, error)
	SecurityNote() string
}

func Current() Platform { return current }

// DataDir resolves the data directory. AGENT_PWD_HOME overrides it for tests
// and for users who want to relocate the vault.
func DataDir() (string, error) {
	if h := os.Getenv("AGENT_PWD_HOME"); h != "" {
		abs, err := filepath.Abs(h)
		if err != nil {
			return "", err
		}
		if err := ensurePrivateDir(abs); err != nil {
			return "", err
		}
		return abs, nil
	}
	return current.DataDir()
}
