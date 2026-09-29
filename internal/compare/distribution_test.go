package compare

import "testing"

// bucketCounts pulls the counts out of a histogram for whole-shape assertions,
// which catch a value landing in the wrong bin rather than only a missing one.
func bucketCounts(h Hist) []int {
	counts := make([]int, len(h.Buckets))
	for i, b := range h.Buckets {
		counts[i] = b.Count
	}
	return counts
}

func equalInts(a, b []int) bool {
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

func TestHistogram(t *testing.T) {
	// Range 5..95 over 10 buckets is 9 wide each: 5 lands in bucket 0, 15 in
	// bucket 1, both 25s in bucket 2, and 95 (the maximum) in bucket 9.
	values := []int{5, 15, 25, 25, 95}

	h := Histogram(values, 10)

	want := []int{1, 1, 2, 0, 0, 0, 0, 0, 0, 1}
	if got := bucketCounts(h); !equalInts(got, want) {
		t.Errorf("bucket counts = %v, want %v", got, want)
	}
	if h.Buckets[0].Lo != 5 {
		t.Errorf("first bucket Lo = %d, want the minimum 5", h.Buckets[0].Lo)
	}
	if h.Buckets[9].Hi != 95 {
		t.Errorf("last bucket Hi = %d, want the maximum 95", h.Buckets[9].Hi)
	}
}

func TestHistogram_BucketBoundsTileTheRange(t *testing.T) {
	// Range 0..100 over 10 buckets divides evenly, so every edge is an exact
	// multiple of ten. Asserting only the two outer edges leaves the interior
	// arithmetic free to shift; this pins all twenty bounds in sequence.
	h := Histogram([]int{0, 100}, 10)

	if len(h.Buckets) != 10 {
		t.Fatalf("got %d buckets, want 10", len(h.Buckets))
	}
	for i, b := range h.Buckets {
		wantLo, wantHi := i*10, (i+1)*10
		if b.Lo != wantLo || b.Hi != wantHi {
			t.Errorf("bucket %d = [%d,%d), want [%d,%d)", i, b.Lo, b.Hi, wantLo, wantHi)
		}
		// Buckets must tile the range with no gap and no overlap: one
		// bucket's upper edge is the next one's lower edge.
		if i > 0 && b.Lo != h.Buckets[i-1].Hi {
			t.Errorf("bucket %d starts at %d, but bucket %d ended at %d",
				i, b.Lo, i-1, h.Buckets[i-1].Hi)
		}
	}
}

func TestHistogram_UnevenRangeBinsAgainstItsOwnLabels(t *testing.T) {
	// 0..100 over 7 buckets does not divide evenly, which is the normal case
	// for real latency ranges. The labels are integers, so binning must use
	// those same integer edges: a value sitting exactly on an edge belongs to
	// the bucket whose label starts there, never to the one below, or the
	// dashboard shows a value counted in a bucket that excludes it.
	//
	// Each edge value is repeated a different number of times, so the counts
	// identify exactly which bucket each one landed in. With one of each, a
	// whole-row shift would leave most counts unchanged and hide itself.
	values := []int{0}
	for reps, edge := range map[int]int{1: 14, 2: 28, 3: 42, 4: 57, 5: 71, 6: 85} {
		for i := 0; i < reps; i++ {
			values = append(values, edge)
		}
	}
	values = append(values, 100)

	h := Histogram(values, 7)

	wantLo := []int{0, 14, 28, 42, 57, 71, 85}
	wantHi := []int{14, 28, 42, 57, 71, 85, 100}
	wantCounts := []int{1, 1, 2, 3, 4, 5, 7} // the maximum joins the last bucket

	if len(h.Buckets) != len(wantLo) {
		t.Fatalf("got %d buckets, want %d", len(h.Buckets), len(wantLo))
	}
	for i, b := range h.Buckets {
		if b.Lo != wantLo[i] || b.Hi != wantHi[i] {
			t.Errorf("bucket %d = [%d,%d), want [%d,%d)", i, b.Lo, b.Hi, wantLo[i], wantHi[i])
		}
		if b.Count != wantCounts[i] {
			t.Errorf("bucket %d [%d,%d) count = %d, want %d: the value on edge %d "+
				"must be counted in the bucket its own label starts",
				i, b.Lo, b.Hi, b.Count, wantCounts[i], wantLo[i])
		}
	}
}

func TestHistogram_UnevenRangeStillTiles(t *testing.T) {
	// The tiling guarantee must survive a range that does not divide evenly:
	// no gaps, no overlaps, and the outer edges are the real min and max.
	h := Histogram([]int{3, 40, 77, 91, 100}, 7)

	if len(h.Buckets) != 7 {
		t.Fatalf("got %d buckets, want 7", len(h.Buckets))
	}
	if h.Buckets[0].Lo != 3 {
		t.Errorf("first bucket Lo = %d, want the minimum 3", h.Buckets[0].Lo)
	}
	if h.Buckets[6].Hi != 100 {
		t.Errorf("last bucket Hi = %d, want the maximum 100", h.Buckets[6].Hi)
	}
	for i := 1; i < len(h.Buckets); i++ {
		if h.Buckets[i].Lo != h.Buckets[i-1].Hi {
			t.Errorf("bucket %d starts at %d, but bucket %d ended at %d",
				i, h.Buckets[i].Lo, i-1, h.Buckets[i-1].Hi)
		}
	}
}

func TestHistogram_MaximumLandsInTheLastBucket(t *testing.T) {
	// The maximum divides exactly onto the upper edge, which is one past the
	// last bucket. It must be pulled back in rather than dropped or panicking.
	h := Histogram([]int{0, 10}, 2)

	want := []int{1, 1}
	if got := bucketCounts(h); !equalInts(got, want) {
		t.Errorf("bucket counts = %v, want %v: the maximum must fall in the last bucket", got, want)
	}
}

func TestHistogram_IdenticalValuesDoNotDivideByZero(t *testing.T) {
	// A run where every proxy reports the same latency has a zero-width range.
	h := Histogram([]int{7, 7, 7}, 4)

	if len(h.Buckets) != 4 {
		t.Fatalf("got %d buckets, want 4", len(h.Buckets))
	}
	total := 0
	for _, b := range h.Buckets {
		total += b.Count
	}
	if total != 3 {
		t.Errorf("counted %d values, want all 3", total)
	}
	// The synthetic hi = lo + 1 range is narrower than the bucket count, so
	// the integer labels collapse: buckets 0..2 are all [7,7) and hold
	// nothing, and [7,8) is the only label that contains the value. Binning
	// follows the labels, so that is where all three land.
	if h.Buckets[3].Count != 3 {
		t.Errorf("bucket 3 %v count = %d, want all 3 identical values in the only "+
			"bucket whose label contains them", h.Buckets[3], h.Buckets[3].Count)
	}
}

func TestHistogram_EveryValueLandsInABucketItsLabelContains(t *testing.T) {
	// The invariant behind the binning: no value may be counted in a bucket
	// whose own [Lo,Hi) label excludes it. The sole exception is the maximum,
	// which joins the last bucket by design.
	//
	// The range is swept densely rather than sampled, because a violation only
	// shows at a value sitting exactly on a truncated edge. A handful of
	// scattered values misses those edges and passes against binning that is
	// in fact inconsistent with its own labels.
	values := make([]int, 0, 101)
	for v := 0; v <= 100; v++ {
		values = append(values, v)
	}

	for _, n := range []int{2, 3, 7, 11, 13, 30, 64} {
		h := Histogram(values, n)
		max := values[len(values)-1]

		total := 0
		for _, b := range h.Buckets {
			total += b.Count
		}
		if total != len(values) {
			t.Fatalf("n=%d: counted %d values, want %d", n, total, len(values))
		}

		// Walk the buckets in order, handing each its counted values in
		// ascending order, and check every one against the label it landed in.
		sorted := append([]int(nil), values...)
		at := 0
		for i, b := range h.Buckets {
			isLast := i == len(h.Buckets)-1
			for k := 0; k < b.Count; k++ {
				v := sorted[at]
				at++
				if v < b.Lo || (v >= b.Hi && !(v == max && isLast)) {
					t.Errorf("n=%d: value %d counted in bucket %d [%d,%d), which excludes it",
						n, v, i, b.Lo, b.Hi)
				}
			}
		}
	}
}

func TestHistogram_CountsEveryValueExactlyOnce(t *testing.T) {
	// Sorted: 1 1 1 3 3 4 57 90 91 200, so lo=1 and hi=200. The expected
	// vectors below are derived from the integer edges by hand; asserting the
	// per-bucket shape rather than only the total is what distinguishes
	// correct binning from routing every value into one bucket.
	values := []int{3, 3, 4, 90, 91, 200, 1, 1, 1, 57}

	tests := []struct {
		name       string
		n          int
		wantCounts []int
	}{
		// Edges [1,200): everything in the single bucket.
		{"one bucket", 1, []int{10}},
		// Edges [1,100,200): nine below 100, then the maximum.
		{"two buckets", 2, []int{9, 1}},
		// Edges [1,50,100,150,200): 1,1,1,3,3,4 | 57,90,91 | - | 200.
		{"four buckets", 4, []int{6, 3, 0, 1}},
		// Edges [1,20,40,60,80,100,120,140,160,180,200).
		{"ten buckets", 10, []int{6, 0, 1, 0, 2, 0, 0, 0, 0, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Histogram(values, tt.n)
			if len(h.Buckets) != tt.n {
				t.Fatalf("got %d buckets, want %d", len(h.Buckets), tt.n)
			}
			if got := bucketCounts(h); !equalInts(got, tt.wantCounts) {
				t.Errorf("counts = %v, want %v (buckets %+v)", got, tt.wantCounts, h.Buckets)
			}
		})
	}

	t.Run("far more buckets than values", func(t *testing.T) {
		// Too many buckets to spell out, but no value may be lost or doubled.
		h := Histogram(values, 64)
		if len(h.Buckets) != 64 {
			t.Fatalf("got %d buckets, want 64", len(h.Buckets))
		}
		total := 0
		for _, b := range h.Buckets {
			total += b.Count
		}
		if total != len(values) {
			t.Errorf("counted %d values, want %d", total, len(values))
		}
	})
}

func TestHistogram_DegenerateArguments(t *testing.T) {
	tests := []struct {
		name   string
		values []int
		n      int
	}{
		{"nil values", nil, 10},
		{"no values", []int{}, 10},
		{"zero buckets", []int{1, 2, 3}, 0},
		{"negative buckets", []int{1, 2, 3}, -4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if h := Histogram(tt.values, tt.n); len(h.Buckets) != 0 {
				t.Errorf("got %d buckets, want 0", len(h.Buckets))
			}
		})
	}
}

func TestECDF(t *testing.T) {
	// Unsorted on input: the curve is only meaningful ascending.
	points := ECDF([]int{30, 10, 40, 20})

	wantValues := []int{10, 20, 30, 40}
	wantP := []float64{0.25, 0.5, 0.75, 1.0}
	if len(points) != len(wantValues) {
		t.Fatalf("got %d points, want %d", len(points), len(wantValues))
	}
	for i := range wantValues {
		if points[i].Value != wantValues[i] {
			t.Errorf("point %d value = %d, want %d (ascending)", i, points[i].Value, wantValues[i])
		}
		if points[i].P != wantP[i] {
			t.Errorf("point %d P = %v, want %v", i, points[i].P, wantP[i])
		}
	}
}

func TestECDF_KeepsDuplicates(t *testing.T) {
	// Repeated latencies are real data; collapsing them would flatten the
	// curve exactly where the mass is.
	points := ECDF([]int{5, 5, 5, 5})

	// Every step is checked, not just the ends: an off-by-one confined to the
	// interior leaves the first and last points correct.
	wantP := []float64{0.25, 0.5, 0.75, 1.0}
	if len(points) != len(wantP) {
		t.Fatalf("got %d points, want %d", len(points), len(wantP))
	}
	for i, want := range wantP {
		if points[i].Value != 5 {
			t.Errorf("point %d value = %d, want 5", i, points[i].Value)
		}
		if points[i].P != want {
			t.Errorf("point %d P = %v, want %v", i, points[i].P, want)
		}
	}
}

func TestECDF_EmptyInput(t *testing.T) {
	if got := ECDF(nil); got != nil {
		t.Errorf("ECDF(nil) = %v, want nil", got)
	}
	if got := ECDF([]int{}); len(got) != 0 {
		t.Errorf("ECDF(empty) = %v, want no points", got)
	}
}

func TestECDF_DoesNotMutateInput(t *testing.T) {
	in := []int{30, 10, 20}
	want := []int{30, 10, 20}

	ECDF(in)

	if !equalInts(in, want) {
		t.Errorf("input mutated: got %v, want %v", in, want)
	}
}
