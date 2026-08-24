//go:build !unix

package lockfile

import (
	"errors"
	"os"
)

// ErrUnsupported means this platform has no advisory locking here, so nothing
// can prove the server is stopped. Callers must refuse rather than assume.
var ErrUnsupported = errors.New("advisory locking is not supported on this platform")

func tryLock(file *os.File) error { return ErrUnsupported }

func unlock(file *os.File) error { return nil }

func isLockedErr(err error) bool { return false }
