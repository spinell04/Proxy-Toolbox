package compare

import "sort"

// Bucket is one histogram bar.
type Bucket struct {
	Lo    int `json:"lo"`
	Hi    int `json:"hi"`
	Count int `json:"count"`
}

// Hist is a fixed-bucket histogram over a value range.
type Hist struct {
	Buckets []Bucket `json:"buckets"`
}

// Histogram bins values into n equal-width buckets spanning min..max.
//
// Bucket bounds are integers and values are binned against those same bounds,
// so a value always falls in a bucket whose own label contains it. Two labels
// are approximate by design: the maximum joins the last bucket even though the
// half-open [lo,hi) label formally excludes it, and when every value is
// identical the synthetic hi = lo + 1 range gives several buckets the same
// degenerate label.
func Histogram(values []int, n int) Hist {
	if len(values) == 0 || n <= 0 {
		return Hist{}
	}

	lo, hi := values[0], values[0]
	for _, v := range values {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi == lo {
		hi = lo + 1 // avoid a zero-width range
	}

	// Edges are computed once and shared by the labels and the binning. A
	// range that does not divide evenly truncates to different integer edges
	// than the exact float width implies, so binning on the width instead
	// would count values into buckets their own labels exclude.
	width := float64(hi-lo) / float64(n)
	edges := make([]int, n+1)
	for i := range edges {
		edges[i] = lo + int(float64(i)*width)
	}

	buckets := make([]Bucket, n)
	for i := range buckets {
		buckets[i] = Bucket{Lo: edges[i], Hi: edges[i+1]}
	}

	for _, v := range values {
		// Largest i with edges[i] <= v, so a value on an edge falls in the
		// bucket whose label starts at that edge, not the one below it.
		idx := sort.SearchInts(edges, v+1) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		buckets[idx].Count++
	}
	return Hist{Buckets: buckets}
}

// Point is one step of an empirical cumulative distribution.
type Point struct {
	Value int     `json:"value"`
	P     float64 `json:"p"`
}

// ECDF returns the empirical cumulative distribution of values, ascending.
// Overlaying two ECDFs shows which run is faster at every percentile, not
// just on average.
func ECDF(values []int) []Point {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)

	points := make([]Point, len(sorted))
	for i, v := range sorted {
		points[i] = Point{Value: v, P: float64(i+1) / float64(len(sorted))}
	}
	return points
}
