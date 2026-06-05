package tools

import (
	"errors"
	"testing"
	"time"

	"proxytoolbox/internal/proxy"
)

func TestEvaluateAlert(t *testing.T) {
	now := time.Now()

	type step struct {
		success bool
		want    alertAction
	}

	tests := []struct {
		name        string
		downT, upT  int
		steps       []step
		wantAlerted bool
	}{
		{
			name:  "single failure below threshold",
			downT: 3, upT: 2,
			steps: []step{
				{false, alertNone},
			},
			wantAlerted: false,
		},
		{
			name:  "three failures trigger down once",
			downT: 3, upT: 2,
			steps: []step{
				{false, alertNone},
				{false, alertNone},
				{false, alertDown},
				{false, alertNone},
			},
			wantAlerted: true,
		},
		{
			name:  "single success after down does not recover",
			downT: 3, upT: 2,
			steps: []step{
				{false, alertNone},
				{false, alertNone},
				{false, alertDown},
				{true, alertNone},
			},
			wantAlerted: true,
		},
		{
			name:  "two successes after down recover",
			downT: 3, upT: 2,
			steps: []step{
				{false, alertNone},
				{false, alertNone},
				{false, alertDown},
				{true, alertNone},
				{true, alertUp},
				{true, alertNone},
			},
			wantAlerted: false,
		},
		{
			name:  "flap resets before threshold",
			downT: 3, upT: 2,
			steps: []step{
				{false, alertNone},
				{false, alertNone},
				{true, alertNone},
				{false, alertNone},
				{false, alertNone},
				{false, alertDown},
			},
			wantAlerted: true,
		},
		{
			name:  "successes without prior down never alert",
			downT: 3, upT: 2,
			steps: []step{
				{true, alertNone},
				{true, alertNone},
				{true, alertNone},
			},
			wantAlerted: false,
		},
		{
			name:  "thresholds of 1 alert on every transition",
			downT: 1, upT: 1,
			steps: []step{
				{false, alertDown},
				{true, alertUp},
				{false, alertDown},
				{true, alertUp},
			},
			wantAlerted: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &proxyStats{}
			for i, st := range tc.steps {
				got := evaluateAlert(s, st.success, tc.downT, tc.upT, now)
				if got != st.want {
					t.Fatalf("step %d: got %v, want %v", i, got, st.want)
				}
			}
			if s.AlertedDown != tc.wantAlerted {
				t.Fatalf("final AlertedDown: got %v, want %v", s.AlertedDown, tc.wantAlerted)
			}
		})
	}
}

func TestEvaluateAlertDownSinceSet(t *testing.T) {
	s := &proxyStats{}
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	evaluateAlert(s, false, 2, 2, now)
	if !s.DownSince.IsZero() {
		t.Fatalf("DownSince set before threshold crossed")
	}
	later := now.Add(time.Second)
	if got := evaluateAlert(s, false, 2, 2, later); got != alertDown {
		t.Fatalf("expected alertDown on threshold, got %v", got)
	}
	if !s.DownSince.Equal(later) {
		t.Fatalf("DownSince = %v, want %v", s.DownSince, later)
	}
}

func TestBuildDownEmbedShape(t *testing.T) {
	p := proxy.Proxy{Host: "1.2.3.4", Port: "8080"}
	e := buildDownEmbed(p, "https://example.com", errors.New("connection refused"), 3, 7)
	if e.Color != colorDown {
		t.Errorf("Color = %#x, want %#x", e.Color, colorDown)
	}
	if e.Title != "Proxy DOWN" {
		t.Errorf("Title = %q", e.Title)
	}
	if len(e.Fields) == 0 {
		t.Fatal("no fields")
	}
	found := false
	for _, f := range e.Fields {
		if f.Name == "Proxy" && f.Value == "`1.2.3.4:8080`" {
			found = true
		}
	}
	if !found {
		t.Errorf("Proxy field missing or malformed: %+v", e.Fields)
	}
}

func TestBuildUpEmbedShape(t *testing.T) {
	p := proxy.Proxy{Host: "1.2.3.4", Port: "8080"}
	e := buildUpEmbed(p, "https://example.com", 42*time.Second, 10)
	if e.Color != colorUp {
		t.Errorf("Color = %#x, want %#x", e.Color, colorUp)
	}
	if e.Title != "Proxy RECOVERED" {
		t.Errorf("Title = %q", e.Title)
	}
	found := false
	for _, f := range e.Fields {
		if f.Name == "Downtime" && f.Value == "42s" {
			found = true
		}
	}
	if !found {
		t.Errorf("Downtime field missing: %+v", e.Fields)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("short truncate: %q", got)
	}
	if got := truncate("abcdefghij", 5); got != "abcde..." {
		t.Errorf("long truncate: %q", got)
	}
}
