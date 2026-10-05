package tools

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"

	"github.com/charmbracelet/huh"
)

// IPMode selects which address families a check asks the IP endpoints for.
//
// It exists because the endpoints used to be dual-stack: a dual-stack proxy
// prefers IPv6 where an AAAA record exists, so the same proxy answered with
// its v4 exit from one endpoint and its v6 exit from the next, in the same
// second. The session monitor read every such pair as a rotation and the
// uniqueness count was taken across two address spaces.
type IPMode string

const (
	IPModeV4   IPMode = "ipv4"
	IPModeV6   IPMode = "ipv6"
	IPModeBoth IPMode = "both"
)

// Endpoint sets, every host verified single-family. A dual-stack host must
// never be added to either: the mode's guarantee is that the family of the
// answer is known before it is read.
var (
	ipv4Endpoints = []string{
		"https://api.ipify.org",
		"https://ipv4.icanhazip.com",
		"https://v4.ident.me",
	}
	ipv6Endpoints = []string{
		"https://api6.ipify.org",
		"https://ipv6.icanhazip.com",
		"https://v6.ident.me",
	}
)

// Family labels. A rotation report that does not say which family rotated is
// unreadable in both mode, so these reach the log line and the webhook.
const (
	familyV4 = "IPv4"
	familyV6 = "IPv6"
)

// maxBodyEcho caps how much of a rejected body is quoted back. A throttle page
// is an HTML document and the whole of it does not belong in an error column.
const maxBodyEcho = 60

// ParseIPMode reads a mode name from config or a prompt. Blank or unrecognised
// input falls back to the default: a typo in config.txt should not stop a run.
func ParseIPMode(s string) IPMode { return ParseIPModeOr(s, IPModeV4) }

// ParseIPModeOr is ParseIPMode with the fallback named, so a runtime prompt can
// fall back to what config supplied rather than to the built-in default.
func ParseIPModeOr(s string, def IPMode) IPMode {
	switch IPMode(strings.ToLower(strings.TrimSpace(s))) {
	case IPModeV4:
		return IPModeV4
	case IPModeV6:
		return IPModeV6
	case IPModeBoth:
		return IPModeBoth
	default:
		return def
	}
}

// Label is the mode as a banner reads it.
func (m IPMode) Label() string {
	switch m {
	case IPModeV6:
		return "IPv6 only"
	case IPModeBoth:
		return "IPv4 and IPv6"
	default:
		return "IPv4 only"
	}
}

// lookupsPerCheck is how many requests one check costs. The session monitor
// quotes lookups per minute against free third-party endpoints, and both mode
// doubles that figure.
func (m IPMode) lookupsPerCheck() int {
	if m == IPModeBoth {
		return 2
	}
	return 1
}

// ipFamily is one address family as the tools display and tabulate it.
type ipFamily struct {
	Label string // "IPv4" / "IPv6"
	Width int    // terminal column width
	Get   func(ipResult) string
}

// families returns the families this mode tracks, v4 first. Looping over this
// keeps the uniqueness maths, the tables and the CSV from each growing their
// own three-way branch on the mode.
func (m IPMode) families() []ipFamily {
	v4 := ipFamily{Label: familyV4, Width: ipV4ColWidth, Get: func(r ipResult) string { return r.IPv4 }}
	v6 := ipFamily{Label: familyV6, Width: ipV6ColWidth, Get: func(r ipResult) string { return r.IPv6 }}
	switch m {
	case IPModeV6:
		return []ipFamily{v6}
	case IPModeBoth:
		return []ipFamily{v4, v6}
	default:
		return []ipFamily{v4}
	}
}

func (m IPMode) wantsV4() bool { return m == IPModeV4 || m == IPModeBoth }
func (m IPMode) wantsV6() bool { return m == IPModeV6 || m == IPModeBoth }

// promptIPMode offers the configured default and accepts an override. Both
// tools ask, so the wording lives here.
//
// A menu rather than typed input: the three modes are a closed set, so there is
// nothing to type that a list cannot offer, and typing invites a misspelling
// that silently falls back to the default — the one outcome the user would not
// notice. Arrow keys also match every other choice in the toolbox.
//
// The configured value is pre-selected, so Enter still means "use what
// config.txt says" exactly as the typed prompt did.
func promptIPMode(def IPMode) IPMode {
	mode := def

	err := huh.NewSelect[IPMode]().
		Title("IP mode").
		Description(fmt.Sprintf("From config.txt: %s", def)).
		Options(
			huh.NewOption("IPv4 only     — Exit IPv4 address, the family most sites see", IPModeV4),
			huh.NewOption("IPv6 only     — Exit IPv6 address", IPModeV6),
			huh.NewOption("IPv4 and IPv6 — Both, tracked separately (two lookups per check)", IPModeBoth),
		).
		Value(&mode).
		Run()
	if err != nil {
		// Ctrl+C or no terminal. The typed prompt treated a blank line as
		// "keep the configured value"; cancelling means the same thing.
		return def
	}
	return mode
}

// lookupIP returns the first endpoint answer that parses as an address of the
// requested family, trying the set in a rotated order so no single endpoint
// carries the whole run.
func lookupIP(client *http.Client, endpoints []string, wantV4 bool) (string, error) {
	offset := rand.Intn(len(endpoints))
	var lastErr error
	for i := range endpoints {
		ep := endpoints[(offset+i)%len(endpoints)]
		resp, err := client.Get(ep)
		if err != nil {
			lastErr = fmt.Errorf("[%s] %w", ep, err)
			continue
		}
		// Status before body. At the session monitor's sustained volume against
		// free endpoints, a throttle page becoming an "exit IP" is a matter of
		// when, not whether.
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("[%s] HTTP %d", ep, resp.StatusCode)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("[%s] read error: %w", ep, err)
			continue
		}
		ip, err := parseFamily(string(body), wantV4)
		if err != nil {
			lastErr = fmt.Errorf("[%s] %w", ep, err)
			continue
		}
		return ip, nil
	}
	return "", lastErr
}

// parseFamily validates one endpoint's body against the family that was asked
// for.
//
// A mismatched family is a bug in the endpoint, not data: the single-family
// sets exist precisely so the answer's family is known, so a v6 reply from a v4
// host is discarded and the next endpoint tried rather than stored under the
// wrong heading.
func parseFamily(body string, wantV4 bool) (string, error) {
	s := strings.TrimSpace(body)
	ip := net.ParseIP(s)
	if ip == nil {
		return "", fmt.Errorf("not an IP address: %q", truncate(s, maxBodyEcho))
	}
	if (ip.To4() != nil) != wantV4 {
		return "", fmt.Errorf("wanted %s, got %s", familyName(wantV4), s)
	}
	return s, nil
}

func familyName(v4 bool) string {
	if v4 {
		return familyV4
	}
	return familyV6
}
