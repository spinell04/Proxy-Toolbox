//go:build !windows

package update

import (
	"fmt"
	"os"
	"syscall"
)

// restart replaces the current process with the new binary.
//
// syscall.Exec keeps the same PID and the same terminal: the file descriptors,
// the process group and the shell's notion of what it is waiting on all carry
// over, so Ctrl+C behaves exactly as it did before the update. It does not
// return on success.
//
// Spawning a child instead would leave this process alive as a parent for the
// rest of the session and put two processes in the foreground process group,
// where Ctrl+C would reach both.
func restart(exe string, args []string) error {
	if err := syscall.Exec(exe, args, os.Environ()); err != nil {
		return fmt.Errorf("restarting %s: %w", exe, err)
	}
	return nil // unreachable
}
