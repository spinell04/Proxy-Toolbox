package compare

import (
	"sort"
	"strconv"
)

// maxCodeMessages caps the raw messages carried by one group. Only the "other"
// and "unknown" buckets keep messages at all, and those are exactly the buckets
// whose text is unbounded: a run where every row fails in a novel way would
// otherwise ship one verbatim string per proxy to the browser.
const maxCodeMessages = 20

// Code returns the short token identifying a non-OK result: the HTTP status
// where there is one, the taxonomy kind otherwise. An OK result returns "",
// which callers treat as "not a failure, skip it".
//
// Status takes precedence over kind. A 403 that also carries an error string is
// a block, and the block is the more specific fact; filing it under "timeout"
// would hide a refusal among transport failures.
func Code(r ProxyResult) string {
	if r.Outcome == OutcomeOK {
		return ""
	}
	if r.Status >= 400 {
		return strconv.Itoa(r.Status)
	}
	if r.ErrorKind != "" {
		return r.ErrorKind
	}
	return "unknown"
}

// CodeGroup is one failure code within one run.
type CodeGroup struct {
	Code     string   `json:"code"`
	Count    int      `json:"count"`
	Proxies  []string `json:"proxies"`  // ProxyIDs carrying this code, input order
	Messages []string `json:"messages"` // raw errors, only for "other"/"unknown"
}

// CodeBreakdown groups non-OK results by Code, ordered by descending count and
// then by code for a stable tie-break.
//
// Messages are suppressed for every classified code: the raw text is the same
// boilerplate prefix on every row and carries nothing the code does not already
// say. "other" and "unknown" are the exceptions — they mean "matched no known
// pattern", so without the text they are unactionable by construction, and they
// are also the only signal that the taxonomy needs a new entry.
//
// The returned slice and every slice inside it are non-nil even when empty: the
// JSON is consumed by JavaScript that maps over them, and null would break it.
func CodeBreakdown(results []ProxyResult) []CodeGroup {
	order := make([]string, 0, len(results))
	byCode := make(map[string]*CodeGroup, len(results))
	seen := make(map[string]map[string]bool)

	for _, r := range results {
		code := Code(r)
		if code == "" {
			continue
		}

		g, ok := byCode[code]
		if !ok {
			g = &CodeGroup{Code: code, Proxies: []string{}, Messages: []string{}}
			byCode[code] = g
			seen[code] = make(map[string]bool)
			order = append(order, code)
		}

		g.Count++
		g.Proxies = append(g.Proxies, r.ProxyID)

		if !keepsMessages(code) || r.ErrorRaw == "" || seen[code][r.ErrorRaw] {
			continue
		}
		seen[code][r.ErrorRaw] = true
		if len(g.Messages) < maxCodeMessages {
			g.Messages = append(g.Messages, r.ErrorRaw)
		}
	}

	groups := make([]CodeGroup, 0, len(order))
	for _, code := range order {
		groups = append(groups, *byCode[code])
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		return groups[i].Code < groups[j].Code
	})
	return groups
}

func keepsMessages(code string) bool {
	return code == "other" || code == "unknown"
}
