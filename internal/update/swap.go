package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	// oldSuffix names the displaced binary.
	//
	// Not a rollback window: cleanupOld runs at startup and the swap re-execs
	// at once, so the new binary deletes this file as its first act. It is a
	// safety net for the case where that does not happen — a new binary that
	// cannot start leaves its predecessor in place, because nothing got far
	// enough to remove it. Renaming it back is then the manual recovery.
	oldSuffix = ".old"

	// maxAssetBytes caps a downloaded asset. Release binaries are ~13 MB; this
	// is twenty times that, so it bounds a pathological response without ever
	// constraining a real one.
	maxAssetBytes = 256 << 20

	// newSuffix names the download in progress. It sits in the same directory
	// as the target so the final step is a rename within one filesystem, which
	// is atomic. A download to the temp directory could land on a different
	// volume, where os.Rename degrades to a copy and stops being atomic.
	newSuffix = ".new"
)

// stage downloads rel's asset to staged and verifies it, leaving an executable
// file there on success.
//
// It does NOT clean up after itself: a checksum failure leaves the downloaded
// file at staged, because download only removes it on its own I/O errors.
// Removing it is the caller's job, and both callers do it with a defer
// registered before this is called. A third caller that forgets would leak a
// ~13 MB file per failed attempt.
//
// This is the half the updater and the installer have in common, and the half
// where the order matters: nothing is placed anywhere until the SHA-256 of what
// arrived matches what the release published. Both callers then move the staged
// file into position by their own rules — the updater displaces a running
// binary, the installer writes into an empty folder — and neither can reach
// that step with an unverified file.
func (c Client) stage(ctx context.Context, rel Release, asset, staged string) error {
	assetURL := rel.Assets[asset]
	if assetURL == "" {
		return fmt.Errorf("release %s has no %s asset", rel.Tag, asset)
	}

	// No SHA256SUMS means the download cannot be verified, and an unverifiable
	// binary is not installed. Refusing is the conservative direction: the
	// consequence is staying on a working version.
	sumsURL := rel.Assets[sumsAsset]
	if sumsURL == "" {
		return fmt.Errorf("release %s has no %s, so the download cannot be verified", rel.Tag, sumsAsset)
	}

	sums, err := c.fetchSums(ctx, sumsURL)
	if err != nil {
		return err
	}
	want, listed := sums[asset]
	if !listed {
		return fmt.Errorf("%s does not list %s", sumsAsset, asset)
	}

	if err := c.download(ctx, assetURL, staged); err != nil {
		return err
	}
	if err := verifySHA256(staged, want); err != nil {
		return err
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		return fmt.Errorf("making the download executable: %w", err)
	}
	return nil
}

// install downloads the release asset and puts it in place of current.
//
// The order of operations is the design, not an implementation detail: stage
// verifies before anything touches the running binary, so a truncated transfer
// or a substituted file cannot become the thing the user runs. Any failure
// inside stage leaves the original in place and untouched, and the only trace
// is a .new file that is removed on the way out.
func (c Client) install(ctx context.Context, rel Release, asset, current string) error {
	next := current + newSuffix

	// Removed on every path. Leaving a 13 MB .new file beside the binary would
	// accumulate one per failed attempt.
	defer func() {
		if _, err := os.Stat(next); err == nil {
			os.Remove(next)
		}
	}()

	if err := c.stage(ctx, rel, asset, next); err != nil {
		return err
	}
	return swap(current, next)
}

// Install downloads the release's asset for this platform into dir.
//
// The installer's counterpart to install: the same verified download, but
// nothing is displaced, so there is no .old and no re-exec. A failure leaves
// the directory exactly as it was found — which matters because the user is
// watching a one-shot installer, and a half-written file would look like
// success.
func (c Client) Install(ctx context.Context, rel Release, asset, dir string) error {
	dest := filepath.Join(dir, asset)
	staged := dest + newSuffix

	defer func() {
		if _, err := os.Stat(staged); err == nil {
			os.Remove(staged)
		}
	}()

	if err := c.stage(ctx, rel, asset, staged); err != nil {
		return err
	}
	if err := os.Rename(staged, dest); err != nil {
		return fmt.Errorf("putting %s in place: %w", asset, err)
	}
	return nil
}

// fetchSums downloads and parses the release's SHA256SUMS.
func (c Client) fetchSums(ctx context.Context, url string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building the %s request: %w", sumsAsset, err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", sumsAsset, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s returned %d", sumsAsset, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBytes))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sumsAsset, err)
	}
	return parseSums(string(body))
}

// download writes url to dest, removing dest if anything goes wrong.
func (c Client) download(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building the download request: %w", err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading returned %d", resp.StatusCode)
	}

	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dest, err)
	}

	// Bounded like the other two readers in this package. The URL comes from
	// the GitHub API over HTTPS, so this is depth rather than a live hole, but
	// an unbounded Copy is the one place a redirected or hostile response
	// could fill the disk, and a 5-minute timeout is a poor substitute for a
	// limit. A truncated read fails the checksum, exactly as it already would.
	if _, err := io.Copy(f, io.LimitReader(resp.Body, maxAssetBytes)); err != nil {
		f.Close()
		os.Remove(dest)
		return fmt.Errorf("writing the download: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(dest)
		return fmt.Errorf("closing the download: %w", err)
	}
	return nil
}

// swap moves next into current's place, displacing current to current+".old".
//
// Rename rather than overwrite because Windows and macOS both refuse to let a
// process write over its own running executable image, and both permit
// renaming it. That asymmetry is what makes self-replacement possible at all.
func swap(current, next string) error {
	old := current + oldSuffix

	// A stale .old from an interrupted run must not block the swap. On Windows
	// os.Rename fails if the destination exists, so clear it first.
	os.Remove(old)

	if err := os.Rename(current, old); err != nil {
		return fmt.Errorf("moving the current binary aside: %w", err)
	}
	if err := os.Rename(next, current); err != nil {
		// Put the original back. Failing here with the binary missing would
		// leave the user with nothing to run and no indication why.
		if restoreErr := os.Rename(old, current); restoreErr != nil {
			return fmt.Errorf("installing the new binary failed (%w) and restoring the old one also failed (%v) — rename %s back to %s by hand",
				err, restoreErr, old, current)
		}
		return fmt.Errorf("installing the new binary: %w", err)
	}
	return nil
}

// cleanupOld removes the displaced binary from a previous update.
//
// Called on every startup rather than at the end of an update, because the
// process re-execs the instant the swap is done and there is no later point
// in the old process to run from.
//
// Note what that means in sequence: the launch that removes a .old is the
// one the update itself started, milliseconds later. So this is not a
// deferred cleanup of an old file, it is the new binary tidying up after
// its own installation — and a .old that outlives it is the signal that the
// new binary never ran. See oldSuffix.
//
// Errors are ignored: a leftover .old is cosmetic, and the file may still be
// locked by a sibling process that is shutting down.
func cleanupOld(current string) {
	os.Remove(current + oldSuffix)
}
