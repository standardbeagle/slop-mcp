//go:build !windows

package filelock

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Lock acquires an exclusive advisory lock associated with path by flock-ing a
// sidecar "<path>.lock" file. It blocks until the lock is available. Release it
// via the returned Unlocker.
func Lock(path string) (Unlocker, error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	return func() error {
		// Closing the fd releases the flock; unlock explicitly first so the
		// intent is clear and the lock drops even if Close is delayed.
		unlockErr := unix.Flock(int(f.Fd()), unix.LOCK_UN)
		closeErr := f.Close()
		if unlockErr != nil {
			return unlockErr
		}
		return closeErr
	}, nil
}
