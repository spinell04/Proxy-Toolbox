// Package bootstrap lays out the files the toolbox expects beside its binary.
//
// The binary is distributed on its own and dropped into an empty folder, so
// everything it needs has to appear on first run. Paths resolve against the
// executable's directory (see internal/basedir), not the working directory, so
// this works the same whether it is double-clicked or run from a shell.
package bootstrap

import (
	_ "embed"
	"fmt"
	"os"

	"proxytoolbox/internal/basedir"
)

// defaultConfig is the template written when no config.txt exists.
//
// A separate file rather than an embed of the repository's own config.txt:
// that one holds real settings, including a Discord webhook, which is a bearer
// credential and must never be compiled into a binary handed to someone else.
//
//go:embed config.default.txt
var defaultConfig []byte

// Dirs are created empty on first run.
//
// results/ is created lazily by the tools that write to it, but proxyfiles/
// cannot be: every tool reads its input from there, so without this the first
// thing a new user does fails with "cannot read proxyfiles/ directory" — the
// folder telling you where to put your input would only appear as a
// side effect of producing output.
var Dirs = []string{"proxyfiles", "results"}

// ConfigFile is written only when absent, never overwritten or migrated.
const ConfigFile = "config.txt"

// Ensure creates whatever is missing beside the binary and reports what it
// made, in the order it made it. An empty result means nothing was needed.
//
// A failure is returned rather than swallowed: if proxyfiles/ cannot be
// created the toolbox has nowhere to read from, and saying so now beats an
// unexplained failure inside the first tool the user picks.
func Ensure() ([]string, error) {
	var created []string

	for _, dir := range Dirs {
		path := basedir.Path(dir)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return created, fmt.Errorf("creating %s/: %w", dir, err)
		}
		created = append(created, dir+"/")
	}

	path := basedir.Path(ConfigFile)
	if _, err := os.Stat(path); err == nil {
		return created, nil
	}
	// O_EXCL, not a plain create: Stat-then-write is a race, and losing it
	// would overwrite settings that another instance had just written.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return created, nil
		}
		return created, fmt.Errorf("creating %s: %w", ConfigFile, err)
	}
	defer f.Close()

	if _, err := f.Write(defaultConfig); err != nil {
		return created, fmt.Errorf("writing %s: %w", ConfigFile, err)
	}
	return append(created, ConfigFile), nil
}
