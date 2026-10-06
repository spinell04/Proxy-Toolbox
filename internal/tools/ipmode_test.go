package tools

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"proxytoolbox/internal/config"
	"proxytoolbox/internal/proxy"
)

func TestParseIPMode(t *testing.T) {
	tests := []struct {
		in   string
		want IPMode
	}{
		{"ipv4", IPModeV4},
		{"ipv6", IPModeV6},
		{"both", IPModeBoth},
		{"IPv6", IPModeV6},       // the prompt text spells it this way
		{"  both  ", IPModeBoth}, // a line read from config.txt keeps its spacing
		{"", IPModeV4},           // blank is the default, not an error
		{"v4", IPModeV4},         // a near miss is still a miss
		{"ipv5", IPModeV4},
		{"dual", IPModeV4},
		{"ipv4 ipv6", IPModeV4},
	}
	for _, tt := range tests {
		if got := ParseIPMode(tt.in); got != tt.want {
			t.Errorf("ParseIPMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// A runtime prompt falls back to what config supplied, not to the built-in
// default: typing nonsense at the prompt must not silently switch a configured
// ipv6 run to ipv4.
func TestParseIPModeOr_FallsBackToTheGivenDefault(t *testing.T) {
	for _, in := range []string{"", "   ", "garbage", "ipv5"} {
		if got := ParseIPModeOr(in, IPModeBoth); got != IPModeBoth {
			t.Errorf("ParseIPModeOr(%q, both) = %q, want both", in, got)
		}
	}
	if got := ParseIPModeOr("ipv6", IPModeBoth); got != IPModeV6 {
		t.Errorf("ParseIPModeOr(ipv6, both) = %q, want ipv6", got)
	}
}

// The config template's default and the parser's fallback are two statements of
// the same default and can drift apart silently.
func TestDefaultIPModeMatchesConfig(t *testing.T) {
	if config.DefaultIPMode != string(IPModeV4) {
		t.Errorf("config.DefaultIPMode = %q, want %q", config.DefaultIPMode, IPModeV4)
	}
	if ParseIPMode(config.DefaultIPMode) != IPModeV4 {
		t.Errorf("config.DefaultIPMode %q does not parse to the default mode", config.DefaultIPMode)
	}
}

func TestIPModeFamilies(t *testing.T) {
	tests := []struct {
		mode    IPMode
		labels  []string
		lookups int
	}{
		{IPModeV4, []string{familyV4}, 1},
		{IPModeV6, []string{familyV6}, 1},
		// v4 first, and two lookups: the session monitor's load figure is built
		// on this number.
		{IPModeBoth, []string{familyV4, familyV6}, 2},
	}
	for _, tt := range tests {
		var got []string
		for _, f := range tt.mode.families() {
			got = append(got, f.Label)
		}
		if strings.Join(got, ",") != strings.Join(tt.labels, ",") {
			t.Errorf("%s families = %v, want %v", tt.mode, got, tt.labels)
		}
		if n := tt.mode.lookupsPerCheck(); n != tt.lookups {
			t.Errorf("%s lookupsPerCheck = %d, want %d", tt.mode, n, tt.lookups)
		}
	}
}

func TestIPModeFamiliesReadTheRightField(t *testing.T) {
	r := ipResult{IPv4: "9.9.9.9", IPv6: "2001:db8::1"}
	for _, f := range IPModeBoth.families() {
		want := "9.9.9.9"
		if f.Label == familyV6 {
			want = "2001:db8::1"
		}
		if got := f.Get(r); got != want {
			t.Errorf("%s Get = %q, want %q", f.Label, got, want)
		}
	}
}

func TestParseFamily(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantV4  bool
		want    string
		wantErr string
	}{
		{name: "v4 from a v4 endpoint", body: "9.9.9.9\n", wantV4: true, want: "9.9.9.9"},
		{name: "v6 from a v6 endpoint", body: " 2001:db8::1 ", wantV4: false, want: "2001:db8::1"},
		{
			// The assertion the single-family endpoint sets exist to make
			// enforceable. A v6 answer from a v4 host is a bug in the host, and
			// storing it is how one proxy came to report two different exits.
			name: "v6 from a v4 endpoint is rejected",
			body: "2001:db8::1", wantV4: true, wantErr: "wanted IPv4",
		},
		{
			name: "v4 from a v6 endpoint is rejected",
			body: "9.9.9.9", wantV4: false, wantErr: "wanted IPv6",
		},
		{
			// A throttle page is not an exit IP.
			name:   "an HTML throttle page is rejected",
			body:   "<html><body>429 Too Many Requests</body></html>",
			wantV4: true, wantErr: "not an IP address",
		},
		{name: "empty body is rejected", body: "   ", wantV4: true, wantErr: "not an IP address"},
		{name: "a hostname is rejected", body: "ipv4.example.invalid", wantV4: true, wantErr: "not an IP address"},
		{name: "a truncated address is rejected", body: "9.9.9.", wantV4: true, wantErr: "not an IP address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFamily(tt.body, tt.wantV4)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseFamily(%q, %v) = %q, want an error", tt.body, tt.wantV4, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to mention %q", err, tt.wantErr)
				}
				if got != "" {
					t.Errorf("a rejected body still returned %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFamily(%q, %v) errored: %v", tt.body, tt.wantV4, err)
			}
			if got != tt.want {
				t.Errorf("parseFamily(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

// bodyServer answers every request with status and body.
func bodyServer(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestLookupIP(t *testing.T) {
	good := bodyServer(t, http.StatusOK, "9.9.9.9\n")
	// A throttle response whose body happens to parse as an address. Without the
	// status check this would be stored as the proxy's exit IP.
	throttled := bodyServer(t, http.StatusTooManyRequests, "1.2.3.4")
	html := bodyServer(t, http.StatusOK, "<html>Too Many Requests</html>")
	wrongFamily := bodyServer(t, http.StatusOK, "2001:db8::1")

	tests := []struct {
		name      string
		endpoints []string
		wantV4    bool
		want      string
		wantErr   string
	}{
		{name: "a valid answer is returned", endpoints: []string{good}, wantV4: true, want: "9.9.9.9"},
		{
			name:      "a non-200 is rejected even when its body parses",
			endpoints: []string{throttled}, wantV4: true, wantErr: "HTTP 429",
		},
		{
			name:      "a non-IP body is rejected rather than stored",
			endpoints: []string{html}, wantV4: true, wantErr: "not an IP address",
		},
		{
			name:      "a v6 answer from a v4 endpoint is rejected",
			endpoints: []string{wrongFamily}, wantV4: true, wantErr: "wanted IPv4",
		},
		{
			// Order within the set is randomised, so this holds whichever host is
			// tried first.
			name:      "a throttled endpoint falls through to a working one",
			endpoints: []string{throttled, good}, wantV4: true, want: "9.9.9.9",
		},
		{
			name:      "a lying endpoint falls through to a working one",
			endpoints: []string{wrongFamily, good}, wantV4: true, want: "9.9.9.9",
		},
		{
			name:      "every endpoint failing reports why",
			endpoints: []string{throttled, html}, wantV4: true, wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{}
			// Run it enough times that the random start order cannot hide a case.
			for i := 0; i < 20; i++ {
				got, err := lookupIP(client, tt.endpoints, tt.wantV4)
				if tt.want != "" {
					if err != nil {
						t.Fatalf("lookupIP errored: %v", err)
					}
					if got != tt.want {
						t.Fatalf("lookupIP = %q, want %q", got, tt.want)
					}
					continue
				}
				if err == nil {
					t.Fatalf("lookupIP = %q, want an error", got)
				}
				if got != "" {
					t.Fatalf("a failed lookup returned %q", got)
				}
				if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to mention %q", err, tt.wantErr)
				}
			}
		})
	}
}

// The endpoint sets are the mode's whole guarantee: a host in both sets, or a
// set emptied by an edit, silently reintroduces the dual-stack ambiguity.
func TestEndpointSetsAreDisjointAndPopulated(t *testing.T) {
	if len(ipv4Endpoints) == 0 || len(ipv6Endpoints) == 0 {
		t.Fatal("an empty endpoint set leaves a mode with nothing to ask")
	}
	in4 := make(map[string]bool, len(ipv4Endpoints))
	for _, ep := range ipv4Endpoints {
		in4[ep] = true
	}
	for _, ep := range ipv6Endpoints {
		if in4[ep] {
			t.Errorf("%s is in both sets, so its family is not known from the set it is in", ep)
		}
	}
	for _, ep := range append(append([]string{}, ipv4Endpoints...), ipv6Endpoints...) {
		if !strings.HasPrefix(ep, "https://") {
			t.Errorf("%s is not https; an exit IP read over plaintext is not evidence", ep)
		}
	}
}

func TestIPModeLabel(t *testing.T) {
	for mode, want := range map[IPMode]string{
		IPModeV4:   "IPv4 only",
		IPModeV6:   "IPv6 only",
		IPModeBoth: "IPv4 and IPv6",
	} {
		if got := mode.Label(); got != want {
			t.Errorf("%s Label = %q, want %q", mode, got, want)
		}
	}
}

// The session monitor's banner quotes lookups per minute against free
// third-party endpoints, and both mode asks twice per check. Rebuilt from the
// same arithmetic the tool uses.
func TestLoadBannerDoublesInBothMode(t *testing.T) {
	const proxies = 100
	const perMinuteV4 = 100.0
	for mode, want := range map[IPMode]float64{
		IPModeV4:   perMinuteV4,
		IPModeV6:   perMinuteV4,
		IPModeBoth: 2 * perMinuteV4,
	} {
		got := float64(proxies * mode.lookupsPerCheck())
		if got != want {
			t.Errorf("%s load = %.0f lookups/min, want %.0f", mode, got, want)
		}
	}
}

// TestIPClient_DirectDoesNotUseAProxy. Same technique as the ping tests: a
// proxied request arrives with an absolute request URI, a direct one with a
// path. Asserting on what the server sees rather than on the transport's
// fields keeps the test honest if the construction is ever refactored.
func TestIPClient_DirectDoesNotUseAProxy(t *testing.T) {
	var gotAbsoluteURI bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAbsoluteURI = strings.HasPrefix(r.RequestURI, "http://")
		w.Write([]byte("203.0.113.9"))
	}))
	defer srv.Close()

	p, ok := proxy.ParseLine("direct")
	if !ok || !p.Direct {
		t.Fatal(`ParseLine("direct") did not produce a direct proxy`)
	}

	client, err := ipClient(p, nil)
	if err != nil {
		t.Fatalf("ipClient: %v", err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()

	if gotAbsoluteURI {
		t.Error("the lookup arrived in proxy form; a direct check must not set a proxy")
	}
}

// TestIPClient_ProxiedStillUsesTheProxy is the regression guard: without it,
// dropping the Proxy field entirely would satisfy the test above while silently
// making every exit-IP lookup report the user's own address.
func TestIPClient_ProxiedStillUsesTheProxy(t *testing.T) {
	var gotAbsoluteURI bool
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAbsoluteURI = strings.HasPrefix(r.RequestURI, "http://")
		w.Write([]byte("198.51.100.4"))
	}))
	defer proxySrv.Close()

	host, port, err := net.SplitHostPort(strings.TrimPrefix(proxySrv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := proxy.ParseLine(host + ":" + port + ":user:pass")
	if !ok || p.Direct {
		t.Fatalf("ParseLine did not produce a real proxy: %+v", p)
	}

	client, err := ipClient(p, nil)
	if err != nil {
		t.Fatalf("ipClient: %v", err)
	}
	resp, err := client.Get("http://example.invalid/")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()

	if !gotAbsoluteURI {
		t.Error("the lookup did not arrive in proxy form; a real proxy must still be used")
	}
}
