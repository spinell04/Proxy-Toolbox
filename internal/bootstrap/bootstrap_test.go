package bootstrap

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"proxytoolbox/internal/basedir"
	"proxytoolbox/internal/config"
)

// withRoot points basedir at a temp dir for the duration of one test.
func withRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := basedir.Root
	basedir.Root = dir
	t.Cleanup(func() { basedir.Root = old })
	return dir
}

func TestEnsure_CreatesEverythingInAnEmptyFolder(t *testing.T) {
	dir := withRoot(t)

	created, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure() returned %v, want nil", err)
	}

	want := []string{"proxyfiles/", "results/", ConfigFile}
	if len(created) != len(want) {
		t.Fatalf("created %v, want %v", created, want)
	}
	for i := range want {
		if created[i] != want[i] {
			t.Errorf("created[%d] = %q, want %q", i, created[i], want[i])
		}
	}

	for _, d := range Dirs {
		info, err := os.Stat(filepath.Join(dir, d))
		if err != nil {
			t.Errorf("%s/ was not created: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s exists but is not a directory", d)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ConfigFile)); err != nil {
		t.Errorf("%s was not created: %v", ConfigFile, err)
	}
}

// TestEnsure_NeverOverwritesAnExistingConfig is the one that matters: a user's
// settings, including their webhook, must survive every later run.
func TestEnsure_NeverOverwritesAnExistingConfig(t *testing.T) {
	dir := withRoot(t)
	path := filepath.Join(dir, ConfigFile)

	const mine = "workers=7\ndiscord_webhook=https://example.invalid/hook\n"
	if err := os.WriteFile(path, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	created, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure() returned %v, want nil", err)
	}
	for _, c := range created {
		if c == ConfigFile {
			t.Error("Ensure reported creating a config that already existed")
		}
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != mine {
		t.Errorf("config was rewritten:\ngot  %q\nwant %q", got, mine)
	}
}

func TestEnsure_IsIdempotent(t *testing.T) {
	withRoot(t)

	if _, err := Ensure(); err != nil {
		t.Fatalf("first Ensure() returned %v", err)
	}
	created, err := Ensure()
	if err != nil {
		t.Fatalf("second Ensure() returned %v", err)
	}
	if len(created) != 0 {
		t.Errorf("second run created %v, want nothing", created)
	}
}

func TestEnsure_ReportsOnlyWhatWasMissing(t *testing.T) {
	dir := withRoot(t)
	if err := os.MkdirAll(filepath.Join(dir, "proxyfiles"), 0o755); err != nil {
		t.Fatal(err)
	}

	created, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure() returned %v", err)
	}
	for _, c := range created {
		if c == "proxyfiles/" {
			t.Error("reported creating proxyfiles/, which already existed")
		}
	}
	if len(created) != 2 {
		t.Errorf("created %v, want results/ and %s only", created, ConfigFile)
	}
}

// TestDefaultConfig_CoversEveryKeyTheParserKnows guards the drift this design
// invites: the template is a second file, so a key added to the parser without
// a line here would be invisible to anyone who never reads the docs.
func TestDefaultConfig_CoversEveryKeyTheParserKnows(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "config", "config.go"))
	if err != nil {
		t.Fatalf("reading the parser: %v", err)
	}

	// Every key the parser switches on, taken from the source rather than a
	// hand-kept list — a hand-kept list drifts the same way the template would.
	keys := regexp.MustCompile(`case "([a-z_]+)":`).FindAllStringSubmatch(string(src), -1)
	if len(keys) < 5 {
		t.Fatalf("found %d config keys in the parser, expected the full set — has the switch changed shape?", len(keys))
	}

	tmpl := string(defaultConfig)
	for _, m := range keys {
		key := m[1]
		if !strings.Contains(tmpl, "\n"+key+"=") {
			t.Errorf("config.default.txt has no %q= line, so a new install cannot discover that setting", key)
		}
	}
}

// TestDefaultConfig_MatchesTheBuiltInDefaults keeps the template and the Go
// constants from drifting into two different "defaults".
//
// They can disagree silently: the template is what a new install receives, the
// constants are what runs when config.txt is absent. `workers` was 100 in the
// template and 20 in Go, so the same binary ran at five times the concurrency
// depending only on whether a file existed beside it.
func TestDefaultConfig_MatchesTheBuiltInDefaults(t *testing.T) {
	want := map[string]string{
		"workers":                strconv.Itoa(config.DefaultWorkers),
		"discord_down_threshold": strconv.Itoa(config.DefaultDownThreshold),
		"discord_up_threshold":   strconv.Itoa(config.DefaultUpThreshold),
		"monitor_interval_ms":    strconv.Itoa(config.DefaultMonitorIntervalMs),
		"session_interval_ms":    strconv.Itoa(config.DefaultSessionIntervalMs),
		"ip_mode":                config.DefaultIPMode,

		// Spelled the way the template spells it, not as a Go bool: the
		// template says "on" because that is what a person reads, and this
		// test asserts what the file actually contains. It therefore guards
		// the direction `workers` drifted — someone changing the shipped
		// value without changing Go — and not a flip of the constant itself.
		"auto_update": "on",
		// Spelled as the template spells it; see the auto_update note above.
		"measure_dns": "off",
	}

	assigned := regexp.MustCompile(`(?m)^([a-z_]+)=(.*)$`)
	got := map[string]string{}
	for _, m := range assigned.FindAllStringSubmatch(string(defaultConfig), -1) {
		got[m[1]] = m[2]
	}

	for key, w := range want {
		g, ok := got[key]
		if !ok {
			t.Errorf("config.default.txt has no %q line", key)
			continue
		}
		if g != w {
			t.Errorf("config.default.txt has %s=%s but config.Default* says %s.\n"+
				"  The template and the constants are two different defaults for the\n"+
				"  same setting; whichever is right, make them agree.", key, g, w)
		}
	}
}

// TestDefaultConfig_ShipsNoCredential: the template is compiled into every
// binary handed to someone else. The repository's own config.txt holds a live
// Discord webhook, and embedding that file instead of this one would publish it.
func TestDefaultConfig_ShipsNoCredential(t *testing.T) {
	tmpl := string(defaultConfig)

	if !strings.Contains(tmpl, "\ndiscord_webhook=\n") {
		t.Error("discord_webhook must ship empty")
	}
	// Any key assigned a URL. The `domain` comment legitimately shows
	// http://google.com as a format example, so this looks at assignments
	// only, not at prose.
	assigned := regexp.MustCompile(`(?m)^([a-z_]+)=(.+)$`)
	for _, m := range assigned.FindAllStringSubmatch(tmpl, -1) {
		key, val := m[1], m[2]
		if key == "domain" {
			continue // a hostname, and the only key that legitimately carries one
		}
		if strings.Contains(val, "://") || strings.Contains(val, "webhooks/") {
			t.Errorf("%s ships the value %q — the template must carry no endpoint or credential", key, val)
		}
	}
}
