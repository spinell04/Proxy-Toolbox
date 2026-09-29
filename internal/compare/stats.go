package compare

import (
	"math"

	"proxytoolbox/internal/util"
)

// Summary is the headline distribution of one run.
type Summary struct {
	Total       int     `json:"total"`
	OK          int     `json:"ok"`
	Blocked     int     `json:"blocked"`
	Errors      int     `json:"errors"`
	SuccessRate float64 `json:"successRate"`
	// LatencyCount is how many samples back the percentile fields below. It
	// is not the same as OK: a successful result that reported no latency is
	// counted in OK but contributes no sample, so a zero here means the
	// distribution is empty rather than genuinely all zero.
	LatencyCount int     `json:"latencyCount"`
	Min          int     `json:"min"`
	P25          int     `json:"p25"`
	P50          int     `json:"p50"`
	P75          int     `json:"p75"`
	P90          int     `json:"p90"`
	P95          int     `json:"p95"`
	P99          int     `json:"p99"`
	Max          int     `json:"max"`
	Mean         float64 `json:"mean"`
	StdDev       float64 `json:"stdDev"`
	IQR          int     `json:"iqr"`
}

// Percentile returns the nearest-rank percentile of values. p is in [0,1].
// The input is not modified.
//
// It delegates to util.Percentile so the dashboard and the CLI tools, which
// work in time.Duration rather than int milliseconds, share one definition of
// the rule and cannot drift apart.
func Percentile(values []int, p float64) int {
	return util.Percentile(values, p)
}

// okLatencies collects latencies from successful results only. Errors carry no
// latency and blocked requests measure rejection speed, not proxy speed, so
// including either would distort the distribution.
func okLatencies(results []ProxyResult) []int {
	var out []int
	for _, r := range results {
		if r.Outcome == OutcomeOK && r.LatencyMs > 0 {
			out = append(out, r.LatencyMs)
		}
	}
	return out
}

// Summarize computes the headline statistics for a set of results.
func Summarize(results []ProxyResult) Summary {
	s := Summary{Total: len(results)}
	for _, r := range results {
		switch r.Outcome {
		case OutcomeOK:
			s.OK++
		case OutcomeBlocked:
			s.Blocked++
		case OutcomeError:
			s.Errors++
		}
	}
	if s.Total > 0 {
		s.SuccessRate = float64(s.OK) / float64(s.Total)
	}

	lat := okLatencies(results)
	if len(lat) == 0 {
		return s
	}

	s.LatencyCount = len(lat)
	s.Min = Percentile(lat, 0)
	s.P25 = Percentile(lat, 0.25)
	s.P50 = Percentile(lat, 0.50)
	s.P75 = Percentile(lat, 0.75)
	s.P90 = Percentile(lat, 0.90)
	s.P95 = Percentile(lat, 0.95)
	s.P99 = Percentile(lat, 0.99)
	s.Max = Percentile(lat, 1)
	s.IQR = s.P75 - s.P25

	sum := 0
	for _, v := range lat {
		sum += v
	}
	s.Mean = float64(sum) / float64(len(lat))

	variance := 0.0
	for _, v := range lat {
		d := float64(v) - s.Mean
		variance += d * d
	}
	s.StdDev = math.Sqrt(variance / float64(len(lat)))

	return s
}
