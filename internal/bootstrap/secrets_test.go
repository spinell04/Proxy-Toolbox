package bootstrap

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// credentialPatterns are shapes that must never appear in a tracked file.
// Each is a bearer credential: holding the string is holding the access.
var credentialPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"Discord webhook URL", regexp.MustCompile(`discord(app)?\.com/api/webhooks/\d+/\S+`)},
	{"Slack webhook URL", regexp.MustCompile(`hooks\.slack\.com/services/\S+`)},
	{"Telegram bot token", regexp.MustCompile(`api\.telegram\.org/bot\d+:\S+`)},
}

// proxyCredential matches user:pass@host:port. RE2 has no lookahead, so the
// reserved-domain exemption is applied afterwards in isFixtureHost rather than
// inside the pattern.
//
// Added after a real proxy credential reached a commit as a test fixture: the
// scanner looked only for webhooks, so the very thing it exists to prevent
// walked straight past it.
var proxyCredential = regexp.MustCompile(
	`[A-Za-z0-9][A-Za-z0-9._-]{2,}:[A-Za-z0-9][A-Za-z0-9._~+/-]{7,}@([A-Za-z0-9.-]+):\d{2,5}`)

// isFixtureHost reports whether a host is reserved for documentation and
// testing (RFC 2606 / RFC 6761). Those are what a fixture should use, and a
// credential on one grants nothing.
func isFixtureHost(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	for _, suffix := range []string{
		".invalid", ".example", ".test", ".localhost",
		".example.com", ".example.net", ".example.org",
	} {
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return h == "localhost" || h == "example.com" || h == "example.net" || h == "example.org"
}

// skipExt are files whose bytes are not worth scanning as text. Binaries are
// built artefacts; a credential reaching one came from a source file, and that
// source file is what this test is looking for.
var skipExt = map[string]bool{
	".exe": true, ".png": true, ".jpg": true, ".gif": true,
	".woff": true, ".woff2": true, ".ico": true, ".pdf": true,
}

// TestNoCredentialsInTrackedFiles fails if anything git would commit carries a
// credential.
//
// It asks git for the tracked set rather than walking the tree, because the
// two differ in exactly the case that matters: .gitignore has no effect on a
// file that is already tracked, so a secret added to .gitignore after the fact
// is still committed on every subsequent commit, silently.
func TestNoCredentialsInTrackedFiles(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git unavailable or not a repository, cannot determine the tracked set: %v", err)
	}

	files := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	if len(files) == 0 || files[0] == "" {
		t.Skip("no tracked files reported")
	}

	scanned := 0
	for _, rel := range files {
		if skipExt[strings.ToLower(filepath.Ext(rel))] {
			continue
		}
		// The scanner itself names the patterns it looks for.
		if rel == filepath.ToSlash(filepath.Join("internal", "bootstrap", "secrets_test.go")) {
			continue
		}

		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue // deleted or unreadable; not this test's business
		}
		if len(b) > 4<<20 {
			continue // a built artefact that slipped past skipExt
		}
		scanned++

		for _, m := range proxyCredential.FindAllSubmatchIndex(b, -1) {
			host := string(b[m[2]:m[3]])
			if isFixtureHost(host) {
				continue
			}
			line := 1 + strings.Count(string(b[:m[0]]), "\n")
			t.Errorf("%s:%d contains what looks like a live proxy credential on %s.\n"+
				"  Fixtures must use a reserved host (.invalid, .example, .test) so the\n"+
				"  string grants nothing. This file is TRACKED and a commit is permanent.",
				rel, line, host)
		}

		for _, p := range credentialPatterns {
			if loc := p.re.FindIndex(b); loc != nil {
				line := 1 + strings.Count(string(b[:loc[0]]), "\n")
				t.Errorf("%s:%d contains a %s.\n"+
					"  This file is TRACKED, so it is committed on every commit.\n"+
					"  Adding it to .gitignore does NOT help: .gitignore is ignored for\n"+
					"  already-tracked files. Run: git rm --cached %s\n"+
					"  and rotate the credential — it is already in the history.",
					rel, line, p.name, rel)
			}
		}
	}

	if scanned == 0 {
		t.Error("scanned no files; the tracked-file listing is not working")
	}
}
