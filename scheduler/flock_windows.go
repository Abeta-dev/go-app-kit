//go:build !unix

package scheduler

import "os"

// tryFlockExclusive falls back to lease-record coordination on non-Unix platforms
// where POSIX flock(2) is unavailable in the Go standard library.
func tryFlockExclusive(_ *os.File) (bool, error) {
	return true, nil
}

// unlockFlock is a no-op on non-Unix platforms without POSIX flock(2).
func unlockFlock(_ *os.File) error {
	return nil
}

// isProcessAliveOS verifies whether pid exists on non-Unix platforms.
func isProcessAliveOS(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.FindProcess(pid)
	return err == nil
}
