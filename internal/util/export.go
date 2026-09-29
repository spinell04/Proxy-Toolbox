package util

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"proxytoolbox/internal/basedir"
)

const resultsDir = "results"

// suggestedFilename builds a default export name that identifies the tool, the
// proxy file it was run against and the moment of the run, e.g.
// "pinger_schroeder_2026-09-19_143207.csv".
//
// The proxy file is part of the name because a user comparing runs in the
// dashboard is nearly always comparing providers, and the file is what names
// the provider. Without it, two runs of the same tool are told apart only by a
// timestamp.
func suggestedFilename(tool, proxyFile string, at time.Time) string {
	stamp := at.Format("2006-01-02_150405")
	if tool == "" {
		tool = "results"
	}
	if slug := proxyFileSlug(proxyFile); slug != "" {
		return tool + "_" + slug + "_" + stamp + ".csv"
	}
	return tool + "_" + stamp + ".csv"
}

// proxyFileSlug reduces a proxy file path to a filename-safe fragment: the
// base name, without its extension, with every character that is not a letter,
// digit, dash or underscore replaced by a dash. A dot is replaced too:
// the extension is already gone by then, and a leading one would hide the file.
//
// The sanitising is not cosmetic. The result goes into a filename, so a path
// separator or a leading dot here would escape the results/ directory or hide
// the export. PromptExport's filepath.Base call is the second guard; this is
// the first.
func proxyFileSlug(proxyFile string) string {
	if proxyFile == "" {
		return ""
	}
	base := filepath.Base(proxyFile)
	base = strings.TrimSuffix(base, filepath.Ext(base))

	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	// Dashes at either end read as noise in the middle of a filename, and a
	// leading one would sit directly after the tool's underscore.
	return strings.Trim(b.String(), "-")
}

// PromptExport asks the user if they want to save results to a CSV file.
// Returns the chosen file path, or "" if skipped.
//
// tool names the tool producing the export ("pinger", "iptester", ...) and
// proxyFile is the proxy list it was run against. Both go into the suggested
// filename, which the user accepts by typing ".".
func PromptExport(tool, proxyFile string) string {
	suggested := suggestedFilename(tool, proxyFile, time.Now())

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nSave results to CSV? (Enter to skip, \".\" for %s, or type filename): ", suggested)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if input == "." {
		input = suggested
	}
	if !strings.HasSuffix(strings.ToLower(input), ".csv") {
		input += ".csv"
	}
	input = filepath.Base(input) // strip any directory components (path traversal)

	dir := basedir.Path(resultsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Error creating results/ directory: %v\n", err)
		return ""
	}
	return filepath.Join(dir, input)
}

// WriteCSV writes header + rows to a CSV file.
func WriteCSV(path string, header []string, rows [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
