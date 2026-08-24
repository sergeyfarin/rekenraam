//go:build !unix

package app

// availableBytes has no portable implementation outside unix. Reporting
// "unknown" rather than guessing keeps the preflight advisory everywhere: it
// was never a guarantee, since space can disappear during the copy anyway.
func availableBytes(path string) (int64, bool) {
	return 0, false
}
