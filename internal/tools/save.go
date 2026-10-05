package tools

import (
	"strings"

	"proxytoolbox/internal/proxy"
)

// passLatency reports whether a latency (ms) should be saved given a threshold.
// maxMs <= 0 disables filtering (everything passes).
func passLatency(latencyMs int64, maxMs int) bool {
	return maxMs <= 0 || latencyMs < int64(maxMs)
}

// dedupByExitIdentity returns the verbatim source line of the first proxy seen
// for each distinct exit identity. Errored results are skipped; input order is
// preserved.
//
// The identity is the whole set of addresses the mode asked for, so in both
// mode two proxies are duplicates only when both their v4 and their v6 exits
// match. Conservative on purpose: this file is reused as input, and dropping a
// proxy because it shares one of two exits loses something the owner paid for.
func dedupByExitIdentity(results []ipResult, proxies []proxy.Proxy, mode IPMode) []string {
	fams := mode.families()
	seen := make(map[string]bool)
	var lines []string
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		parts := make([]string, 0, len(fams))
		answered := false
		for _, f := range fams {
			ip := f.Get(r)
			if ip != "" {
				answered = true
			}
			parts = append(parts, ip)
		}
		if !answered {
			continue
		}
		// A separator no address can contain, so "1.2.3.4" + "" cannot collide
		// with some other split of the same characters.
		key := strings.Join(parts, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		lines = append(lines, proxies[r.Index].Raw)
	}
	return lines
}
