package compare

// Cell is one proxy's result within one of the joined runs.
type Cell struct {
	Present   bool    `json:"present"` // false when the proxy is absent from that run
	LatencyMs int     `json:"latencyMs"`
	Status    int     `json:"status"`
	Outcome   Outcome `json:"outcome"`
	ErrorKind string  `json:"errorKind"`
	ExitIP    string  `json:"exitIp"`
}

// JoinRow is one proxy across every joined run, in the order the runs were given.
type JoinRow struct {
	ProxyID string `json:"proxyId"`
	Cells   []Cell `json:"cells"`
}

// JoinResult is the proxy matrix: the union of proxies across runs, plus how
// many appear in all of them.
type JoinResult struct {
	Runs    []string  `json:"runs"` // file names, matching Cells order
	Rows    []JoinRow `json:"rows"`
	Overlap int       `json:"overlap"` // proxies present in every run
}

// Join builds the proxy matrix across runs, keyed on canonical proxy ID.
//
// The row order follows first appearance across the runs in order, so output
// is deterministic. Rows are the union, not the intersection: a proxy missing
// from one run still appears, with that cell marked absent.
//
// A proxy ID repeated within a single run collapses to one row, the last
// occurrence winning that run's cell. A proxy file with a duplicated line
// produces this, and the matrix is keyed on identity, so it cannot show the
// two separately.
func Join(runs []Run) JoinResult {
	j := JoinResult{Runs: make([]string, len(runs))}
	for i, r := range runs {
		j.Runs[i] = r.File
	}

	index := make(map[string]int) // proxy ID -> row position
	for runIdx, run := range runs {
		for _, res := range run.Results {
			rowIdx, seen := index[res.ProxyID]
			if !seen {
				rowIdx = len(j.Rows)
				index[res.ProxyID] = rowIdx
				j.Rows = append(j.Rows, JoinRow{
					ProxyID: res.ProxyID,
					Cells:   make([]Cell, len(runs)),
				})
			}
			j.Rows[rowIdx].Cells[runIdx] = Cell{
				Present:   true,
				LatencyMs: res.LatencyMs,
				Status:    res.Status,
				Outcome:   res.Outcome,
				ErrorKind: res.ErrorKind,
				ExitIP:    res.ExitIP,
			}
		}
	}

	for _, row := range j.Rows {
		complete := true
		for _, c := range row.Cells {
			if !c.Present {
				complete = false
				break
			}
		}
		if complete {
			j.Overlap++
		}
	}
	return j
}

// okSet returns the IDs of proxies that succeeded in a run.
func okSet(r Run) map[string]bool {
	set := make(map[string]bool)
	for _, res := range r.Results {
		if res.Outcome == OutcomeOK {
			set[res.ProxyID] = true
		}
	}
	return set
}

// OKInFirstNotSecond returns proxies that succeed in a but not in b, in a's
// original order. This is the actionable output of a cross-tool comparison:
// "works on ping, fails on Ticketmaster".
func OKInFirstNotSecond(a, b Run) []string {
	bOK := okSet(b)

	var out []string
	seen := make(map[string]bool)
	for _, res := range a.Results {
		if res.Outcome != OutcomeOK || bOK[res.ProxyID] || seen[res.ProxyID] {
			continue
		}
		seen[res.ProxyID] = true
		out = append(out, res.ProxyID)
	}
	return out
}
