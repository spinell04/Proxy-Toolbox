// Package dashboard serves a local, read-only web UI for comparing exported
// result CSVs. It binds the loopback interface only and never writes.
package dashboard

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"proxytoolbox/internal/compare"
)

// runInventoryItem is one row of the landing table.
type runInventoryItem struct {
	File      string          `json:"file"`
	Tool      string          `json:"tool"`
	RunAt     time.Time       `json:"runAt"`
	ProxyFile string          `json:"proxyFile"`
	Target    string          `json:"target"`
	Workers   int             `json:"workers"`
	IPMode    string          `json:"ipMode"`
	HasMeta   bool            `json:"hasMeta"`
	Summary   compare.Summary `json:"summary"`
}

// runOverview is one run's distributions: everything the timeline and overlay
// charts need, and deliberately not the per-proxy rows.
type runOverview struct {
	runInventoryItem
	Histogram compare.Hist    `json:"histogram"`
	ECDF      []compare.Point `json:"ecdf"`
	Errors    map[string]int  `json:"errors"`
	// Codes is the failure breakdown keyed on the HTTP status where there is
	// one and the taxonomy kind otherwise. Errors, above, is keyed on kind
	// alone, which folds every refusal into the transport buckets and loses
	// the 403/429 distinction that separates a blocked provider from a broken
	// one. Both ship: Errors is what the timeline and detail views already
	// read, and narrowing it to serve head-to-head would break them.
	Codes []compare.CodeGroup `json:"codes"`
	// Success is the working rate with a 95% interval around it. Providers are
	// tested with different numbers of proxies, so a bare rate gap between two
	// of them says nothing about which is better until the intervals separate.
	Success compare.Interval `json:"success"`
}

// runDetail is one run with everything a detail view needs, per-proxy rows
// included. Only /api/run returns this.
type runDetail struct {
	runOverview
	Results []compare.ProxyResult `json:"results"`
}

// compareResponse carries everything the comparison views need for a selection.
//
// Runs carries no per-proxy rows on purpose. Join.Rows already holds every
// proxy's latency, status, outcome, error kind and exit IP in every selected
// run, so shipping Results alongside would encode the same facts twice and
// leave the client to decide which copy is authoritative. A drill-down on one
// run is a separate /api/run call.
type compareResponse struct {
	Runs []runOverview      `json:"runs"`
	Join compare.JoinResult `json:"join"`
	// Pairwise carries, for every pair of selected runs, whether the gap between
	// their success rates is larger than sampling noise. This is a separate
	// statistic from each run's own Success interval, and it has to be: an
	// interval describes one provider's own precision, and reading a pairwise
	// verdict off two of them by checking whether they overlap is a known
	// fallacy. It is far too conservative, and it fails on the reference data —
	// 100/100 against 93/100 has overlapping 95% intervals and a Fisher
	// p of 0.014, so the overlap rule would have argued for the worse provider.
	// Both ship: the interval to describe each rate, this to compare two.
	Pairwise []pairSignificance `json:"pairwise"`
}

// pairSignificance is one pair of runs and the significance of the gap
// between their success rates.
type pairSignificance struct {
	A               string  `json:"a"`               // file name of the first run
	B               string  `json:"b"`               // file name of the second run
	P               float64 `json:"p"`               // two-sided Fisher exact p-value
	Distinguishable bool    `json:"distinguishable"` // p < 0.05
}

// pairwise tests every unordered pair of the selected runs, i < j in the order
// the runs appear in the response, so the client can address a pair by index
// instead of searching for it.
//
// The result is always non-nil. A single-run selection is the dashboard's
// default state and produces no pairs at all; a nil slice would marshal to null
// there and break a client that maps over the field.
func pairwise(runs []runOverview) []pairSignificance {
	pairs := make([]pairSignificance, 0, len(runs)*(len(runs)-1)/2)
	for i := range runs {
		for j := i + 1; j < len(runs); j++ {
			a, b := runs[i], runs[j]
			p, yes := compare.Distinguishable(a.Summary.OK, a.Summary.Total, b.Summary.OK, b.Summary.Total)
			pairs = append(pairs, pairSignificance{A: a.File, B: b.File, P: p, Distinguishable: yes})
		}
	}
	return pairs
}

func newAPI(resultsDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/runs", readOnly(func(w http.ResponseWriter, r *http.Request) {
		runs, err := compare.ScanDir(resultsDir)
		if err != nil {
			// The client gets a fixed string — the real message names a
			// filesystem path — but this is a local tool with the user's
			// terminal in front of them, and this is the only reachable
			// server-side failure in the package.
			log.Printf("dashboard: %v", err)
			http.Error(w, "cannot read results directory", http.StatusInternalServerError)
			return
		}
		items := make([]runInventoryItem, 0, len(runs))
		for _, run := range runs {
			items = append(items, inventoryItem(run))
		}
		writeJSON(w, items)
	}))

	mux.HandleFunc("/api/run", readOnly(func(w http.ResponseWriter, r *http.Request) {
		run, ok := loadRun(resultsDir, r.URL.Query().Get("file"))
		if !ok {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		writeJSON(w, detail(run))
	}))

	mux.HandleFunc("/api/compare", readOnly(func(w http.ResponseWriter, r *http.Request) {
		names := r.URL.Query()["file"]
		if len(names) == 0 {
			http.Error(w, "no files selected", http.StatusBadRequest)
			return
		}
		if len(names) > maxCompareFiles {
			http.Error(w, "too many files selected", http.StatusBadRequest)
			return
		}
		runs := make([]compare.Run, 0, len(names))
		overviews := make([]runOverview, 0, len(names))
		for _, name := range names {
			run, ok := loadRun(resultsDir, name)
			if !ok {
				// Named in the log, not in the body: the client already knows
				// what it asked for, and reflecting it back is free XSS surface.
				log.Printf("dashboard: compare requested a run that would not load: %q", filepath.Base(name))
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			runs = append(runs, run)
			overviews = append(overviews, overview(run))
		}
		writeJSON(w, compareResponse{Runs: overviews, Join: compare.Join(runs), Pairwise: pairwise(overviews)})
	}))

	return mux
}

// readOnly rejects anything but GET. The dashboard never mutates state.
func readOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "read-only", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

// loadRun resolves a requested file strictly inside resultsDir.
//
// Two guards do the real work, and both are load-bearing. filepath.Base strips
// any directory component, so "../../etc/passwd" becomes "passwd". filepath.Join
// re-roots what is left, so an absolute "/etc/passwd" — whose Base is the
// harmless "passwd" — becomes resultsDir/etc/passwd rather than the real file.
// Remove either and the other does not cover for it.
func loadRun(resultsDir, name string) (compare.Run, bool) {
	if name == "" {
		return compare.Run{}, false
	}
	base := filepath.Base(name)
	// Defence in depth, not a third necessary guard: these three reduce to the
	// results directory itself, which ParseFile already rejects because reading
	// a directory as CSV fails. That rejection is an OS behaviour, though, and a
	// security property should not rest on one, so the case is refused here in
	// terms the reader can check.
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return compare.Run{}, false
	}
	run, err := compare.ParseFile(filepath.Join(resultsDir, base))
	if err != nil {
		return compare.Run{}, false
	}
	return run, true
}

func inventoryItem(run compare.Run) runInventoryItem {
	return runInventoryItem{
		File:      run.File,
		Tool:      run.Meta.Tool,
		RunAt:     run.Meta.RunAt,
		ProxyFile: run.Meta.ProxyFile,
		Target:    run.Meta.Target,
		Workers:   run.Meta.Workers,
		IPMode:    run.Meta.IPMode,
		HasMeta:   run.HasMeta(),
		Summary:   compare.Summarize(run.Results),
	}
}

func overview(run compare.Run) runOverview {
	var lat []int
	for _, r := range run.Results {
		if r.Outcome == compare.OutcomeOK && r.LatencyMs > 0 {
			lat = append(lat, r.LatencyMs)
		}
	}
	item := inventoryItem(run)
	return runOverview{
		runInventoryItem: item,
		Histogram:        compare.Histogram(lat, histogramBuckets),
		ECDF:             compare.ECDF(lat),
		Errors:           compare.ErrorBreakdown(run.Results),
		Codes:            compare.CodeBreakdown(run.Results),
		Success:          compare.WilsonInterval(item.Summary.OK, item.Summary.Total),
	}
}

func detail(run compare.Run) runDetail {
	return runDetail{runOverview: overview(run), Results: run.Results}
}

// histogramBuckets is the bar count the latency charts are drawn with.
const histogramBuckets = 30

// maxCompareFiles bounds a single comparison. Well past what anyone reads off
// a chart, and it keeps a scripted caller from asking for unbounded work.
const maxCompareFiles = 50

// writeJSON encodes v into a buffer before touching the response, so a failure
// mid-encode still produces a clean 500 instead of a truncated 200. Both headers
// are set before the first write, which is what commits the status line.
func writeJSON(w http.ResponseWriter, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, "encoding failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// The payload is JSON and nothing else; never let a browser guess otherwise.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(buf.Bytes())
}
