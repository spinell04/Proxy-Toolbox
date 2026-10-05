package update

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// TestDecide covers the whole decision tree without any I/O. MaybeUpdate
// itself ends in a process replacement, so this is where the logic is tested.
func TestDecide(t *testing.T) {
	tests := []struct {
		name    string
		version string
		enabled bool
		asset   string
		want    bool
		reason  string
	}{
		{
			name:    "release build, enabled, asset published",
			version: "v1.0.0", enabled: true, asset: "proxytoolbox.exe", want: true,
		},
		{
			name:    "disabled in config",
			version: "v1.0.0", enabled: false, asset: "proxytoolbox.exe", want: false,
			reason: "disabled",
		},
		{
			name:    "dev build",
			version: "dev", enabled: true, asset: "proxytoolbox.exe", want: false,
			reason: "development build",
		},
		{
			name:    "unparseable version",
			version: "nightly-7", enabled: true, asset: "proxytoolbox.exe", want: false,
			reason: "development build",
		},
		{
			name:    "platform has no published asset",
			version: "v1.0.0", enabled: true, asset: "", want: false,
			reason: "no release build",
		},
		{
			name:    "dev build and disabled reports disabled first",
			version: "dev", enabled: false, asset: "proxytoolbox.exe", want: false,
			reason: "disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := decide(tt.version, tt.enabled, tt.asset)
			if got != tt.want {
				t.Errorf("decide(%q, %v, %q) = %v, want %v", tt.version, tt.enabled, tt.asset, got, tt.want)
			}
			if tt.reason != "" && !strings.Contains(reason, tt.reason) {
				t.Errorf("reason = %q, want it to mention %q", reason, tt.reason)
			}
		})
	}
}

// TestMaybeUpdate_CleansUpOldEvenWhenDisabled: the sweep has to happen
// regardless, or turning auto-update off would leave a ~13 MB .old file
// sitting beside the binary forever.
func TestMaybeUpdate_CleansUpOldEvenWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "proxytoolbox")
	os.WriteFile(exe, []byte("binary"), 0o755)
	os.WriteFile(exe+oldSuffix, []byte("previous"), 0o755)

	maybeUpdate(config{
		version: "v1.0.0",
		enabled: false,
		asset:   "proxytoolbox",
		exe:     exe,
		client:  NewClient(),
	})

	if _, err := os.Stat(exe + oldSuffix); !os.IsNotExist(err) {
		t.Error("the .old file survived a run with auto-update disabled")
	}
}

// TestMaybeUpdate_SurvivesAnUnreachableAPI is the "must start on a plane"
// requirement: no panic, no hang, and the binary untouched.
func TestMaybeUpdate_SurvivesAnUnreachableAPI(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "proxytoolbox")
	os.WriteFile(exe, []byte("binary"), 0o755)

	maybeUpdate(config{
		version: "v1.0.0",
		enabled: true,
		asset:   "proxytoolbox",
		exe:     exe,
		// A port nothing listens on: connection refused immediately.
		client: Client{HTTP: &http.Client{}, APIBase: "http://127.0.0.1:1"},
	})

	if got, _ := os.ReadFile(exe); string(got) != "binary" {
		t.Errorf("the binary was modified: %q", got)
	}
}

// countingServer serves a /releases/latest payload for tag and counts every
// other request it receives, so a test can assert that nothing tried to
// install.
//
// The payload lists real asset URLs pointing back at this server. That detail
// is what gives the count meaning: with an empty asset list, install fails on
// the missing asset before making a single request, and a missing version
// guard would be indistinguishable from a correct skip.
func countingServer(tag string) (*httptest.Server, *atomic.Int64) {
	var fetches atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/"+repo+"/releases/latest" {
			// Built from r.Host rather than the server's own URL so the
			// handler closes over nothing the test goroutine writes.
			base := "http://" + r.Host
			fmt.Fprintf(w, `{"tag_name":%q,"assets":[`+
				`{"name":"proxytoolbox","browser_download_url":"%s/dl/proxytoolbox"},`+
				`{"name":%q,"browser_download_url":"%s/dl/%s"}]}`,
				tag, base, sumsAsset, base, sumsAsset)
			return
		}
		fetches.Add(1)
		w.Write([]byte("should not have been fetched"))
	}))
	return srv, &fetches
}

// TestMaybeUpdate_DoesNothingWhenAlreadyCurrent guards against a pointless
// ~13 MB download on every launch.
func TestMaybeUpdate_DoesNothingWhenAlreadyCurrent(t *testing.T) {
	srv, fetches := countingServer("v1.0.0")
	defer srv.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "proxytoolbox")
	os.WriteFile(exe, []byte("binary"), 0o755)

	maybeUpdate(config{
		version: "v1.0.0",
		enabled: true,
		asset:   "proxytoolbox",
		exe:     exe,
		client:  Client{HTTP: &http.Client{}, APIBase: srv.URL},
	})

	if got := fetches.Load(); got != 0 {
		t.Errorf("made %d asset requests while already on the latest version", got)
	}
	if got, _ := os.ReadFile(exe); string(got) != "binary" {
		t.Errorf("the binary was modified: %q", got)
	}
}

// TestMaybeUpdate_DoesNotDowngrade: if the latest release is older than this
// build — a release deleted, or a rollback in progress — nothing happens.
func TestMaybeUpdate_DoesNotDowngrade(t *testing.T) {
	srv, fetches := countingServer("v0.9.0")
	defer srv.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "proxytoolbox")
	os.WriteFile(exe, []byte("binary"), 0o755)

	maybeUpdate(config{
		version: "v1.0.0", enabled: true, asset: "proxytoolbox", exe: exe,
		client: Client{HTTP: &http.Client{}, APIBase: srv.URL},
	})

	if got := fetches.Load(); got != 0 {
		t.Errorf("made %d asset requests for an older release", got)
	}
	if got, _ := os.ReadFile(exe); string(got) != "binary" {
		t.Errorf("the binary was modified: %q", got)
	}
}
