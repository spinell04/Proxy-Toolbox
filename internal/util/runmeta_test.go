package util

import (
	"testing"
	"time"
)

func TestRunMeta_Rows(t *testing.T) {
	m := RunMeta{
		Tool:      "pinger",
		RunAt:     time.Date(2026, 9, 19, 14, 32, 7, 0, time.UTC),
		ProxyFile: "/Users/someone/Desktop/Proxy-Toolbox/proxyfiles/residential_de.txt",
		Target:    "https://google.com",
		Workers:   100,
	}

	got := m.Rows()
	want := [][]string{
		{"Tool", "pinger"},
		{"Run at", "2026-09-19T14:32:07Z"},
		{"Proxy file", "residential_de.txt"}, // base name only, not the user's full path
		{"Target", "https://google.com"},
		{"Workers", "100"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i][0] != want[i][0] || got[i][1] != want[i][1] {
			t.Errorf("row %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestRunMeta_RowsZeroValues(t *testing.T) {
	tests := []struct {
		name string
		meta RunMeta
		want [][]string
	}{
		{
			name: "fully zero-valued",
			meta: RunMeta{},
			want: [][]string{
				{"Tool", ""},
				{"Run at", "0001-01-01T00:00:00Z"},
				{"Proxy file", ""}, // not ".", which filepath.Base("") would return
				{"Target", ""},
				{"Workers", "0"},
			},
		},
		{
			name: "iptester has no target",
			meta: RunMeta{
				Tool:      "iptester",
				RunAt:     time.Date(2026, 9, 19, 9, 5, 0, 0, time.UTC),
				ProxyFile: "/Users/someone/Desktop/Proxy-Toolbox/proxyfiles/datacenter_us.txt",
				Workers:   20,
			},
			want: [][]string{
				{"Tool", "iptester"},
				{"Run at", "2026-09-19T09:05:00Z"},
				{"Proxy file", "datacenter_us.txt"},
				{"Target", ""},
				{"Workers", "20"},
			},
		},
		{
			name: "no workers configured",
			meta: RunMeta{
				Tool:    "pinger",
				RunAt:   time.Date(2026, 9, 19, 9, 5, 0, 0, time.UTC),
				Target:  "https://google.com",
				Workers: 0,
			},
			want: [][]string{
				{"Tool", "pinger"},
				{"Run at", "2026-09-19T09:05:00Z"},
				{"Proxy file", ""},
				{"Target", "https://google.com"},
				{"Workers", "0"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.meta.Rows()
			if len(got) != len(tt.want) {
				t.Fatalf("got %d rows, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i][0] != tt.want[i][0] || got[i][1] != tt.want[i][1] {
					t.Errorf("row %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
