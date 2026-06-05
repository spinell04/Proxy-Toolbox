package util

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"proxytoolbox/internal/basedir"
)

const proxyFilesDir = "proxyfiles"

// PromptProxyFile asks whether to save filtered proxies to a .txt under
// proxyfiles/. label describes the active filter (e.g. "latency < 1500ms").
// Returns the chosen path, or "" if skipped.
func PromptProxyFile(label string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nSave filtered proxies (%s) to proxyfiles/? (Enter to skip, or type filename): ", label)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(input), ".txt") {
		input += ".txt"
	}
	input = filepath.Base(input) // strip any directory components (path traversal)
	dir := basedir.Path(proxyFilesDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Error creating proxyfiles/ directory: %v\n", err)
		return ""
	}
	return filepath.Join(dir, input)
}

// WriteLines writes one string per line (trailing newline each).
func WriteLines(path string, lines []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, l := range lines {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	return w.Flush()
}
