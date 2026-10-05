package proxy

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Proxy holds parsed host:port:user:pass fields.
type Proxy struct {
	Host     string
	Port     string
	User     string
	Password string
	Raw      string // verbatim source line (trimmed)

	// Direct marks a line that means "no proxy": the request is made from this
	// machine. It is a baseline, so the tools can measure the user's own
	// connection under the same target, timeout and worker pool as the proxies
	// they are comparing it against.
	Direct bool
}

// URL returns the proxy as an http://user:pass@host:port URL.
func (p Proxy) URL() string {
	// Empty rather than a sentinel string: every caller either skips the
	// transport's Proxy field entirely or, in the TM/Bayern path, already tests
	// for an empty URL before setting one.
	if p.Direct {
		return ""
	}
	return fmt.Sprintf("http://%s:%s@%s:%s", p.User, p.Password, p.Host, p.Port)
}

// ID returns the canonical identity of a proxy: user:pass@host:port.
//
// The full credential string is the identity, not host:port. Gateway-style
// pools share a single host and port across many session credentials, so
// host:port would collapse hundreds of distinct proxies into one key.
//
// All accepted input formats normalize to this form, so the same proxy
// written differently in two files still joins. The host is lowercased
// because DNS names are case-insensitive; credentials are not, and are
// kept verbatim.
//
// Credentials are assumed not to contain ":" or "@"; ParseLine's grammar
// cannot produce such fields from the colon-delimited formats.
func (p Proxy) ID() string {
	// The line as written, lowercased. The tables and the CSV both print ID, so
	// a direct run shows the spelling the user chose rather than a token they
	// did not. Lowercased for the same reason the host is below: "Direct" and
	// "direct" are one thing. Two different spellings are two different rows in
	// Compare Results, which is what the file said.
	if p.Direct {
		return strings.ToLower(strings.TrimSpace(p.Raw))
	}
	return fmt.Sprintf("%s:%s@%s:%s", p.User, p.Password, strings.ToLower(p.Host), p.Port)
}

// directLines are the whole-line spellings that mean "make this request with no
// proxy", matched after trimming and lowercasing.
//
// All three are safe because they are meaningless as addresses today: "direct"
// and "localhost" have too few colon-separated fields to parse at all, and
// "localhost:localhost:localhost:localhost" parses but yields the port
// "localhost", which url.Parse rejects. None of them can be confused with a
// real proxy on loopback — "localhost:8080:user:pass" still means the Squid or
// mitmproxy listening on 8080, and must keep meaning that.
var directLines = map[string]bool{
	"direct":    true,
	"localhost": true,
	"localhost:localhost:localhost:localhost": true,
}

// Addr is the proxy as a label: host:port, without credentials.
//
// The monitors print this rather than ID() because a Discord embed and a log
// file on disk are both places a credential should not land.
//
// A direct line has no host and no port, so it labels itself with its own line
// — the string the user wrote, and the one every other tool shows for it.
// Without this it rendered as ":", which is what a bare host:port format does
// with two empty fields.
//
// fmt.Sprintf rather than net.JoinHostPort: the latter brackets IPv6 hosts,
// which would change the existing output for every proxy. This is a label.
func (p Proxy) Addr() string {
	if p.Direct {
		return p.ID()
	}
	return fmt.Sprintf("%s:%s", p.Host, p.Port)
}

// ParseLine parses a proxy line in any common format into a Proxy.
// Supported formats:
//   - host:port:user:pass
//   - user:pass:host:port
//   - user:pass@host:port
//   - http://user:pass@host:port
func ParseLine(line string) (Proxy, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return Proxy{}, false
	}

	if directLines[strings.ToLower(line)] {
		return Proxy{Direct: true, Raw: line}, true
	}

	// http://user:pass@host:port or user:pass@host:port
	if strings.Contains(line, "@") {
		raw := line
		if !strings.HasPrefix(raw, "http") {
			raw = "http://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return Proxy{}, false
		}
		pass, _ := u.User.Password()
		return Proxy{
			Host:     u.Hostname(),
			Port:     u.Port(),
			User:     u.User.Username(),
			Password: pass,
			Raw:      line,
		}, true
	}

	// host:port:user:pass or user:pass:host:port
	parts := strings.SplitN(line, ":", 4)
	if len(parts) != 4 {
		return Proxy{}, false
	}

	// Detect which order: if parts[2] looks like an IP/hostname, it's user:pass:host:port
	if looksLikeHost(parts[2]) && !looksLikeHost(parts[0]) {
		return Proxy{
			Host:     parts[2],
			Port:     parts[3],
			User:     parts[0],
			Password: parts[1],
			Raw:      line,
		}, true
	}

	// Default: host:port:user:pass
	return Proxy{
		Host:     parts[0],
		Port:     parts[1],
		User:     parts[2],
		Password: parts[3],
		Raw:      line,
	}, true
}

// looksLikeHost checks if a string looks like an IP address or hostname with dots.
func looksLikeHost(s string) bool {
	return strings.Contains(s, ".")
}

// LoadRawLines reads a proxy file and returns the raw non-empty, non-comment lines.
func LoadRawLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}
