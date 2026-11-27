//go:build linux

package platform

import (
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func runtimeLinux() bool { return true }

func sameOwner(st os.FileInfo) bool {
	stat, ok := st.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}

func peerUID(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var inner error
	err = raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if e != nil {
			inner = e
			return
		}
		uid = cred.Uid
	})
	if err != nil {
		return 0, err
	}
	if inner != nil {
		return 0, inner
	}
	return uid, nil
}
