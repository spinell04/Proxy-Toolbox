package dashboard

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// secretMarker identifies a CSV planted outside the results directory. It must
// never appear in a response body: the exported CSVs hold proxy credentials in
// plaintext, so a traversal that reads one is a credential leak.
const secretMarker = "topsecret"

// TestHandleRun_RejectsPathTraversal is the security test for the file
// parameter. Every escaping form must fail, and the planted CSV one directory
// above the results directory must never appear in a body. The final case is
// the positive control: without it, a loadRun that rejected everything would
// pass this test.
func TestHandleRun_RejectsPathTraversal(t *testing.T) {
	dir := fixtureDir(t)
	h := newAPI(dir)

	cases := []struct {
		name     string
		query    string
		wantCode int
	}{
		{"parent directory", "file=../secret.csv", http.StatusNotFound},
		{"deep traversal", "file=../../../etc/passwd", http.StatusNotFound},
		{"absolute path", "file=/etc/passwd", http.StatusNotFound},
		{"absolute path to secret", "file=" + filepath.Join(filepath.Dir(dir), "secret.csv"), http.StatusNotFound},
		{"url encoded separator", "file=..%2Fsecret.csv", http.StatusNotFound},
		{"double encoded separator", "file=..%252Fsecret.csv", http.StatusNotFound},
		{"backslash separator", "file=..\\secret.csv", http.StatusNotFound},
		{"dot", "file=.", http.StatusNotFound},
		{"dotdot", "file=..", http.StatusNotFound},
		{"separator", "file=/", http.StatusNotFound},
		{"empty", "file=", http.StatusNotFound},
		{"absent", "", http.StatusNotFound},
		{"legitimate file", "file=pinger_run.csv", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/api/run?"+tc.query)

			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if strings.Contains(rec.Body.String(), secretMarker) {
				t.Errorf("response leaked a file outside the results directory: %s", rec.Body.String())
			}
		})
	}
}

func TestHandleCompare_RejectsPathTraversal(t *testing.T) {
	dir := fixtureDir(t)
	h := newAPI(dir)

	for _, query := range []string{
		"file=pinger_run.csv&file=../secret.csv",
		"file=../secret.csv",
		"file=" + filepath.Join(filepath.Dir(dir), "secret.csv"),
	} {
		t.Run(query, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/api/compare?"+query)

			if rec.Code == http.StatusOK {
				t.Errorf("status = 200, want an error — the selection escapes the results directory")
			}
			if strings.Contains(rec.Body.String(), secretMarker) {
				t.Errorf("response leaked a file outside the results directory: %s", rec.Body.String())
			}
		})
	}
}

// TestAPI_RejectsNonGET is the read-only security test: every endpoint must
// refuse every mutating method, and nothing on disk may change.
func TestAPI_RejectsNonGET(t *testing.T) {
	dir := fixtureDir(t)
	h := newAPI(dir)
	before := dirSnapshot(t, dir)

	endpoints := []string{
		"/api/runs",
		"/api/run?file=pinger_run.csv",
		"/api/compare?file=pinger_run.csv",
	}
	methods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodHead,
		http.MethodOptions,
	}

	for _, endpoint := range endpoints {
		for _, method := range methods {
			t.Run(method+" "+endpoint, func(t *testing.T) {
				rec := do(t, h, method, endpoint)

				if rec.Code != http.StatusMethodNotAllowed {
					t.Errorf("status = %d, want 405 — the dashboard is read-only", rec.Code)
				}
				if strings.Contains(rec.Body.String(), "pinger") {
					t.Errorf("a rejected request still returned run data: %s", rec.Body.String())
				}
			})
		}
	}

	if after := dirSnapshot(t, dir); after != before {
		t.Errorf("results directory changed: %q -> %q", before, after)
	}
}

func TestAPI_GETIsAllowed(t *testing.T) {
	h := newAPI(fixtureDir(t))

	for _, endpoint := range []string{
		"/api/runs",
		"/api/run?file=pinger_run.csv",
		"/api/compare?file=pinger_run.csv",
	} {
		t.Run(endpoint, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, endpoint)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			if nosniff := rec.Header().Get("X-Content-Type-Options"); nosniff != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", nosniff)
			}
		})
	}
}

// dirSnapshot renders a directory's names, sizes and contents as one string, so
// any write, delete or truncation shows up as a difference.
func dirSnapshot(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, e := range entries {
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(e.Name())
		b.WriteString("=")
		b.Write(content)
		b.WriteString("\n")
	}
	return b.String()
}
