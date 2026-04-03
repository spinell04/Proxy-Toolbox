package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"

	"proxytoolbox/internal/basedir"
)

const dirName = "proxyfiles"

// SelectFile lists .txt files in proxyfiles/ and prompts the user to pick one.
func SelectFile() (string, error) {
	dir := basedir.Path(dirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("cannot read %s/ directory: %w", dirName, err)
	}

	var options []huh.Option[string]
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".txt") {
			path := filepath.Join(dir, e.Name())
			options = append(options, huh.NewOption(e.Name(), path))
		}
	}
	if len(options) == 0 {
		return "", fmt.Errorf("no .txt files found in %s/", dirName)
	}

	var selected string
	err = huh.NewSelect[string]().
		Title("Select a proxy file").
		Options(options...).
		Value(&selected).
		Run()

	if err != nil {
		return "", fmt.Errorf("selection cancelled")
	}
	return selected, nil
}
