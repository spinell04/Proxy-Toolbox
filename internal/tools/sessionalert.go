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
	Rotations   int

	FailStreak   int
	FailAlerted  bool // an alert went out for the current streak, so recovery is worth reporting
	FailingSince time.Time
	LastErr      error

	Observed  bool // a successful check has been seen, so CurrentIP is meaningful
	CurrentIP string
	IPSince   time.Time
	IPsSeen   map[string]struct{}
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

// sessionOutcome carries both alerts a single check can earn. Health and
// identity are independent: a proxy can come back from a failure streak and
// come back on a different exit IP in the same check, and the owner wants
// both reported.
type sessionOutcome struct {
	Health healthAction
	IP     ipAction
}

// evaluateSession mutates s with the outcome of one check and returns the
// alerts that outcome earns. Callers snapshot CurrentIP, IPSince, FailingSince
// and LastErr beforehand to describe what the check just replaced.
//
// A failed check is not evidence of a rotation, so it leaves CurrentIP alone.
func evaluateSession(s *sessionStats, ip string, err error, now time.Time) sessionOutcome {
	s.TotalChecks++

	if err != nil {
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

	if s.IPsSeen == nil {
		s.IPsSeen = make(map[string]struct{})
	}
	s.IPsSeen[ip] = struct{}{}

	switch {
	// The first sighting establishes what "unchanged" means; it is not a change.
	case !s.Observed:
		s.Observed = true
		s.CurrentIP = ip
		s.IPSince = now
		out.IP = ipBaseline
	case s.CurrentIP != ip:
		s.CurrentIP = ip
		s.IPSince = now
		s.Rotations++
		out.IP = ipRotated
	}
	return out
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

func buildRotationEmbed(p proxy.Proxy, index int, oldIP, newIP string, held time.Duration, cycle int) discordEmbed {
	return discordEmbed{
		Title:     "Proxy IP ROTATED",
		Color:     colorRotate,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields: []discordEmbedField{
			{Name: "Proxy", Value: fmt.Sprintf("`%s`", p.ID())},
			{Name: "#", Value: strconv.Itoa(index), Inline: true},
			{Name: "Previous IP", Value: fmt.Sprintf("`%s`", oldIP), Inline: true},
			{Name: "New IP", Value: fmt.Sprintf("`%s`", newIP), Inline: true},
			{Name: "Held For", Value: held.Round(time.Second).String(), Inline: true},
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
