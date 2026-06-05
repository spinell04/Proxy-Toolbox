package tools

import "proxytoolbox/internal/proxy"

// passLatency reports whether a latency (ms) should be saved given a threshold.
// maxMs <= 0 disables filtering (everything passes).
func passLatency(latencyMs int64, maxMs int) bool {
	return maxMs <= 0 || latencyMs < int64(maxMs)
}

// dedupByIP returns the verbatim source line of the first proxy seen for each
// distinct exit IP. Errored results are skipped; input order is preserved.
func dedupByIP(results []ipResult, proxies []proxy.Proxy) []string {
	seen := make(map[string]bool)
	var lines []string
	for _, r := range results {
		if r.Err != nil || r.IP == "" || seen[r.IP] {
			continue
		}
		seen[r.IP] = true
		lines = append(lines, proxies[r.Index].Raw)
	}
	return lines
}
