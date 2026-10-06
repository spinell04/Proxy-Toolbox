package config

import (
	"os"
	"path/filepath"
	"testing"

	"proxytoolbox/internal/basedir"
)

func TestParseLatency(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"1500", 1500},
		{"", 0},
		{"abc", 0},
		{"0", 0},
		{"-5", 0},
	}
	for _, tt := range tests {
		if got := parseLatency(tt.in); got != tt.want {
			t.Errorf("parseLatency(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// loadFrom writes line as the whole of a config.txt in a temp dir and parses
// it. Load resolves its path through basedir, which is a package variable, so
// pointing that at a temp dir is the only way to reach the parser from outside.
func loadFrom(t *testing.T, line string) Config {
	t.Helper()
	dir := t.TempDir()
	old := basedir.Root
	basedir.Root = dir
	t.Cleanup(func() { basedir.Root = old })

	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("writing %s: %v", fileName, err)
	}
	return Load()
}

func TestLoad_AutoUpdate(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "on", line: "auto_update=on", want: true},
		{name: "off", line: "auto_update=off", want: false},
		{name: "true", line: "auto_update=true", want: true},
		{name: "false", line: "auto_update=false", want: false},
		{name: "yes", line: "auto_update=yes", want: true},
		{name: "no", line: "auto_update=no", want: false},
		{name: "1", line: "auto_update=1", want: true},
		{name: "0", line: "auto_update=0", want: false},
		{name: "uppercase OFF", line: "auto_update=OFF", want: false},
		{name: "padded", line: "auto_update =  off  ", want: false},

		// Absent or unintelligible falls back to the default, which is on.
		// Defaulting off on a typo would silently strand a user on an old
		// build with no indication anything was wrong.
		{name: "absent", line: "", want: true},
		{name: "blank value", line: "auto_update=", want: true},
		{name: "gibberish", line: "auto_update=maybe", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadFrom(t, tt.line)
			if cfg.AutoUpdate != tt.want {
				t.Errorf("AutoUpdate = %v, want %v for %q", cfg.AutoUpdate, tt.want, tt.line)
			}
		})
	}
}

// autoUpdateFrom points basedir at a temp dir, optionally writes a config.txt
// there, and calls AutoUpdateEnabled. Shaped after loadFrom — same reason: the
// path is resolved through a package variable, so redirecting it is the only
// way in — but it has to be able to leave the file out, which is the case the
// function exists for.
func autoUpdateFrom(t *testing.T, contents string, writeFile bool) bool {
	t.Helper()
	dir := t.TempDir()
	old := basedir.Root
	basedir.Root = dir
	t.Cleanup(func() { basedir.Root = old })

	if writeFile {
		if err := os.WriteFile(filepath.Join(dir, fileName), []byte(contents), 0o644); err != nil {
			t.Fatalf("writing %s: %v", fileName, err)
		}
	}
	return AutoUpdateEnabled()
}

func TestAutoUpdateEnabled(t *testing.T) {
	tests := []struct {
		name      string
		contents  string
		writeFile bool
		want      bool
	}{
		{name: "on", contents: "auto_update=on\n", writeFile: true, want: true},
		{name: "off", contents: "auto_update=off\n", writeFile: true, want: false},
		{name: "uppercase OFF", contents: "auto_update=OFF\n", writeFile: true, want: false},
		{name: "padded", contents: "auto_update =  off  \n", writeFile: true, want: false},
		{name: "off among other keys", contents: "workers=50\n# a comment\nauto_update=no\ndomain=example.com\n", writeFile: true, want: false},

		// Anything unintelligible takes the default rather than reading as
		// false: a typo must not silently strand a user on an old build.
		{name: "gibberish", contents: "auto_update=maybe\n", writeFile: true, want: true},
		{name: "blank value", contents: "auto_update=\n", writeFile: true, want: true},
		{name: "key absent", contents: "workers=50\n", writeFile: true, want: true},
		{name: "empty file", contents: "", writeFile: true, want: true},
		{name: "commented out", contents: "#auto_update=off\n", writeFile: true, want: true},

		// The case this function exists for: a first ever run, before
		// bootstrap has created config.txt.
		{name: "file absent entirely", writeFile: false, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := autoUpdateFrom(t, tt.contents, tt.writeFile); got != tt.want {
				t.Errorf("AutoUpdateEnabled() = %v, want %v for %q", got, tt.want, tt.contents)
			}
		})
	}
}

// TestAutoUpdateEnabled_UnreadableFile covers the other half of "any failure
// takes the default": the file exists but cannot be opened.
func TestAutoUpdateEnabled_Unreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: mode 0o000 is still readable")
	}
	dir := t.TempDir()
	old := basedir.Root
	basedir.Root = dir
	t.Cleanup(func() { basedir.Root = old })

	path := filepath.Join(dir, fileName)
	// auto_update=off, so a successful read would return false and the
	// default is the only way this can come back true.
	if err := os.WriteFile(path, []byte("auto_update=off\n"), 0o000); err != nil {
		t.Fatalf("writing %s: %v", fileName, err)
	}
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("the filesystem ignores mode 0o000, so unreadable cannot be arranged here")
	}

	if got := AutoUpdateEnabled(); got != DefaultAutoUpdate {
		t.Errorf("AutoUpdateEnabled() = %v on an unreadable file, want the default %v", got, DefaultAutoUpdate)
	}
}

// TestAutoUpdateEnabled_AgreesWithLoad is the guard against the two parsers
// drifting apart. They share parseBool today; this fails if anyone gives
// either one its own interpretation of the key.
func TestAutoUpdateEnabled_AgreesWithLoad(t *testing.T) {
	lines := []string{
		"auto_update=on", "auto_update=off", "auto_update=true", "auto_update=false",
		"auto_update=yes", "auto_update=no", "auto_update=1", "auto_update=0",
		"auto_update=OFF", "auto_update =  off  ", "auto_update=maybe",
		"auto_update=", "workers=50",
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			want := loadFrom(t, line).AutoUpdate
			if got := autoUpdateFrom(t, line+"\n", true); got != want {
				t.Errorf("AutoUpdateEnabled() = %v but Load().AutoUpdate = %v for %q", got, want, line)
			}
		})
	}
}

// TestLoad_MeasureDNS covers the key and, in the "absent" case, the built-in
// default itself. The template parity test cannot: it compares the shipped file
// against a hand-written literal, so flipping DefaultMeasureDNS leaves it
// passing. This is what makes the default a tested fact.
func TestLoad_MeasureDNS(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "on", line: "measure_dns=on", want: true},
		{name: "off", line: "measure_dns=off", want: false},
		{name: "true", line: "measure_dns=true", want: true},
		{name: "no", line: "measure_dns=no", want: false},
		{name: "uppercase ON", line: "measure_dns=ON", want: true},
		{name: "padded", line: "measure_dns =  on  ", want: true},

		// The default: excluded, so the number matches what `ping` reports and
		// so one check is not charged for a lookup the other ninety-nine avoid.
		{name: "absent", line: "", want: false},
		{name: "blank value", line: "measure_dns=", want: false},
		{name: "gibberish", line: "measure_dns=sometimes", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadFrom(t, tt.line)
			if cfg.MeasureDNS != tt.want {
				t.Errorf("MeasureDNS = %v, want %v for %q", cfg.MeasureDNS, tt.want, tt.line)
			}
		})
	}
}
