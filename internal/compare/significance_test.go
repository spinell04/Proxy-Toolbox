package compare

import (
	"math"
	"testing"
)

// Every expected p-value below was derived independently of the implementation
// under test, by summing hypergeometric probabilities in exact rational
// arithmetic (integer binomial coefficients over a common denominator, no
// floating point anywhere). None of them is a recomputation of the log-space
// formula in significance.go.

func TestFisherExactGroundingData(t *testing.T) {
	// The case that motivated this test existing: schro 100/100 against
	// mobile 93/100. The Wilson intervals [0.9630, 1.0000] and
	// [0.8625, 0.9657] overlap, so an interval-overlap rule calls this gap
	// noise. It is not.
	//
	// Exact two-sided value: 0.01401776554... One-sided (greater) is
	// 0.00700888277, exactly half, because b = 0 makes every table at or
	// below the observed one as extreme as it, so the lower tail
	// contributes only the observed table's own probability.
	got := FisherExact(100, 0, 93, 7)
	if math.Abs(got-0.0140177655) > 1e-6 {
		t.Errorf("FisherExact(100, 0, 93, 7) = %.10f, want 0.0140177655", got)
	}

	p, yes := Distinguishable(100, 100, 93, 100)
	if !yes {
		t.Errorf("Distinguishable(100, 100, 93, 100) = (%.10f, false), want true: p is below 0.05", p)
	}
	if math.Abs(p-0.0140177655) > 1e-6 {
		t.Errorf("Distinguishable(100, 100, 93, 100) p = %.10f, want 0.0140177655", p)
	}
}

func TestFisherExactTeaTasting(t *testing.T) {
	// Fisher's lady-tasting-tea table: 8 cups, 4 of each preparation, the
	// taster names 3 of 4 correctly.
	//
	//   3 1
	//   1 3
	//
	// The published two-sided p is 0.4857. Verified here by exact
	// enumeration rather than taken on trust: the five tables with these
	// margins have probabilities 1/70, 16/70, 36/70, 16/70, 1/70 for
	// x = 0..4. The observed table is x = 3, p = 16/70. The tables with
	// probability <= 16/70 are x in {0, 1, 3, 4}, summing to 34/70 = 17/35
	// = 0.485714285714..., which is the published figure.
	got := FisherExact(3, 1, 1, 3)
	want := 17.0 / 35.0
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("FisherExact(3, 1, 1, 3) = %.10f, want %.10f (17/35)", got, want)
	}
}

func TestFisherExactNullTable(t *testing.T) {
	// Identical rates: the observed table is the modal one, so every table
	// with these margins has probability <= it, and the sum over all of
	// them is 1 by construction.
	tests := []struct {
		name       string
		a, b, c, d int
	}{
		{"50/100 vs 50/100", 50, 50, 50, 50},
		{"1/2 vs 1/2", 1, 1, 1, 1},
		{"3000/3000 vs 3000/3000", 3000, 0, 3000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FisherExact(tt.a, tt.b, tt.c, tt.d)
			if math.Abs(got-1) > 1e-12 {
				t.Errorf("FisherExact(%d, %d, %d, %d) = %.12f, want 1", tt.a, tt.b, tt.c, tt.d, got)
			}
		})
	}
}

func TestFisherExactInvariantUnderTableOrientation(t *testing.T) {
	// The two-sided p depends only on the table's margins and the observed
	// cell's position in the hypergeometric distribution, so swapping the
	// rows, swapping the columns, or transposing must not move it.
	tables := [][4]int{
		{100, 0, 93, 7},
		{3, 1, 1, 3},
		{12, 30, 25, 9},
		{0, 17, 6, 11},
		{1200, 800, 1000, 1000},
	}
	for _, tb := range tables {
		a, b, c, d := tb[0], tb[1], tb[2], tb[3]
		base := FisherExact(a, b, c, d)

		if got := FisherExact(c, d, a, b); got != base {
			t.Errorf("row swap: FisherExact(%d, %d, %d, %d) = %.12g, want %.12g", c, d, a, b, got, base)
		}
		if got := FisherExact(b, a, d, c); got != base {
			t.Errorf("column swap: FisherExact(%d, %d, %d, %d) = %.12g, want %.12g", b, a, d, c, got, base)
		}
		if got := FisherExact(a, c, b, d); got != base {
			t.Errorf("transpose: FisherExact(%d, %d, %d, %d) = %.12g, want %.12g", a, c, b, d, got, base)
		}
		if got := FisherExact(d, c, b, a); got != base {
			t.Errorf("180 rotation: FisherExact(%d, %d, %d, %d) = %.12g, want %.12g", d, c, b, a, got, base)
		}
	}
}

func TestFisherExactLargeCounts(t *testing.T) {
	// Runs of several thousand proxies put binomial coefficients far past
	// float64's exact-integer range, and C(10000, 5000) is not even
	// representable as a float64. The computation has to stay in log space.
	//
	// Exact two-sided values from rational enumeration:
	//   (4800, 200, 4700, 300) -> 5.21262969474e-06
	//   (2500, 2500, 2600, 2400) -> 0.0476536135552
	tests := []struct {
		name       string
		a, b, c, d int
		want       float64
	}{
		{"4800/5000 vs 4700/5000", 4800, 200, 4700, 300, 5.21262969474e-06},
		{"2500/5000 vs 2600/5000", 2500, 2500, 2600, 2400, 0.0476536135552},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FisherExact(tt.a, tt.b, tt.c, tt.d)
			if math.IsNaN(got) || math.IsInf(got, 0) {
				t.Fatalf("FisherExact(%d, %d, %d, %d) = %v, want a finite value", tt.a, tt.b, tt.c, tt.d, got)
			}
			if got < 0 || got > 1 {
				t.Fatalf("FisherExact(%d, %d, %d, %d) = %.12g, want a value in [0, 1]", tt.a, tt.b, tt.c, tt.d, got)
			}
			if relErr := math.Abs(got-tt.want) / tt.want; relErr > 1e-9 {
				t.Errorf("FisherExact(%d, %d, %d, %d) = %.12g, want %.12g (relative error %.3g)", tt.a, tt.b, tt.c, tt.d, got, tt.want, relErr)
			}
		})
	}
}

func TestDistinguishableThreshold(t *testing.T) {
	// Against a 100/100 arm the exact two-sided p crosses 0.05 between 94
	// and 95 successes out of 100:
	//   94/100 -> 0.0289302800  (significant)
	//   95/100 -> 0.0593832143  (not significant)
	// The large pair is a second crossing far from the first, at n = 5000,
	// where p = 0.0476536136 sits just inside the threshold.
	tests := []struct {
		name                     string
		okA, totalA, okB, totalB int
		wantP                    float64
		wantDistinguishable      bool
	}{
		{"100/100 vs 94/100 is significant", 100, 100, 94, 100, 0.0289302800, true},
		{"100/100 vs 95/100 is not", 100, 100, 95, 100, 0.0593832143, false},
		{"2500/5000 vs 2600/5000 is significant", 2500, 5000, 2600, 5000, 0.0476536136, true},
		{"identical rates are not", 50, 100, 50, 100, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, yes := Distinguishable(tt.okA, tt.totalA, tt.okB, tt.totalB)
			if math.Abs(p-tt.wantP) > 1e-8 {
				t.Errorf("p = %.10f, want %.10f", p, tt.wantP)
			}
			if yes != tt.wantDistinguishable {
				t.Errorf("distinguishable = %v, want %v (p = %.10f)", yes, tt.wantDistinguishable, p)
			}
		})
	}
}

func TestDistinguishableDegenerateInput(t *testing.T) {
	// Nothing countable happened, or the counts are impossible. The answer
	// is "we cannot tell", which is (1, false) — never a significant result.
	tests := []struct {
		name                     string
		okA, totalA, okB, totalB int
	}{
		{"empty first run", 0, 0, 93, 100},
		{"empty second run", 100, 100, 0, 0},
		{"both empty", 0, 0, 0, 0},
		{"negative total", 5, -10, 93, 100},
		{"negative successes", -1, 100, 93, 100},
		{"negative successes in second", 100, 100, -5, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, yes := Distinguishable(tt.okA, tt.totalA, tt.okB, tt.totalB)
			if p != 1 || yes {
				t.Errorf("Distinguishable(%d, %d, %d, %d) = (%v, %v), want (1, false)", tt.okA, tt.totalA, tt.okB, tt.totalB, p, yes)
			}
		})
	}
}

func TestFisherExactDegenerateTable(t *testing.T) {
	// A negative cell or an empty table has no hypergeometric distribution
	// to sum over.
	tests := [][4]int{
		{0, 0, 0, 0},
		{-1, 5, 5, 5},
		{5, 5, 5, -5},
	}
	for _, tb := range tests {
		got := FisherExact(tb[0], tb[1], tb[2], tb[3])
		if got != 1 {
			t.Errorf("FisherExact(%d, %d, %d, %d) = %v, want 1", tb[0], tb[1], tb[2], tb[3], got)
		}
	}
}

func TestFisherExactIncludesExactlyTiedTables(t *testing.T) {
	// The 2x2 tables with margins (2, 11 | 4, 9) have probabilities 6/13,
	// 6/13 and 1/13 for x = 0, 1, 2. The observed table here is x = 1, and
	// x = 0 carries exactly the same probability, so the two-sided sum is
	// every table: p = 1.
	//
	// Those two probabilities are equal as rationals but not as float64s —
	// exp() of their Lgamma sums differ in the last ulp — so an
	// exact-equality "as extreme as" test drops x = 0 and returns 7/13
	// = 0.5384615, understating the p-value by a whole table's weight.
	got := FisherExact(1, 1, 3, 8)
	if math.Abs(got-1) > 1e-12 {
		t.Errorf("FisherExact(1, 1, 3, 8) = %.12f, want 1", got)
	}
	if math.Abs(got-7.0/13.0) < 1e-6 {
		t.Errorf("FisherExact(1, 1, 3, 8) = %.12f, which is 7/13: the tied table was dropped", got)
	}
}

func TestLogChooseIsSymmetricBitForBit(t *testing.T) {
	// C(n, k) == C(n, n-k), and FisherExact compares and sums these values
	// across table orientations, so the identity has to survive as float64
	// rather than only as mathematics.
	for n := 0; n <= 400; n++ {
		for k := 0; k <= n; k++ {
			if got, want := logChoose(n, k), logChoose(n, n-k); got != want {
				t.Fatalf("logChoose(%d, %d) = %.17g, logChoose(%d, %d) = %.17g", n, k, got, n, n-k, want)
			}
		}
	}

	// Spot values, from log of exactly computed integers:
	// C(8, 3) = 56, C(200, 100) = 9.0548514656103281e+58.
	if got := logChoose(8, 3); math.Abs(got-math.Log(56)) > 1e-12 {
		t.Errorf("logChoose(8, 3) = %.17g, want log(56) = %.17g", got, math.Log(56))
	}
	if got, want := logChoose(200, 100), 135.7532360812785; math.Abs(got-want) > 1e-10 {
		t.Errorf("logChoose(200, 100) = %.17g, want %.17g", got, want)
	}
}
