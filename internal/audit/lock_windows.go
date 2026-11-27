//go:build windows

package audit

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(f *os.File) error {
	// LockFileEx is asynchronous on Windows; without an event handle,
	// GetOverlappedResult(wait=true) can block on a non-waitable file handle.
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return fmt.Errorf("audit: lock event: %w", err)
	}
	defer windows.CloseHandle(event)
	var overlapped windows.Overlapped
	overlapped.HEvent = event
	h := windows.Handle(f.Fd())
	err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped)
	if err == windows.ERROR_IO_PENDING {
		err = windows.GetOverlappedResult(h, &overlapped, nil, true)
	}
	if err != nil {
		return fmt.Errorf("audit: lock log: %w", err)
	}
	return nil
}

func unlockFile(f *os.File) {
	var overlapped windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped)
}
