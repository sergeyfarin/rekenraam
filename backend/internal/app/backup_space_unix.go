//go:build unix

package app

import "syscall"

// availableBytes reports free space on the filesystem holding path.
//
// It must be asked about the *backup* directory, not the database: on a
// well-configured deployment those are deliberately different devices, and
// checking the wrong one answers a question nobody asked.
func availableBytes(path string) (int64, bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, false
	}
	return int64(stat.Bavail) * int64(stat.Bsize), true
}
