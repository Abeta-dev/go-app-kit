//go:build unix

package scheduler

import (
	"errors"
	"os"
	"syscall"
)

// tryFlockExclusive attempts a non-blocking OS advisory exclusive lock (LOCK_EX|LOCK_NB)
// on the given open file descriptor. Returns (false, nil) when another open file description
// or OS process already holds the lock.
func tryFlockExclusive(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

// unlockFlock releases an OS advisory lock on the given file descriptor.
func unlockFlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// isProcessAliveOS checks whether pid currently refers to an active OS process via kill(pid, 0).
func isProcessAliveOS(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
