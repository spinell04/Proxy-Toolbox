package update

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name            string
		in              string
		maj, min, patch int
		ok              bool
	}{
		{name: "with v prefix", in: "v1.2.3", maj: 1, min: 2, patch: 3, ok: true},
		{name: "without v prefix", in: "1.2.3", maj: 1, min: 2, patch: 3, ok: true},
		{name: "zeros", in: "v0.0.0", maj: 0, min: 0, patch: 0, ok: true},
		{name: "multi-digit", in: "v12.34.56", maj: 12, min: 34, patch: 56, ok: true},
		{name: "surrounding space", in: "  v1.2.3\n", maj: 1, min: 2, patch: 3, ok: true},

		// Accepted, deliberately. Atoi strips leading zeros, and the release
		// workflow never produces them, so tightening this would add a rejection
		// path for input that cannot occur. Pinned so the choice is visible.
		{name: "leading zeros", in: "v01.02.03", maj: 1, min: 2, patch: 3, ok: true},

		// Everything below must fail rather than guess. An unparseable version
		// means "do not update", which is the safe direction: a wrong guess
		// either skips a real release or installs over a newer build.
		{name: "dev", in: "dev"},
		{name: "empty", in: ""},
		{name: "two components", in: "v1.2"},
		{name: "four components", in: "v1.2.3.4"},
		{name: "pre-release suffix", in: "v1.2.3-beta"},
		{name: "build metadata", in: "v1.2.3+build7"},
		{name: "non-numeric", in: "v1.x.3"},
		{name: "negative", in: "v1.-2.3"},

		// Three components, one of them empty. These are the inputs where the
		// p == "" guard in parseVersion is the only thing standing between a
		// malformed version and p[0] on a zero-length string. Without them a
		// future edit that reorders that condition turns "refuse to update"
		// into a panic at startup, and the suite stays green.
		{name: "empty middle component", in: "v1..3"},
		{name: "empty leading component", in: "v.2.3"},
		{name: "empty trailing component", in: "v1.2."},
		{name: "all components empty", in: "v.."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maj, min, patch, ok := parseVersion(tt.in)
			if ok != tt.ok {
				t.Fatalf("parseVersion(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			}
			if !tt.ok {
				return
			}
			if maj != tt.maj || min != tt.min || patch != tt.patch {
				t.Errorf("parseVersion(%q) = %d.%d.%d, want %d.%d.%d",
					tt.in, maj, min, patch, tt.maj, tt.min, tt.patch)
			}
		})
	}
}

// TestNewer_TreatsTenAsGreaterThanNine is the test this package exists for.
// Lexically "v1.10.0" < "v1.9.0", so a string comparison would report that
// 1.10.0 is older than 1.9.0 and never offer the upgrade.
func TestNewer_TreatsTenAsGreaterThanNine(t *testing.T) {
	if !Newer("v1.9.0", "v1.10.0") {
		t.Error("Newer(v1.9.0, v1.10.0) = false, want true — this is the lexical-comparison bug")
	}
	if Newer("v1.10.0", "v1.9.0") {
		t.Error("Newer(v1.10.0, v1.9.0) = true, want false")
	}
}

func TestNewer(t *testing.T) {
	tests := []struct {
		name          string
		local, remote string
		want          bool
	}{
		{name: "patch ahead", local: "v1.2.3", remote: "v1.2.4", want: true},
		{name: "minor ahead", local: "v1.2.9", remote: "v1.3.0", want: true},
		{name: "major ahead", local: "v1.9.9", remote: "v2.0.0", want: true},
		{name: "identical", local: "v1.2.3", remote: "v1.2.3", want: false},
		{name: "remote behind on patch", local: "v1.2.4", remote: "v1.2.3", want: false},
		{name: "remote behind on minor", local: "v1.3.0", remote: "v1.2.9", want: false},
		{name: "remote behind on major", local: "v2.0.0", remote: "v1.9.9", want: false},

		// Major dominates minor dominates patch. A big patch number must not
		// outrank a minor bump.
		{name: "patch does not outrank minor", local: "v1.2.99", remote: "v1.3.0", want: true},
		{name: "minor does not outrank major", local: "v1.99.0", remote: "v2.0.0", want: true},

		// Unparseable on either side means no update.
		{name: "local is dev", local: "dev", remote: "v1.0.0", want: false},
		{name: "remote unparseable", local: "v1.0.0", remote: "nightly", want: false},
		{name: "both unparseable", local: "dev", remote: "dev", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Newer(tt.local, tt.remote); got != tt.want {
				t.Errorf("Newer(%q, %q) = %v, want %v", tt.local, tt.remote, got, tt.want)
			}
		})
	}
}

func TestIsRelease(t *testing.T) {
	if IsRelease("dev") {
		t.Error(`IsRelease("dev") = true, want false — a dev build must never update itself`)
	}
	if !IsRelease("v1.0.0") {
		t.Error(`IsRelease("v1.0.0") = false, want true`)
	}
}
