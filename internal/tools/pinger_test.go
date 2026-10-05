package tools

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"proxytoolbox/internal/proxy"
)

// A proxied HTTP request arrives at the server with an absolute request URI
// ("http://host/path"); a direct one arrives with just a path. That difference
// is the only thing visible from the server side that distinguishes the two, so
// it is what these tests assert on rather than anything about the transport.

func TestPingHTTP_DirectDoesNotUseAProxy(t *testing.T) {
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

	res := pingHTTP(0, p, srv.URL)
	if res.Err != nil {
		t.Fatalf("pingHTTP: %v", res.Err)
	}
	if res.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", res.Status)
	}
	if gotAbsoluteURI {
		t.Error("the request arrived in proxy form; a direct ping must not set a proxy")
	}
	if res.ProxyID != "direct" {
		t.Errorf("ProxyID = %q, want %q", res.ProxyID, "direct")
	}
}

// TestPingHTTP_ProxiedStillUsesTheProxy is the regression guard. Without it,
// deleting the Proxy field outright would satisfy the direct test above and
// quietly stop every real proxy from being used.
func TestPingHTTP_ProxiedStillUsesTheProxy(t *testing.T) {
	var gotAbsoluteURI bool
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAbsoluteURI = strings.HasPrefix(r.RequestURI, "http://")
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

	// Any absolute target: the request goes to the proxy, not to this host.
	res := pingHTTP(0, p, "http://example.invalid/")
	if res.Err != nil {
		t.Fatalf("pingHTTP: %v", res.Err)
	}
	if !gotAbsoluteURI {
		t.Error("the request did not arrive in proxy form; a real proxy must still be used")
	}
}

func TestPingRawTCP_DirectDialsTheTargetWithoutCONNECT(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		buf := make([]byte, 128)
		n, _ := conn.Read(buf)
		received <- string(buf[:n])
	}()

	p, ok := proxy.ParseLine("localhost")
	if !ok || !p.Direct {
		t.Fatal(`ParseLine("localhost") did not produce a direct proxy`)
	}

	res := pingRawTCPTo(0, p, ln.Addr().String())
	if res.Err != nil {
		t.Fatalf("pingRawTCPTo: %v", res.Err)
	}
	if res.ProxyID != "localhost" {
		t.Errorf("ProxyID = %q, want %q", res.ProxyID, "localhost")
	}

	select {
	case got := <-received:
		if strings.HasPrefix(got, "CONNECT") {
			t.Errorf("a direct raw ping sent %q; there is no proxy to CONNECT through", got)
		}
	case <-time.After(400 * time.Millisecond):
		// Nothing sent at all is the correct outcome: a direct raw ping is a
		// connect, and the connection is closed without writing.
	}
}

// TestPingRawTCP_ProxiedStillSendsCONNECT is the matching regression guard:
// taking the direct branch for everyone would otherwise pass the test above.
func TestPingRawTCP_ProxiedStillSendsCONNECT(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		buf := make([]byte, 256)
		n, _ := conn.Read(buf)
		received <- string(buf[:n])
		conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	}()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	p, ok := proxy.ParseLine(host + ":" + port + ":user:pass")
	if !ok || p.Direct {
		t.Fatalf("ParseLine did not produce a real proxy: %+v", p)
	}

	pingRawTCPTo(0, p, "example.invalid:80")

	select {
	case got := <-received:
		if !strings.HasPrefix(got, "CONNECT") {
			t.Errorf("a proxied raw ping sent %q, want a CONNECT request", got)
		}
	case <-time.After(400 * time.Millisecond):
		t.Error("the proxy received nothing; a proxied raw ping must send CONNECT")
	}
}

// TestRawPingTarget pins the port the bare-host mode dials, so the split into
// pingRawTCPTo cannot silently change what it measures.
//
// Asserted on the composed string rather than on a dial error: an unresolvable
// name fails at DNS before the port is used, and "127.0.0.1:8080" contains
// "127.0.0.1:80", so an error-text check accepts the wrong port.
func TestRawPingTarget(t *testing.T) {
	tests := []struct{ name, host, want string }{
		{"loopback", "127.0.0.1", "127.0.0.1:80"},
		{"hostname", "example.invalid", "example.invalid:80"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rawPingTarget(tt.host); got != tt.want {
				t.Errorf("rawPingTarget(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}
