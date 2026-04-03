package util

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"proxytoolbox/internal/basedir"
)

const resultsDir = "results"

// PromptExport asks the user if they want to save results to a CSV file.
// Returns the chosen file path, or "" if skipped.
func PromptExport(defaultName string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nSave results to CSV? (Enter to skip, or type filename): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(input), ".csv") {
		input += ".csv"
	}

	dir := basedir.Path(resultsDir)
	os.MkdirAll(dir, 0755)
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
