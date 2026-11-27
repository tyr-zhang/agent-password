//go:build windows

package platform

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode/utf16"
)

const helloScript = `$ErrorActionPreference = 'Stop'
try {
  Add-Type -AssemblyName System.Runtime.WindowsRuntime
  $verifier = [Windows.Security.Credentials.UI.UserConsentVerifier, Windows.Security.Credentials.UI, ContentType=WindowsRuntime]
  $op = $verifier::RequestVerificationAsync('Confirm a high-risk agent-pwd action')
  $result = $op.AsTask().GetAwaiter().GetResult()
  if ($result.ToString() -eq 'Verified') { Write-Output 'OK'; exit 0 }
  exit 3
} catch { exit 4 }`

func encodePowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	b := make([]byte, 0, len(units)*2)
	for _, u := range units {
		b = append(b, byte(u), byte(u>>8))
	}
	return base64.StdEncoding.EncodeToString(b)
}

func (windowsPlatform) BiometricAvailable() bool {
	_, err := exec.LookPath("powershell.exe")
	return err == nil
}

func (windowsPlatform) VerifyBiometric([]byte, string) (bool, error) {
	if !Current().BiometricAvailable() {
		return false, ErrBiometricUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShell(helloScript))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%w: Windows Hello: %v (%s)", ErrBiometricUnavailable, err, describeClient(string(out)))
	}
	return strings.TrimSpace(string(out)) == "OK", nil
}
