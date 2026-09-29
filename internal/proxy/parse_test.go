package proxy

import "testing"

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
