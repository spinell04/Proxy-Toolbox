package update

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
)

// config is everything maybeUpdate needs, injected so the decision logic can
// be tested without a real executable, a real network or a real process
// replacement.
type config struct {
	version string
	enabled bool
	asset   string
	exe     string
	client  Client
}

// decide reports whether to check for an update, and why not when it declines.
//
// Split out from maybeUpdate because it is the whole policy and it is pure:
// everything else in the update path touches the network or the filesystem.
func decide(version string, enabled bool, asset string) (bool, string) {
	// Checked before the dev test so that someone who turned it off is told
	// that, rather than being told their build is a development build.
	if !enabled {
		return false, "auto-update is disabled in config.txt"
	}
	if !IsRelease(version) {
		return false, fmt.Sprintf("this is a development build (%s), not a release", version)
	}
	if asset == "" {
		return false, fmt.Sprintf("no release build is published for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return true, ""
}

// MaybeUpdate installs a newer release over this binary and re-execs into it.
//
// Called before anything else in main. It returns normally when there is
// nothing to do, and does not return at all when an update succeeds.
//
// Every failure is reported in one line and swallowed. The toolbox's job is
// testing proxies, and it has to start when GitHub is down, when the network
// is behind a captive portal, and when the directory is read-only. Failing to
// update is never a reason to fail to run.
func MaybeUpdate(version string, enabled bool) {
	exe, err := os.Executable()
	if err != nil {
		// Without the path to the running binary there is nothing to replace
		// and nothing to clean up. Silent: this is not actionable by the user.
		return
	}

	maybeUpdate(config{
		version: version,
		enabled: enabled,
		asset:   AssetName(),
		exe:     exe,
		client:  NewClient(),
	})
}

func maybeUpdate(cfg config) {
	// Unconditionally, before any decision: the previous update's displaced
	// binary is ~13 MB, and it is this launch's job to remove it whether or
	// not this launch updates anything.
	cleanupOld(cfg.exe)

	if ok, _ := decide(cfg.version, cfg.enabled, cfg.asset); !ok {
		// The reason is deliberately not printed. These are all steady states
		// — disabled, a dev build, an unsupported platform — and a line about
		// them on every single launch is noise.
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	rel, err := cfg.client.Latest(ctx)
	if err != nil {
		fmt.Printf("[update] check failed: %v\n", err)
		return
	}
	if !Newer(cfg.version, rel.Tag) {
		return
	}

	fmt.Printf("[update] %s -> %s, downloading...\n", cfg.version, rel.Tag)

	// A separate, longer deadline: the check is a 3-second budget against
	// someone else's availability, but a ~13 MB transfer on a slow connection
	// is legitimately slow and must not be cut off by the check's timeout.
	dlCtx, dlCancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer dlCancel()

	dlClient := cfg.client
	dlClient.HTTP = &http.Client{Timeout: downloadTimeout}

	if err := dlClient.install(dlCtx, rel, cfg.asset, cfg.exe); err != nil {
		fmt.Printf("[update] failed, staying on %s: %v\n", cfg.version, err)
		return
	}

	fmt.Printf("[update] installed %s, restarting...\n\n", rel.Tag)
	if err := restart(cfg.exe, os.Args); err != nil {
		// The new binary is in place but could not be launched. Say so
		// explicitly: the next manual launch will be the new version, so this
		// is recoverable, and a silent return here would show the user the old
		// version's menu from a binary that no longer exists on disk.
		fmt.Printf("[update] %s is installed but could not be started: %v\n", rel.Tag, err)
		fmt.Println("[update] restart the toolbox manually to use it.")
	}
}
