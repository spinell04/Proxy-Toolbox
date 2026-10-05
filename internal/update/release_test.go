package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLatest_DecodesTheRealResponseShape(t *testing.T) {
	body, err := os.ReadFile("testdata/release.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	rel, err := testClient(srv.URL).Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest() error: %v", err)
	}

	if want := "/repos/spinell04/Proxy-Toolbox/releases/latest"; gotPath != want {
		t.Errorf("requested %q, want %q", gotPath, want)
	}
	if rel.Tag != "v1.4.2" {
		t.Errorf("Tag = %q, want v1.4.2", rel.Tag)
	}
	if got := len(rel.Assets); got != 4 {
		t.Fatalf("decoded %d assets, want 4", got)
	}
	if got := rel.Assets["proxytoolbox.exe"]; !strings.HasSuffix(got, "/v1.4.2/proxytoolbox.exe") {
		t.Errorf("exe URL = %q, want it to end in /v1.4.2/proxytoolbox.exe", got)
	}
	if rel.Assets[sumsAsset] == "" {
		t.Errorf("no %s asset decoded", sumsAsset)
	}
}

// TestLatest_FailsClosed covers every way the call can go wrong. Each must
// return an error rather than a half-populated Release: the caller's next step
// is replacing a binary, so "probably fine" is not an option.
func TestLatest_FailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{
			name: "no releases yet",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			},
			wantErr: "404",
		},
		{
			name: "rate limited",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-RateLimit-Remaining", "0")
				http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
			},
			wantErr: "rate limit",
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			},
			wantErr: "500",
		},
		{
			name: "malformed json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"tag_name": `))
			},
			wantErr: "decoding",
		},
		{
			name: "html instead of json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`<!DOCTYPE html><html><body>maintenance</body></html>`))
			},
			wantErr: "decoding",
		},
		{
			name: "empty tag",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"tag_name":"","assets":[]}`))
			},
			wantErr: "no tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			_, err := testClient(srv.URL).Latest(context.Background())
			if err == nil {
				t.Fatal("Latest() succeeded, want an error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestLatest_HonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := testClient(srv.URL).Latest(ctx); err == nil {
		t.Fatal("Latest() succeeded against a hanging server, want a timeout error")
	}
}

// testClient points a Client at a test server.
func testClient(base string) Client {
	return Client{HTTP: &http.Client{}, APIBase: base}
}
