package config

import "testing"

func TestParseLatency(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"1500", 1500},
		{"", 0},
		{"abc", 0},
		{"0", 0},
		{"-5", 0},
	}
	for _, tt := range tests {
		if got := parseLatency(tt.in); got != tt.want {
			t.Errorf("parseLatency(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
