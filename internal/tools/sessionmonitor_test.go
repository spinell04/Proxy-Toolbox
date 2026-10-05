package tools

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"proxytoolbox/internal/proxy"
)

var errCheck = errors.New("i/o timeout")

// step is one check fed to evaluateSession, with the alerts it must earn. The
// bare helpers below describe a single-family (IPv4) run, which is what the
// failure-ladder and recovery cases exercise; the both-mode cases build their
// own steps.
type step struct {
	r      ipResult
	health healthAction
	ips    []ipChange
}

func v4(ip string) ipResult { return ipResult{IPv4: ip} }

func fail() step      { return step{r: ipResult{Err: errCheck}, health: healthNone} }
func failAlert() step { return step{r: ipResult{Err: errCheck}, health: healthFailAlert} }
func ok(ip string) step {
	return step{r: v4(ip)}
}
func okBaseline(ip string) step {
	return step{r: v4(ip), ips: []ipChange{{Family: familyV4, Action: ipBaseline, NewIP: ip}}}
}
func okRotated(prev, ip string) step {
	return step{r: v4(ip), ips: []ipChange{{Family: familyV4, Action: ipRotated, PrevIP: prev, NewIP: ip}}}
}
func okRecovered(ip string) step { return step{r: v4(ip), health: healthRecovered} }

func runSteps(t *testing.T, s *sessionStats, mode IPMode, steps []step) {
	t.Helper()
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	for i, st := range steps {
		now = now.Add(time.Second)
		got := evaluateSession(s, st.r, mode, now)
		if got.Health != st.health {
			t.Fatalf("step %d: Health = %v, want %v", i+1, got.Health, st.health)
		}
		if len(got.IPs) != len(st.ips) {
			t.Fatalf("step %d: IPs = %+v, want %+v", i+1, got.IPs, st.ips)
		}
		for j, want := range st.ips {
			g := got.IPs[j]
			if g.Family != want.Family || g.Action != want.Action ||
				g.PrevIP != want.PrevIP || g.NewIP != want.NewIP {
				t.Fatalf("step %d change %d = %+v, want family=%s action=%v prev=%q new=%q",
					i+1, j, g, want.Family, want.Action, want.PrevIP, want.NewIP)
			}
		}
	}
}

// repeat builds n identical steps, used to walk the ladder without writing
// thirty literals.
func repeat(n int, s step) []step {
	out := make([]step, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func concat(groups ...[]step) []step {
	var out []step
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func TestSessionFailureLadder(t *testing.T) {
	tests := []struct {
		name  string
		steps []step
	}{
		{
			name:  "failures 1-4 are silent",
			steps: repeat(4, fail()),
		},
		{
			name:  "fifth consecutive failure alerts",
			steps: concat(repeat(4, fail()), []step{failAlert()}),
		},
		{
			name:  "failures 6-29 are silent",
			steps: concat(repeat(4, fail()), []step{failAlert()}, repeat(24, fail())),
		},
		{
			name: "thirtieth consecutive failure alerts again",
			steps: concat(
				repeat(4, fail()), []step{failAlert()},
				repeat(24, fail()), []step{failAlert()},
			),
		},
		{
			name: "past thirty the ladder stays silent",
			steps: concat(
				repeat(4, fail()), []step{failAlert()},
				repeat(24, fail()), []step{failAlert()},
				repeat(40, fail()),
			),
		},
		{
			name: "success re-arms the ladder",
			steps: concat(
				repeat(4, fail()), []step{failAlert()},
				[]step{{r: v4("1.1.1.1"), health: healthRecovered, ips: []ipChange{{Family: familyV4, Action: ipBaseline, NewIP: "1.1.1.1"}}}},
				repeat(4, fail()), []step{failAlert()},
			),
		},
		{
			name: "a success below the threshold also re-arms",
			steps: concat(
				repeat(4, fail()),
				[]step{okBaseline("1.1.1.1")},
				repeat(4, fail()), []step{failAlert()},
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSteps(t, &sessionStats{}, IPModeV4, tc.steps)
		})
	}
}

func TestSessionRecovery(t *testing.T) {
	tests := []struct {
		name  string
		steps []step
	}{
		{
			name:  "failures 1-4 then success is silent",
			steps: concat(repeat(4, fail()), []step{okBaseline("1.1.1.1")}),
		},
		{
			name: "success with no prior failures is silent",
			steps: []step{
				okBaseline("1.1.1.1"),
				ok("1.1.1.1"),
				ok("1.1.1.1"),
			},
		},
		{
			name: "alerted streak then success recovers exactly once",
			steps: concat(
				repeat(4, fail()), []step{failAlert()},
				[]step{{r: v4("1.1.1.1"), health: healthRecovered, ips: []ipChange{{Family: familyV4, Action: ipBaseline, NewIP: "1.1.1.1"}}}},
				[]step{ok("1.1.1.1"), ok("1.1.1.1")},
			),
		},
		{
			name: "two ladder alerts still recover only once",
			steps: concat(
				repeat(4, fail()), []step{failAlert()},
				repeat(24, fail()), []step{failAlert()},
				repeat(10, fail()),
				[]step{{r: v4("1.1.1.1"), health: healthRecovered, ips: []ipChange{{Family: familyV4, Action: ipBaseline, NewIP: "1.1.1.1"}}}},
				[]step{ok("1.1.1.1")},
			),
		},
		{
			name: "two down/up cycles recover once each",
			steps: concat(
				[]step{okBaseline("1.1.1.1")},
				repeat(4, fail()), []step{failAlert()},
				[]step{okRecovered("1.1.1.1")},
				[]step{ok("1.1.1.1")},
				repeat(4, fail()), []step{failAlert()},
				[]step{okRecovered("1.1.1.1")},
				[]step{ok("1.1.1.1")},
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSteps(t, &sessionStats{}, IPModeV4, tc.steps)
		})
	}
}

func TestSessionIPChangeDetection(t *testing.T) {
	tests := []struct {
		name      string
		steps     []step
		wantIP    string
		wantRots  int
		wantSeen  int
		wantFails int
	}{
		{
			name:     "first observation is a silent baseline",
			steps:    []step{okBaseline("1.1.1.1")},
			wantIP:   "1.1.1.1",
			wantRots: 0,
			wantSeen: 1,
		},
		{
			name:     "same IP repeated is silent",
			steps:    []step{okBaseline("1.1.1.1"), ok("1.1.1.1"), ok("1.1.1.1")},
			wantIP:   "1.1.1.1",
			wantRots: 0,
			wantSeen: 1,
		},
		{
			name:     "a different IP is a rotation",
			steps:    []step{okBaseline("1.1.1.1"), okRotated("1.1.1.1", "2.2.2.2")},
			wantIP:   "2.2.2.2",
			wantRots: 1,
			wantSeen: 2,
		},
		{
			name: "every rotation counts, including a return to an earlier IP",
			steps: []step{
				okBaseline("1.1.1.1"),
				okRotated("1.1.1.1", "2.2.2.2"),
				okRotated("2.2.2.2", "1.1.1.1"),
				okRotated("1.1.1.1", "2.2.2.2"),
			},
			wantIP:   "2.2.2.2",
			wantRots: 3,
			wantSeen: 2,
		},
		{
			name: "a failed check is not a rotation and keeps the last known IP",
			steps: concat(
				[]step{okBaseline("1.1.1.1")},
				repeat(4, fail()),
				[]step{ok("1.1.1.1")},
			),
			wantIP:    "1.1.1.1",
			wantRots:  0,
			wantSeen:  1,
			wantFails: 4,
		},
		{
			name: "a failure before any success leaves no baseline",
			steps: concat(
				repeat(3, fail()),
				[]step{okBaseline("1.1.1.1")},
			),
			wantIP:    "1.1.1.1",
			wantRots:  0,
			wantSeen:  1,
			wantFails: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &sessionStats{}
			runSteps(t, s, IPModeV4, tc.steps)
			if s.V4.Current != tc.wantIP {
				t.Errorf("V4.Current = %q, want %q", s.V4.Current, tc.wantIP)
			}
			if s.Rotations != tc.wantRots {
				t.Errorf("Rotations = %d, want %d", s.Rotations, tc.wantRots)
			}
			if len(s.V4.Seen) != tc.wantSeen {
				t.Errorf("V4.Seen = %d, want %d", len(s.V4.Seen), tc.wantSeen)
			}
			if s.Failures != tc.wantFails {
				t.Errorf("Failures = %d, want %d", s.Failures, tc.wantFails)
			}
			if s.TotalChecks != len(tc.steps) {
				t.Errorf("TotalChecks = %d, want %d", s.TotalChecks, len(tc.steps))
			}
		})
	}
}

// A failure must not disturb the IP the caller reports as "previous", nor the
// timestamp the hold duration is measured from.
func TestSessionFailureLeavesIPStateUntouched(t *testing.T) {
	s := &sessionStats{}
	base := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	evaluateSession(s, v4("1.1.1.1"), IPModeV4, base)
	since := s.V4.Since

	for i := 1; i <= 10; i++ {
		evaluateSession(s, ipResult{Err: errCheck}, IPModeV4, base.Add(time.Duration(i)*time.Minute))
		if s.V4.Current != "1.1.1.1" {
			t.Fatalf("failure %d overwrote V4.Current: %q", i, s.V4.Current)
		}
		if !s.V4.Since.Equal(since) {
			t.Fatalf("failure %d moved V4.Since", i)
		}
	}
	if len(s.V4.Seen) != 1 {
		t.Errorf("failures recorded an exit IP: %v", s.V4.Seen)
	}
}

func TestSessionFailingSinceAndLastErr(t *testing.T) {
	s := &sessionStats{}
	base := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	first := errors.New("first")
	evaluateSession(s, ipResult{Err: first}, IPModeV4, base)
	if !s.FailingSince.Equal(base) {
		t.Fatalf("FailingSince = %v, want %v", s.FailingSince, base)
	}

	// A continuing streak keeps its start time but tracks the latest error.
	last := errors.New("last")
	evaluateSession(s, ipResult{Err: last}, IPModeV4, base.Add(time.Minute))
	if !s.FailingSince.Equal(base) {
		t.Fatalf("FailingSince moved mid-streak: %v", s.FailingSince)
	}
	if s.LastErr != last {
		t.Fatalf("LastErr = %v, want %v", s.LastErr, last)
	}

	// A new streak after a success restarts the clock.
	evaluateSession(s, v4("1.1.1.1"), IPModeV4, base.Add(2*time.Minute))
	restart := base.Add(3 * time.Minute)
	evaluateSession(s, ipResult{Err: errCheck}, IPModeV4, restart)
	if !s.FailingSince.Equal(restart) {
		t.Fatalf("FailingSince = %v, want %v", s.FailingSince, restart)
	}
}

func TestSessionRotationTimestamps(t *testing.T) {
	s := &sessionStats{}
	base := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	evaluateSession(s, v4("1.1.1.1"), IPModeV4, base)
	if !s.V4.Since.Equal(base) {
		t.Fatalf("baseline V4.Since = %v, want %v", s.V4.Since, base)
	}

	// An unchanged IP must not reset the hold clock, or every rotation embed
	// would report a hold of one interval.
	evaluateSession(s, v4("1.1.1.1"), IPModeV4, base.Add(time.Minute))
	if !s.V4.Since.Equal(base) {
		t.Fatalf("unchanged IP moved V4.Since to %v", s.V4.Since)
	}

	rot := base.Add(2 * time.Minute)
	out := evaluateSession(s, v4("2.2.2.2"), IPModeV4, rot)
	if !s.V4.Since.Equal(rot) {
		t.Fatalf("rotation V4.Since = %v, want %v", s.V4.Since, rot)
	}
	// The hold the outcome reports is measured from the previous answer, not
	// from this one.
	if len(out.IPs) != 1 || out.IPs[0].Held != 2*time.Minute {
		t.Fatalf("outcome = %+v, want one change held 2m", out.IPs)
	}
}

func TestFormatSamplingPeriod(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{time.Second, "1s"},
		{59 * time.Second, "59s"},
		{time.Minute, "1m"},
		{90 * time.Second, "1m"},
		{59*time.Minute + 59*time.Second, "59m"},
		{time.Hour, "60m"},
		{100 * time.Minute, "100m"},
		{119*time.Minute + 59*time.Second, "119m"},
		{2 * time.Hour, "2.0h"},
		{150 * time.Minute, "2.5h"},
		{25 * time.Hour, "25.0h"},
	}
	for _, tt := range tests {
		if got := formatSamplingPeriod(tt.d); got != tt.want {
			t.Errorf("formatSamplingPeriod(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// The banner line the owner specified, rebuilt from the same inputs the tool
// uses, so a change to either half of the arithmetic shows up here.
func TestSamplingPeriodBannerArithmetic(t *testing.T) {
	proxies := 100
	interval := 60000 * time.Millisecond
	got := fmt.Sprintf("%d proxies — each checked once every ~%s",
		proxies, formatSamplingPeriod(time.Duration(proxies)*interval))
	want := "100 proxies — each checked once every ~100m"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func fieldValue(e discordEmbed, name string) (string, bool) {
	for _, f := range e.Fields {
		if f.Name == name {
			return f.Value, true
		}
	}
	return "", false
}

func TestBuildRotationEmbedShape(t *testing.T) {
	p := proxy.Proxy{Host: "1.2.3.4", Port: "8080", User: "u", Password: "p"}
	e := buildRotationEmbed(p, 12, ipChange{
		Family: familyV6, Action: ipRotated,
		PrevIP: "2001:db8::1", NewIP: "2001:db8::2", Held: 90 * time.Second,
	}, 4)

	if e.Color != colorRotate {
		t.Errorf("Color = %#x, want %#x", e.Color, colorRotate)
	}
	// The family is in the title and in a field of its own: in both mode the
	// same proxy has two exits and a report that does not name one is unreadable.
	if e.Title != "Proxy IPv6 ROTATED" {
		t.Errorf("Title = %q", e.Title)
	}
	for name, want := range map[string]string{
		"#":           "12",
		"Family":      familyV6,
		"Previous IP": "`2001:db8::1`",
		"New IP":      "`2001:db8::2`",
		"Held For":    "1m30s",
		"Proxy":       fmt.Sprintf("`%s`", p.ID()),
	} {
		got, ok := fieldValue(e, name)
		if !ok {
			t.Errorf("field %q missing", name)
		} else if got != want {
			t.Errorf("field %q = %q, want %q", name, got, want)
		}
	}
}

func TestBuildSessionFailEmbedShape(t *testing.T) {
	p := proxy.Proxy{Host: "1.2.3.4", Port: "8080"}
	e := buildSessionFailEmbed(p, 7, errors.New("connection refused"), 5, 2)

	if e.Color != colorDown {
		t.Errorf("Color = %#x, want %#x", e.Color, colorDown)
	}
	if e.Title != "Proxy CHECK FAILING" {
		t.Errorf("Title = %q", e.Title)
	}
	if got, _ := fieldValue(e, "#"); got != "7" {
		t.Errorf("# = %q, want %q", got, "7")
	}
	if got, _ := fieldValue(e, "Consecutive Failures"); got != "5" {
		t.Errorf("Consecutive Failures = %q, want %q", got, "5")
	}
	if got, _ := fieldValue(e, "Error"); got != "connection refused" {
		t.Errorf("Error = %q", got)
	}
}

func TestBuildSessionRecoveredEmbedShape(t *testing.T) {
	p := proxy.Proxy{Host: "1.2.3.4", Port: "8080"}
	e := buildSessionRecoveredEmbed(p, 7, 5*time.Minute, errors.New("i/o timeout"), 9)

	if e.Color != colorUp {
		t.Errorf("Color = %#x, want %#x", e.Color, colorUp)
	}
	if e.Title != "Proxy CHECK RECOVERED" {
		t.Errorf("Title = %q", e.Title)
	}
	if got, _ := fieldValue(e, "#"); got != "7" {
		t.Errorf("# = %q, want %q", got, "7")
	}
	if got, _ := fieldValue(e, "Failing For"); got != "5m0s" {
		t.Errorf("Failing For = %q", got)
	}
	if got, _ := fieldValue(e, "Last Error"); got != "i/o timeout" {
		t.Errorf("Last Error = %q", got)
	}
}

// TestSessionProxyCol_FitsTheLongestID is the property that keeps a gateway
// pool readable: proxies from one pool differ only by a session id buried in
// the middle of the username, so any elision makes two of them identical in
// the one tool whose job is naming which proxy rotated.
func TestSessionProxyCol_FitsTheLongestID(t *testing.T) {
	// Synthetic credentials on a .invalid host, the same length as a real
	// gateway id and differing only in the session id in the middle — that
	// shape is the whole point of the test. Never paste a real credential
	// here: a test file is committed, and a commit is permanent.
	const a = "gwuser-aaaaaaaa:wwwwwwwwwwwwwwwwwwwwwwwww_country-DE_session-935814_lifetime-60@pool.example.invalid:1111"
	const b = "gwuser-aaaaaaaa:wwwwwwwwwwwwwwwwwwwwwwwww_country-DE_session-374010_lifetime-60@pool.example.invalid:1111"

	got := sessionProxyCol([]string{a, b})
	if got < len(a) {
		t.Errorf("sessionProxyCol = %d, want at least %d so the id is never cut", got, len(a))
	}
	// Both ids are the same length and differ only in the middle. A column that
	// fits them whole is the only one that keeps them distinguishable.
	if len(a) != len(b) {
		t.Fatal("fixture ids should be the same length")
	}
}

func TestSessionProxyCol_NeverNarrowerThanItsHeader(t *testing.T) {
	if got := sessionProxyCol(nil); got < len("Proxy") {
		t.Errorf("sessionProxyCol(nil) = %d, want at least %d", got, len("Proxy"))
	}
	if got := sessionProxyCol([]string{"ab"}); got < len("Proxy") {
		t.Errorf("sessionProxyCol(short) = %d, want the header width %d", got, len("Proxy"))
	}
}

// TestSessionProxyCol_CountsRunesNotBytes: a multi-byte id padded by byte count
// renders short, which breaks the column the table is aligned on.
func TestSessionProxyCol_CountsRunesNotBytes(t *testing.T) {
	const id = "üüüüüüüüüü" // 10 runes, 20 bytes
	if got := sessionProxyCol([]string{id}); got != 10 {
		t.Errorf("sessionProxyCol = %d, want 10 runes (not %d bytes)", got, len(id))
	}
}

func both(v4ip, v6ip string) ipResult { return ipResult{IPv4: v4ip, IPv6: v6ip} }

func chg(family string, action ipAction, prev, next string) ipChange {
	return ipChange{Family: family, Action: action, PrevIP: prev, NewIP: next}
}

// TestSessionBothModeRotationRules pins the rule the whole change rests on: a
// rotation is reported only when a family answers with a *different* address
// than it last answered with. Each rule is its own case, because each one is a
// separate way of reintroducing the bug being fixed.
func TestSessionBothModeRotationRules(t *testing.T) {
	tests := []struct {
		name     string
		steps    []step
		wantV4   string
		wantV6   string
		wantRots int
	}{
		{
			name: "the first answer from each family is a silent baseline",
			steps: []step{{
				r: both("1.1.1.1", "2001:db8::1"),
				ips: []ipChange{
					chg(familyV4, ipBaseline, "", "1.1.1.1"),
					chg(familyV6, ipBaseline, "", "2001:db8::1"),
				},
			}},
			wantV4: "1.1.1.1", wantV6: "2001:db8::1", wantRots: 0,
		},
		{
			name: "the same pair repeated is silent",
			steps: []step{
				{r: both("1.1.1.1", "2001:db8::1"), ips: []ipChange{
					chg(familyV4, ipBaseline, "", "1.1.1.1"),
					chg(familyV6, ipBaseline, "", "2001:db8::1"),
				}},
				{r: both("1.1.1.1", "2001:db8::1")},
				{r: both("1.1.1.1", "2001:db8::1")},
			},
			wantV4: "1.1.1.1", wantV6: "2001:db8::1", wantRots: 0,
		},
		{
			name: "v4 rotates while v6 holds",
			steps: []step{
				{r: both("1.1.1.1", "2001:db8::1"), ips: []ipChange{
					chg(familyV4, ipBaseline, "", "1.1.1.1"),
					chg(familyV6, ipBaseline, "", "2001:db8::1"),
				}},
				{r: both("2.2.2.2", "2001:db8::1"), ips: []ipChange{
					chg(familyV4, ipRotated, "1.1.1.1", "2.2.2.2"),
				}},
			},
			wantV4: "2.2.2.2", wantV6: "2001:db8::1", wantRots: 1,
		},
		{
			name: "v6 rotates while v4 holds",
			steps: []step{
				{r: both("1.1.1.1", "2001:db8::1"), ips: []ipChange{
					chg(familyV4, ipBaseline, "", "1.1.1.1"),
					chg(familyV6, ipBaseline, "", "2001:db8::1"),
				}},
				{r: both("1.1.1.1", "2001:db8::2"), ips: []ipChange{
					chg(familyV6, ipRotated, "2001:db8::1", "2001:db8::2"),
				}},
			},
			wantV4: "1.1.1.1", wantV6: "2001:db8::2", wantRots: 1,
		},
		{
			name: "both families rotate on one check",
			steps: []step{
				{r: both("1.1.1.1", "2001:db8::1"), ips: []ipChange{
					chg(familyV4, ipBaseline, "", "1.1.1.1"),
					chg(familyV6, ipBaseline, "", "2001:db8::1"),
				}},
				{r: both("2.2.2.2", "2001:db8::2"), ips: []ipChange{
					chg(familyV4, ipRotated, "1.1.1.1", "2.2.2.2"),
					chg(familyV6, ipRotated, "2001:db8::1", "2001:db8::2"),
				}},
			},
			wantV4: "2.2.2.2", wantV6: "2001:db8::2", wantRots: 2,
		},
		{
			// The rule that keeps the fix from reintroducing the bug. A proxy
			// with no IPv6 route and a transient IPv6 endpoint failure are
			// indistinguishable from here, so neither is a rotation and neither
			// clears what was last known.
			name: "a family going quiet is silent and keeps its address",
			steps: []step{
				{r: both("1.1.1.1", "2001:db8::1"), ips: []ipChange{
					chg(familyV4, ipBaseline, "", "1.1.1.1"),
					chg(familyV6, ipBaseline, "", "2001:db8::1"),
				}},
				{r: both("1.1.1.1", "")},
				{r: both("1.1.1.1", "")},
				{r: both("1.1.1.1", "")},
			},
			wantV4: "1.1.1.1", wantV6: "2001:db8::1", wantRots: 0,
		},
		{
			// And coming back with the address it had is still not a change.
			name: "a family that goes quiet and returns unchanged is silent",
			steps: []step{
				{r: both("1.1.1.1", "2001:db8::1"), ips: []ipChange{
					chg(familyV4, ipBaseline, "", "1.1.1.1"),
					chg(familyV6, ipBaseline, "", "2001:db8::1"),
				}},
				{r: both("1.1.1.1", "")},
				{r: both("1.1.1.1", "2001:db8::1")},
			},
			wantV4: "1.1.1.1", wantV6: "2001:db8::1", wantRots: 0,
		},
		{
			// Returning with a different address is a rotation, and the previous
			// address it reports is the one from before the gap.
			name: "a family that goes quiet and returns changed rotates",
			steps: []step{
				{r: both("1.1.1.1", "2001:db8::1"), ips: []ipChange{
					chg(familyV4, ipBaseline, "", "1.1.1.1"),
					chg(familyV6, ipBaseline, "", "2001:db8::1"),
				}},
				{r: both("1.1.1.1", "")},
				{r: both("1.1.1.1", "2001:db8::2"), ips: []ipChange{
					chg(familyV6, ipRotated, "2001:db8::1", "2001:db8::2"),
				}},
			},
			wantV4: "1.1.1.1", wantV6: "2001:db8::2", wantRots: 1,
		},
		{
			// Unknown -> known is a baseline, exactly like the first observation
			// of a proxy: a v6 route that only appears on the fourth check has
			// not rotated, it has been seen for the first time.
			name: "a family that answers for the first time later is a silent baseline",
			steps: []step{
				{r: both("1.1.1.1", ""), ips: []ipChange{
					chg(familyV4, ipBaseline, "", "1.1.1.1"),
				}},
				{r: both("1.1.1.1", "")},
				{r: both("1.1.1.1", "2001:db8::1"), ips: []ipChange{
					chg(familyV6, ipBaseline, "", "2001:db8::1"),
				}},
				{r: both("1.1.1.1", "2001:db8::1")},
			},
			wantV4: "1.1.1.1", wantV6: "2001:db8::1", wantRots: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &sessionStats{}
			runSteps(t, s, IPModeBoth, tc.steps)
			if s.V4.Current != tc.wantV4 {
				t.Errorf("V4.Current = %q, want %q", s.V4.Current, tc.wantV4)
			}
			if s.V6.Current != tc.wantV6 {
				t.Errorf("V6.Current = %q, want %q", s.V6.Current, tc.wantV6)
			}
			if s.Rotations != tc.wantRots {
				t.Errorf("Rotations = %d, want %d", s.Rotations, tc.wantRots)
			}
		})
	}
}

// A single check can earn a health recovery and a rotation at once: the proxy
// came back, and it came back on a different exit. Both must be reported.
func TestSessionRotationAndRecoveryInOneCheck(t *testing.T) {
	s := &sessionStats{}
	steps := concat(
		[]step{{r: both("1.1.1.1", "2001:db8::1"), ips: []ipChange{
			chg(familyV4, ipBaseline, "", "1.1.1.1"),
			chg(familyV6, ipBaseline, "", "2001:db8::1"),
		}}},
		repeat(4, fail()),
		[]step{failAlert()},
		[]step{{
			r:      both("2.2.2.2", "2001:db8::1"),
			health: healthRecovered,
			ips:    []ipChange{chg(familyV4, ipRotated, "1.1.1.1", "2.2.2.2")},
		}},
	)
	runSteps(t, s, IPModeBoth, steps)
	if s.Rotations != 1 {
		t.Errorf("Rotations = %d, want 1", s.Rotations)
	}
}

// A failed check says nothing about either family, so neither address moves and
// neither is recorded as seen.
func TestSessionBothModeFailureLeavesEveryFamilyUntouched(t *testing.T) {
	s := &sessionStats{}
	base := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	evaluateSession(s, both("1.1.1.1", "2001:db8::1"), IPModeBoth, base)
	v4Since, v6Since := s.V4.Since, s.V6.Since

	for i := 1; i <= 10; i++ {
		out := evaluateSession(s, ipResult{Err: errCheck}, IPModeBoth,
			base.Add(time.Duration(i)*time.Minute))
		if len(out.IPs) != 0 {
			t.Fatalf("failure %d reported %+v", i, out.IPs)
		}
	}
	if s.V4.Current != "1.1.1.1" || s.V6.Current != "2001:db8::1" {
		t.Fatalf("failures changed the addresses: v4=%q v6=%q", s.V4.Current, s.V6.Current)
	}
	if !s.V4.Since.Equal(v4Since) || !s.V6.Since.Equal(v6Since) {
		t.Fatal("failures moved a hold clock")
	}
	if len(s.V4.Seen) != 1 || len(s.V6.Seen) != 1 {
		t.Fatalf("failures recorded an address: v4=%v v6=%v", s.V4.Seen, s.V6.Seen)
	}
}

// The mode decides which families are tracked at all. In ipv4 mode a v6 address
// arriving in the result must not create v6 state nobody asked for.
func TestSessionModeSelectsTrackedFamilies(t *testing.T) {
	base := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	s := &sessionStats{}
	evaluateSession(s, both("1.1.1.1", "2001:db8::1"), IPModeV4, base)
	if s.V6.Observed || s.V6.Current != "" || len(s.V6.Seen) != 0 {
		t.Errorf("ipv4 mode tracked a v6 address: %+v", s.V6)
	}

	s = &sessionStats{}
	out := evaluateSession(s, both("1.1.1.1", "2001:db8::1"), IPModeV6, base)
	if s.V4.Observed {
		t.Errorf("ipv6 mode tracked a v4 address: %+v", s.V4)
	}
	if len(out.IPs) != 1 || out.IPs[0].Family != familyV6 {
		t.Errorf("ipv6 mode reported %+v, want one IPv6 baseline", out.IPs)
	}
}

// checkOutcome's rule: a check fails only when no requested family answered.
func TestCheckOutcome(t *testing.T) {
	v4Err := errors.New("v4 down")
	v6Err := errors.New("v6 down")

	tests := []struct {
		name       string
		v4, v6     string
		errV4      error
		errV6      error
		wantErr    bool
		wantPartly string
	}{
		{name: "both answered", v4: "1.1.1.1", v6: "2001:db8::1"},
		{
			// A v4-only proxy asked for both. Not a broken proxy.
			name: "v4 only is a success that still reports the v6 failure",
			v4:   "1.1.1.1", errV6: v6Err,
			wantPartly: "IPv6: v6 down",
		},
		{
			name: "v6 only is a success that still reports the v4 failure",
			v6:   "2001:db8::1", errV4: v4Err,
			wantPartly: "IPv4: v4 down",
		},
		{
			name:  "neither answered is the only failure",
			errV4: v4Err, errV6: v6Err,
			wantErr: true,
		},
		{
			name:    "single-family failure is a failure",
			errV4:   v4Err,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkOutcome(3, "u:p@host.invalid:1", tt.v4, tt.v6, tt.errV4, tt.errV6)
			if (r.Err != nil) != tt.wantErr {
				t.Fatalf("Err = %v, want error: %v", r.Err, tt.wantErr)
			}
			if r.IPv4 != tt.v4 || r.IPv6 != tt.v6 {
				t.Errorf("addresses = %q/%q, want %q/%q", r.IPv4, r.IPv6, tt.v4, tt.v6)
			}
			if got := r.partialErr(); got != tt.wantPartly {
				t.Errorf("partialErr = %q, want %q", got, tt.wantPartly)
			}
			if tt.wantErr {
				// Both families' reasons reach the one line a table row and a log
				// line have room for.
				if strings.Contains(r.Err.Error(), "\n") {
					t.Errorf("Err spans lines: %q", r.Err)
				}
				if tt.errV4 != nil && !strings.Contains(r.Err.Error(), tt.errV4.Error()) {
					t.Errorf("Err = %q, want it to mention the v4 failure", r.Err)
				}
				if tt.errV6 != nil && !strings.Contains(r.Err.Error(), tt.errV6.Error()) {
					t.Errorf("Err = %q, want it to mention the v6 failure", r.Err)
				}
			}
		})
	}
}
