package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"proxytoolbox/internal/update"
)

func TestRun_ReportsAnExistingInstall(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "proxytoolbox"), []byte("installed"), 0o755); err != nil {
		t.Fatal(err)
	}

	var reached bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	defer srv.Close()

	var out bytes.Buffer
	code := run(options{
		dir:    dir,
		asset:  "proxytoolbox",
		client: update.Client{HTTP: srv.Client(), APIBase: srv.URL},
		out:    &out,
	})

	if code != 0 {
		t.Errorf("exit code = %d, want 0 — an existing install is not an error", code)
	}
	if reached {
		t.Error("contacted GitHub although a toolbox was already installed")
	}
	if got := out.String(); !strings.Contains(got, "already installed") {
		t.Errorf("output did not mention an existing install:\n%s", got)
	}
}

func TestRun_InstallsWhenTheFolderIsEmpty(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("the toolbox binary")

	srv := releaseServer(t, "v1.2.3", "proxytoolbox", payload)
	defer srv.Close()

	var out bytes.Buffer
	code := run(options{
		dir:    dir,
		asset:  "proxytoolbox",
		client: update.Client{HTTP: srv.Client(), APIBase: srv.URL},
		out:    &out,
	})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0. Output:\n%s", code, out.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, "proxytoolbox"))
	if err != nil {
		t.Fatalf("binary was not installed: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("installed %q, want %q", got, payload)
	}
	if s := out.String(); !strings.Contains(s, "v1.2.3") || !strings.Contains(s, dir) {
		t.Errorf("output should name the version and the directory:\n%s", s)
	}
}

func TestRun_FailsLoudlyAndLeavesNothing(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantOut string
	}{
		{
			name: "no releases published yet",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			},
			wantOut: "404",
		},
		{
			name: "release has no SHA256SUMS",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/releases/latest") {
					fmt.Fprintf(w, `{"tag_name":"v1.0.0","assets":[{"name":"proxytoolbox","browser_download_url":"http://%s/x"}]}`, r.Host)
					return
				}
				w.Write([]byte("unverifiable"))
			},
			wantOut: "SHA256SUMS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			var out bytes.Buffer
			code := run(options{
				dir:    dir,
				asset:  "proxytoolbox",
				client: update.Client{HTTP: srv.Client(), APIBase: srv.URL},
				out:    &out,
			})

			if code == 0 {
				t.Errorf("exit code = 0, want non-zero. Output:\n%s", out.String())
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("output does not mention %q:\n%s", tt.wantOut, out.String())
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("directory is not empty after a failed install")
			}
		})
	}
}

// TestRun_RefusesAnUnsupportedPlatform: AssetName returns "" where no binary is
// published, and the installer must say so rather than reaching the network.
func TestRun_RefusesAnUnsupportedPlatform(t *testing.T) {
	var reached bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	defer srv.Close()

	var out bytes.Buffer
	code := run(options{
		dir:    t.TempDir(),
		asset:  "",
		client: update.Client{HTTP: srv.Client(), APIBase: srv.URL},
		out:    &out,
	})

	if code == 0 {
		t.Error("exit code = 0, want non-zero on an unsupported platform")
	}
	if reached {
		t.Error("contacted GitHub on a platform with no published binary")
	}
}

// releaseServer serves a release whose asset and SHA256SUMS are consistent.
func releaseServer(t *testing.T, tag, asset string, payload []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprintf(w, `{"tag_name":%q,"assets":[`+
				`{"name":%q,"browser_download_url":"%s/dl/%s"},`+
				`{"name":"SHA256SUMS","browser_download_url":"%s/dl/SHA256SUMS"}]}`,
				tag, asset, base, asset, base)
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS"):
			sum := sha256.Sum256(payload)
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), asset)
		default:
			w.Write(payload)
		}
	}))
}
