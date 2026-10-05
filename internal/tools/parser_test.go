package tools

import (
	"strings"
	"testing"

	"proxytoolbox/internal/proxy"
)

// TestParserPassesDirectLinesThrough: converting a file between formats must
// not destroy a direct line. Every formatter builds from Host/Port/User/Pass,
// all empty for a direct entry, so without the guard a conversion emits ":::"
// and the user's baseline is silently gone.
func TestParserPassesDirectLinesThrough(t *testing.T) {
	direct, ok := proxy.ParseLine("direct")
	if !ok || !direct.Direct {
		t.Fatal(`ParseLine("direct") did not produce a direct proxy`)
	}
	realProxy, ok := proxy.ParseLine("1.2.3.4:8080:admin:secret")
	if !ok {
		t.Fatal("ParseLine rejected a real proxy")
	}

	for _, f := range proxyFormats {
		t.Run(f.Name, func(t *testing.T) {
			// The real conversion, not a copy of it. An earlier version of
			// this test inlined the guard here, so deleting the guard from
			// parser.go changed nothing the test could see and the mutant
			// survived.
			got := convertLines([]proxy.Proxy{realProxy, direct}, f.Format)

			if got[1] != "direct" {
				t.Errorf("direct line became %q, want it unchanged", got[1])
			}
			if strings.Contains(got[1], "::") {
				t.Errorf("direct line was run through a formatter: %q", got[1])
			}
			if got[0] == "" || strings.HasPrefix(got[0], ":") {
				t.Errorf("real proxy converted to %q", got[0])
			}
		})
	}
}

// TestParserFormattersWouldMangleADirectLine documents why the guard exists.
// It asserts the thing the guard prevents, so the cost of removing the guard is
// visible in the test file rather than only in a user's converted file.
func TestParserFormattersWouldMangleADirectLine(t *testing.T) {
	direct, _ := proxy.ParseLine("direct")

	for _, f := range proxyFormats {
		t.Run(f.Name, func(t *testing.T) {
			unguarded := f.Format(direct)

			// Degeneracy, stated as "carries none of the line's characters"
			// rather than as a literal: the formats differ ("::" vs ":@:" vs
			// "http://:@:"), and an earlier version of this test asserted on
			// "::" and failed on the two @ forms for that reason.
			if strings.ContainsFunc(unguarded, func(r rune) bool {
				return r != ':' && r != '@' && r != '/' && r != 'h' && r != 't' && r != 'p'
			}) {
				t.Errorf("formatter produced %q, which carries real content; this "+
					"test's premise — that an unguarded direct line is mangled — "+
					"no longer holds, so the guard in RunParser may need revisiting",
					unguarded)
			}
			if strings.Contains(unguarded, "direct") {
				t.Errorf("formatter produced %q; it preserved the line, so the guard may be redundant", unguarded)
			}
		})
	}
}
