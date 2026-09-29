package util

import (
	"cmp"
	"math"
	"slices"
)

// Percentile returns the nearest-rank percentile of values. p is in [0,1].
// The input is not modified.
//
// Nearest rank is the quantile function of the empirical distribution,
// Q(p) = inf{x : F(x) >= p}: it sorts the samples and returns the one at
// 1-based rank ceil(p*n). The reported figure is therefore always a value
// some sample actually recorded, never an interpolation between two of them.
//
// This is the single definition of the rule in the codebase. The CLI tools
// instantiate it on time.Duration and compare.Percentile delegates to it on
// int milliseconds, so terminal output, CSV exports and the dashboard all
// report the same percentiles for the same run.
//
// p <= 0 returns the minimum, p >= 1 the maximum, and an empty input the
// zero value of T.
func Percentile[T cmp.Ordered](values []T, p float64) T {
	if len(values) == 0 {
		var zero T
		return zero
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)

	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	rank := int(math.Ceil(p * float64(len(sorted))))
	return sorted[rank-1]
}
