package compare

import (
	"math"
	"testing"
)

// tol4 is the tolerance for a literal quoted to 4 decimal places.
const tol4 = 5e-5

func assertClose(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	if math.Abs(got-want) > tol4 {
		t.Errorf("%s = %.6f, want %.4f (tolerance %g)", label, got, want, tol4)
	}
}

// TestWilsonIntervalNoTrials pins the guard. Without it n=0 divides by zero and
// every field comes back NaN, which would propagate silently to any caller.
func TestWilsonIntervalNoTrials(t *testing.T) {
	for _, n := range []int{0, -1, -100} {
		got := WilsonInterval(0, n)
		if got != (Interval{}) {
			t.Errorf("WilsonInterval(0, %d) = %+v, want zero Interval", n, got)
		}
		got = WilsonInterval(5, n)
		if got != (Interval{}) {
			t.Errorf("WilsonInterval(5, %d) = %+v, want zero Interval", n, got)
		}
	}
}

// TestWilsonIntervalReference93of100 is the anchor value, derived by hand.
//
// Wilson bounds are the roots of (p̂-p)² = z²·p(1-p)/n, i.e. of
// p²(1 + z²/n) − p(2p̂ + z²/n) + p̂² = 0. With z = 1.96 (z² = 3.8416),
// n = 100, k = 93, p̂ = 0.93:
//
//	a = 1 + 3.8416/100            = 1.038416
//	b = −(2·0.93 + 3.8416/100)    = −1.898416
//	c = 0.93²                     = 0.8649
//	b² − 4ac = 3.604983... − 3.593504... = 0.011479315456
//	√(b²−4ac) = 0.107142035...
//	roots = (1.898416 ∓ 0.107142035) / 2.076832
//	      = 0.8625033  and  0.9656812
//
// Cross-checked against the centre±half-width form,
// (p̂ + z²/2n)/(1+z²/n) ± (z/(1+z²/n))·√(p̂(1-p̂)/n + z²/4n²)
// = 0.914092233 ± 0.051588943, which agrees to 12 places.
func TestWilsonIntervalReference93of100(t *testing.T) {
	got := WilsonInterval(93, 100)

	assertClose(t, "Rate", got.Rate, 0.93)
	assertClose(t, "Low", got.Low, 0.8625)
	assertClose(t, "High", got.High, 0.9657)
}

// TestWilsonIntervalReference50of100 is a second anchor, chosen because
// [0.4038, 0.5962] for 50/100 is the textbook worked example of the Wilson
// interval and can be checked against any published table.
func TestWilsonIntervalReference50of100(t *testing.T) {
	got := WilsonInterval(50, 100)

	assertClose(t, "Rate", got.Rate, 0.5)
	assertClose(t, "Low", got.Low, 0.4038)
	assertClose(t, "High", got.High, 0.5962)
}

// TestWilsonIntervalAllSuccesses is the case the normal approximation gets
// wrong: three of the user's four providers sit at 100/100.
//
// At p̂ = 1 the algebra collapses: centre = (1 + z²/2n)/(1 + z²/n), half-width
// = (z/(1+z²/n))·√(z²/4n²) = (z²/2n)/(1 + z²/n), so centre + half = 1 exactly
// and centre − half = 1/(1 + z²/n) = 1/1.038416 = 0.9630052.
func TestWilsonIntervalAllSuccesses(t *testing.T) {
	got := WilsonInterval(100, 100)

	assertClose(t, "Rate", got.Rate, 1)
	assertClose(t, "Low", got.Low, 0.9630)
	if got.High != 1 {
		t.Errorf("High = %v, want exactly 1", got.High)
	}
	if got.Low >= 1 {
		t.Errorf("Low = %v, want strictly below 1", got.Low)
	}
	if got.Low <= 0.9 {
		t.Errorf("Low = %v, want strictly above 0.9", got.Low)
	}
}

// TestWilsonIntervalNoSuccesses mirrors the above at the other end: by the same
// collapse, Low is 0 exactly and High is 1 − 1/(1+z²/n) = 0.0369948.
func TestWilsonIntervalNoSuccesses(t *testing.T) {
	got := WilsonInterval(0, 100)

	assertClose(t, "Rate", got.Rate, 0)
	assertClose(t, "High", got.High, 0.0370)
	if got.Low != 0 {
		t.Errorf("Low = %v, want exactly 0", got.Low)
	}
	if got.High <= 0 {
		t.Errorf("High = %v, want strictly above 0", got.High)
	}
}

// TestWilsonIntervalBracketsRate is the interval's defining property: the
// interval straddles the observed rate and, at 93%, does not reach certainty.
func TestWilsonIntervalBracketsRate(t *testing.T) {
	got := WilsonInterval(93, 100)

	if !(got.Low < 0.93 && 0.93 < got.High) {
		t.Errorf("[%v, %v] does not contain 0.93", got.Low, got.High)
	}
	if got.High >= 1 {
		t.Errorf("High = %v, want strictly below 1 at 93/100", got.High)
	}
}

// TestWilsonIntervalNarrowsWithSampleSize is the whole point of the statistic:
// at the same 90% rate, ten trials must say much less than a thousand.
// Hand-derived widths: 9/10 spans [0.5958436, 0.9821243] (width 0.3862806) and
// 900/1000 spans [0.8798476, 0.9170908] (width 0.0372432).
func TestWilsonIntervalNarrowsWithSampleSize(t *testing.T) {
	small := WilsonInterval(9, 10)
	large := WilsonInterval(900, 1000)

	assertClose(t, "small.Low", small.Low, 0.5958)
	assertClose(t, "small.High", small.High, 0.9821)
	assertClose(t, "large.Low", large.Low, 0.8798)
	assertClose(t, "large.High", large.High, 0.9171)

	smallWidth := small.High - small.Low
	largeWidth := large.High - large.Low
	if smallWidth <= largeWidth {
		t.Errorf("width at n=10 (%v) must exceed width at n=1000 (%v)", smallWidth, largeWidth)
	}
}

// TestWilsonIntervalRate checks Rate is k/n and not, say, the interval centre,
// which is a different number for every k except k = n/2.
func TestWilsonIntervalRate(t *testing.T) {
	cases := []struct {
		k, n int
		want float64
	}{
		{0, 7, 0},
		{1, 4, 0.25},
		{3, 4, 0.75},
		{7, 8, 0.875},
		{1, 3, 0.3333},
		{100, 100, 1},
	}
	for _, c := range cases {
		got := WilsonInterval(c.k, c.n).Rate
		if math.Abs(got-c.want) > tol4 {
			t.Errorf("WilsonInterval(%d, %d).Rate = %v, want %v", c.k, c.n, got, c.want)
		}
	}
}

// TestWilsonIntervalClampsNonsensicalCounts pins the documented contract for
// input that cannot happen: k outside [0, n] is clamped to the nearest valid
// count, so Rate stays inside [0, 1] rather than reporting a 150% success rate.
func TestWilsonIntervalClampsNonsensicalCounts(t *testing.T) {
	over := WilsonInterval(150, 100)
	if over != WilsonInterval(100, 100) {
		t.Errorf("WilsonInterval(150, 100) = %+v, want same as 100/100 %+v", over, WilsonInterval(100, 100))
	}
	if over.Rate != 1 {
		t.Errorf("Rate = %v, want 1", over.Rate)
	}

	under := WilsonInterval(-5, 100)
	if under != WilsonInterval(0, 100) {
		t.Errorf("WilsonInterval(-5, 100) = %+v, want same as 0/100 %+v", under, WilsonInterval(0, 100))
	}
	if under.Rate != 0 {
		t.Errorf("Rate = %v, want 0", under.Rate)
	}
}

// TestWilsonIntervalStaysInUnitRange sweeps every k for a few n and asserts the
// bounds never escape [0, 1] and never invert.
func TestWilsonIntervalStaysInUnitRange(t *testing.T) {
	for _, n := range []int{1, 2, 3, 7, 100, 1000} {
		for k := 0; k <= n; k++ {
			got := WilsonInterval(k, n)
			if got.Low < 0 || got.High > 1 || got.Low > got.High {
				t.Errorf("WilsonInterval(%d, %d) = %+v, out of range", k, n, got)
			}
		}
	}
}

// TestWilsonIntervalContainsRate sweeps for the invariant a caller reads
// the interval through: an interval that excluded its own observed rate would
// mark a provider as distinguishable from itself.
func TestWilsonIntervalContainsRate(t *testing.T) {
	for _, n := range []int{1, 2, 3, 7, 100, 1000} {
		for k := 0; k <= n; k++ {
			got := WilsonInterval(k, n)
			if got.Low > got.Rate || got.High < got.Rate {
				t.Errorf("WilsonInterval(%d, %d) = %+v, does not contain its rate", k, n, got)
			}
		}
	}
}
