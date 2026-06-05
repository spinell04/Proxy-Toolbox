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
