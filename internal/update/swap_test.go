package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSwap_ReplacesInPlaceAndKeepsTheOld(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "proxytoolbox")
	next := filepath.Join(dir, "proxytoolbox.new")

	os.WriteFile(current, []byte("old binary"), 0o755)
	os.WriteFile(next, []byte("new binary"), 0o755)

	if err := swap(current, next); err != nil {
		t.Fatalf("swap() error: %v", err)
	}

	if got, _ := os.ReadFile(current); string(got) != "new binary" {
		t.Errorf("current binary = %q, want \"new binary\"", got)
	}
	if got, _ := os.ReadFile(current + oldSuffix); string(got) != "old binary" {
		t.Errorf("%s = %q, want \"old binary\" — manual rollback depends on this", oldSuffix, got)
	}
	if _, err := os.Stat(next); !os.IsNotExist(err) {
		t.Errorf("%s still exists after the swap", next)
	}
}

// TestSwap_OverwritesAStaleOld covers the second update in one session, or an
// .old left behind because the process died before cleanup.
func TestSwap_OverwritesAStaleOld(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "proxytoolbox")
	next := filepath.Join(dir, "proxytoolbox.new")

	os.WriteFile(current, []byte("v2"), 0o755)
	os.WriteFile(current+oldSuffix, []byte("v0 left behind"), 0o755)
	os.WriteFile(next, []byte("v3"), 0o755)

	if err := swap(current, next); err != nil {
		t.Fatalf("swap() error: %v", err)
	}
	if got, _ := os.ReadFile(current + oldSuffix); string(got) != "v2" {
		t.Errorf("%s = %q, want \"v2\" — a stale .old must not block the swap", oldSuffix, got)
	}
}

func TestCleanupOld(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "proxytoolbox")
	os.WriteFile(current, []byte("binary"), 0o755)
	os.WriteFile(current+oldSuffix, []byte("previous"), 0o755)

	cleanupOld(current)

	if _, err := os.Stat(current + oldSuffix); !os.IsNotExist(err) {
		t.Error("cleanupOld() left the .old file behind")
	}
	if _, err := os.Stat(current); err != nil {
		t.Errorf("cleanupOld() disturbed the live binary: %v", err)
	}
}

func TestCleanupOld_SilentWhenThereIsNothingToClean(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "proxytoolbox")
	os.WriteFile(current, []byte("binary"), 0o755)

	cleanupOld(current) // must not panic or complain

	if _, err := os.Stat(current); err != nil {
		t.Errorf("cleanupOld() disturbed the live binary: %v", err)
	}
}

func TestDownload_WritesAndIsExecutable(t *testing.T) {
	payload := []byte("pretend this is 19 MB of Go binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "downloaded")
	if err := testClient(srv.URL).download(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("download() error: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading the download: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("downloaded %q, want %q", got, payload)
	}

	// The asset arrives without a mode; a binary that is not executable
	// replaces a working toolbox with one that cannot start.
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("mode = %v, want the executable bits set", info.Mode().Perm())
	}
}

func TestDownload_FailsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "downloaded")
	if err := testClient(srv.URL).download(context.Background(), srv.URL, dest); err == nil {
		t.Fatal("download() succeeded on a 404")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("download() left a file behind after failing")
	}
}

// TestInstall_LeavesTheOriginalAloneWhenTheHashIsWrong is the most important
// test in this package. The update is unattended, so a corrupted or
// substituted download must never reach the path the user runs.
func TestInstall_LeavesTheOriginalAloneWhenTheHashIsWrong(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "proxytoolbox")
	os.WriteFile(current, []byte("the working binary"), 0o755)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA256SUMS":
			// A digest for different content than the asset serves.
			fmt.Fprintf(w, "%s  proxytoolbox\n", hex.EncodeToString(sha256Of("what we expected")))
		default:
			w.Write([]byte("something else entirely"))
		}
	}))
	defer srv.Close()

	rel := Release{Tag: "v9.9.9", Assets: map[string]string{
		"proxytoolbox": srv.URL + "/proxytoolbox",
		sumsAsset:      srv.URL + "/SHA256SUMS",
	}}

	err := testClient(srv.URL).install(context.Background(), rel, "proxytoolbox", current)
	if err == nil {
		t.Fatal("install() succeeded with a mismatched checksum")
	}

	if got, _ := os.ReadFile(current); string(got) != "the working binary" {
		t.Errorf("the live binary was modified: %q", got)
	}
	if _, err := os.Stat(current + oldSuffix); !os.IsNotExist(err) {
		t.Error("install() renamed the original aside before verifying the download")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("install() left %d files in the directory, want just the original", len(entries))
	}
}

func TestInstall_RequiresTheSumsAsset(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "proxytoolbox")
	os.WriteFile(current, []byte("the working binary"), 0o755)

	// The asset itself is served successfully. A missing SHA256SUMS has to be
	// what stops the install; pointing the asset at a dead port would let the
	// test pass on a connection error even if verification were skipped.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("an unverifiable binary"))
	}))
	defer srv.Close()

	rel := Release{Tag: "v9.9.9", Assets: map[string]string{
		"proxytoolbox": srv.URL + "/proxytoolbox",
	}}

	err := testClient(srv.URL).install(context.Background(), rel, "proxytoolbox", current)
	if err == nil {
		t.Fatal("install() succeeded on a release with no SHA256SUMS")
	}
	if got, _ := os.ReadFile(current); string(got) != "the working binary" {
		t.Errorf("the live binary was modified: %q", got)
	}
	if _, err := os.Stat(current + oldSuffix); !os.IsNotExist(err) {
		t.Error("install() renamed the original aside without a checksum to verify against")
	}
}

func TestInstall_RequiresTheAssetForThisPlatform(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "proxytoolbox")
	os.WriteFile(current, []byte("the working binary"), 0o755)

	rel := Release{Tag: "v9.9.9", Assets: map[string]string{sumsAsset: "http://127.0.0.1:1/SHA256SUMS"}}

	if err := testClient("http://127.0.0.1:1").install(context.Background(), rel, "proxytoolbox", current); err == nil {
		t.Fatal("install() succeeded on a release missing this platform's asset")
	}
}

func TestInstall_Succeeds(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "proxytoolbox")
	os.WriteFile(current, []byte("old"), 0o755)

	payload := []byte("a genuinely new binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA256SUMS":
			fmt.Fprintf(w, "%s  proxytoolbox\n", hex.EncodeToString(sha256Of(string(payload))))
		default:
			w.Write(payload)
		}
	}))
	defer srv.Close()

	rel := Release{Tag: "v9.9.9", Assets: map[string]string{
		"proxytoolbox": srv.URL + "/proxytoolbox",
		sumsAsset:      srv.URL + "/SHA256SUMS",
	}}

	if err := testClient(srv.URL).install(context.Background(), rel, "proxytoolbox", current); err != nil {
		t.Fatalf("install() error: %v", err)
	}
	if got, _ := os.ReadFile(current); string(got) != string(payload) {
		t.Errorf("installed %q, want %q", got, payload)
	}
	if got, _ := os.ReadFile(current + oldSuffix); string(got) != "old" {
		t.Errorf(".old = %q, want \"old\"", got)
	}
}

func sha256Of(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

// TestInstall_PlacesTheBinaryInAnEmptyFolder is the installer's happy path:
// nothing is displaced, so there is no .old and no .new left behind.
func TestInstall_PlacesTheBinaryInAnEmptyFolder(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("a freshly installed toolbox")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+sumsAsset {
			fmt.Fprintf(w, "%s  proxytoolbox\n", hex.EncodeToString(sha256Of(string(payload))))
			return
		}
		w.Write(payload)
	}))
	defer srv.Close()

	rel := Release{Tag: "v1.2.3", Assets: map[string]string{
		"proxytoolbox": srv.URL + "/proxytoolbox",
		sumsAsset:      srv.URL + "/" + sumsAsset,
	}}

	if err := testClient(srv.URL).Install(context.Background(), rel, "proxytoolbox", dir); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "proxytoolbox"))
	if err != nil {
		t.Fatalf("reading the installed binary: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("installed %q, want %q", got, payload)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want just the binary — no .new or .old", names)
	}
}

func TestInstall_SetsTheExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no executable bit; Go reports 0666 whatever the chmod did")
	}

	dir := t.TempDir()
	payload := []byte("toolbox")

	// Pre-create the staged path at a mode with no execute bit, which is what
	// an interrupted earlier run leaves behind. Without this the test proves
	// nothing: download opens with O_CREATE|O_TRUNC 0755, so on a path that
	// does not exist yet the file is born executable and the chmod in stage
	// could be deleted with every test still passing. O_CREATE applies its
	// mode only when it creates the file — on an existing one the mode is
	// untouched, and the chmod is the only thing that sets it.
	if err := os.WriteFile(filepath.Join(dir, "proxytoolbox"+newSuffix), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+sumsAsset {
			fmt.Fprintf(w, "%s  proxytoolbox\n", hex.EncodeToString(sha256Of(string(payload))))
			return
		}
		w.Write(payload)
	}))
	defer srv.Close()

	rel := Release{Tag: "v1.2.3", Assets: map[string]string{
		"proxytoolbox": srv.URL + "/proxytoolbox",
		sumsAsset:      srv.URL + "/" + sumsAsset,
	}}

	if err := testClient(srv.URL).Install(context.Background(), rel, "proxytoolbox", dir); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// A toolbox the OS will not exec is worse than no toolbox: the user has a
	// file that looks installed and does nothing.
	info, err := os.Stat(filepath.Join(dir, "proxytoolbox"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("mode = %v, want the executable bits set", info.Mode().Perm())
	}
}

// TestInstall_LeavesNothingBehindOnABadChecksum is the installer's equivalent
// of the updater's most important test. The download is unattended, so a
// corrupted or substituted file must not end up in the user's folder.
func TestInstall_LeavesNothingBehindOnABadChecksum(t *testing.T) {
	dir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+sumsAsset {
			fmt.Fprintf(w, "%s  proxytoolbox\n", hex.EncodeToString(sha256Of("what we expected")))
			return
		}
		w.Write([]byte("something else entirely"))
	}))
	defer srv.Close()

	rel := Release{Tag: "v1.2.3", Assets: map[string]string{
		"proxytoolbox": srv.URL + "/proxytoolbox",
		sumsAsset:      srv.URL + "/" + sumsAsset,
	}}

	if err := testClient(srv.URL).Install(context.Background(), rel, "proxytoolbox", dir); err == nil {
		t.Fatal("Install() succeeded with a mismatched checksum")
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v after a failed install, want it empty", names)
	}
}

func TestInstall_RefusesWithoutSums(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("an unverifiable binary"))
	}))
	defer srv.Close()

	rel := Release{Tag: "v1.2.3", Assets: map[string]string{
		"proxytoolbox": srv.URL + "/proxytoolbox",
	}}

	if err := testClient(srv.URL).Install(context.Background(), rel, "proxytoolbox", dir); err == nil {
		t.Fatal("Install() succeeded on a release with no SHA256SUMS")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("directory is not empty after refusing to install")
	}
}

func TestInstall_FailsOnAnUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; mode 0o500 would not block a write")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	payload := []byte("toolbox")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+sumsAsset {
			fmt.Fprintf(w, "%s  proxytoolbox\n", hex.EncodeToString(sha256Of(string(payload))))
			return
		}
		w.Write(payload)
	}))
	defer srv.Close()

	rel := Release{Tag: "v1.2.3", Assets: map[string]string{
		"proxytoolbox": srv.URL + "/proxytoolbox",
		sumsAsset:      srv.URL + "/" + sumsAsset,
	}}

	if err := testClient(srv.URL).Install(context.Background(), rel, "proxytoolbox", dir); err == nil {
		t.Fatal("Install() succeeded into a read-only directory")
	}
}
