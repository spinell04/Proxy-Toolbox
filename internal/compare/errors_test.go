package compare

import "testing"

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"dial timeout", "dial tcp 1.2.3.4:8080: i/o timeout", "timeout"},
		{"deadline", "context deadline exceeded", "timeout"},
		{"timed out", "Client.Timeout exceeded while awaiting headers", "timeout"},
		{"refused", "dial tcp 1.2.3.4:8080: connect: connection refused", "conn_refused"},
		{"reset", "read tcp: connection reset by peer", "conn_reset"},
		{"broken pipe", "write tcp: broken pipe", "conn_reset"},
		{"tls handshake", "remote error: tls: handshake failure", "tls"},
		{"bad certificate", "x509: certificate signed by unknown authority", "tls"},
		{"dns", "lookup bad.host: no such host", "dns"},
		{"proxy auth", "proxy rejected: HTTP/1.1 407 Proxy Authentication Required", "auth"},
		{"eof", "unexpected EOF", "eof"},
		{"unknown", "something nobody has seen before", "other"},
		{"empty is not an error", "", ""},
		{"whitespace is not an error", "   \t ", ""},
		{"uppercase still matches", "DIAL TCP: I/O TIMEOUT", "timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyError(tt.raw); got != tt.want {
				t.Errorf("classifyError(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestClassifyError_FirstMatchingBucketWins(t *testing.T) {
	// Real errors name several causes at once. The bucket order is the whole
	// design: a general pattern placed ahead of a specific one swallows it
	// silently, and every case below is a pair whose order would flip.
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"auth beats timeout", "proxy authentication required after i/o timeout", "auth"},
		{"auth beats eof", "HTTP/1.1 407 rejected, unexpected EOF", "auth"},
		{"timeout beats tls", "dial tcp: i/o timeout during tls handshake", "timeout"},
		{"timeout beats dns", "lookup bad.host: no such host: i/o timeout", "timeout"},
		{"conn_refused beats dns", "connect: connection refused, lookup bad.host: no such host", "conn_refused"},
		{"conn_reset beats tls", "connection reset by peer during tls handshake", "conn_reset"},
		{"tls beats eof", "remote error: tls: handshake failure, unexpected EOF", "tls"},
		{"dns beats eof", "lookup bad.host: no such host, unexpected EOF", "dns"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyError(tt.raw); got != tt.want {
				t.Errorf("classifyError(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestErrorBreakdown(t *testing.T) {
	results := []ProxyResult{
		{ErrorKind: "timeout"},
		{ErrorKind: "timeout"},
		{ErrorKind: "auth"},
		{ErrorKind: ""}, // successful result, contributes nothing
	}

	got := ErrorBreakdown(results)

	if len(got) != 2 {
		t.Errorf("got %d kinds, want 2: %v", len(got), got)
	}
	if got["timeout"] != 2 {
		t.Errorf("timeout = %d, want 2", got["timeout"])
	}
	if got["auth"] != 1 {
		t.Errorf("auth = %d, want 1", got["auth"])
	}
	if _, present := got[""]; present {
		t.Error("empty kind must not appear in the breakdown")
	}
}

func TestErrorBreakdown_NoErrors(t *testing.T) {
	got := ErrorBreakdown([]ProxyResult{{ErrorKind: ""}, {ErrorKind: ""}})

	if got == nil {
		t.Fatal("ErrorBreakdown returned nil, want an empty map callers can index")
	}
	if len(got) != 0 {
		t.Errorf("got %v, want no kinds", got)
	}
}

func TestErrorBreakdown_OverAParsedRun(t *testing.T) {
	// The parser fills ErrorKind as it reads, so the taxonomy has to be wired
	// into ParseFile and not merely exist. The fixture carries one timeout,
	// one 407 and one truncated response.
	run, err := ParseFile("testdata/pinger_full.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	wantKinds := []string{"", "", "timeout", "auth", "eof"}
	if len(run.Results) != len(wantKinds) {
		t.Fatalf("got %d results, want %d", len(run.Results), len(wantKinds))
	}
	for i, want := range wantKinds {
		if got := run.Results[i].ErrorKind; got != want {
			t.Errorf("result %d ErrorKind = %q, want %q (from %q)",
				i, got, want, run.Results[i].ErrorRaw)
		}
	}

	breakdown := ErrorBreakdown(run.Results)
	want := map[string]int{"timeout": 1, "auth": 1, "eof": 1}
	if len(breakdown) != len(want) {
		t.Fatalf("breakdown = %v, want %v", breakdown, want)
	}
	for kind, n := range want {
		if breakdown[kind] != n {
			t.Errorf("breakdown[%q] = %d, want %d", kind, breakdown[kind], n)
		}
	}
}
