package update

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sha256sum output is "<64 hex><two spaces><name>" for text mode and
// "<64 hex><space><asterisk><name>" for binary mode. Both must parse: the
// workflow uses one, and a hand-made release might use the other.
func TestParseSums(t *testing.T) {
	const in = `d2a84f4b8b650937ec8f73cd8be2c74add5a911ba64df27458ed8229da804a26  proxytoolbox.exe
5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8 *proxytoolbox-mac-AppleSiliconCPU

# a comment line
6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b  proxytoolbox-macOS-IntelCPU
`

	got, err := parseSums(in)
	if err != nil {
		t.Fatalf("parseSums() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("parsed %d entries, want 3: %v", len(got), got)
	}
	if h := got["proxytoolbox.exe"]; h != "d2a84f4b8b650937ec8f73cd8be2c74add5a911ba64df27458ed8229da804a26" {
		t.Errorf("exe hash = %q", h)
	}
	if h := got["proxytoolbox-mac-AppleSiliconCPU"]; h != "5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8" {
		t.Errorf("binary-mode entry did not parse: %q", h)
	}
}

func TestParseSums_Rejects(t *testing.T) {
	tests := []struct{ name, in string }{
		{name: "empty", in: ""},
		{name: "only comments", in: "# nothing here\n"},
		{name: "hash too short", in: "abc123  proxytoolbox.exe\n"},
		{name: "not hex", in: strings.Repeat("z", 64) + "  proxytoolbox.exe\n"},
		{name: "no filename", in: strings.Repeat("a", 64) + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseSums(tt.in); err == nil {
				t.Errorf("parseSums(%q) succeeded, want an error", tt.in)
			}
		})
	}
}

func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload")
	content := []byte("the new binary")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	correct := hex.EncodeToString(sum[:])

	t.Run("matching hash passes", func(t *testing.T) {
		if err := verifySHA256(path, correct); err != nil {
			t.Errorf("verifySHA256() with the correct hash: %v", err)
		}
	})

	t.Run("uppercase hash passes", func(t *testing.T) {
		if err := verifySHA256(path, strings.ToUpper(correct)); err != nil {
			t.Errorf("verifySHA256() rejected an uppercase hex digest: %v", err)
		}
	})

	t.Run("wrong hash fails", func(t *testing.T) {
		if err := verifySHA256(path, strings.Repeat("0", 64)); err == nil {
			t.Error("verifySHA256() accepted a mismatched hash")
		}
	})

	t.Run("truncated file fails", func(t *testing.T) {
		short := filepath.Join(dir, "short")
		os.WriteFile(short, content[:4], 0o644)
		if err := verifySHA256(short, correct); err == nil {
			t.Error("verifySHA256() accepted a truncated file — this is the interrupted-download case")
		}
	})

	t.Run("missing file fails", func(t *testing.T) {
		if err := verifySHA256(filepath.Join(dir, "absent"), correct); err == nil {
			t.Error("verifySHA256() accepted a missing file")
		}
	})
}
