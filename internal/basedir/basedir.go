package basedir

import (
	"os"
	"path/filepath"
	"strings"
)

// Root is the directory containing the executable (or the working directory
// when running via `go run`). All relative paths (config.txt, proxyfiles/)
// are resolved against this.
var Root string

func init() {
	// Try executable path first (works for compiled binaries)
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		// go run places the binary in a temp/cache dir — detect and fall back to cwd
		if !isGoRunTemp(dir) {
			Root = dir
			return
		}
	}
	Root, _ = os.Getwd()
}

// Path resolves a relative path against the project root.
func Path(rel string) string {
	return filepath.Join(Root, rel)
}

// isGoRunTemp detects if a path is inside Go's temp/cache build directory.
func isGoRunTemp(dir string) bool {
	tmp := os.TempDir()
	cache, _ := os.UserCacheDir()
	return strings.HasPrefix(dir, tmp) ||
		(cache != "" && strings.HasPrefix(dir, cache))
}
