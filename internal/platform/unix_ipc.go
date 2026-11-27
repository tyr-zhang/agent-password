//go:build !windows

package platform

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const socketName = "agent-pwd.sock"

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("platform: create data dir: %w", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("platform: insecure data dir %s (want 0700, got %#o)", path, st.Mode().Perm())
	}
	return nil
}

// socketDir returns the directory used for the IPC socket. Long sun_path
// values fall back to a short, private path in the OS temp directory.
func socketDir(dataDir string) (string, error) {
	preferred := dataDir
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" && runtimeLinux() {
		preferred = filepath.Join(d, "agent-pwd-"+fmt.Sprint(os.Getuid()))
	}
	candidate := filepath.Join(preferred, socketName)
	if len(candidate) <= 100 {
		if err := ensurePrivateDir(preferred); err != nil {
			return "", err
		}
		return preferred, nil
	}
	fallback := filepath.Join(os.TempDir(), fmt.Sprintf("agent-pwd-%d", os.Getuid()))
	if err := ensurePrivateDir(fallback); err != nil {
		return "", err
	}
	return fallback, nil
}

func socketPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	d, err := socketDir(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, socketName), nil
}

func listenUnix() (net.Listener, error) {
	p, err := socketPath()
	if err != nil {
		return nil, err
	}
	if st, err := os.Lstat(p); err == nil {
		if st.Mode()&os.ModeSocket == 0 || !sameOwner(st) {
			return nil, fmt.Errorf("platform: refusing to remove %s: not our socket", p)
		}
		if err := os.Remove(p); err != nil {
			return nil, err
		}
	}
	ln, err := net.Listen("unix", p)
	if err != nil {
		return nil, fmt.Errorf("platform: listen on socket: %w", err)
	}
	if err := os.Chmod(p, 0600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func dialUnix() (net.Conn, error) {
	p, err := socketPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(p); err != nil {
		return nil, err
	}
	return net.Dial("unix", p)
}

func checkPeerPath(conn net.Conn, peerUID func(*net.UnixConn) (uint32, error)) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("platform: unexpected connection type %T", conn)
	}
	uid, err := peerUID(uc)
	if err != nil {
		return fmt.Errorf("platform: peer credentials unavailable: %w", err)
	}
	if uid != uint32(os.Getuid()) {
		return fmt.Errorf("platform: peer uid %d does not match %d", uid, os.Getuid())
	}
	return nil
}

func describeClient(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if len(name) > 160 {
		name = name[:160]
	}
	return name
}
