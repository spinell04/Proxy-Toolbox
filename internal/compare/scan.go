package compare

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScanDir parses every .csv in dir, newest run first.
//
// A file that fails to parse is skipped rather than failing the whole scan —
// one hand-edited CSV must not take the dashboard down. A missing directory
// yields no runs and no error, which is the state before a first export.
func ScanDir(dir string) ([]Run, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}

	var runs []Run
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			continue
		}
		run, err := ParseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		runs = append(runs, run)
	}

	// Newest first. Runs without metadata sort last, since their zero time
	// carries no information.
	sort.SliceStable(runs, func(i, j int) bool {
		ti, tj := runs[i].Meta.RunAt, runs[j].Meta.RunAt
		if ti.IsZero() != tj.IsZero() {
			return !ti.IsZero()
		}
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return runs[i].File < runs[j].File
	})
	return runs, nil
}
