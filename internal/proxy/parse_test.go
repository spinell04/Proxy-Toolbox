package proxy

import (
	"strings"
	"testing"
)

func TestParseLinePreservesRaw(t *testing.T) {
	tests := []struct {
		name string
		line string
		raw  string
	}{
		{"host:port:user:pass", "1.2.3.4:8080:admin:secret", "1.2.3.4:8080:admin:secret"},
		{"with surrounding spaces", "  1.2.3.4:8080:admin:secret  ", "1.2.3.4:8080:admin:secret"},
		{"user:pass@host:port", "admin:secret@1.2.3.4:8080", "admin:secret@1.2.3.4:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := ParseLine(tt.line)
			if !ok {
				t.Fatalf("ParseLine(%q) returned ok=false", tt.line)
			}
			if p.Raw != tt.raw {
				t.Errorf("Raw = %q, want %q", p.Raw, tt.raw)
			}
		})
	}
}

func TestProxyID_NormalizesAllInputFormats(t *testing.T) {
	// Every accepted input format for the same proxy must produce one ID.
	lines := []string{
		"1.2.3.4:8080:alice:secret",
		"alice:secret:1.2.3.4:8080",
		"alice:secret@1.2.3.4:8080",
		"http://alice:secret@1.2.3.4:8080",
	}
	const want = "alice:secret@1.2.3.4:8080"

	for _, line := range lines {
		p, ok := ParseLine(line)
		if !ok {
			t.Fatalf("ParseLine(%q) failed to parse", line)
		}
		if got := p.ID(); got != want {
			t.Errorf("ParseLine(%q).ID() = %q, want %q", line, got, want)
		}
	}
}

func TestProxyID_DistinguishesGatewaySessions(t *testing.T) {
	// Gateway pools share host:port and differ only by credentials.
	a, _ := ParseLine("gate.provider.com:7000:user-session-aaa:pw")
	b, _ := ParseLine("gate.provider.com:7000:user-session-bbb:pw")

	if a.ID() == b.ID() {
		t.Errorf("gateway sessions collapsed to one ID: %q", a.ID())
	}
}

func TestProxyID_LowercasesHost(t *testing.T) {
	// DNS hostnames are case-insensitive, and ID() is a cross-file join key.
	// Differing case must not split one proxy into two identities.
	upper, _ := ParseLine("Gate.Provider.COM:7000:alice:secret")
	lower, _ := ParseLine("gate.provider.com:7000:alice:secret")

	if upper.ID() != lower.ID() {
		t.Errorf("host case split one proxy into two IDs: %q vs %q", upper.ID(), lower.ID())
	}
}

func TestProxyID_PreservesCredentialCase(t *testing.T) {
	// Credentials are case-sensitive and must survive verbatim.
	p, _ := ParseLine("1.2.3.4:8080:Alice:SeCreT")

	if got := p.ID(); got != "Alice:SeCreT@1.2.3.4:8080" {
		t.Errorf("ID() = %q, want credentials unchanged", got)
	}
}

func TestParseLine_Direct(t *testing.T) {
	direct := []struct{ name, line string }{
		{"the word direct", "direct"},
		{"uppercase", "DIRECT"},
		{"mixed case", "Direct"},
		{"localhost", "localhost"},
		{"localhost with surrounding space", "  localhost  "},
		{"the four-field localhost form", "localhost:localhost:localhost:localhost"},
		{"four-field, mixed case", "LocalHost:localhost:LOCALHOST:localhost"},
	}
	for _, tt := range direct {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := ParseLine(tt.line)
			if !ok {
				t.Fatalf("ParseLine(%q) rejected the line", tt.line)
			}
			if !p.Direct {
				t.Errorf("Direct = false, want true for %q", tt.line)
			}
			if got := p.URL(); got != "" {
				t.Errorf("URL() = %q, want empty — a direct proxy has no dial address", got)
			}
			if got, want := p.ID(), strings.ToLower(strings.TrimSpace(tt.line)); got != want {
				t.Errorf("ID() = %q, want %q", got, want)
			}
		})
	}
}

// TestParseLine_LocalProxiesAreNotDirect is the test this feature could most
// easily break. People run Squid, Charles and mitmproxy on loopback; a line
// naming a port is an address, not a sentinel.
func TestParseLine_LocalProxiesAreNotDirect(t *testing.T) {
	tests := []struct {
		name, line, wantHost, wantPort string
	}{
		{"local proxy by name", "localhost:8080:user:pass", "localhost", "8080"},
		{"local proxy by loopback IP", "127.0.0.1:8080:user:pass", "127.0.0.1", "8080"},
		{"local proxy in @ form", "user:pass@localhost:8080", "localhost", "8080"},
		{"local proxy with scheme", "http://user:pass@localhost:3128", "localhost", "3128"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := ParseLine(tt.line)
			if !ok {
				t.Fatalf("ParseLine(%q) rejected a real local proxy", tt.line)
			}
			if p.Direct {
				t.Fatalf("Direct = true for %q — a local proxy was swallowed by the sentinel", tt.line)
			}
			if p.Host != tt.wantHost || p.Port != tt.wantPort {
				t.Errorf("host:port = %s:%s, want %s:%s", p.Host, p.Port, tt.wantHost, tt.wantPort)
			}
			if p.URL() == "" {
				t.Error("URL() is empty for a real proxy")
			}
		})
	}
}

// TestParseLine_NearMissesAreStillRejected: only the exact spellings count.
func TestParseLine_NearMissesAreStillRejected(t *testing.T) {
	for _, line := range []string{
		"localhost:8080",
		"localhost:localhost",
		"localhost:localhost:localhost",
		"directly",
		"direct:direct:direct:direct",
		"notlocalhost",
	} {
		t.Run(line, func(t *testing.T) {
			p, ok := ParseLine(line)
			if ok && p.Direct {
				t.Errorf("ParseLine(%q) was treated as direct", line)
			}
		})
	}
}

func TestAddr(t *testing.T) {
	tests := []struct{ name, line, want string }{
		{"real proxy", "1.2.3.4:8080:admin:secret", "1.2.3.4:8080"},
		{"real proxy, @ form", "admin:secret@gw.example.com:9000", "gw.example.com:9000"},
		{"direct", "direct", "direct"},
		{"localhost", "localhost", "localhost"},
		{"four-field localhost", "localhost:localhost:localhost:localhost", "localhost:localhost:localhost:localhost"},
		{"local proxy is still an address", "localhost:8080:user:pass", "localhost:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := ParseLine(tt.line)
			if !ok {
				t.Fatalf("ParseLine(%q) rejected the line", tt.line)
			}
			if got := p.Addr(); got != tt.want {
				t.Errorf("Addr() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAddr_CarriesNoCredential is the property the monitors depend on: Addr is
// what reaches a Discord embed and results/monitor.log, and neither is a place
// for a password.
func TestAddr_CarriesNoCredential(t *testing.T) {
	for _, line := range []string{
		"1.2.3.4:8080:admin:hunter2",
		"admin:hunter2@gw.example.com:9000",
		"http://admin:hunter2@gw.example.com:9000",
	} {
		t.Run(line, func(t *testing.T) {
			p, ok := ParseLine(line)
			if !ok {
				t.Fatalf("ParseLine(%q) rejected the line", line)
			}
			if strings.Contains(p.Addr(), "hunter2") || strings.Contains(p.Addr(), "admin") {
				t.Errorf("Addr() = %q, which leaks a credential into logs and webhooks", p.Addr())
			}
		})
	}
}
