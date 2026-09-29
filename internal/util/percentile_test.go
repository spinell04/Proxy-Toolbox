// Package util_test holds the percentile tests. It is an external test package
// so it can import internal/compare alongside internal/util and assert the two
// exported Percentile functions agree — util cannot import compare, but a
// test binary outside util can import both.
package util_test

import (
	"testing"
	"time"

	"proxytoolbox/internal/compare"
	"proxytoolbox/internal/util"
)

// schroLatenciesMs is the 100 successful latencies, in milliseconds, from the
// real results/schro.csv speedtester run that exposed the p50/p95 disagreement
// between the CLI tools and the compare dashboard. Listed in sorted order so
// the expected percentiles below can be counted by hand.
func schroLatenciesMs() []int {
	return []int{
		571, 575, 577, 584, 604, 610, 614, 621, 622, 635,
		635, 635, 638, 641, 641, 648, 649, 656, 656, 656,
		656, 656, 665, 666, 668, 669, 680, 680, 685, 687,
		700, 700, 700, 702, 703, 703, 703, 708, 711, 712,
		712, 712, 712, 714, 715, 715, 722, 723, 726, 728,
		736, 737, 737, 745, 760, 764, 768, 768, 772, 775,
		779, 780, 782, 783, 788, 799, 805, 814, 819, 829,
		831, 850, 863, 868, 879, 880, 883, 905, 925, 928,
		931, 985, 1003, 1020, 1049, 1062, 1093, 1287, 1308, 1329,
		1338, 1422, 1583, 1595, 1850, 2188, 2423, 4956, 5819, 8403,
	}
}

func TestPercentileInts(t *testing.T) {
	// Deliberately unsorted so every case also proves the helper sorts
	// before indexing.
	ten := []int{50, 10, 90, 30, 100, 20, 80, 40, 70, 60}

	tests := []struct {
		name   string
		values []int
		p      float64
		want   int
	}{
		{"nil returns zero value", nil, 0.5, 0},
		{"empty slice returns zero value", []int{}, 0.95, 0},
		{"single element", []int{42}, 0.5, 42},
		{"p zero returns min", ten, 0, 10},
		{"p negative returns min", ten, -0.5, 10},
		{"p one returns max", ten, 1, 100},
		{"p above one returns max", ten, 1.5, 100},
		// n=10, nearest rank = ceil(p*10), 1-based.
		{"p10 is first element", ten, 0.10, 10},
		{"p50 is fifth element", ten, 0.50, 50},
		{"p51 rounds up to sixth", ten, 0.51, 60},
		{"p90 is ninth element", ten, 0.90, 90},
		{"p95 rounds up to tenth", ten, 0.95, 100},
		// n=3: ceil(0.5*3)=2 -> second smallest.
		{"odd length median", []int{3, 1, 2}, 0.5, 2},
		// n=4: ceil(0.5*4)=2 -> lower of the two middle values.
		{"even length median takes lower middle", []int{4, 1, 3, 2}, 0.5, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := util.Percentile(tt.values, tt.p); got != tt.want {
				t.Errorf("Percentile(%v, %v) = %d, want %d", tt.values, tt.p, got, tt.want)
			}
		})
	}
}

func TestPercentileDoesNotMutateInput(t *testing.T) {
	values := []int{50, 10, 90, 30, 20}
	want := []int{50, 10, 90, 30, 20}

	for _, p := range []float64{-1, 0, 0.25, 0.5, 0.95, 1, 2} {
		util.Percentile(values, p)
	}

	if len(values) != len(want) {
		t.Fatalf("length changed: got %d, want %d", len(values), len(want))
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("input reordered at index %d: got %v, want %v", i, values, want)
		}
	}
}

// TestPercentileDurations pins the generic instantiation the CLI tools use.
func TestPercentileDurations(t *testing.T) {
	values := []time.Duration{
		300 * time.Millisecond,
		100 * time.Millisecond,
		500 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
	}

	tests := []struct {
		name string
		p    float64
		want time.Duration
	}{
		{"min", 0, 100 * time.Millisecond},
		{"p50", 0.5, 300 * time.Millisecond},
		{"p60 rounds up", 0.6, 300 * time.Millisecond},
		{"p61 rounds up to fourth", 0.61, 400 * time.Millisecond},
		{"p95", 0.95, 500 * time.Millisecond},
		{"max", 1, 500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := util.Percentile(values, tt.p); got != tt.want {
				t.Errorf("Percentile(durations, %v) = %v, want %v", tt.p, got, tt.want)
			}
		})
	}

	var empty []time.Duration
	if got := util.Percentile(empty, 0.5); got != 0 {
		t.Errorf("Percentile(nil durations, 0.5) = %v, want 0", got)
	}
}

// TestPercentileHandComputedAtN100 counts the expected values out of the
// sorted schro fixture by hand: with n=100 the nearest rank is ceil(p*100),
// so p50 is the 50th sorted value and p95 the 95th.
func TestPercentileHandComputedAtN100(t *testing.T) {
	values := schroLatenciesMs()
	if len(values) != 100 {
		t.Fatalf("fixture has %d values, want 100", len(values))
	}

	if got, want := util.Percentile(values, 0.50), 728; got != want {
		t.Errorf("p50 = %d, want %d", got, want)
	}
	if got, want := util.Percentile(values, 0.95), 1850; got != want {
		t.Errorf("p95 = %d, want %d", got, want)
	}
	// The defective tools computed sorted[int(n*p)] — the 96th value for p95,
	// 2188ms — which overstated the tail. Pin that it is gone.
	if got := util.Percentile(values, 0.95); got == 2188 {
		t.Errorf("p95 = %d, the old off-by-one 96th value", got)
	}
}

// TestPercentileAgreesWithComparePercentile is the regression test for the
// defect: the CLI tools (via util.Percentile, on time.Duration) and the
// compare dashboard (via compare.Percentile, on int milliseconds) must report
// the same p50 and p95 for the same run. Nothing but this test stops the two
// from drifting apart again.
func TestPercentileAgreesWithComparePercentile(t *testing.T) {
	ms := schroLatenciesMs()

	durations := make([]time.Duration, len(ms))
	for i, v := range ms {
		durations[i] = time.Duration(v) * time.Millisecond
	}

	for _, p := range []float64{0, 0.25, 0.50, 0.75, 0.90, 0.95, 0.99, 1} {
		wantMs := compare.Percentile(ms, p)

		if got := util.Percentile(ms, p); got != wantMs {
			t.Errorf("p=%v: util.Percentile(ints) = %d, compare.Percentile = %d", p, got, wantMs)
		}
		gotDur := util.Percentile(durations, p)
		if gotDur.Milliseconds() != int64(wantMs) {
			t.Errorf("p=%v: util.Percentile(durations) = %dms, compare.Percentile = %dms",
				p, gotDur.Milliseconds(), wantMs)
		}
	}

	// Anchored so the test still fails if both implementations regress the
	// same way: these are the values the dashboard reports for this run.
	if got := compare.Percentile(ms, 0.50); got != 728 {
		t.Errorf("compare p50 = %d, want 728", got)
	}
	if got := compare.Percentile(ms, 0.95); got != 1850 {
		t.Errorf("compare p95 = %d, want 1850", got)
	}
}
