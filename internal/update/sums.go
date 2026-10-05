package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// hexDigestLen is the length of a SHA-256 digest in hex.
const hexDigestLen = 64

// parseSums reads `sha256sum` output into a name → digest map.
//
// Both output modes are accepted: two spaces for text mode, space-asterisk for
// binary mode. The workflow produces one of them, and nothing should break if
// a release is ever cut by hand with the other.
//
// An empty result is an error, not an empty map. A SHA256SUMS asset that
// parsed to nothing would make every binary "not listed" and silently disable
// updates — a failure that looks like working software.
func parseSums(s string) (map[string]string, error) {
	out := map[string]string{}

	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		digest, name, found := strings.Cut(line, " ")
		if !found {
			return nil, fmt.Errorf("malformed SHA256SUMS line: %q", line)
		}
		if len(digest) != hexDigestLen {
			return nil, fmt.Errorf("malformed digest in SHA256SUMS: %q", digest)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return nil, fmt.Errorf("digest is not hex in SHA256SUMS: %q", digest)
		}

		// Binary mode prefixes the name with '*'; text mode leaves a second
		// space, which TrimSpace removes.
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if name == "" {
			return nil, fmt.Errorf("SHA256SUMS entry has no filename: %q", line)
		}
		out[name] = strings.ToLower(digest)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("SHA256SUMS listed no files")
	}
	return out, nil
}

// verifySHA256 reports whether the file at path hashes to want.
//
// Streamed rather than read whole: the file is ~13 MB and there is no reason
// to hold it in memory on a machine already running a worker pool.
func verifySHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening the download: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hashing the download: %w", err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: downloaded %s, release says %s", got, strings.ToLower(want))
	}
	return nil
}
