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
}

// URL returns the proxy as an http://user:pass@host:port URL.
func (p Proxy) URL() string {
	return fmt.Sprintf("http://%s:%s@%s:%s", p.User, p.Password, p.Host, p.Port)
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
		}, true
	}

	// Default: host:port:user:pass
	return Proxy{
		Host:     parts[0],
		Port:     parts[1],
		User:     parts[2],
		Password: parts[3],
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
