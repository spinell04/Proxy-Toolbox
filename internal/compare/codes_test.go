package compare

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestCode(t *testing.T) {
	tests := []struct {
		name string
		in   ProxyResult
		want string
	}{
		{"ok is not a failure", ProxyResult{Outcome: OutcomeOK, Status: 200}, ""},
		{"ok wins even with a status worth coding", ProxyResult{Outcome: OutcomeOK, Status: 403}, ""},
		{"blocked status", ProxyResult{Outcome: OutcomeBlocked, Status: 403}, "403"},
		{"rate limited status", ProxyResult{Outcome: OutcomeBlocked, Status: 429}, "429"},
		{"server error status", ProxyResult{Outcome: OutcomeError, Status: 502}, "502"},
		{"kind when no status", ProxyResult{Outcome: OutcomeError, ErrorKind: "timeout"}, "timeout"},
		{"eof kind", ProxyResult{Outcome: OutcomeError, ErrorKind: "eof"}, "eof"},
		{"neither status nor kind", ProxyResult{Outcome: OutcomeError}, "unknown"},
		// A 3xx is below the threshold and carries no failure meaning, so it
		// falls through to the kind exactly as a status-less row would.
		{"sub-400 status falls through to kind", ProxyResult{Outcome: OutcomeError, Status: 302, ErrorKind: "eof"}, "eof"},
		{"sub-400 status with no kind", ProxyResult{Outcome: OutcomeError, Status: 302}, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Code(tt.in); got != tt.want {
				t.Errorf("Code(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCode_StatusBeatsKind(t *testing.T) {
	// A 403 that also carries an error string is a block. The block is the more
	// specific fact, so the status must win; coding it as "timeout" would file
	// a refusal under a transport failure.
	r := ProxyResult{
		Outcome:   OutcomeBlocked,
		Status:    403,
		ErrorRaw:  "context deadline exceeded",
		ErrorKind: "timeout",
	}

	if got := Code(r); got != "403" {
		t.Errorf("Code = %q, want %q", got, "403")
	}
}

// realMobileErrors are the verbatim errored rows of results/mobile.csv. Every
// one has the Status column "ERROR", so Status is 0 and the code can only come
// from the taxonomy.
var realMobileErrors = []string{
	`Get "https://www.ticketmaster.de": unexpected EOF`,
	`Get "https://www.ticketmaster.de": unexpected EOF`,
	`Get "https://www.ticketmaster.de": unexpected EOF`,
	`Get "https://www.ticketmaster.de": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`,
	`Get "https://www.ticketmaster.de": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`,
	`Get "https://www.ticketmaster.de": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`,
	`Get "https://www.ticketmaster.de": read tcp 192.168.0.38:62061->91.99.38.111:8888: i/o timeout (Client.Timeout exceeded while awaiting headers)`,
}

// mobileResults rebuilds those rows the way parseProxyRows would: Status 0,
// Outcome error, ErrorKind from the taxonomy.
func mobileResults() []ProxyResult {
	results := make([]ProxyResult, 0, len(realMobileErrors))
	for i, raw := range realMobileErrors {
		results = append(results, ProxyResult{
			ProxyID:   fmt.Sprintf("host%d:8888", i),
			Outcome:   OutcomeError,
			ErrorRaw:  raw,
			ErrorKind: classifyError(raw),
		})
	}
	return results
}

func TestCodeBreakdown_RealMobileRun(t *testing.T) {
	got := CodeBreakdown(mobileResults())

	want := []CodeGroup{
		{Code: "timeout", Count: 4, Proxies: []string{"host3:8888", "host4:8888", "host5:8888", "host6:8888"}},
		{Code: "eof", Count: 3, Proxies: []string{"host0:8888", "host1:8888", "host2:8888"}},
	}
	assertGroups(t, got, want)
}

func TestCodeBreakdown_ExcludesOK(t *testing.T) {
	results := []ProxyResult{
		{ProxyID: "a", Outcome: OutcomeOK, Status: 200},
		{ProxyID: "b", Outcome: OutcomeError, ErrorKind: "timeout"},
		{ProxyID: "c", Outcome: OutcomeOK, Status: 200},
	}

	got := CodeBreakdown(results)

	want := []CodeGroup{{Code: "timeout", Count: 1, Proxies: []string{"b"}}}
	assertGroups(t, got, want)
}

func TestCodeBreakdown_AllOKIsEmptyNotNil(t *testing.T) {
	got := CodeBreakdown([]ProxyResult{{ProxyID: "a", Outcome: OutcomeOK}})

	if got == nil {
		t.Fatal("CodeBreakdown returned nil; the JSON must be [] for the JS to map over it")
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want no groups", got)
	}
	if b, err := json.Marshal(got); err != nil || string(b) != "[]" {
		t.Errorf("json = %s (err %v), want []", b, err)
	}
}

func TestCodeBreakdown_EmptyInputIsEmptyNotNil(t *testing.T) {
	got := CodeBreakdown(nil)

	if got == nil {
		t.Fatal("CodeBreakdown(nil) returned nil, want an empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want no groups", got)
	}
}

func TestCodeBreakdown_OrderedByCountThenCode(t *testing.T) {
	// "403" and "429" tie at two each, so only the alphabetical tie-break
	// decides their order; without it the map iteration would flip them.
	results := []ProxyResult{
		{ProxyID: "p1", Outcome: OutcomeBlocked, Status: 429},
		{ProxyID: "p2", Outcome: OutcomeError, ErrorKind: "timeout"},
		{ProxyID: "p3", Outcome: OutcomeBlocked, Status: 403},
		{ProxyID: "p4", Outcome: OutcomeError, ErrorKind: "timeout"},
		{ProxyID: "p5", Outcome: OutcomeBlocked, Status: 429},
		{ProxyID: "p6", Outcome: OutcomeBlocked, Status: 403},
		{ProxyID: "p7", Outcome: OutcomeError, ErrorKind: "timeout"},
	}

	got := CodeBreakdown(results)

	want := []CodeGroup{
		{Code: "timeout", Count: 3, Proxies: []string{"p2", "p4", "p7"}},
		{Code: "403", Count: 2, Proxies: []string{"p3", "p6"}},
		{Code: "429", Count: 2, Proxies: []string{"p1", "p5"}},
	}
	assertGroups(t, got, want)
}

func TestCodeBreakdown_MessagesOnlyForOtherAndUnknown(t *testing.T) {
	// "other" means "matched no known pattern", so its raw text is the only
	// actionable signal. Every classified code suppresses it as noise.
	results := []ProxyResult{
		{ProxyID: "p1", Outcome: OutcomeError, ErrorRaw: "novel failure A", ErrorKind: "other"},
		{ProxyID: "p2", Outcome: OutcomeError, ErrorRaw: "novel failure B", ErrorKind: "other"},
		{ProxyID: "p3", Outcome: OutcomeError, ErrorRaw: "dial tcp: i/o timeout", ErrorKind: "timeout"},
		{ProxyID: "p4", Outcome: OutcomeError, ErrorRaw: "dial tcp: i/o timeout", ErrorKind: "timeout"},
		{ProxyID: "p5", Outcome: OutcomeError, ErrorRaw: "no pattern, no kind"},
		{ProxyID: "p6", Outcome: OutcomeBlocked, Status: 403, ErrorRaw: "blocked by target"},
	}

	got := CodeBreakdown(results)

	want := []CodeGroup{
		{Code: "other", Count: 2, Proxies: []string{"p1", "p2"},
			Messages: []string{"novel failure A", "novel failure B"}},
		{Code: "timeout", Count: 2, Proxies: []string{"p3", "p4"}},
		{Code: "403", Count: 1, Proxies: []string{"p6"}},
		{Code: "unknown", Count: 1, Proxies: []string{"p5"},
			Messages: []string{"no pattern, no kind"}},
	}
	assertGroups(t, got, want)
}

func TestCodeBreakdown_SuppressedMessagesMarshalAsEmptyArray(t *testing.T) {
	got := CodeBreakdown([]ProxyResult{
		{ProxyID: "p1", Outcome: OutcomeError, ErrorRaw: "dial tcp: i/o timeout", ErrorKind: "timeout"},
	})

	if len(got) != 1 {
		t.Fatalf("got %d groups, want 1", len(got))
	}
	b, err := json.Marshal(got[0])
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"code":"timeout","count":1,"proxies":["p1"],"messages":[]}`
	if string(b) != want {
		t.Errorf("json = %s, want %s", b, want)
	}
}

func TestCodeBreakdown_MessagesDeduplicatedInFirstSeenOrder(t *testing.T) {
	results := []ProxyResult{
		{ProxyID: "p1", Outcome: OutcomeError, ErrorRaw: "zebra failure", ErrorKind: "other"},
		{ProxyID: "p2", Outcome: OutcomeError, ErrorRaw: "apple failure", ErrorKind: "other"},
		{ProxyID: "p3", Outcome: OutcomeError, ErrorRaw: "zebra failure", ErrorKind: "other"},
		{ProxyID: "p4", Outcome: OutcomeError, ErrorRaw: "mango failure", ErrorKind: "other"},
		{ProxyID: "p5", Outcome: OutcomeError, ErrorRaw: "apple failure", ErrorKind: "other"},
	}

	got := CodeBreakdown(results)

	want := []CodeGroup{{
		Code:     "other",
		Count:    5,
		Proxies:  []string{"p1", "p2", "p3", "p4", "p5"},
		Messages: []string{"zebra failure", "apple failure", "mango failure"},
	}}
	assertGroups(t, got, want)
}

func TestCodeBreakdown_MessagesCappedAtTwenty(t *testing.T) {
	// A pathological run must not ship an unbounded payload. The count keeps
	// the true total; only the message list is truncated.
	const distinct = 25
	results := make([]ProxyResult, 0, distinct)
	for i := 0; i < distinct; i++ {
		results = append(results, ProxyResult{
			ProxyID:   fmt.Sprintf("p%02d", i),
			Outcome:   OutcomeError,
			ErrorRaw:  fmt.Sprintf("novel failure %02d", i),
			ErrorKind: "other",
		})
	}

	got := CodeBreakdown(results)

	if len(got) != 1 {
		t.Fatalf("got %d groups, want 1", len(got))
	}
	g := got[0]
	if g.Count != distinct {
		t.Errorf("Count = %d, want %d: the cap truncates messages, not the tally", g.Count, distinct)
	}
	if len(g.Proxies) != distinct {
		t.Errorf("len(Proxies) = %d, want %d: the cap must not touch the proxy list", len(g.Proxies), distinct)
	}
	if len(g.Messages) != maxCodeMessages {
		t.Fatalf("len(Messages) = %d, want %d", len(g.Messages), maxCodeMessages)
	}
	for i, msg := range g.Messages {
		want := fmt.Sprintf("novel failure %02d", i)
		if msg != want {
			t.Errorf("Messages[%d] = %q, want %q: the cap keeps the first seen", i, msg, want)
		}
	}
}

// assertGroups compares a breakdown against the expected groups in order. A
// nil want.Messages means "suppressed", which must still be an empty non-nil
// slice on the value under test.
func assertGroups(t *testing.T, got, want []CodeGroup) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d groups %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Code != w.Code {
			t.Errorf("group %d: Code = %q, want %q", i, g.Code, w.Code)
		}
		if g.Count != w.Count {
			t.Errorf("group %d (%s): Count = %d, want %d", i, w.Code, g.Count, w.Count)
		}
		assertStrings(t, fmt.Sprintf("group %d (%s) Proxies", i, w.Code), g.Proxies, w.Proxies)
		assertStrings(t, fmt.Sprintf("group %d (%s) Messages", i, w.Code), g.Messages, w.Messages)
	}
}

func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()

	if got == nil {
		t.Errorf("%s is nil, want a non-nil slice so the JSON is [] and not null", label)
		return
	}
	if len(got) != len(want) {
		t.Errorf("%s = %q, want %q", label, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s = %q, want %q", label, got, want)
			return
		}
	}
}
