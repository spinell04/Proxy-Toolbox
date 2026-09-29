package compare

import "math"

// zScore95 is the standard normal quantile for a two-sided 95% interval.
const zScore95 = 1.96

// Interval is a proportion with a confidence interval.
type Interval struct {
	Rate float64 `json:"rate"`
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// WilsonInterval returns the 95% Wilson score interval for k successes in n
// trials. n <= 0 returns a zero Interval.
//
// Wilson rather than the normal approximation p̂ ± z·√(p̂(1-p̂)/n): that form
// collapses to zero width at k == n, and three of the four providers in the
// grounding data sit at exactly 100/100. A zero-width interval there asserts
// certainty that 100 trials do not support, which would defeat the reason for
// quoting an interval at all. Wilson keeps a finite lower bound (0.963 at
// 100/100) because it inverts the score test rather than centring on p̂.
//
// k outside [0, n] cannot arise from counting outcomes, so it is clamped to
// the nearest valid count rather than propagated; Rate reports the clamped
// value and therefore always lies in [0, 1].
func WilsonInterval(k, n int) Interval {
	if n <= 0 {
		return Interval{}
	}
	if k < 0 {
		k = 0
	}
	if k > n {
		k = n
	}

	nf := float64(n)
	p := float64(k) / nf

	z2 := zScore95 * zScore95
	denom := 1 + z2/nf
	center := (p + z2/(2*nf)) / denom
	halfWidth := zScore95 / denom * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf))

	// The Wilson interval always contains p̂, and at p̂ = 0 and p̂ = 1 the
	// algebra puts a bound exactly on 0 and 1 respectively. Rounding breaks
	// both facts: at 100/100 the sum evaluates to 0.9999999999999999, which
	// renders as 100% but leaves High below Rate. Clamping against Rate as
	// well as against [0, 1] restores what the closed form already guarantees.
	return Interval{
		Rate: p,
		Low:  math.Min(p, math.Max(0, center-halfWidth)),
		High: math.Max(p, math.Min(1, center+halfWidth)),
	}
}
