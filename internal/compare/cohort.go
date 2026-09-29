package compare

import "math"

// Pearson returns the correlation coefficient of two equal-length series.
// Mismatched lengths, empty input, or zero variance return 0 rather than NaN,
// so the dashboard can render "no relationship" instead of a broken chart.
func Pearson(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) == 0 {
		return 0
	}

	var sumX, sumY float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
	}
	meanX := sumX / float64(len(xs))
	meanY := sumY / float64(len(ys))

	var cov, varX, varY float64
	for i := range xs {
		dx := xs[i] - meanX
		dy := ys[i] - meanY
		cov += dx * dy
		varX += dx * dx
		varY += dy * dy
	}
	if varX == 0 || varY == 0 {
		return 0
	}
	return cov / math.Sqrt(varX*varY)
}
