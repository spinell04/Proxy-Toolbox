package tools

import (
	"context"
	"net"
	"net/url"
	"strings"

	"proxytoolbox/internal/config"
	"proxytoolbox/internal/proxy"
	"proxytoolbox/internal/util"
)

// runResolver returns the resolver a run should dial through, or nil when the
// config asks for the DNS lookup to count toward the latency.
//
// nil rather than a flag on the resolver: a nil receiver reads at every call
// site as "no resolution step", which is exactly the old behaviour, and makes
// the measured-DNS path literally the code that shipped before this existed.
func runResolver(cfg config.Config) *util.Resolver {
	if cfg.MeasureDNS {
		return nil
	}
	return &util.Resolver{}
}

// targetDialAddr is the host:port a direct line would dial for a ping target.
//
// The Ping Test accepts three forms — a bare host for raw TCP, and http:// or
// https:// for a full request — so the port depends on the scheme the user
// chose, not on a default.
func targetDialAddr(target string) string {
	switch {
	case strings.HasPrefix(target, "http://"), strings.HasPrefix(target, "https://"):
		u, err := url.Parse(target)
		if err != nil || u.Hostname() == "" {
			return ""
		}
		port := u.Port()
		if port == "" {
			port = "80"
			if u.Scheme == "https" {
				port = "443"
			}
		}
		return net.JoinHostPort(u.Hostname(), port)
	case target == "":
		return ""
	default:
		return rawPingTarget(target)
	}
}

// dialAddrs is every address a run will dial, so they can be resolved before
// any of them is timed.
//
// A proxy contributes its gateway; a direct line contributes the target, since
// that is what it connects to. Duplicates are fine — the resolver caches per
// host, so a file of 100 proxies behind one gateway costs one lookup.
func dialAddrs(proxies []proxy.Proxy, target string) []string {
	var addrs []string
	seenDirect := false
	for _, p := range proxies {
		if p.Direct {
			if !seenDirect {
				if a := targetDialAddr(target); a != "" {
					addrs = append(addrs, a)
				}
				seenDirect = true
			}
			continue
		}
		addrs = append(addrs, net.JoinHostPort(p.Host, p.Port))
	}
	return addrs
}

// warm resolves a run's addresses before the first measurement, so no check
// pays for a lookup. A no-op when the lookup is meant to be measured.
func warm(res *util.Resolver, proxies []proxy.Proxy, target string) {
	if res == nil {
		return
	}
	res.Warm(context.Background(), dialAddrs(proxies, target))
}

// ipDialAddrs is every address an exit-IP run will dial.
//
// A proxy contributes its gateway. A direct line contributes the lookup
// endpoints themselves, and all of the mode's endpoints rather than one,
// because lookupIP picks a starting point at random and falls through to the
// others when one fails — so any of them may be the address that gets timed.
func ipDialAddrs(proxies []proxy.Proxy, mode IPMode) []string {
	var addrs []string
	direct := false
	for _, p := range proxies {
		if p.Direct {
			direct = true
			continue
		}
		addrs = append(addrs, net.JoinHostPort(p.Host, p.Port))
	}
	if !direct {
		return addrs
	}

	var endpoints []string
	if mode.wantsV4() {
		endpoints = append(endpoints, ipv4Endpoints...)
	}
	if mode.wantsV6() {
		endpoints = append(endpoints, ipv6Endpoints...)
	}
	for _, ep := range endpoints {
		if u, err := url.Parse(ep); err == nil && u.Hostname() != "" {
			port := u.Port()
			if port == "" {
				port = "443"
				if u.Scheme == "http" {
					port = "80"
				}
			}
			addrs = append(addrs, net.JoinHostPort(u.Hostname(), port))
		}
	}
	return addrs
}

// warmIP is warm for the exit-IP tools.
func warmIP(res *util.Resolver, proxies []proxy.Proxy, mode IPMode) {
	if res == nil {
		return
	}
	res.Warm(context.Background(), ipDialAddrs(proxies, mode))
}
