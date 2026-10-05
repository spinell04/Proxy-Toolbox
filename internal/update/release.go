package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// repo is the only place the project's GitHub identity appears.
	repo = "spinell04/Proxy-Toolbox"

	// defaultAPIBase is overridden in tests by an httptest server URL.
	defaultAPIBase = "https://api.github.com"

	// sumsAsset carries the SHA-256 of every binary in the release. A release
	// without it is not installable — see verify in swap.go.
	sumsAsset = "SHA256SUMS"

	// checkTimeout bounds the startup delay. This runs before the first menu
	// draws, so it is a budget for someone else's availability: a GitHub
	// outage, or a captive portal that swallows connections, must not keep the
	// toolbox from starting.
	checkTimeout = 3 * time.Second

	// downloadTimeout covers ~13 MB over a bad connection.
	downloadTimeout = 5 * time.Minute

	// maxJSONBytes caps the API response. The real one is a few KB; this
	// bounds the damage if something upstream serves an unbounded stream.
	maxJSONBytes = 1 << 20
)

// Release is the part of a GitHub release this package uses.
type Release struct {
	Tag    string
	Assets map[string]string // asset name → browser download URL
}

// Client talks to the GitHub releases API.
//
// HTTP and APIBase are fields rather than package state so tests can point the
// whole thing at an httptest server. Nothing here needs authentication: the
// repository is public, and an unauthenticated client is one less credential
// to ship inside a binary handed to someone else.
type Client struct {
	HTTP    *http.Client
	APIBase string
}

// NewClient returns a Client with the production timeout and endpoint.
func NewClient() Client {
	return Client{
		HTTP:    &http.Client{Timeout: checkTimeout},
		APIBase: defaultAPIBase,
	}
}

func (c Client) base() string {
	if c.APIBase == "" {
		return defaultAPIBase
	}
	return c.APIBase
}

// Latest returns the newest published release.
//
// GitHub's /releases/latest already excludes drafts and pre-releases, which is
// what makes a future `beta` branch publishing pre-releases safe to add
// without touching the client.
func (c Client) Latest(ctx context.Context) (Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.base(), repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("querying %s: %w", repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// A 403 with the rate-limit header is the one failure worth naming
		// distinctly: unauthenticated GitHub allows 60 requests an hour per IP,
		// and on a shared address that ceiling is reachable without the user
		// doing anything wrong. Saying "rate limit" stops it reading as a
		// permissions problem with a public repository.
		if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return Release{}, fmt.Errorf("GitHub API rate limit reached, no update check this run")
		}
		return Release{}, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("decoding the release: %w", err)
	}
	if payload.TagName == "" {
		return Release{}, fmt.Errorf("release has no tag")
	}

	rel := Release{Tag: payload.TagName, Assets: make(map[string]string, len(payload.Assets))}
	for _, a := range payload.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}
