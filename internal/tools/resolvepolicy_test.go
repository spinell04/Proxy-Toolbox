package tools

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"proxytoolbox/internal/config"
	"proxytoolbox/internal/proxy"
	"proxytoolbox/internal/util"
)

func TestRunResolver_FollowsTheConfigKey(t *testing.T) {
	if got := runResolver(config.Config{MeasureDNS: false}); got == nil {
		t.Error("measure_dns=off should give a resolver, so the lookup is excluded")
	}
	// nil is the whole mechanism for "measure it": every call site then dials
	// by name, which is the code that shipped before this existed.
	if got := runResolver(config.Config{MeasureDNS: true}); got != nil {
		t.Error("measure_dns=on should give no resolver, so dials happen by name")
	}
}

func TestTargetDialAddr(t *testing.T) {
	tests := []struct{ name, target, want string }{
		{"bare host is the raw TCP port", "google.com", "google.com:80"},
		{"http default port", "http://google.com", "google.com:80"},
		{"https default port", "https://google.com", "google.com:443"},
		{"explicit port wins", "https://google.com:8443", "google.com:8443"},
		{"empty target", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := targetDialAddr(tt.target); got != tt.want {
				t.Errorf("targetDialAddr(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestDialAddrs(t *testing.T) {
	parse := func(lines ...string) []proxy.Proxy {
		var out []proxy.Proxy
		for _, l := range lines {
			p, ok := proxy.ParseLine(l)
			if !ok {
				t.Fatalf("ParseLine(%q) failed", l)
			}
			out = append(out, p)
		}
		return out
	}

	t.Run("a proxy contributes its gateway", func(t *testing.T) {
		got := dialAddrs(parse("gw.example.com:9000:u:p"), "https://target.example")
		want := []string{"gw.example.com:9000"}
		if len(got) != 1 || got[0] != want[0] {
			t.Errorf("dialAddrs = %v, want %v", got, want)
		}
	})

	t.Run("a direct line contributes the target", func(t *testing.T) {
		got := dialAddrs(parse("direct"), "https://target.example")
		if len(got) != 1 || got[0] != "target.example:443" {
			t.Errorf("dialAddrs = %v, want [target.example:443]", got)
		}
	})

	// Several direct lines are one address to resolve, not N.
	t.Run("repeated direct lines add the target once", func(t *testing.T) {
		got := dialAddrs(parse("direct", "localhost", "direct"), "google.com")
		if len(got) != 1 || got[0] != "google.com:80" {
			t.Errorf("dialAddrs = %v, want [google.com:80]", got)
		}
	})
}

func TestIPDialAddrs_IncludesTheEndpointsOnlyForDirectLines(t *testing.T) {
	p, _ := proxy.ParseLine("gw.example.com:9000:u:p")
	d, _ := proxy.ParseLine("direct")

	onlyProxy := ipDialAddrs([]proxy.Proxy{p}, IPModeV4)
	if len(onlyProxy) != 1 || onlyProxy[0] != "gw.example.com:9000" {
		t.Errorf("proxy-only run = %v, want just the gateway — the endpoints are the proxy's to resolve", onlyProxy)
	}

	// A direct line reaches the endpoints itself, and lookupIP starts at a
	// random one and falls through, so every endpoint of the mode may be timed.
	withDirect := ipDialAddrs([]proxy.Proxy{p, d}, IPModeV4)
	if len(withDirect) != 1+len(ipv4Endpoints) {
		t.Errorf("got %d addresses, want %d (gateway + every v4 endpoint)", len(withDirect), 1+len(ipv4Endpoints))
	}
	for _, a := range withDirect[1:] {
		if _, _, err := net.SplitHostPort(a); err != nil {
			t.Errorf("endpoint address %q is not host:port: %v", a, err)
		}
	}

	both := ipDialAddrs([]proxy.Proxy{d}, IPModeBoth)
	if len(both) != len(ipv4Endpoints)+len(ipv6Endpoints) {
		t.Errorf("both mode gave %d endpoints, want %d", len(both), len(ipv4Endpoints)+len(ipv6Endpoints))
	}
}

func TestWarm_IsANoOpWithoutAResolver(t *testing.T) {
	p, _ := proxy.ParseLine("direct")
	// Must not panic; nil is the measured-DNS path.
	warm(nil, []proxy.Proxy{p}, "google.com")
	warmIP(nil, []proxy.Proxy{p}, IPModeV4)
}

// TestPingRawTCPTo_DialsTheResolvedAddress proves the ping path actually
// consults the resolver, rather than only compiling against it.
//
// The name used here resolves nowhere, so connecting at all is only possible if
// the resolved address was dialled. Timing would not be an assertion; this is.
func TestPingRawTCPTo_DialsTheResolvedAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	p, ok := proxy.ParseLine("direct")
	if !ok || !p.Direct {
		t.Fatal("ParseLine(\"direct\") did not produce a direct proxy")
	}
	target := "nowhere.invalid:" + port

	res := &util.Resolver{Lookup: func(ctx context.Context, host string) ([]string, error) {
		return []string{"127.0.0.1"}, nil
	}}

	if got := pingRawTCPTo(0, p, target, res); got.Err != nil {
		t.Errorf("with a resolver: %v — the resolved address was not dialled", got.Err)
	}

	// The control. Without a resolver the name is dialled as written, and a
	// reserved name cannot connect. If this ever passes, the test above proves
	// nothing.
	if got := pingRawTCPTo(0, p, target, nil); got.Err == nil {
		t.Error("without a resolver the reserved name connected; the test above is not discriminating")
	}
}

// TestIPClient_DialsTheResolvedAddress is the ipClient counterpart: proof the
// exit-IP path consults the resolver rather than merely compiling against it.
func TestIPClient_DialsTheResolvedAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("203.0.113.9"))
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))

	p, _ := proxy.ParseLine("direct")
	url := "http://nowhere.invalid:" + port + "/"

	res := &util.Resolver{Lookup: func(ctx context.Context, host string) ([]string, error) {
		return []string{"127.0.0.1"}, nil
	}}

	client, err := ipClient(p, res)
	if err != nil {
		t.Fatalf("ipClient: %v", err)
	}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("with a resolver: %v — the resolved address was not dialled", err)
	}
	resp.Body.Close()

	// The control: without one, the reserved name cannot connect.
	plain, err := ipClient(p, nil)
	if err != nil {
		t.Fatalf("ipClient(nil): %v", err)
	}
	if resp, err := plain.Get(url); err == nil {
		resp.Body.Close()
		t.Error("without a resolver the reserved name connected; the check above is not discriminating")
	}
}
