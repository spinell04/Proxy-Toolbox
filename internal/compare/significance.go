package compare

import "math"

// significanceLevel is the conventional two-sided 5% threshold.
const significanceLevel = 0.05

// probTieEpsilon is the relative slack allowed when deciding whether another
// table is "as extreme as" the observed one. The two-sided p-value sums the
// tables whose probability is <= the observed table's, and symmetric tables
// carry mathematically identical probabilities that exp() of a sum of Lgamma
// terms reproduces only to within a few ulps. An exact-equality comparison
// therefore drops half of a symmetric pair at random, which makes the p-value
// jump by the weight of a whole table.
const probTieEpsilon = 1e-7

// logChoose returns log C(n, k). The two denominator terms are added before
// being subtracted, which makes logChoose(n, k) and logChoose(n, n-k) agree bit
// for bit; subtracting them one at a time does not, for roughly 70% of n <= 400.
// canonicalTable cannot pick a unique orientation when a table's margins tie,
// and this keeps the two survivors term-for-term identical.
func logChoose(n, k int) float64 {
	logN, _ := math.Lgamma(float64(n) + 1)
	logK, _ := math.Lgamma(float64(k) + 1)
	logNK, _ := math.Lgamma(float64(n-k) + 1)
	return logN - (logK + logNK)
}

// canonicalTable reorients a 2x2 table to the one member of its symmetry class
// with the smallest first row and first column. Swapping the rows, swapping the
// columns and transposing all leave the p-value unchanged mathematically, but
// each reverses or re-groups the summation, so the float64 result can differ in
// the last ulp. Normalizing first makes the equality exact, and it also puts
// the shorter margin on the summation axis.
func canonicalTable(a, b, c, d int) (int, int, int, int) {
	if a+b > c+d {
		a, b, c, d = c, d, a, b
	}
	if a+c > b+d {
		a, b, c, d = b, a, d, c
	}
	// Transpose, which swaps the two off-diagonal cells, when the first row
	// still exceeds the first column.
	if b > c {
		b, c = c, b
	}
	return a, b, c, d
}

// FisherExact returns the two-sided p-value for the 2x2 table
//
//	a  b
//	c  d
//
// under Fisher's exact test: the sum of the hypergeometric probabilities of
// every table sharing these row and column margins whose probability does not
// exceed that of the observed table.
//
// Exact rather than chi-square or a two-proportion z: the cells that decide a
// provider comparison are small (single-digit failure counts) and one arm
// routinely sits at exactly 100%, which is where the normal approximations are
// least trustworthy.
//
// Probabilities are accumulated through math.Lgamma rather than by multiplying
// binomial coefficients. C(200, 100) is already past float64's exact-integer
// range and C(10000, 5000) is not representable at all, so the direct product
// returns Inf for run sizes this dashboard sees routinely.
//
// A negative cell or an all-zero table has no distribution to sum over and
// returns 1, matching Distinguishable's "we cannot tell" convention.
func FisherExact(a, b, c, d int) float64 {
	if a < 0 || b < 0 || c < 0 || d < 0 {
		return 1
	}
	n := a + b + c + d
	if n == 0 {
		return 1
	}

	a, b, c, d = canonicalTable(a, b, c, d)
	row1, row2, col1 := a+b, c+d, a+c
	logDenom := logChoose(n, col1)
	logProb := func(x int) float64 {
		return logChoose(row1, x) + logChoose(row2, col1-x) - logDenom
	}

	// Cells outside [lo, hi] would need a negative count somewhere in the
	// table, so they are not tables at all.
	lo := max(0, col1-row2)
	hi := min(row1, col1)

	cutoff := math.Exp(logProb(a)) * (1 + probTieEpsilon)
	sum := 0.0
	for x := lo; x <= hi; x++ {
		if p := math.Exp(logProb(x)); p <= cutoff {
			sum += p
		}
	}

	// The sum is a probability by construction; rounding across thousands of
	// terms can push it a few ulps past 1.
	return math.Min(1, sum)
}

// Distinguishable reports whether two providers' success rates differ by more
// than sampling noise, at the conventional 5% level. It returns the two-sided
// Fisher p-value alongside the verdict so the caller can show the number it
// acted on.
//
// Degenerate input — a non-positive total, a negative count, or more successes
// than trials — returns (1, false). Such a run carries no evidence, and the
// only safe reading of no evidence is "we cannot tell": reporting a significant
// difference from it would recommend a provider on the strength of a broken
// export.
//
// This, not the overlap of the two providers' Wilson intervals, is the pairwise
// claim. Overlapping confidence intervals do not imply an insignificant
// difference: 100/100 against 93/100 has overlapping 95% intervals and
// p = 0.014.
func Distinguishable(okA, totalA, okB, totalB int) (p float64, yes bool) {
	if totalA <= 0 || totalB <= 0 || okA < 0 || okB < 0 || okA > totalA || okB > totalB {
		return 1, false
	}
	p = FisherExact(okA, totalA-okA, okB, totalB-okB)
	return p, p < significanceLevel
}
