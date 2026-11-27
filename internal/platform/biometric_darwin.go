//go:build darwin

package platform

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

//go:embed assets/touchid-helper-arm64 assets/touchid-helper-amd64
var touchIDHelpers embed.FS

func (darwinPlatform) BiometricAvailable() bool {
	return true // binary is embedded; actual availability is checked at use time
}

func (darwinPlatform) VerifyBiometric(challenge []byte, reason string) (bool, error) {
	name := "touchid-helper-" + runtime.GOARCH
	data, err := touchIDHelpers.ReadFile("assets/" + name)
	if err != nil {
		return false, fmt.Errorf("%w: no helper for %s: %v", ErrBiometricUnavailable, runtime.GOARCH, err)
	}
	dir, err := os.MkdirTemp("", "agent-pwd-biometric-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(dir)
	helper := filepath.Join(dir, "touchid-helper")
	if err := os.WriteFile(helper, data, 0700); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helper, fmt.Sprintf("%x", challenge), "Confirm high-risk agent-pwd action")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return false, fmt.Errorf("platform: Touch ID timed out: %w", ctx.Err())
	}
	if err != nil {
		return false, fmt.Errorf("%w: Touch ID: %v (%s)", ErrBiometricUnavailable, err, describeClient(string(out)))
	}
	return true, nil
}
