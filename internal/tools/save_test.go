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

func TestDedupByExitIdentity(t *testing.T) {
	proxies := []proxy.Proxy{
		{Raw: "p0"}, {Raw: "p1"}, {Raw: "p2"}, {Raw: "p3"}, {Raw: "p4"}, {Raw: "p5"},
	}

	tests := []struct {
		name    string
		mode    IPMode
		results []ipResult
		want    []string
	}{
		{
			name: "single family dedupes on that family",
			mode: IPModeV4,
			results: []ipResult{
				{Index: 0, IPv4: "9.9.9.9"},
				{Index: 1, IPv4: "8.8.8.8"},
				{Index: 2, IPv4: "9.9.9.9"}, // duplicate of p0
				{Index: 3, Err: errTest},    // errored, skipped
			},
			want: []string{"p0", "p1"},
		},
		{
			name: "ipv6 mode ignores the v4 address entirely",
			mode: IPModeV6,
			results: []ipResult{
				{Index: 0, IPv4: "9.9.9.9", IPv6: "2001:db8::1"},
				// A different v4 exit on the same v6 exit is a duplicate here,
				// because this run was only ever asked about v6.
				{Index: 1, IPv4: "8.8.8.8", IPv6: "2001:db8::1"},
				{Index: 2, IPv4: "9.9.9.9", IPv6: "2001:db8::2"},
			},
			want: []string{"p0", "p2"},
		},
		{
			name: "both mode drops a proxy only when both addresses match",
			mode: IPModeBoth,
			results: []ipResult{
				{Index: 0, IPv4: "9.9.9.9", IPv6: "2001:db8::1"},
				{Index: 1, IPv4: "9.9.9.9", IPv6: "2001:db8::1"}, // the whole pair repeats
				{Index: 2, IPv4: "8.8.8.8", IPv6: "2001:db8::2"},
			},
			want: []string{"p0", "p2"},
		},
		{
			// The conservative half of the rule, and the one that costs money if
			// it is wrong: a shared v4 exit with distinct v6 exits is two proxies,
			// and so is a shared v6 exit with distinct v4 exits.
			name: "both mode keeps a partial match",
			mode: IPModeBoth,
			results: []ipResult{
				{Index: 0, IPv4: "9.9.9.9", IPv6: "2001:db8::1"},
				{Index: 1, IPv4: "9.9.9.9", IPv6: "2001:db8::2"}, // same v4, different v6
				{Index: 2, IPv4: "8.8.8.8", IPv6: "2001:db8::1"}, // same v6, different v4
				// A missing v6 is its own identity, not a match for either of the
				// pairs above: nothing is known about its v6 exit.
				{Index: 3, IPv4: "9.9.9.9"},
				{Index: 4, IPv4: "9.9.9.9"}, // same partial pair, so a duplicate
				{Index: 5},                  // nothing answered, skipped
			},
			want: []string{"p0", "p1", "p2", "p3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupByExitIdentity(tt.results, proxies, tt.mode)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d]=%q want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

var errTest = &testErr{}

type testErr struct{}

func (*testErr) Error() string { return "boom" }
