package util

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSuggestedFilename(t *testing.T) {
	at := time.Date(2026, 9, 19, 14, 32, 7, 0, time.UTC)

	tests := []struct {
		name      string
		tool      string
		proxyFile string
		at        time.Time
		want      string
	}{
		{
			name:      "tool, proxy file and timestamp",
			tool:      "pinger",
			proxyFile: "schroeder.txt",
			at:        at,
			want:      "pinger_schroeder_2026-09-19_143207.csv",
		},
		{
			name: "no proxy file leaves the name unchanged",
			tool: "pinger",
			at:   at,
			want: "pinger_2026-09-19_143207.csv",
		},
		{
			name:      "empty tool falls back to results",
			tool:      "",
			proxyFile: "schroeder.txt",
			at:        at,
			want:      "results_schroeder_2026-09-19_143207.csv", // not a leading underscore
		},
		{
			name:      "only the base name is used, never the directory",
			tool:      "pinger",
			proxyFile: "/Users/someone/proxies/eu/schroeder.txt",
			at:        at,
			want:      "pinger_schroeder_2026-09-19_143207.csv",
		},
		{
			name:      "path separators cannot escape the results directory",
			tool:      "pinger",
			proxyFile: "../../etc/passwd",
			at:        at,
			want:      "pinger_passwd_2026-09-19_143207.csv",
		},
		{
			name:      "spaces and punctuation become dashes",
			tool:      "iptester",
			proxyFile: "Schroeder EU (residential).txt",
			at:        at,
			want:      "iptester_Schroeder-EU--residential_2026-09-19_143207.csv",
		},
		{
			name:      "leading and trailing dashes are trimmed",
			tool:      "iptester",
			proxyFile: " schroeder .txt",
			at:        at,
			want:      "iptester_schroeder_2026-09-19_143207.csv",
		},
		{
			name:      "a name that sanitises to nothing is dropped entirely",
			tool:      "pinger",
			proxyFile: "!!!.txt",
			at:        at,
			want:      "pinger_2026-09-19_143207.csv",
		},
		{
			name:      "only the final extension is stripped",
			tool:      "pinger",
			proxyFile: "schroeder.eu.txt",
			at:        at,
			want:      "pinger_schroeder-eu_2026-09-19_143207.csv",
		},
		{
			name:      "single-digit fields keep their leading zeros",
			tool:      "iptester",
			proxyFile: "proxies.txt",
			at:        time.Date(2026, 9, 19, 9, 5, 3, 0, time.UTC),
			want:      "iptester_proxies_2026-09-19_090503.csv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := suggestedFilename(tt.tool, tt.proxyFile, tt.at)
			if got != tt.want {
				t.Errorf("suggestedFilename(%q, %q, %v) = %q, want %q",
					tt.tool, tt.proxyFile, tt.at, got, tt.want)
			}
		})
	}
}

// TestWriteCSV_RoundTrip pins the on-disk shape that Phase 2's parser reads
// back: one file holding a ragged mix of 2-column metadata and 5-column detail
// rows, with values that contain the separators CSV has to quote.
func TestWriteCSV_RoundTrip(t *testing.T) {
	const (
		commaValue   = "dial tcp 1.2.3.4:8080: connect: connection refused, giving up"
		newlineValue = "proxy returned 407\nProxy-Authenticate: Basic"
	)

	header := []string{"Field", "Value"}
	rows := [][]string{
		{"Tool", "pinger"},
		{"Run at", "2026-09-19T14:32:07Z"},
		{"", ""}, // blank separator between the metadata block and the detail block
		{"Proxy", "Latency", "Status", "Country", "Error"},
		{"1.2.3.4:8080", "142", "ok", "DE", ""}, // empty trailing field on success
		{"5.6.7.8:3128", "0", "fail", "US", commaValue},
		{"9.10.11.12:80", "0", "fail", "FR", newlineValue},
	}

	path := filepath.Join(t.TempDir(), "roundtrip.csv")
	if err := WriteCSV(path, header, rows); err != nil {
		t.Fatalf("WriteCSV returned %v, want nil", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening written file: %v", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // ragged by design; 0 would reject the 5-column rows

	got, err := r.ReadAll()
	if err != nil {
		t.Fatalf("reading back written file: %v", err)
	}

	want := append([][]string{header}, rows...)
	if len(got) != len(want) {
		t.Fatalf("read back %d records, want %d", len(got), len(want))
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Errorf("record %d has %d fields, want %d", i, len(got[i]), len(want[i]))
			continue
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Errorf("record %d field %d = %q, want %q", i, j, got[i][j], want[i][j])
			}
		}
	}
}
