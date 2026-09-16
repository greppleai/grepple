//go:build windows

package metrics

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lockJournalFile(file *os.File, exclusive bool) error {
	var flags uint32
	if exclusive {
		flags = windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.Errno(0)) {
		return nil
	}
	return err
}

func unlockJournalFile(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
	if errors.Is(err, windows.Errno(0)) {
		return nil
	}
	return err
}
