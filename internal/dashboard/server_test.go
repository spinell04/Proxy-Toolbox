package dashboard

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// loopbackHost is the Host a browser sends when it opens the dashboard's own
// URL. httptest.NewRequest defaults to "example.com", which the Host guard
// refuses, so every request through handler() must set this explicitly.
const loopbackHost = "127.0.0.1:54321"

func doHost(t *testing.T, h http.Handler, method, target, host string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestListen_BindsLoopbackOnly is the security test for the listener. The
// exported CSVs carry user:pass@host:port in plaintext, so a listener on the
// wildcard address would hand a LAN neighbour the user's proxy credentials.
func TestListen_BindsLoopbackOnly(t *testing.T) {
	ln, err := listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("Addr = %T, want *net.TCPAddr", ln.Addr())
	}
	if addr.IP.IsUnspecified() {
		t.Fatalf("listening on %s — the wildcard address is reachable from the network", addr.IP)
	}
	if !addr.IP.IsLoopback() {
		t.Errorf("listening on %s, want a loopback address", addr.IP)
	}
	if addr.Port == 0 {
		t.Error("Addr reports port 0; the real bound port must be reported")
	}
}

// TestListen_PortIsNotHardcoded pins that the port comes from the OS: two
// listeners must coexist, which a fixed port would forbid.
func TestListen_PortIsNotHardcoded(t *testing.T) {
	first, err := listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer first.Close()

	second, err := listen()
	if err != nil {
		t.Fatalf("second listen failed, so the port is fixed: %v", err)
	}
	defer second.Close()

	if first.Addr().String() == second.Addr().String() {
		t.Errorf("both listeners report %s", first.Addr())
	}
}

// TestHandler_RoutesAPIAndStatic pins the wiring Serve depends on: the API is
// reachable under its prefix, the embedded UI is served everywhere else, and
// the static half never exposes the results directory.
func TestHandler_RoutesAPIAndStatic(t *testing.T) {
	dir := fixtureDir(t)
	h, err := handler(dir)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	cases := []struct {
		name     string
		method   string
		target   string
		wantCode int // ignored when wantRedirect is set
		wantBody string
		// wantRedirect asserts "any 3xx" instead of an exact code. The
		// stdlib picks the code for a cleaned path and has changed it:
		// ServeMux cleans with 301 on Go 1.25 and 307 on Go 1.26. Pinning
		// either one makes the suite pass on one toolchain and fail on the
		// other — which is exactly how this test passed locally and broke
		// the release build. What the case is actually for is that the
		// traversal is redirected rather than served, and that nothing
		// leaks; both are asserted below regardless of the code.
		wantRedirect bool
	}{
		{name: "api inventory", method: http.MethodGet, target: "/api/runs", wantCode: http.StatusOK, wantBody: "pinger_run.csv"},
		{name: "api detail", method: http.MethodGet, target: "/api/run?file=pinger_run.csv", wantCode: http.StatusOK, wantBody: "\"summary\""},
		{name: "api stays read-only", method: http.MethodPost, target: "/api/runs", wantCode: http.StatusMethodNotAllowed},
		{name: "static index", method: http.MethodGet, target: "/", wantCode: http.StatusOK, wantBody: "<title>Compare · Proxy Toolbox</title>"},
		{name: "static module", method: http.MethodGet, target: "/app.js", wantCode: http.StatusOK, wantBody: "uPlot 1.6.32"},
		{name: "static vendored chart library", method: http.MethodGet, target: "/vendor/uPlot.min.js", wantCode: http.StatusOK, wantBody: "uPlot"},
		{name: "static cannot reach results", method: http.MethodGet, target: "/pinger_run.csv", wantCode: http.StatusNotFound},
		// A traversing path is cleaned and redirected rather than served; the
		// target it redirects to is itself not served, which the next case pins.
		{name: "static traversal is cleaned away", method: http.MethodGet, target: "/../secret.csv", wantRedirect: true},
		{name: "static cannot reach the parent directory", method: http.MethodGet, target: "/secret.csv", wantCode: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doHost(t, h, tc.method, tc.target, loopbackHost)

			switch {
			case tc.wantRedirect:
				if rec.Code < 300 || rec.Code > 399 {
					t.Errorf("status = %d, want a 3xx redirect", rec.Code)
				}
				// The destination is the security content: the traversal must
				// resolve inside the served tree, and the next case pins that
				// the cleaned target is itself a 404.
				if loc := rec.Header().Get("Location"); loc != "/secret.csv" {
					t.Errorf("Location = %q, want %q", loc, "/secret.csv")
				}
			case rec.Code != tc.wantCode:
				t.Errorf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tc.wantBody)
			}
			if strings.Contains(rec.Body.String(), secretMarker) {
				t.Errorf("response leaked a file outside the results directory: %s", rec.Body.String())
			}
		})
	}
}

// TestHandler_RejectsNonLoopbackHost is the DNS-rebinding guard. A hostname the
// attacker controls, resolved to 127.0.0.1, reaches this server as an
// ordinary same-origin request from the browser's point of view — the Host
// header is the only thing that still names the attacker. Every route must
// refuse it, including the static HTML that would issue the API calls.
func TestHandler_RejectsNonLoopbackHost(t *testing.T) {
	dir := fixtureDir(t)
	h, err := handler(dir)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	routes := []struct {
		target string
		// allowedCode is what a loopback Host gets. A missing asset yields 404,
		// and that is the point of including one: a refused Host must get 403
		// on that same route, which can only happen if the guard runs before
		// the file server looks at the path at all.
		allowedCode int
	}{
		{"/", http.StatusOK},
		{"/does-not-exist.css", http.StatusNotFound},
		{"/api/runs", http.StatusOK},
		{"/api/run?file=pinger_run.csv", http.StatusOK},
		{"/api/compare?file=pinger_run.csv", http.StatusOK},
	}

	hosts := []struct {
		name    string
		host    string
		allowed bool
	}{
		{"attacker hostname", "evil.example.com", false},
		{"attacker hostname with port", "evil.example.com:54321", false},
		{"hostname containing localhost", "localhost.evil.example.com", false},
		{"hostname prefixed with localhost", "localhost.evil.com:54321", false},
		{"public ip", "203.0.113.7:54321", false},
		{"lan ip", "192.168.1.20:54321", false},
		{"empty host", "", false},
		{"ipv4 loopback", "127.0.0.1:54321", true},
		{"ipv4 loopback without port", "127.0.0.1", true},
		{"ipv6 loopback", "[::1]:54321", true},
		{"localhost", "localhost:54321", true},
		{"localhost cased", "LOCALHOST:54321", true},
	}

	for _, hc := range hosts {
		for _, route := range routes {
			t.Run(hc.name+" "+route.target, func(t *testing.T) {
				want := http.StatusForbidden
				if hc.allowed {
					want = route.allowedCode
				}

				rec := doHost(t, h, http.MethodGet, route.target, hc.host)

				if rec.Code != want {
					t.Errorf("status = %d, want %d", rec.Code, want)
				}
				if !hc.allowed && strings.Contains(rec.Body.String(), "pinger") {
					t.Errorf("a refused request still returned run data: %s", rec.Body.String())
				}
			})
		}
	}
}
