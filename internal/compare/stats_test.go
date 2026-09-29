package compare

import (
	"math"
	"testing"
)

func TestPercentile_NearestRank(t *testing.T) {
	// Sorted: 10 20 30 40 50 60 70 80 90 100. Deliberately unsorted on input
	// so a percentile that forgets to sort cannot pass.
	in := []int{50, 20, 90, 10, 70, 30, 100, 40, 80, 60}

	tests := []struct {
		name string
		p    float64
		want int
	}{
		{"zero is the minimum", 0.0, 10},
		{"below zero clamps to the minimum", -0.5, 10},
		{"p25 is the 3rd value", 0.25, 30},
		{"p50 is the 5th value", 0.50, 50},
		{"p75 is the 8th value", 0.75, 80},
		{"p90 is the 9th value", 0.90, 90},
		{"p95 rounds up to the 10th value", 0.95, 100},
		{"one is the maximum", 1.0, 100},
		{"above one clamps to the maximum", 1.5, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Percentile(in, tt.p); got != tt.want {
				t.Errorf("Percentile(p=%v) = %d, want %d", tt.p, got, tt.want)
			}
		})
	}
}

func TestPercentile_RankRoundsUp(t *testing.T) {
	// Seven values make every rank a fraction, so a floor-based or
	// interpolating implementation lands on a different element than
	// nearest-rank does. Sorted: 1 2 3 4 5 6 7.
	in := []int{7, 6, 5, 4, 3, 2, 1}

	tests := []struct {
		name string
		p    float64
		want int
	}{
		{"p10 rounds up to rank 1", 0.10, 1},                // ceil(0.7) = 1
		{"p50 rounds up to rank 4", 0.50, 4},                // ceil(3.5) = 4
		{"p90 rounds up to rank 7", 0.90, 7},                // ceil(6.3) = 7
		{"an exact rank is not rounded past", 2.0 / 7.0, 2}, // ceil(2.0) = 2
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Percentile(in, tt.p); got != tt.want {
				t.Errorf("Percentile(p=%v) = %d, want %d", tt.p, got, tt.want)
			}
		})
	}
}

func TestPercentile_EmptyInputIsZero(t *testing.T) {
	if got := Percentile(nil, 0.5); got != 0 {
		t.Errorf("Percentile(nil) = %d, want 0", got)
	}
	if got := Percentile([]int{}, 0.5); got != 0 {
		t.Errorf("Percentile(empty) = %d, want 0", got)
	}
}

func TestPercentile_DoesNotMutateInput(t *testing.T) {
	// Callers hold on to their latency slices; sorting in place would silently
	// reorder a run's results underneath them.
	in := []int{30, 10, 20}
	want := []int{30, 10, 20}

	for _, p := range []float64{0, 0.25, 0.5, 1} {
		Percentile(in, p)
	}

	if len(in) != len(want) {
		t.Fatalf("length changed: %v", in)
	}
	for i := range want {
		if in[i] != want[i] {
			t.Fatalf("input mutated: got %v, want %v", in, want)
		}
	}
}

func TestSummarize(t *testing.T) {
	results := []ProxyResult{
		{LatencyMs: 100, Outcome: OutcomeOK},
		{LatencyMs: 200, Outcome: OutcomeOK},
		{LatencyMs: 300, Outcome: OutcomeOK},
		{LatencyMs: 0, Outcome: OutcomeBlocked},
		{LatencyMs: 0, Outcome: OutcomeError},
	}

	s := Summarize(results)

	if s.Total != 5 {
		t.Errorf("Total = %d, want 5", s.Total)
	}
	if s.OK != 3 || s.Blocked != 1 || s.Errors != 1 {
		t.Errorf("counts = ok:%d blocked:%d err:%d, want 3/1/1", s.OK, s.Blocked, s.Errors)
	}
	if s.Min != 100 || s.Max != 300 {
		t.Errorf("Min/Max = %d/%d, want 100/300", s.Min, s.Max)
	}
	if s.P50 != 200 {
		t.Errorf("P50 = %d, want 200 (errors and blocks excluded)", s.P50)
	}
	if s.P75 != 300 || s.P90 != 300 || s.P95 != 300 || s.P99 != 300 {
		t.Errorf("upper percentiles = %d/%d/%d/%d, want 300 each", s.P75, s.P90, s.P95, s.P99)
	}
	if s.P25 != 100 {
		t.Errorf("P25 = %d, want 100", s.P25)
	}
	if s.IQR != 200 {
		t.Errorf("IQR = %d, want 200 (p75 300 - p25 100)", s.IQR)
	}
	if s.IQR != s.P75-s.P25 {
		t.Errorf("IQR = %d, want P75-P25 = %d: a box plot must not have to invert it",
			s.IQR, s.P75-s.P25)
	}
	if s.LatencyCount != 3 {
		t.Errorf("LatencyCount = %d, want 3", s.LatencyCount)
	}
	if math.Abs(s.SuccessRate-0.6) > 1e-9 {
		t.Errorf("SuccessRate = %v, want 0.6", s.SuccessRate)
	}
	if math.Abs(s.Mean-200) > 1e-9 {
		t.Errorf("Mean = %v, want 200", s.Mean)
	}
	// Population standard deviation of 100/200/300 is sqrt(20000/3).
	if math.Abs(s.StdDev-81.64965809277261) > 1e-9 {
		t.Errorf("StdDev = %v, want 81.64965809277261", s.StdDev)
	}
}

func TestSummarize_ExcludesNonOKLatencies(t *testing.T) {
	// Blocked requests measure how fast the target said no, and an errored row
	// can still carry a latency from a partially-read response. Either leaking
	// into the distribution would move every statistic below.
	results := []ProxyResult{
		{LatencyMs: 100, Outcome: OutcomeOK},
		{LatencyMs: 300, Outcome: OutcomeOK},
		{LatencyMs: 5, Outcome: OutcomeBlocked},  // would become the new Min
		{LatencyMs: 9000, Outcome: OutcomeError}, // would become the new Max
	}

	s := Summarize(results)

	if s.Min != 100 {
		t.Errorf("Min = %d, want 100: a blocked row's latency leaked in", s.Min)
	}
	if s.Max != 300 {
		t.Errorf("Max = %d, want 300: an errored row's latency leaked in", s.Max)
	}
	if s.P50 != 100 {
		t.Errorf("P50 = %d, want 100 over exactly the two OK latencies", s.P50)
	}
	if math.Abs(s.Mean-200) > 1e-9 {
		t.Errorf("Mean = %v, want 200 over exactly the two OK latencies", s.Mean)
	}
	if s.Total != 4 || s.OK != 2 {
		t.Errorf("Total/OK = %d/%d, want 4/2: the rows are still counted", s.Total, s.OK)
	}
}

func TestSummarize_LatencyCountSeparatesNoSamplesFromAllZero(t *testing.T) {
	// A success that reported no latency is still a success, but it is not a
	// sample. Without LatencyCount this run is indistinguishable from one with
	// no data at all, and the serve layer would render an all-zero chart
	// instead of declining to draw one.
	results := []ProxyResult{
		{LatencyMs: 0, Outcome: OutcomeOK},
		{LatencyMs: 0, Outcome: OutcomeError},
	}

	s := Summarize(results)

	if s.OK != 1 {
		t.Errorf("OK = %d, want 1: the result still succeeded", s.OK)
	}
	if s.LatencyCount != 0 {
		t.Errorf("LatencyCount = %d, want 0: it reported no latency to measure", s.LatencyCount)
	}
	if s.Min != 0 || s.P50 != 0 || s.Mean != 0 {
		t.Errorf("distribution = %+v, want zero with no samples", s)
	}
}

func TestSummarize_LatencyCountCountsOnlySamples(t *testing.T) {
	// Blocked and errored rows carrying a latency are excluded from the
	// distribution, so they must not be counted as samples either.
	results := []ProxyResult{
		{LatencyMs: 100, Outcome: OutcomeOK},
		{LatencyMs: 300, Outcome: OutcomeOK},
		{LatencyMs: 5, Outcome: OutcomeBlocked},
		{LatencyMs: 9000, Outcome: OutcomeError},
	}

	s := Summarize(results)

	if s.LatencyCount != 2 {
		t.Errorf("LatencyCount = %d, want 2 of the 4 rows", s.LatencyCount)
	}
	if s.Total != 4 {
		t.Errorf("Total = %d, want 4: every row is still counted", s.Total)
	}
}

func TestSummarize_NoLatenciesLeavesDistributionZero(t *testing.T) {
	results := []ProxyResult{
		{Outcome: OutcomeError},
		{Outcome: OutcomeError},
	}

	s := Summarize(results)

	if s.Total != 2 || s.Errors != 2 {
		t.Errorf("Total/Errors = %d/%d, want 2/2", s.Total, s.Errors)
	}
	if s.SuccessRate != 0 {
		t.Errorf("SuccessRate = %v, want 0", s.SuccessRate)
	}
	if s.LatencyCount != 0 {
		t.Errorf("LatencyCount = %d, want 0", s.LatencyCount)
	}
	if s.Min != 0 || s.Max != 0 || s.P50 != 0 || s.Mean != 0 || s.StdDev != 0 {
		t.Errorf("distribution = %+v, want all zero with no OK latencies", s)
	}
}

func TestSummarize_EmptyInput(t *testing.T) {
	s := Summarize(nil)

	if s.Total != 0 || s.OK != 0 || s.SuccessRate != 0 {
		t.Errorf("Summarize(nil) = %+v, want the zero Summary", s)
	}
}

func TestSummarize_OverAParsedRun(t *testing.T) {
	// End to end over the real fixture: five rows, of which one is OK
	// (300ms), one blocked (380ms) and three errored — including one that
	// reports 90ms alongside its error. Only the 300ms row is a latency.
	run, err := ParseFile("testdata/pinger_full.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	s := Summarize(run.Results)

	if s.Total != 5 {
		t.Errorf("Total = %d, want 5", s.Total)
	}
	if s.OK != 1 || s.Blocked != 1 || s.Errors != 3 {
		t.Errorf("counts = ok:%d blocked:%d err:%d, want 1/1/3", s.OK, s.Blocked, s.Errors)
	}
	if math.Abs(s.SuccessRate-0.2) > 1e-9 {
		t.Errorf("SuccessRate = %v, want 0.2", s.SuccessRate)
	}
	if s.Min != 300 || s.Max != 300 || s.P50 != 300 {
		t.Errorf("min/max/p50 = %d/%d/%d, want 300 each: only the OK row's latency counts",
			s.Min, s.Max, s.P50)
	}
	if s.LatencyCount != 1 {
		t.Errorf("LatencyCount = %d, want 1: one OK row carried a latency", s.LatencyCount)
	}
	if math.Abs(s.Mean-300) > 1e-9 {
		t.Errorf("Mean = %v, want 300, not the 340ms average the tool printed", s.Mean)
	}
}
