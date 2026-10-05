//go:build windows

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// restart runs the new binary and exits with its status.
//
// Windows has no exec that replaces the running image, so this spawns the new
// binary as a child with the console handles inherited — the TUI draws to the
// same window and keyboard input reaches it — waits for it, and exits with its
// code. It does not return on success.
//
// The parent lingers for the session as a result. That is the cost of the
// platform: it holds no proxy connections and does no work, and only one
// update happens per launch, so the nesting cannot accumulate.
//
// Ctrl+C is worse here than on unix, and not only because of the extra
// process. A console control event goes to every process attached to the
// console, so the parent receives it too and, with Go's default handler,
// can exit before the child has finished shutting down. Nothing depends on
// the parent outliving the child today, so this is recorded rather than
// handled: installing a handler to ignore the event in the parent is the
// fix if it ever matters.
func restart(exe string, args []string) error {
	cmd := exec.Command(exe, args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("restarting %s: %w", exe, err)
	}
	os.Exit(0)
	return nil // unreachable
}
