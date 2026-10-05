package tools

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"proxytoolbox/internal/proxy"
)

// TestTestSingleProxy_DirectDoesNotUseAProxy confirms the TM/Bayern path needs
// no change for direct mode: testSingleProxy already guards with
// `if proxyURL != ""`, which was dead code until Proxy.URL() started returning
// empty for a direct line. This test is what makes that claim checkable rather
// than assumed.
func TestTestSingleProxy_DirectDoesNotUseAProxy(t *testing.T) {
	var gotAbsoluteURI bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAbsoluteURI = strings.HasPrefix(r.RequestURI, "http://")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p, ok := proxy.ParseLine("direct")
	if !ok || !p.Direct {
		t.Fatal(`ParseLine("direct") did not produce a direct proxy`)
	}

	res := testSingleProxy(0, p, srv.URL)
	if res.Err != nil {
		t.Fatalf("testSingleProxy: %v", res.Err)
	}
	if res.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", res.Status)
	}
	if gotAbsoluteURI {
		t.Error("the request arrived in proxy form; a direct check must not set a proxy")
	}
	if res.ProxyID != "direct" {
		t.Errorf("ProxyID = %q, want %q", res.ProxyID, "direct")
	}
}

// TestTestSingleProxy_ProxiedStillUsesTheProxy is the regression guard for the
// same branch, so "direct works" cannot ship alongside "no proxy ever works".
func TestTestSingleProxy_ProxiedStillUsesTheProxy(t *testing.T) {
	var gotLine string
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLine = r.Method + " " + r.RequestURI
		w.WriteHeader(http.StatusOK)
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

	// example.invalid does not resolve, so the request can only succeed by
	// going through the proxy. That is the proof here, rather than the
	// absolute-URI check used for the net/http paths: tls-client reaches the
	// proxy with an origin-form request line ("GET /"), so URI shape says
	// nothing about whether a proxy was used.
	res := testSingleProxy(0, p, "http://example.invalid/")

	if gotLine == "" {
		t.Error("the proxy server received nothing; a real proxy must still be used")
	}
	if res.Err != nil {
		t.Errorf("request failed (%v); it reached an unresolvable host, so the proxy was bypassed", res.Err)
	}
}
