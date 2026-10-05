// Package compare parses exported result CSVs and computes comparison
// statistics across runs and across tools.
package compare

import "time"

// Outcome classifies what happened to one proxy in one run.
type Outcome string

const (
	OutcomeOK      Outcome = "ok"      // request succeeded
	OutcomeBlocked Outcome = "blocked" // reached the target, was refused (403, 429)
	OutcomeError   Outcome = "error"   // never got a usable response
)

// Meta is the self-describing header of a run, recovered from the CSV.
// Fields absent from older exports are left zero and rendered as "unknown".
type Meta struct {
	Tool      string
	RunAt     time.Time
	ProxyFile string
	Target    string
	Workers   int
	// IPMode is "ipv4", "ipv6" or "both", and empty when the export did not
	// record it — exports made before modes existed, and every tool but
	// iptester. Empty means unknown, never "the same as the others".
	IPMode string
}

// ProxyResult is one proxy's outcome within one run.
type ProxyResult struct {
	ProxyID   string // canonical user:pass@host:port; the join key
	LatencyMs int    // 0 when there is no latency (errors)
	Status    int    // HTTP status, 0 when not applicable
	Outcome   Outcome
	ErrorRaw  string // verbatim error text from the CSV
	ErrorKind string // taxonomy bucket, see classifyError
	ExitIP    string // iptester only, empty elsewhere; the legacy single-address view
	// ExitIPv4 and ExitIPv6 are the family-named columns written since IP modes
	// existed. An older export's single "Exit IP" column is sorted into whichever
	// of these its address parses as, so both fields mean the same thing for
	// every file.
	ExitIPv4 string
	ExitIPv6 string
}

// Run is one parsed CSV.
type Run struct {
	File    string // base name, e.g. "pinger_2026-09-19_143207.csv"
	Meta    Meta
	Results []ProxyResult
}

// HasMeta reports whether the run carried metadata rows. Runs exported before
// metadata existed parse fine but cannot be placed on a timeline or checked
// for comparability.
func (r Run) HasMeta() bool {
	return r.Meta.Tool != "" && !r.Meta.RunAt.IsZero()
}
