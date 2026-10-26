package auth

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// lockPollInterval is how often lockFile retries a held lock.
const lockPollInterval = 50 * time.Millisecond

// errLockHeld is what tryLock returns when another holder owns the lock.
var errLockHeld = errors.New("lock held")

// lockFile takes an exclusive advisory lock on path (created with mode 0600),
// polling until timeout. The lock belongs to the open file description, so two
// goroutines of one process exclude each other as two processes do. The
// returned func releases the lock and closes the file; the lock file stays.
func lockFile(path string, timeout time.Duration) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening credentials lock: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for {
		err := tryLock(f)
		if err == nil {
			return func() {
				_ = unlock(f)
				_ = f.Close()
			}, nil
		}
		if !errors.Is(err, errLockHeld) {
			_ = f.Close()
			return nil, fmt.Errorf("locking credentials: %w", err)
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("another weside process holds %s for more than %s — retry, or remove the file if no weside process runs", path, timeout)
		}
		time.Sleep(lockPollInterval)
	}
}
