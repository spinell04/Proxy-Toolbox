package tools

import (
	"testing"

	"proxytoolbox/internal/proxy"
)

func TestPassLatency(t *testing.T) {
	tests := []struct {
		name  string
		ms    int64
		maxMs int
		want  bool
	}{
		{"under limit", 900, 1000, true},
		{"over limit", 1200, 1000, false},
		{"equal is excluded", 1000, 1000, false},
		{"no limit saves all", 5000, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := passLatency(tt.ms, tt.maxMs); got != tt.want {
				t.Errorf("passLatency(%d,%d) = %v, want %v", tt.ms, tt.maxMs, got, tt.want)
			}
		})
	}
}

func TestDedupByIP(t *testing.T) {
	proxies := []proxy.Proxy{
		{Raw: "p0"}, {Raw: "p1"}, {Raw: "p2"}, {Raw: "p3"},
	}
	results := []ipResult{
		{Index: 0, IP: "9.9.9.9"},
		{Index: 1, IP: "8.8.8.8"},
		{Index: 2, IP: "9.9.9.9"}, // duplicate of p0
		{Index: 3, Err: errTest},  // errored, skipped
	}
	got := dedupByIP(results, proxies)
	want := []string{"p0", "p1"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

var errTest = &testErr{}

type testErr struct{}

func (*testErr) Error() string { return "boom" }
