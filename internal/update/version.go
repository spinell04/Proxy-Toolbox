// Package update installs newer releases of the toolbox over the running
// binary.
//
// The binary is distributed as a single file dropped into a folder, with no
// installer and no package manager, so there is nothing else that could keep
// it current. Updating is automatic and unattended, which sets the standard
// everything here is written to: a failure must leave the existing binary
// working, and must never stop the toolbox from starting.
package update

import (
	"strconv"
	"strings"
)

// devVersion is the value of main.version in any build that did not go through
// the release workflow: `go run .`, `go build`, or an IDE.
const devVersion = "dev"

// parseVersion splits a "v1.2.3" tag into its three components.
//
// Strict on purpose. Anything that is not exactly three non-negative integers
// — a pre-release suffix, build metadata, a two- or four-part version — fails
// rather than being coerced, because ok=false means "do not update" and that
// is the only safe default for an unattended binary replacement. A guess in
// either direction is worse than doing nothing: too low skips a real release,
// too high installs an older build over a newer one.
func parseVersion(s string) (maj, min, patch int, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}

	out := make([]int, 3)
	for i, p := range parts {
		// strconv.Atoi accepts a leading sign, so "-2" would parse to a
		// negative component and compare as older than everything. Reject the
		// sign rather than the result, so "+2" is refused too.
		if p == "" || p[0] == '-' || p[0] == '+' {
			return 0, 0, 0, false
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, 0, 0, false
		}
		out[i] = n
	}
	return out[0], out[1], out[2], true
}

// IsRelease reports whether a version string came from the release workflow.
//
// A dev build must never update itself. Without this check, `go run .` would
// download the published release over the developer's own working copy —
// precisely when that is least wanted, and with no obvious cause.
func IsRelease(version string) bool {
	if version == devVersion {
		return false
	}
	_, _, _, ok := parseVersion(version)
	return ok
}

// Newer reports whether remote is strictly newer than local.
//
// Numeric component comparison, not string comparison: lexically
// "v1.10.0" < "v1.9.0", so comparing as strings would hide every release after
// the ninth minor version.
func Newer(local, remote string) bool {
	lMaj, lMin, lPatch, lOK := parseVersion(local)
	rMaj, rMin, rPatch, rOK := parseVersion(remote)
	if !lOK || !rOK {
		return false
	}

	switch {
	case rMaj != lMaj:
		return rMaj > lMaj
	case rMin != lMin:
		return rMin > lMin
	default:
		return rPatch > lPatch
	}
}
