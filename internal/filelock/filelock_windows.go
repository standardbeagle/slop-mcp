//go:build windows

package filelock

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// Lock acquires an exclusive lock associated with path by LockFileEx-ing a
// sidecar "<path>.lock" file. It blocks until the lock is available. Release it
// via the returned Unlocker.
func Lock(path string) (Unlocker, error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	// Lock a single byte exclusively; the range is nominal (advisory mutual
	// exclusion between our own processes).
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	return func() error {
		unlockErr := windows.UnlockFileEx(h, 0, 1, 0, ol)
		closeErr := f.Close()
		if unlockErr != nil {
			return unlockErr
		}
		return closeErr
	}, nil
}
