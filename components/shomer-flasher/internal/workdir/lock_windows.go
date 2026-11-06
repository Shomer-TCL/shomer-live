//go:build windows

package workdir

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// The lock is one byte far past the end of the file, so the file itself (the
// holder's process id) stays readable for the second copy's message.
const lockOffset = 0x7fff0000

func lockFile(f *os.File) error {
	ol := &windows.Overlapped{Offset: lockOffset}
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return errLocked
	}
	return err
}

func unlockFile(f *os.File) {
	ol := &windows.Overlapped{Offset: lockOffset}
	windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
