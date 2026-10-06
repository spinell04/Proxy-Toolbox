package util

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestResolver_IPLiteralIsUnchanged(t *testing.T) {
	var r Resolver
	for _, in := range []string{
		"127.0.0.1:8080",
		"1.2.3.4:9000",
		"[::1]:8080",
	} {
		t.Run(in, func(t *testing.T) {
			got, err := r.Resolve(context.Background(), in)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", in, err)
			}
			if got != in {
				t.Errorf("Resolve(%q) = %q, want it unchanged — an IP needs no lookup", in, got)
			}
		})
	}
}

func TestResolver_ResolvesAHostnameToAnAddress(t *testing.T) {
	var r Resolver
	got, err := r.Resolve(context.Background(), "localhost:8080")
	if err != nil {
		t.Skipf("localhost did not resolve on this machine: %v", err)
	}
	host, port, err := net.SplitHostPort(got)
	if err != nil {
		t.Fatalf("Resolve returned %q, which is not host:port: %v", got, err)
	}
	if net.ParseIP(host) == nil {
		t.Errorf("Resolve(%q) = %q; the host is not an IP, so the lookup was not applied", "localhost:8080", got)
	}
	if port != "8080" {
		t.Errorf("port = %q, want 8080 — the port must survive resolution", port)
	}
}

// TestResolver_CachesPerHost pins the property the whole type exists for: the
// second call must not reach the resolver, or every check would still pay a
// lookup and nothing would have been excluded.
//
// Counted rather than inferred. An earlier version only checked that the same
// host gave the same address, which is true with or without a cache, so
// deleting the cache left it passing.
func TestResolver_CachesPerHost(t *testing.T) {
	var lookups int32
	r := Resolver{Lookup: func(ctx context.Context, host string) ([]string, error) {
		atomic.AddInt32(&lookups, 1)
		return []string{"203.0.113.7"}, nil
	}}

	first, err := r.Resolve(context.Background(), "gw.example.invalid:1111")
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	// A different port, same host: must reuse the cached address, keep the port,
	// and not look anything up again.
	second, err := r.Resolve(context.Background(), "gw.example.invalid:2222")
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}

	if n := atomic.LoadInt32(&lookups); n != 1 {
		t.Errorf("resolver was called %d times for one host, want 1 — the cache is not working", n)
	}
	if first != "203.0.113.7:1111" {
		t.Errorf("first = %q, want 203.0.113.7:1111", first)
	}
	if second != "203.0.113.7:2222" {
		t.Errorf("second = %q, want 203.0.113.7:2222", second)
	}
}

func TestResolver_UnresolvableHostIsReturnedUnchanged(t *testing.T) {
	var r Resolver
	const in = "no-such-host.invalid:8080"

	got, err := r.Resolve(context.Background(), in)
	if err == nil {
		t.Fatal("expected an error for a reserved unresolvable name")
	}
	// Unchanged, so the caller can still dial by name and surface the dial's
	// own error rather than one invented here.
	if got != in {
		t.Errorf("Resolve(%q) = %q, want it unchanged so the caller can still dial it", in, got)
	}
}

func TestResolver_IsRaceFree(t *testing.T) {
	var r Resolver
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.Resolve(context.Background(), "localhost:8080")
		}()
	}
	wg.Wait()
}

// TestResolver_DialContextKeepsTheHostHeader is the test that matters most for
// the HTTP paths. Substituting an IP at dial time is only safe because net/http
// takes the Host header and the TLS server name from the request URL. If that
// stopped being true, every virtual-hosted target would silently get the wrong
// site back, and the latency numbers would look fine.
func TestResolver_DialContextKeepsTheHostHeader(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotHost = req.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ip, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}

	// The cache is seeded rather than resolved, so the test does not depend on
	// how this machine maps "localhost" — an earlier version did, resolved it
	// to ::1 while httptest listened on 127.0.0.1, and skipped itself.
	r := Resolver{cache: map[string]string{"example.invalid": ip}}

	client := &http.Client{Transport: &http.Transport{
		DialContext: r.DialContext(&net.Dialer{}),
	}}

	// A name that does not resolve anywhere: reaching the server at all proves
	// the dial address came from the cache.
	resp, err := client.Get("http://example.invalid:" + port + "/")
	if err != nil {
		t.Fatalf("request failed, so DialContext did not substitute the address: %v", err)
	}
	resp.Body.Close()

	if want := "example.invalid:" + port; gotHost != want {
		t.Errorf("Host header = %q, want %q — the dial address leaked into the request", gotHost, want)
	}
}

func TestResolver_WarmDoesNotFailOnABadName(t *testing.T) {
	var r Resolver
	// Must not panic and must not block the caller; a name that will not
	// resolve is the dial's problem to report, not Warm's.
	r.Warm(context.Background(), []string{
		"no-such-host.invalid:80",
		"127.0.0.1:80",
		"not-a-host-port",
	})
}
