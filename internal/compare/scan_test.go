package compare

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRun copies the pinger fixture into dir under name, rewriting its
// "Run at" row. An empty runAt drops the row entirely, producing a run
// without a usable timestamp.
func writeRun(t *testing.T, dir, name, runAt string) {
	t.Helper()
	src, err := os.ReadFile("testdata/pinger_full.csv")
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(string(src), "\n")
	out := make([]string, 0, len(lines))
	replaced := false
	for _, line := range lines {
		if strings.HasPrefix(line, "Run at,") {
			replaced = true
			if runAt == "" {
				continue
			}
			line = "Run at," + runAt
		}
		out = append(out, line)
	}
	if !replaced {
		t.Fatal("fixture no longer has a Run at row; writeRun needs updating")
	}

	if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(out, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanDir_ParsesEveryCSV(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile("testdata/pinger_full.csv")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.csv", "b.CSV", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2 (non-CSV ignored)", len(runs))
	}
	got := map[string]bool{runs[0].File: true, runs[1].File: true}
	if !got["a.csv"] || !got["b.CSV"] {
		t.Errorf("files = %v, want a.csv and b.CSV", got)
	}
	for _, run := range runs {
		if len(run.Results) != 5 {
			t.Errorf("%s: got %d results, want 5 — the file was listed but not parsed", run.File, len(run.Results))
		}
	}
}

// TestScanDir_DirectoryNamedCSVIsNotARun checks only that such a directory does
// not turn up in the results. It cannot tell you why: ScanDir's IsDir filter and
// its generic skip-on-parse-error both exclude it, and no test can separate them,
// because being a directory is exactly what makes the read fail. The IsDir guard
// is kept regardless — a filter over what gets parsed should not rest on an OS
// errno — but this test does not pin it.
func TestScanDir_DirectoryNamedCSVIsNotARun(t *testing.T) {
	dir := t.TempDir()
	writeRun(t, dir, "good.csv", "2026-09-19T14:32:07Z")
	if err := os.Mkdir(filepath.Join(dir, "archive.csv"), 0o755); err != nil {
		t.Fatal(err)
	}

	runs, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].File != "good.csv" {
		t.Errorf("File = %q, want %q", runs[0].File, "good.csv")
	}
}

func TestScanDir_SkipsUnparseableFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.csv"), []byte("\"unterminated"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRun(t, dir, "good.csv", "2026-09-19T14:32:07Z")

	runs, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("one bad file must not fail the scan: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1 (broken skipped, good kept)", len(runs))
	}
	if runs[0].File != "good.csv" {
		t.Errorf("File = %q, want %q", runs[0].File, "good.csv")
	}
}

func TestScanDir_MissingDirIsEmptyNotError(t *testing.T) {
	runs, err := ScanDir(filepath.Join(t.TempDir(), "does-not-exist"))

	if err != nil {
		t.Errorf("missing directory should not error: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("got %d runs, want 0", len(runs))
	}
}

// TestScanDir_SortsNewestFirst pins the order against every plausible
// alternative: the wanted order matches neither ascending nor descending file
// name, neither oldest-first, nor an ordering that puts the timestampless run
// at the front.
func TestScanDir_SortsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	writeRun(t, dir, "a_old.csv", "2026-01-02T00:00:00Z")
	writeRun(t, dir, "z_new.csv", "2026-06-02T00:00:00Z")
	writeRun(t, dir, "m_nometa.csv", "")

	runs, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	want := []string{"z_new.csv", "a_old.csv", "m_nometa.csv"}
	if len(runs) != len(want) {
		t.Fatalf("got %d runs, want %d", len(runs), len(want))
	}
	for i, name := range want {
		if runs[i].File != name {
			t.Errorf("runs[%d].File = %q, want %q", i, runs[i].File, name)
		}
	}
	if !runs[2].Meta.RunAt.IsZero() {
		t.Error("the timestampless run should have a zero RunAt")
	}
}

func TestScanDir_EqualTimestampsBreakOnFileName(t *testing.T) {
	dir := t.TempDir()
	const same = "2026-03-04T05:06:07Z"
	writeRun(t, dir, "b.csv", same)
	writeRun(t, dir, "a.csv", same)

	runs, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	want := []string{"a.csv", "b.csv"}
	for i, name := range want {
		if runs[i].File != name {
			t.Errorf("runs[%d].File = %q, want %q", i, runs[i].File, name)
		}
	}
}

func TestScanDir_EmptyDirIsEmptyNotError(t *testing.T) {
	runs, err := ScanDir(t.TempDir())

	if err != nil {
		t.Errorf("empty directory should not error: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("got %d runs, want 0", len(runs))
	}
}

// TestScanDir_UnreadableDirIsAnErrorNamingThePath covers the one failure
// ScanDir does report: a path that exists but cannot be listed. The wrapped
// message must name the directory, since the caller logs it and has nothing
// else to go on.
func TestScanDir_UnreadableDirIsAnErrorNamingThePath(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "results")
	if err := os.WriteFile(notADir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	runs, err := ScanDir(notADir)

	if err == nil {
		t.Fatal("ScanDir = nil error, want a failure")
	}
	if !strings.Contains(err.Error(), notADir) {
		t.Errorf("error %q does not name the directory %q", err, notADir)
	}
	if len(runs) != 0 {
		t.Errorf("got %d runs alongside the error, want 0", len(runs))
	}
}
