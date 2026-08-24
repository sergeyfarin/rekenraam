// Package lockfile holds the advisory lock that says a server process is using
// a database.
//
// Restore needs to know that the app is stopped, and the ways of guessing are
// all wrong: an idle SQLite connection proves nothing, an absent -wal file
// proves nothing (a cleanly stopped database has none), and a PID file alone
// goes stale the moment a process dies badly. Only a lock a live process holds
// answers the question, because the operating system releases it when that
// process ends however it ends.
package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Lock is an acquired advisory lock. Closing it releases the lock; so does the
// process exiting, which is the property that makes it trustworthy.
type Lock struct {
	file *os.File
	path string
}

// ErrLocked means another live process holds the lock.
type ErrLocked struct {
	Path string
	PID  int
}

func (e ErrLocked) Error() string {
	if e.PID > 0 {
		return fmt.Sprintf("%s is locked by process %d", e.Path, e.PID)
	}
	return fmt.Sprintf("%s is locked by another process", e.Path)
}

// Path is the lock file that belongs to a database path.
func Path(databasePath string) string {
	return databasePath + ".lock"
}

// Acquire takes the lock for the life of this process, recording the PID inside
// it so a human can see who holds it.
func Acquire(databasePath string) (*Lock, error) {
	path := Path(databasePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := tryLock(file); err != nil {
		holder := readPID(path)
		file.Close()
		if isLockedErr(err) {
			return nil, ErrLocked{Path: path, PID: holder}
		}
		return nil, err
	}

	if err := file.Truncate(0); err == nil {
		_, _ = file.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
		_ = file.Sync()
	}

	return &Lock{file: file, path: path}, nil
}

// Close releases the lock and removes the file. A failure to remove it is not
// an error worth propagating: the lock is already released, and a stale empty
// file locks nobody out.
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unlock(l.file)
	closeErr := l.file.Close()
	_ = os.Remove(l.path)
	l.file = nil
	if err != nil {
		return err
	}
	return closeErr
}

// CheckAvailable reports whether the lock is free — that is, whether the server
// is stopped. It takes the lock briefly and releases it, which is the only
// honest way to ask.
//
// On a platform without advisory locking it returns ErrUnsupported so a caller
// can refuse rather than assume, because assuming here means restoring over a
// running database.
func CheckAvailable(databasePath string) error {
	lock, err := Acquire(databasePath)
	if err != nil {
		return err
	}
	return lock.Close()
}

func readPID(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return 0
	}
	return pid
}
