//go:build unix

package lockfile

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive advisory lock without blocking, so a caller finds
// out immediately rather than waiting behind a server that is not going to stop
// on its own.
func tryLock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

func isLockedErr(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EACCES)
}
