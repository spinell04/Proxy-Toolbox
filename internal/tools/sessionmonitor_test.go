package tools

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"proxytoolbox/internal/proxy"
)

var errCheck = errors.New("i/o timeout")

// step is one check fed to evaluateSession: either a failure or a success
// carrying an exit IP.
type step struct {
	ip     string
	err    error
	health healthAction
	ipAct  ipAction
}

func fail() step                 { return step{err: errCheck, health: healthNone} }
func failAlert() step            { return step{err: errCheck, health: healthFailAlert} }
func ok(ip string) step          { return step{ip: ip} }
func okBaseline(ip string) step  { return step{ip: ip, ipAct: ipBaseline} }
func okRotated(ip string) step   { return step{ip: ip, ipAct: ipRotated} }
func okRecovered(ip string) step { return step{ip: ip, health: healthRecovered} }

func runSteps(t *testing.T, s *sessionStats, steps []step) {
	t.Helper()
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	for i, st := range steps {
		now = now.Add(time.Second)
		got := evaluateSession(s, st.ip, st.err, now)
		if got.Health != st.health {
			t.Fatalf("step %d: Health = %v, want %v", i+1, got.Health, st.health)
		}
		if got.IP != st.ipAct {
			t.Fatalf("step %d: IP = %v, want %v", i+1, got.IP, st.ipAct)
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
				[]step{{ip: "1.1.1.1", health: healthRecovered, ipAct: ipBaseline}},
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
			runSteps(t, &sessionStats{}, tc.steps)
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
				[]step{{ip: "1.1.1.1", health: healthRecovered, ipAct: ipBaseline}},
				[]step{ok("1.1.1.1"), ok("1.1.1.1")},
			),
		},
		{
			name: "two ladder alerts still recover only once",
			steps: concat(
				repeat(4, fail()), []step{failAlert()},
				repeat(24, fail()), []step{failAlert()},
				repeat(10, fail()),
				[]step{{ip: "1.1.1.1", health: healthRecovered, ipAct: ipBaseline}},
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
			runSteps(t, &sessionStats{}, tc.steps)
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
			steps:    []step{okBaseline("1.1.1.1"), okRotated("2.2.2.2")},
			wantIP:   "2.2.2.2",
			wantRots: 1,
			wantSeen: 2,
		},
		{
			name: "every rotation counts, including a return to an earlier IP",
			steps: []step{
				okBaseline("1.1.1.1"),
				okRotated("2.2.2.2"),
				okRotated("1.1.1.1"),
				okRotated("2.2.2.2"),
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
			runSteps(t, s, tc.steps)
			if s.CurrentIP != tc.wantIP {
				t.Errorf("CurrentIP = %q, want %q", s.CurrentIP, tc.wantIP)
			}
			if s.Rotations != tc.wantRots {
				t.Errorf("Rotations = %d, want %d", s.Rotations, tc.wantRots)
			}
			if len(s.IPsSeen) != tc.wantSeen {
				t.Errorf("IPsSeen = %d, want %d", len(s.IPsSeen), tc.wantSeen)
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

	evaluateSession(s, "1.1.1.1", nil, base)
	since := s.IPSince

	for i := 1; i <= 10; i++ {
		evaluateSession(s, "", errCheck, base.Add(time.Duration(i)*time.Minute))
		if s.CurrentIP != "1.1.1.1" {
			t.Fatalf("failure %d overwrote CurrentIP: %q", i, s.CurrentIP)
		}
		if !s.IPSince.Equal(since) {
			t.Fatalf("failure %d moved IPSince", i)
		}
	}
	if len(s.IPsSeen) != 1 {
		t.Errorf("failures recorded an exit IP: %v", s.IPsSeen)
	}
}

func TestSessionFailingSinceAndLastErr(t *testing.T) {
	s := &sessionStats{}
	base := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	first := errors.New("first")
	evaluateSession(s, "", first, base)
	if !s.FailingSince.Equal(base) {
		t.Fatalf("FailingSince = %v, want %v", s.FailingSince, base)
	}

	// A continuing streak keeps its start time but tracks the latest error.
	last := errors.New("last")
	evaluateSession(s, "", last, base.Add(time.Minute))
	if !s.FailingSince.Equal(base) {
		t.Fatalf("FailingSince moved mid-streak: %v", s.FailingSince)
	}
	if s.LastErr != last {
		t.Fatalf("LastErr = %v, want %v", s.LastErr, last)
	}

	// A new streak after a success restarts the clock.
	evaluateSession(s, "1.1.1.1", nil, base.Add(2*time.Minute))
	restart := base.Add(3 * time.Minute)
	evaluateSession(s, "", errCheck, restart)
	if !s.FailingSince.Equal(restart) {
		t.Fatalf("FailingSince = %v, want %v", s.FailingSince, restart)
	}
}

func TestSessionRotationTimestamps(t *testing.T) {
	s := &sessionStats{}
	base := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	evaluateSession(s, "1.1.1.1", nil, base)
	if !s.IPSince.Equal(base) {
		t.Fatalf("baseline IPSince = %v, want %v", s.IPSince, base)
	}

	// An unchanged IP must not reset the hold clock, or every rotation embed
	// would report a hold of one interval.
	evaluateSession(s, "1.1.1.1", nil, base.Add(time.Minute))
	if !s.IPSince.Equal(base) {
		t.Fatalf("unchanged IP moved IPSince to %v", s.IPSince)
	}

	rot := base.Add(2 * time.Minute)
	evaluateSession(s, "2.2.2.2", nil, rot)
	if !s.IPSince.Equal(rot) {
		t.Fatalf("rotation IPSince = %v, want %v", s.IPSince, rot)
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
	e := buildRotationEmbed(p, 12, "9.9.9.9", "8.8.8.8", 90*time.Second, 4)

	if e.Color != colorRotate {
		t.Errorf("Color = %#x, want %#x", e.Color, colorRotate)
	}
	if e.Title != "Proxy IP ROTATED" {
		t.Errorf("Title = %q", e.Title)
	}
	for name, want := range map[string]string{
		"#":           "12",
		"Previous IP": "`9.9.9.9`",
		"New IP":      "`8.8.8.8`",
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
	const a = "Quantum-dmgck8gz:TuyUdS9o4H5oWwfh0wTq_country-DE_session-935814_lifetime-60@schro.quantumproxies.net:1111"
	const b = "Quantum-dmgck8gz:TuyUdS9o4H5oWwfh0wTq_country-DE_session-374010_lifetime-60@schro.quantumproxies.net:1111"

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
