//go:build linux

package platform

func (linuxPlatform) BiometricAvailable() bool { return false }

func (linuxPlatform) VerifyBiometric([]byte, string) (bool, error) {
	return false, ErrBiometricUnavailable
}
