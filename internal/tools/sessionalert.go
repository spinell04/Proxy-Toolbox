package tools

import (
	"fmt"
	"strconv"
	"time"

	"proxytoolbox/internal/proxy"
)

// Failure ladder. A single failed check says nothing about a proxy, so the
// first alert waits for a streak; the second marks the streak as entrenched.
// Past the second the proxy is known-dead and further alerts are noise.
const (
	sessionFailAlertAt  = 5
	sessionFailRepeatAt = 30
)

const colorRotate = 0xF1C40F

type sessionStats struct {
	ProxyID     string
	TotalChecks int
	Failures    int
	// Rotations counts per family, so a check that rotates both exits counts
	// two. They are two independent facts about the proxy and folding them into
	// one would hide a v6 pool churning behind a stable v4 exit.
	Rotations int

	FailStreak   int
	FailAlerted  bool // an alert went out for the current streak, so recovery is worth reporting
	FailingSince time.Time
	LastErr      error

	// Tracked independently: a check can rotate one family, both, or neither.
	V4 ipState
	V6 ipState
}

// ipState is one address family's view of one proxy.
type ipState struct {
	Observed bool // an answer for this family has been seen, so Current is meaningful
	Current  string
	Since    time.Time
	Seen     map[string]struct{}
}

// state resolves a family label to the field that tracks it.
func (s *sessionStats) state(family string) *ipState {
	if family == familyV6 {
		return &s.V6
	}
	return &s.V4
}

type healthAction int

const (
	healthNone healthAction = iota
	healthFailAlert
	healthRecovered
)

type ipAction int

const (
	ipNone ipAction = iota
	ipBaseline
	ipRotated
)

// ipChange is what one family's answer earned on one check, with everything a
// rotation report needs. Held is how long the previous address lasted.
type ipChange struct {
	Family string // "IPv4" / "IPv6"
	Action ipAction
	PrevIP string
	NewIP  string
	Held   time.Duration
}

// sessionOutcome carries every alert a single check can earn. Health and
// identity are independent: a proxy can come back from a failure streak and
// come back on a different exit IP in the same check, and the owner wants both
// reported. IPs holds one entry per family that changed, so a check can report
// a v4 rotation, a v6 rotation, or both.
type sessionOutcome struct {
	Health healthAction
	IPs    []ipChange
}

// rotations returns only the entries worth alerting on. A baseline is the first
// answer a family gave and is not a change.
func (o sessionOutcome) rotations() []ipChange {
	var out []ipChange
	for _, c := range o.IPs {
		if c.Action == ipRotated {
			out = append(out, c)
		}
	}
	return out
}

// evaluateSession mutates s with the outcome of one check and returns the
// alerts that outcome earns. Callers snapshot FailingSince and LastErr
// beforehand to describe the failure streak that just ended; what each family's
// answer replaced comes back in the outcome.
//
// A failed check is not evidence of a rotation, so it leaves both families'
// addresses alone.
func evaluateSession(s *sessionStats, r ipResult, mode IPMode, now time.Time) sessionOutcome {
	s.TotalChecks++

	if err := r.Err; err != nil {
		s.Failures++
		s.FailStreak++
		if s.FailStreak == 1 {
			s.FailingSince = now
		}
		s.LastErr = err
		if s.FailStreak == sessionFailAlertAt || s.FailStreak == sessionFailRepeatAt {
			s.FailAlerted = true
			return sessionOutcome{Health: healthFailAlert}
		}
		return sessionOutcome{}
	}

	out := sessionOutcome{}
	// Only a streak loud enough to have alerted is worth an all-clear.
	if s.FailAlerted {
		s.FailAlerted = false
		out.Health = healthRecovered
	}
	s.FailStreak = 0

	for _, f := range mode.families() {
		c := s.state(f.Label).observe(f.Label, f.Get(r), now)
		if c.Action == ipNone {
			continue
		}
		if c.Action == ipRotated {
			s.Rotations++
		}
		out.IPs = append(out.IPs, c)
	}
	return out
}

// observe folds one family's answer for one check into that family's state.
//
// An empty ip is unknown for this check, not absent. A proxy with no route for
// this family and a transient failure of that family's endpoints are
// indistinguishable from here, so the last known address is kept and nothing is
// reported. Treating either as "the exit disappeared" is how the dual-stack
// endpoints came to alert continuously, and it is the rule this whole change
// rests on.
func (st *ipState) observe(family, ip string, now time.Time) ipChange {
	if ip == "" {
		return ipChange{Family: family}
	}
	if st.Seen == nil {
		st.Seen = make(map[string]struct{})
	}
	st.Seen[ip] = struct{}{}

	switch {
	// The first answer establishes what "unchanged" means for this family; it
	// is not a change. Unknown -> known is that same baseline, later.
	case !st.Observed:
		st.Observed = true
		st.Current = ip
		st.Since = now
		return ipChange{Family: family, Action: ipBaseline, NewIP: ip}
	case st.Current != ip:
		prev, held := st.Current, now.Sub(st.Since)
		st.Current = ip
		st.Since = now
		return ipChange{Family: family, Action: ipRotated, PrevIP: prev, NewIP: ip, Held: held}
	}
	return ipChange{Family: family, NewIP: ip}
}

// hourlyFrom is where the period stops reading in minutes. It sits at two
// hours rather than one because a period like 100m compares directly against
// an interval quoted in milliseconds, while 1.7h does not.
const hourlyFrom = 2 * time.Hour

// formatSamplingPeriod renders how often a single proxy comes back around.
// Units are truncated, not rounded up, so the figure never overstates coverage.
func formatSamplingPeriod(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < hourlyFrom:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	default:
		return fmt.Sprintf("%.1fh", d.Hours())
	}
}

// buildRotationEmbed names the family that rotated. In both mode the same proxy
// has two exits and a report that does not say which one moved is unreadable.
func buildRotationEmbed(p proxy.Proxy, index int, c ipChange, cycle int) discordEmbed {
	return discordEmbed{
		Title:     "Proxy " + c.Family + " ROTATED",
		Color:     colorRotate,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields: []discordEmbedField{
			{Name: "Proxy", Value: fmt.Sprintf("`%s`", p.ID())},
			{Name: "#", Value: strconv.Itoa(index), Inline: true},
			{Name: "Family", Value: c.Family, Inline: true},
			{Name: "Previous IP", Value: fmt.Sprintf("`%s`", c.PrevIP), Inline: true},
			{Name: "New IP", Value: fmt.Sprintf("`%s`", c.NewIP), Inline: true},
			{Name: "Held For", Value: c.Held.Round(time.Second).String(), Inline: true},
			{Name: "Cycle", Value: strconv.Itoa(cycle), Inline: true},
		},
	}
}

func buildSessionFailEmbed(p proxy.Proxy, index int, err error, consecutive, cycle int) discordEmbed {
	return discordEmbed{
		Title:     "Proxy CHECK FAILING",
		Color:     colorDown,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields: []discordEmbedField{
			{Name: "Proxy", Value: fmt.Sprintf("`%s`", p.ID())},
			{Name: "#", Value: strconv.Itoa(index), Inline: true},
			{Name: "Consecutive Failures", Value: strconv.Itoa(consecutive), Inline: true},
			{Name: "Cycle", Value: strconv.Itoa(cycle), Inline: true},
			{Name: "Error", Value: truncate(err.Error(), maxErrLen)},
		},
	}
}

func buildSessionRecoveredEmbed(p proxy.Proxy, index int, downtime time.Duration, lastErr error, cycle int) discordEmbed {
	errText := "-"
	if lastErr != nil {
		errText = truncate(lastErr.Error(), maxErrLen)
	}
	return discordEmbed{
		Title:     "Proxy CHECK RECOVERED",
		Color:     colorUp,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields: []discordEmbedField{
			{Name: "Proxy", Value: fmt.Sprintf("`%s`", p.ID())},
			{Name: "#", Value: strconv.Itoa(index), Inline: true},
			{Name: "Failing For", Value: downtime.Round(time.Second).String(), Inline: true},
			{Name: "Cycle", Value: strconv.Itoa(cycle), Inline: true},
			{Name: "Last Error", Value: errText},
		},
	}
}
