package util

import (
	"context"
	"net"
	"sync"
)

// Resolver turns host:port into ip:port, once per host.
//
// It exists so a DNS lookup can be paid before a stopwatch starts rather than
// inside it. Every tool used to time a dial by hostname, which resolves and then
// connects, so the reported latency carried a lookup that `ping` reports
// separately — and the gap was widest exactly where the network was fastest,
// because on a sub-millisecond path the lookup was the whole number.
//
// Deliberately not "warm the OS cache and dial by name": Go's pure-Go resolver
// does not cache, and a Linux box with nothing behind libc would keep paying on
// every check. That heuristic would work on Windows and macOS and silently fail
// elsewhere, which is worse than not doing it.
//
// The zero value is ready to use. Safe for concurrent use: every tool runs its
// checks through a worker pool.
type Resolver struct {
	// Lookup resolves a host to addresses. Nil uses the system resolver.
	//
	// Exported so a test can prove a caller actually consults the resolver.
	// Without it the only way to tell "dialed the resolved address" from
	// "dialed the name" is timing, which is not an assertion.
	Lookup func(ctx context.Context, host string) ([]string, error)

	mu    sync.RWMutex
	cache map[string]string // host -> ip
}

// Resolve returns hostport with its host replaced by an address, resolving at
// most once per host for the Resolver's lifetime.
//
// An IP literal is returned unchanged, so a file of raw addresses never touches
// the network. An unresolvable host is returned unchanged with the error: the
// caller can still dial by name and let the dial report the failure, which keeps
// a broken name producing the error it always did rather than a new one from
// here.
func (r *Resolver) Resolve(ctx context.Context, hostport string) (string, error) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport, err
	}
	if net.ParseIP(host) != nil {
		return hostport, nil
	}

	r.mu.RLock()
	ip, ok := r.cache[host]
	r.mu.RUnlock()
	if ok {
		return net.JoinHostPort(ip, port), nil
	}

	lookup := r.Lookup
	if lookup == nil {
		lookup = net.DefaultResolver.LookupHost
	}
	addrs, err := lookup(ctx, host)
	if err != nil || len(addrs) == 0 {
		return hostport, err
	}

	// The first answer, not a random one: a second call for the same host must
	// return the same address, or two checks of one proxy would be measured
	// against two different servers and the comparison would be meaningless.
	ip = addrs[0]

	r.mu.Lock()
	if r.cache == nil {
		r.cache = make(map[string]string)
	}
	r.cache[host] = ip
	r.mu.Unlock()

	return net.JoinHostPort(ip, port), nil
}

// Warm resolves each address so no later call pays for the lookup.
//
// Errors are dropped on purpose. A name that will not resolve fails at dial
// time with the error the user needs to see, and failing the whole run here
// would turn one bad line in a proxy file into no results at all.
func (r *Resolver) Warm(ctx context.Context, hostports []string) {
	for _, hp := range hostports {
		_, _ = r.Resolve(ctx, hp)
	}
}

// DialContext dials addr with its host pre-resolved.
//
// Written for http.Transport.DialContext. Substituting the address here is safe
// in a way it is not for the TLS fingerprinting client: net/http takes the SNI
// name and the Host header from the request URL, never from the dial address,
// so the connection still presents the hostname the caller asked for.
func (r *Resolver) DialContext(d *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		resolved, err := r.Resolve(ctx, addr)
		if err != nil {
			// Dial the name and let the dial report the failure.
			resolved = addr
		}
		return d.DialContext(ctx, network, resolved)
	}
}
