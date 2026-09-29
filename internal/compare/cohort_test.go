package compare

import (
	"math"
	"testing"
)

func TestPearson(t *testing.T) {
	tests := []struct {
		name   string
		xs, ys []float64
		want   float64
	}{
		{"perfectly correlated", []float64{1, 2, 3}, []float64{2, 4, 6}, 1.0},
		{"perfectly anti-correlated", []float64{1, 2, 3}, []float64{3, 2, 1}, -1.0},
		{"offset does not change it", []float64{1, 2, 3}, []float64{101, 102, 103}, 1.0},
		{"partially correlated", []float64{1, 2, 3, 4}, []float64{1, 2, 3, 10}, 0.8854377448471463},
		{"mismatched lengths", []float64{1, 2}, []float64{1}, 0},
		{"both empty", nil, nil, 0},
		{"zero variance in x", []float64{5, 5, 5}, []float64{1, 2, 3}, 0},
		{"zero variance in y", []float64{1, 2, 3}, []float64{5, 5, 5}, 0},
		{"single point has no variance", []float64{1}, []float64{2}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pearson(tt.xs, tt.ys)
			if math.IsNaN(got) {
				t.Fatalf("Pearson = NaN, want %v: NaN breaks the chart rather than reading as no relationship", tt.want)
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("Pearson = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPearson_DoesNotMutateInput(t *testing.T) {
	xs := []float64{3, 1, 2}
	ys := []float64{9, 4, 6}

	Pearson(xs, ys)

	if xs[0] != 3 || xs[1] != 1 || xs[2] != 2 {
		t.Errorf("xs mutated: %v", xs)
	}
	if ys[0] != 9 || ys[1] != 4 || ys[2] != 6 {
		t.Errorf("ys mutated: %v", ys)
	}
}
