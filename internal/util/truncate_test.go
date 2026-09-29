package util

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateID(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		width int
		want  string
	}{
		{
			name:  "shorter than width is unchanged",
			id:    "bob:pw@1.2.3.4:8080",
			width: 44,
			want:  "bob:pw@1.2.3.4:8080",
		},
		{
			name:  "exactly width is unchanged",
			id:    "abcd",
			width: 4,
			want:  "abcd",
		},
		{
			name:  "longer than width elides the middle",
			id:    "alice:secret@proxy.provider.example.com:7777",
			width: 20,
			want:  "alice:sec...com:7777",
		},
		{
			name:  "smallest width that still elides",
			id:    "bob:pw@1.2.3.4:8080",
			width: 4,
			want:  "b...",
		},
		{
			name:  "zero width is unchanged",
			id:    "bob:pw@1.2.3.4:8080",
			width: 0,
			want:  "bob:pw@1.2.3.4:8080",
		},
		{
			name:  "negative width is unchanged",
			id:    "bob:pw@1.2.3.4:8080",
			width: -5,
			want:  "bob:pw@1.2.3.4:8080",
		},
		{
			name:  "width below the ellipsis cuts plainly",
			id:    "bob:pw@1.2.3.4:8080",
			width: 3,
			want:  "bob",
		},
		{
			name:  "width of one cuts plainly",
			id:    "bob:pw@1.2.3.4:8080",
			width: 1,
			want:  "b",
		},
		{
			name:  "empty id is unchanged",
			id:    "",
			width: 44,
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateID(tt.id, tt.width)
			if got != tt.want {
				t.Errorf("TruncateID(%q, %d) = %q, want %q", tt.id, tt.width, got, tt.want)
			}
		})
	}
}

// TestTruncateID_KeepsBothEnds pins both ends against literal expectations.
// Asserting against fragments split out of the result would only compare the
// output to itself and could never fail.
func TestTruncateID_KeepsBothEnds(t *testing.T) {
	const width = 20
	const id = "alice:secret@gw.provider.com:7777"

	got := TruncateID(id, width)

	if n := utf8.RuneCountInString(got); n != width {
		t.Fatalf("TruncateID(%q, %d) = %q, which is %d runes, want %d", id, width, got, n, width)
	}
	if !strings.HasPrefix(got, "alice:sec") {
		t.Errorf("TruncateID(%q, %d) = %q, want it to start with %q", id, width, got, "alice:sec")
	}
	// This is the assertion that fails under tail truncation.
	if !strings.HasSuffix(got, "com:7777") {
		t.Errorf("TruncateID(%q, %d) = %q, want it to end with %q", id, width, got, "com:7777")
	}
}

// commonPrefixLen reports how many leading runes a and b share. The gateway
// test uses it to prove its own inputs still exercise what it claims to.
func commonPrefixLen(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return n
}

// TestTruncateID_PreservesTailWhenPrefixIsShared is the regression this helper
// exists to prevent. Tail truncation keeps only the head, so two proxies whose
// IDs share a prefix longer than the column render identically.
func TestTruncateID_PreservesTailWhenPrefixIsShared(t *testing.T) {
	const width = 44

	// A long gateway username pushes the difference past where a tail cut would
	// reach: the first 41 characters are identical, so truncating to the head
	// alone would render these two proxies the same. Middle elision keeps the
	// tail, so the differing port still shows.
	const shared = "customer-acme-region-eu-west-rotating-residential-session-x:tok@gw.provider.com:"
	first := shared + "7777"
	second := shared + "8888"

	// Guard the guard: if the inputs ever stop sharing more than the head a
	// tail cut would keep, this test silently stops testing anything.
	headroom := width - len("...")
	if got := commonPrefixLen(first, second); got <= headroom {
		t.Fatalf("test data is wrong: IDs share %d leading runes, need more than %d", got, headroom)
	}

	gotFirst := TruncateID(first, width)
	gotSecond := TruncateID(second, width)

	if gotFirst == gotSecond {
		t.Errorf("TruncateID collapsed two distinct proxies to %q", gotFirst)
	}
	// This is the assertion that fails under tail truncation.
	if !strings.HasSuffix(gotFirst, "7777") {
		t.Errorf("TruncateID(first, %d) = %q, want it to end with %q", width, gotFirst, "7777")
	}
	if !strings.HasSuffix(gotSecond, "8888") {
		t.Errorf("TruncateID(second, %d) = %q, want it to end with %q", width, gotSecond, "8888")
	}
}

// TestTruncateID_ElidedMiddleIsNotDistinguishing documents an accepted
// limitation rather than a guarantee. Proxies that share both head and tail and
// differ only in the elided middle — a pool on one host and port whose members
// differ only by session credential — render identically in the terminal. That
// is fine: the "#" column identifies the row and the exported CSV carries the
// full untruncated ID. Do not "fix" this without widening the column.
func TestTruncateID_ElidedMiddleIsNotDistinguishing(t *testing.T) {
	const width = 44

	first := "customer-acme-rotating-session-aaa111:tok@gw.provider.com:7777"
	second := "customer-acme-rotating-session-bbb222:tok@gw.provider.com:7777"

	gotFirst := TruncateID(first, width)
	gotSecond := TruncateID(second, width)

	if gotFirst != gotSecond {
		t.Errorf("expected these to be indistinguishable once truncated, got %q and %q", gotFirst, gotSecond)
	}
}

func TestTruncateID_MultiByte(t *testing.T) {
	const id = "üsér-ñoñó-sessión-ключ:töken@gw.provider.example.com:7777"
	const width = 30

	got := TruncateID(id, width)

	if n := utf8.RuneCountInString(got); n != width {
		t.Errorf("TruncateID(%q, %d) = %q, which is %d runes, want %d", id, width, got, n, width)
	}
	if !utf8.ValidString(got) {
		t.Errorf("TruncateID(%q, %d) = %q, which is not valid UTF-8", id, width, got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Errorf("TruncateID(%q, %d) = %q, which contains a replacement character", id, width, got)
	}
}

func TestTruncateID_NarrowWidthHasNoEllipsis(t *testing.T) {
	for width := 1; width < 4; width++ {
		got := TruncateID("bob:pw@1.2.3.4:8080", width)
		if strings.Contains(got, ".") {
			t.Errorf("TruncateID at width %d = %q, want no ellipsis fragment", width, got)
		}
		if utf8.RuneCountInString(got) != width {
			t.Errorf("TruncateID at width %d = %q, want %d runes", width, got, width)
		}
	}
}
