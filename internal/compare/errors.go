package compare

import "strings"

// errorPatterns maps a taxonomy bucket to the substrings that identify it.
// Order matters: the first matching bucket wins, so specific patterns come
// before general ones. A real error often names several causes at once
// ("i/o timeout during tls handshake"), and moving a general bucket above a
// specific one silently swallows it.
var errorPatterns = []struct {
	kind    string
	substrs []string
}{
	{"auth", []string{"407", "proxy authentication", "authentication required"}},
	{"timeout", []string{"timeout", "timed out", "deadline exceeded"}},
	{"conn_refused", []string{"connection refused"}},
	{"conn_reset", []string{"connection reset", "broken pipe"}},
	{"tls", []string{"tls:", "x509", "certificate", "handshake"}},
	{"dns", []string{"no such host", "dns", "name resolution"}},
	// The bare three-character "eof" is deliberate: it catches "unexpected
	// EOF", "EOF" alone and "early EOF" alike. It is also the greediest entry
	// in this table, matching inside unrelated words, so it stays last.
	{"eof", []string{"eof"}},
}

// classifyError buckets a raw error string into a taxonomy kind. An empty
// input returns an empty kind, meaning "this result is not an error".
func classifyError(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	for _, p := range errorPatterns {
		for _, s := range p.substrs {
			if strings.Contains(lower, s) {
				return p.kind
			}
		}
	}
	return "other"
}

// ErrorBreakdown counts results by error kind. Non-errors are excluded.
func ErrorBreakdown(results []ProxyResult) map[string]int {
	counts := make(map[string]int)
	for _, r := range results {
		if r.ErrorKind == "" {
			continue
		}
		counts[r.ErrorKind]++
	}
	return counts
}
