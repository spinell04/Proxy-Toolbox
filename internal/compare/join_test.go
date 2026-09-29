package compare

import "testing"

func fixtureRun(tool string, results ...ProxyResult) Run {
	return Run{File: tool + ".csv", Meta: Meta{Tool: tool}, Results: results}
}

func rowIDs(j JoinResult) []string {
	ids := make([]string, len(j.Rows))
	for i, r := range j.Rows {
		ids[i] = r.ProxyID
	}
	return ids
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestJoin_PairsByProxyID(t *testing.T) {
	a := fixtureRun("pinger",
		ProxyResult{ProxyID: "u:p@1.1.1.1:80", LatencyMs: 100, Status: 200, Outcome: OutcomeOK},
		ProxyResult{ProxyID: "u:p@2.2.2.2:80", LatencyMs: 200, Status: 200, Outcome: OutcomeOK},
		ProxyResult{ProxyID: "u:p@3.3.3.3:80", LatencyMs: 300, Status: 200, Outcome: OutcomeOK},
	)
	b := fixtureRun("speedtester",
		ProxyResult{ProxyID: "u:p@1.1.1.1:80", LatencyMs: 150, Status: 200, Outcome: OutcomeOK},
		ProxyResult{ProxyID: "u:p@2.2.2.2:80", LatencyMs: 180, Status: 403, Outcome: OutcomeBlocked},
	)

	j := Join([]Run{a, b})

	if !equalStrings(j.Runs, []string{"pinger.csv", "speedtester.csv"}) {
		t.Errorf("Runs = %v, want the file names in the order given", j.Runs)
	}
	if j.Overlap != 2 {
		t.Errorf("Overlap = %d, want 2", j.Overlap)
	}
	if got := rowIDs(j); !equalStrings(got, []string{"u:p@1.1.1.1:80", "u:p@2.2.2.2:80", "u:p@3.3.3.3:80"}) {
		t.Fatalf("rows = %v, want the union in first-appearance order", got)
	}

	// The cell order must track the run order, not the order rows were filled.
	row := j.Rows[1]
	if len(row.Cells) != 2 {
		t.Fatalf("got %d cells, want one per run", len(row.Cells))
	}
	if row.Cells[0].LatencyMs != 200 || row.Cells[1].LatencyMs != 180 {
		t.Errorf("latencies = %d then %d, want 200 then 180", row.Cells[0].LatencyMs, row.Cells[1].LatencyMs)
	}
	if row.Cells[0].Outcome != OutcomeOK || row.Cells[1].Outcome != OutcomeBlocked {
		t.Errorf("outcomes = %q then %q, want ok then blocked", row.Cells[0].Outcome, row.Cells[1].Outcome)
	}
	if row.Cells[1].Status != 403 {
		t.Errorf("Status = %d, want 403 carried onto the cell", row.Cells[1].Status)
	}
}

func TestJoin_CarriesEveryResultFieldOntoTheCell(t *testing.T) {
	a := fixtureRun("iptester", ProxyResult{
		ProxyID:   "u:p@1.1.1.1:80",
		LatencyMs: 120,
		Status:    407,
		Outcome:   OutcomeError,
		ErrorKind: "auth",
		ExitIP:    "9.9.9.1",
	})

	got := Join([]Run{a}).Rows[0].Cells[0]
	want := Cell{Present: true, LatencyMs: 120, Status: 407, Outcome: OutcomeError, ErrorKind: "auth", ExitIP: "9.9.9.1"}
	if got != want {
		t.Errorf("cell = %+v, want %+v", got, want)
	}
}

func TestJoin_IsTheUnionNotTheIntersection(t *testing.T) {
	a := fixtureRun("pinger",
		ProxyResult{ProxyID: "both", Outcome: OutcomeOK},
		ProxyResult{ProxyID: "only-in-a", Outcome: OutcomeOK},
	)
	b := fixtureRun("speedtester",
		ProxyResult{ProxyID: "both", Outcome: OutcomeOK},
		ProxyResult{ProxyID: "only-in-b", Outcome: OutcomeOK},
	)

	j := Join([]Run{a, b})

	// First appearance across runs in order: a's rows, then b's new one.
	want := []string{"both", "only-in-a", "only-in-b"}
	if got := rowIDs(j); !equalStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if j.Overlap != 1 {
		t.Errorf("Overlap = %d, want 1: only \"both\" appears in every run", j.Overlap)
	}
}

func TestJoin_MissingCellIsMarkedAbsent(t *testing.T) {
	a := fixtureRun("pinger",
		ProxyResult{ProxyID: "only-in-a", LatencyMs: 100, Outcome: OutcomeOK},
	)
	b := fixtureRun("speedtester",
		ProxyResult{ProxyID: "only-in-b", LatencyMs: 200, Outcome: OutcomeOK},
	)

	j := Join([]Run{a, b})

	if len(j.Rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(j.Rows))
	}
	if !j.Rows[0].Cells[0].Present {
		t.Error("cell present in run a must have Present=true")
	}
	if j.Rows[0].Cells[1].Present {
		t.Error("cell absent from run b must have Present=false")
	}
	if j.Rows[1].Cells[0].Present {
		t.Error("cell absent from run a must have Present=false")
	}
	if !j.Rows[1].Cells[1].Present {
		t.Error("cell present in run b must have Present=true")
	}
	// An absent cell must not be mistakable for a real zero-latency success.
	if j.Rows[0].Cells[1].LatencyMs != 0 || j.Rows[0].Cells[1].Outcome != "" {
		t.Errorf("absent cell = %+v, want the zero Cell", j.Rows[0].Cells[1])
	}
	if j.Overlap != 0 {
		t.Errorf("Overlap = %d, want 0: the runs share no proxy", j.Overlap)
	}
}

func TestJoin_OverlapCountsOnlyProxiesInEveryRun(t *testing.T) {
	// Three runs separate "present in all" from "present in more than one",
	// which two runs cannot tell apart.
	a := fixtureRun("a",
		ProxyResult{ProxyID: "in-all", Outcome: OutcomeOK},
		ProxyResult{ProxyID: "in-a-and-b", Outcome: OutcomeOK},
		ProxyResult{ProxyID: "in-a-only", Outcome: OutcomeOK},
	)
	b := fixtureRun("b",
		ProxyResult{ProxyID: "in-all", Outcome: OutcomeOK},
		ProxyResult{ProxyID: "in-a-and-b", Outcome: OutcomeOK},
	)
	c := fixtureRun("c",
		ProxyResult{ProxyID: "in-all", Outcome: OutcomeOK},
		ProxyResult{ProxyID: "in-c-only", Outcome: OutcomeOK},
	)

	j := Join([]Run{a, b, c})

	if len(j.Rows) != 4 {
		t.Fatalf("got %d rows, want 4: %v", len(j.Rows), rowIDs(j))
	}
	if j.Overlap != 1 {
		t.Errorf("Overlap = %d, want 1: only \"in-all\" is in all three runs", j.Overlap)
	}
}

func TestJoin_DegenerateInput(t *testing.T) {
	t.Run("no runs", func(t *testing.T) {
		j := Join(nil)
		if len(j.Runs) != 0 || len(j.Rows) != 0 || j.Overlap != 0 {
			t.Errorf("Join(nil) = %+v, want an empty result", j)
		}
	})

	t.Run("runs with no results", func(t *testing.T) {
		j := Join([]Run{fixtureRun("a"), fixtureRun("b")})
		if len(j.Rows) != 0 {
			t.Errorf("got %d rows, want 0", len(j.Rows))
		}
		if j.Overlap != 0 {
			t.Errorf("Overlap = %d, want 0 with no proxies at all", j.Overlap)
		}
		if !equalStrings(j.Runs, []string{"a.csv", "b.csv"}) {
			t.Errorf("Runs = %v, want both file names", j.Runs)
		}
	})
}

func TestOKInFirstNotSecond(t *testing.T) {
	tests := []struct {
		name string
		a, b Run
		want []string
	}{
		{
			name: "succeeds in a, blocked in b",
			a: fixtureRun("pinger",
				ProxyResult{ProxyID: "good", Outcome: OutcomeOK},
				ProxyResult{ProxyID: "both-ok", Outcome: OutcomeOK},
			),
			b: fixtureRun("speedtester",
				ProxyResult{ProxyID: "good", Outcome: OutcomeBlocked},
				ProxyResult{ProxyID: "both-ok", Outcome: OutcomeOK},
			),
			want: []string{"good"},
		},
		{
			name: "absent from b counts as not OK",
			a:    fixtureRun("pinger", ProxyResult{ProxyID: "only-in-a", Outcome: OutcomeOK}),
			b:    fixtureRun("speedtester"),
			want: []string{"only-in-a"},
		},
		{
			name: "a proxy that failed in a is never listed",
			a: fixtureRun("pinger",
				ProxyResult{ProxyID: "failed-in-a", Outcome: OutcomeError},
				ProxyResult{ProxyID: "blocked-in-a", Outcome: OutcomeBlocked},
			),
			b:    fixtureRun("speedtester"),
			want: nil,
		},
		{
			name: "order follows a, and duplicates collapse",
			a: fixtureRun("pinger",
				ProxyResult{ProxyID: "zebra", Outcome: OutcomeOK},
				ProxyResult{ProxyID: "alpha", Outcome: OutcomeOK},
				ProxyResult{ProxyID: "zebra", Outcome: OutcomeOK},
			),
			b:    fixtureRun("speedtester"),
			want: []string{"zebra", "alpha"},
		},
		{
			name: "everything succeeds in b too",
			a:    fixtureRun("pinger", ProxyResult{ProxyID: "x", Outcome: OutcomeOK}),
			b:    fixtureRun("speedtester", ProxyResult{ProxyID: "x", Outcome: OutcomeOK}),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OKInFirstNotSecond(tt.a, tt.b); !equalStrings(got, tt.want) {
				t.Errorf("OKInFirstNotSecond = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOKSet(t *testing.T) {
	// OKInFirstNotSecond can only observe okSet through a difference, so a
	// set that over-collects on one side can hide behind the other.
	r := fixtureRun("pinger",
		ProxyResult{ProxyID: "ok", Outcome: OutcomeOK},
		ProxyResult{ProxyID: "blocked", Outcome: OutcomeBlocked},
		ProxyResult{ProxyID: "errored", Outcome: OutcomeError},
	)

	set := okSet(r)

	if len(set) != 1 {
		t.Fatalf("okSet = %v, want only the successful proxy", set)
	}
	if !set["ok"] {
		t.Error("the successful proxy is missing from okSet")
	}
}
