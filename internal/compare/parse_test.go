package compare

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseFile_ReadsMetadata(t *testing.T) {
	run, err := ParseFile("testdata/pinger_full.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if run.Meta.Tool != "pinger" {
		t.Errorf("Tool = %q, want %q", run.Meta.Tool, "pinger")
	}
	want := time.Date(2026, 9, 19, 14, 32, 7, 0, time.UTC)
	if !run.Meta.RunAt.Equal(want) {
		t.Errorf("RunAt = %v, want %v", run.Meta.RunAt, want)
	}
	if run.Meta.Workers != 100 {
		t.Errorf("Workers = %d, want 100", run.Meta.Workers)
	}
	if run.Meta.Target != "https://google.com" {
		t.Errorf("Target = %q, want %q", run.Meta.Target, "https://google.com")
	}
	if run.Meta.ProxyFile != "residential_de.txt" {
		t.Errorf("ProxyFile = %q, want %q", run.Meta.ProxyFile, "residential_de.txt")
	}
	if run.File != "pinger_full.csv" {
		t.Errorf("File = %q, want %q", run.File, "pinger_full.csv")
	}
	if !run.HasMeta() {
		t.Error("HasMeta = false, want true")
	}
}

func TestParseFile_ReadsProxyRows(t *testing.T) {
	run, err := ParseFile("testdata/pinger_full.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(run.Results) != 5 {
		t.Fatalf("got %d results, want 5", len(run.Results))
	}

	first := run.Results[0]
	if first.ProxyID != "alice:pw@1.2.3.4:8080" {
		t.Errorf("ProxyID = %q", first.ProxyID)
	}
	if first.LatencyMs != 300 {
		t.Errorf("LatencyMs = %d, want 300", first.LatencyMs)
	}
	if first.Status != 200 {
		t.Errorf("Status = %d, want 200", first.Status)
	}
	if first.Outcome != OutcomeOK {
		t.Errorf("Outcome = %q, want %q", first.Outcome, OutcomeOK)
	}
	if first.ErrorRaw != "" {
		t.Errorf("ErrorRaw = %q, want empty", first.ErrorRaw)
	}
	if first.ExitIP != "" {
		t.Errorf("ExitIP = %q, want empty for pinger", first.ExitIP)
	}

	// Errored rows keep the error verbatim, including embedded commas and
	// colons that a manual comma split would have shredded.
	third := run.Results[2]
	if third.ErrorRaw != "dial tcp 1.2.3.6:8080: i/o timeout" {
		t.Errorf("ErrorRaw = %q, want %q", third.ErrorRaw, "dial tcp 1.2.3.6:8080: i/o timeout")
	}
	if third.LatencyMs != 0 {
		t.Errorf("LatencyMs = %d, want 0 for an errored row", third.LatencyMs)
	}
}

func TestParseFile_ClassifiesOutcomes(t *testing.T) {
	run, err := ParseFile("testdata/pinger_full.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	want := []Outcome{OutcomeOK, OutcomeBlocked, OutcomeError, OutcomeError, OutcomeError}
	if len(run.Results) != len(want) {
		t.Fatalf("got %d results, want %d", len(run.Results), len(want))
	}
	for i, w := range want {
		if got := run.Results[i].Outcome; got != w {
			t.Errorf("result %d outcome = %q, want %q", i, got, w)
		}
	}
}

func TestParseFile_NonEmptyErrorOverridesASuccessStatus(t *testing.T) {
	// A tool can record a status from a partially-read response and still fail
	// afterwards, so a non-empty Error must win over a success-looking Status.
	// The other errored rows in the fixture also say Status "ERROR", so this
	// contradictory row is the only one that exercises the override on a
	// status-bearing tool.
	run, err := ParseFile("testdata/pinger_full.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	const idx = 4
	if len(run.Results) <= idx {
		t.Fatalf("got %d results, want more than %d", len(run.Results), idx)
	}
	got := run.Results[idx]
	if got.ProxyID != "eve:pw@1.2.3.8:8080" {
		t.Fatalf("ProxyID = %q, want the contradictory row", got.ProxyID)
	}
	if got.Outcome != OutcomeError {
		t.Errorf("Outcome = %q, want %q: a non-empty Error must override Status %q",
			got.Outcome, OutcomeError, "HTTP 200")
	}
	// The status and latency it did report are still preserved verbatim.
	if got.Status != 200 {
		t.Errorf("Status = %d, want 200 preserved alongside the error", got.Status)
	}
	if got.LatencyMs != 90 {
		t.Errorf("LatencyMs = %d, want 90 preserved alongside the error", got.LatencyMs)
	}
	if got.ErrorRaw != "unexpected EOF after headers" {
		t.Errorf("ErrorRaw = %q", got.ErrorRaw)
	}
}

func TestClassifyStatus_AllToolVocabularies(t *testing.T) {
	// The four tools spell success three different ways. Any of them parsing
	// as an error would silently mis-bucket whole runs.
	tests := []struct {
		name        string
		in          string
		wantCode    int
		wantOutcome Outcome
	}{
		{"pinger bare OK", "OK", 0, OutcomeOK},
		{"pinger with code", "HTTP 200", 200, OutcomeOK},
		{"speedtester and bayerntester", "200 OK", 200, OutcomeOK},
		{"blocked", "403 BLOCKED", 403, OutcomeBlocked},
		{"rate limited", "429 BLOCKED", 429, OutcomeBlocked},
		{"explicit error", "ERROR", 0, OutcomeError},
		{"server error", "HTTP 503", 503, OutcomeError},
		{"absent", "", 0, OutcomeError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, outcome := classifyStatus(tt.in)
			if code != tt.wantCode || outcome != tt.wantOutcome {
				t.Errorf("classifyStatus(%q) = (%d, %q), want (%d, %q)",
					tt.in, code, outcome, tt.wantCode, tt.wantOutcome)
			}
		})
	}
}

func TestParseFile_IPTesterHasNoStatusColumn(t *testing.T) {
	// iptester reports exit IPs and emits no Status column. Success must be
	// inferred from an empty Error, or every successful proxy reads as failed.
	run, err := ParseFile("testdata/iptester_full.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(run.Results) != 3 {
		t.Fatalf("got %d results, want 3", len(run.Results))
	}

	want := []Outcome{OutcomeOK, OutcomeOK, OutcomeError}
	for i, w := range want {
		if got := run.Results[i].Outcome; got != w {
			t.Errorf("result %d outcome = %q, want %q", i, got, w)
		}
	}
	if run.Results[0].ExitIP != "9.9.9.1" {
		t.Errorf("ExitIP = %q, want %q", run.Results[0].ExitIP, "9.9.9.1")
	}
	if run.Results[0].LatencyMs != 120 {
		t.Errorf("LatencyMs = %d, want 120", run.Results[0].LatencyMs)
	}
	// iptester has no Status column, so no HTTP code can be invented for it.
	if run.Results[0].Status != 0 {
		t.Errorf("Status = %d, want 0 where the tool writes no Status column", run.Results[0].Status)
	}
}

func TestParseFile_SkipsTheRepeatedIPSection(t *testing.T) {
	// The repeated-IP table sits between the summary and the detail table and
	// must be ignored, not parsed as proxy rows. Its data row ("9.9.9.1,2,...")
	// would otherwise appear as a fourth result.
	run, err := ParseFile("testdata/iptester_full.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	wantIDs := []string{
		"alice:pw@1.2.3.4:8080",
		"bob:pw@1.2.3.5:8080",
		"carol:pw@1.2.3.6:8080",
	}
	if len(run.Results) != len(wantIDs) {
		t.Fatalf("got %d results, want %d — a repeated-IP row leaked: %+v",
			len(run.Results), len(wantIDs), run.Results)
	}
	for i, want := range wantIDs {
		if got := run.Results[i].ProxyID; got != want {
			t.Errorf("result %d ProxyID = %q, want %q", i, got, want)
		}
	}
}

func TestParseFile_LegacyExportWithoutMetadata(t *testing.T) {
	run, err := ParseFile("testdata/legacy_no_meta.csv")
	if err != nil {
		t.Fatalf("legacy CSV must parse, got error: %v", err)
	}

	if run.HasMeta() {
		t.Error("HasMeta = true, want false for a legacy export")
	}
	if len(run.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(run.Results))
	}
	// The old Host column is still the identifier, even though it is not
	// a canonical ID. It simply will not join against newer runs.
	if run.Results[0].ProxyID != "1.2.3.4" {
		t.Errorf("ProxyID = %q, want %q", run.Results[0].ProxyID, "1.2.3.4")
	}
	if run.Results[0].Outcome != OutcomeOK {
		t.Errorf("Outcome = %q, want %q for a bare OK status", run.Results[0].Outcome, OutcomeOK)
	}
	if run.Results[1].Outcome != OutcomeError {
		t.Errorf("Outcome = %q, want %q", run.Results[1].Outcome, OutcomeError)
	}
}

func TestParseFile_MissingFileIsAnError(t *testing.T) {
	if _, err := ParseFile("testdata/does_not_exist.csv"); err == nil {
		t.Error("ParseFile on a missing file = nil error, want an error")
	}
}

func TestIsProxyDetail_RejectsOtherSections(t *testing.T) {
	// The repeated-IP test above can only see a leak that survives into
	// Results; a parser that mis-classifies a section but is then saved by
	// section ordering would slip past it. This pins the classifier itself.
	tests := []struct {
		name   string
		header []string
		want   bool
	}{
		{"pinger, speedtester, bayerntester detail", []string{"#", "Proxy", "Latency", "Status", "Error"}, true},
		{"iptester detail", []string{"#", "Proxy", "Exit IP", "Latency", "Error"}, true},
		{"legacy detail", []string{"#", "Host", "Latency", "Status", "Error"}, true},
		{"iptester repeated-IP section", []string{"Repeated IP", "Times", "Lines"}, false},
		{"summary section", []string{"Summary", "Value"}, false},
		{"a summary key/value row", []string{"Proxies tested", "4"}, false},
		{"hypothetical future section", []string{"#", "Subnet", "Count"}, false},
		{"short row", []string{"#"}, false},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProxyDetail(tt.header); got != tt.want {
				t.Errorf("isProxyDetail(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

// writeTempCSV writes content to a throwaway file and returns its path. Kept
// separate from testdata/ so malformed input cannot muddy the three fixtures,
// whose job is to mirror real tool output exactly.
func writeTempCSV(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "run.csv")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing temp CSV: %v", err)
	}
	return path
}

func TestParseFile_EmptyFile(t *testing.T) {
	// A truncated or zero-length export must not error or panic; it simply
	// carries no metadata and no results.
	run, err := ParseFile(writeTempCSV(t, ""))
	if err != nil {
		t.Fatalf("ParseFile on an empty file: %v", err)
	}
	if run.HasMeta() {
		t.Error("HasMeta = true, want false")
	}
	if len(run.Results) != 0 {
		t.Errorf("got %d results, want 0", len(run.Results))
	}
	if run.File != "run.csv" {
		t.Errorf("File = %q, want %q", run.File, "run.csv")
	}
}

func TestParseFile_RowShorterThanItsHeader(t *testing.T) {
	// A row truncated mid-write still carries an ID but no Latency, Status or
	// Error. Those must read as empty rather than panic on an index past the
	// end of the row.
	const content = `Summary,Value
Tool,pinger
Run at,2026-09-19T14:32:07Z
,
#,Proxy,Latency,Status,Error
1,alice:pw@1.2.3.4:8080
2,bob:pw@1.2.3.5:8080,300ms,HTTP 200,
`
	run, err := ParseFile(writeTempCSV(t, content))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(run.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(run.Results))
	}

	short := run.Results[0]
	if short.ProxyID != "alice:pw@1.2.3.4:8080" {
		t.Errorf("ProxyID = %q, want the ID the short row did carry", short.ProxyID)
	}
	if short.LatencyMs != 0 {
		t.Errorf("LatencyMs = %d, want 0", short.LatencyMs)
	}
	if short.ErrorRaw != "" {
		t.Errorf("ErrorRaw = %q, want empty", short.ErrorRaw)
	}
	if short.ExitIP != "" {
		t.Errorf("ExitIP = %q, want empty", short.ExitIP)
	}
	// An absent Status column value is not a success; it reads as an error.
	if short.Outcome != OutcomeError {
		t.Errorf("Outcome = %q, want %q for a row with no Status value", short.Outcome, OutcomeError)
	}

	// The intact row alongside it is unaffected.
	if run.Results[1].Status != 200 || run.Results[1].Outcome != OutcomeOK {
		t.Errorf("intact row = (%d, %q), want (200, %q)",
			run.Results[1].Status, run.Results[1].Outcome, OutcomeOK)
	}
}
