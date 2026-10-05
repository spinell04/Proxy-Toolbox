// Command setup installs the current Proxy Toolbox release into its own folder.
//
// It is downloaded once and run once. There is no version baked into it: it
// resolves /releases/latest at run time, so a copy downloaded a year from now
// installs whatever is current then. That is the same property that lets the
// updater keep working across releases, and it is why this file never needs to
// change.
//
// Everything it prints ends with a wait for Enter. A double-clicked .exe opens
// a console that closes the instant the process exits, so without that the
// window flashes and the user learns nothing — including on the failure paths,
// where the exit code is not what the person double-clicking is reading.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"proxytoolbox/internal/update"
)

// version is stamped by the release workflow. It drives nothing — it is here so
// a support question can name which installer was run.
var version = "dev"

// installTimeout covers the API call plus a ~13 MB download on a bad
// connection. One budget rather than the updater's two, because nothing here
// runs ahead of a menu the user is waiting for.
const installTimeout = 5 * time.Minute

// options is everything run needs, injected so the whole path is testable.
// Unlike the updater, nothing here ends in a process replacement, so every
// branch below is reachable from a test.
type options struct {
	dir    string
	asset  string
	client update.Client
	out    io.Writer
}

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("Cannot determine where this installer is running from:", err)
		pause()
		os.Exit(1)
	}

	// NewClient's timeout is the toolbox's 3-second startup budget, and
	// http.Client.Timeout spans the body read, not just the handshake — left
	// alone it would abort every download well short of 13 MB. The updater
	// swaps in a longer-lived client for exactly this reason; the context
	// deadline below cannot do it, because the shorter of the two wins. An
	// empty APIBase still resolves to the production endpoint.
	client := update.NewClient()
	client.HTTP = &http.Client{Timeout: installTimeout}

	code := run(options{
		dir:    filepath.Dir(exe),
		asset:  update.AssetName(),
		client: client,
		out:    os.Stdout,
	})

	pause()
	os.Exit(code)
}

// pause holds the window open. Scanln's error is ignored: a closed or piped
// stdin means there is nobody waiting to read the message anyway.
func pause() {
	fmt.Print("\nPress Enter to close...")
	fmt.Scanln()
}

func run(o options) int {
	fmt.Fprintf(o.out, "Proxy Toolbox installer %s\n\n", version)

	if o.asset == "" {
		fmt.Fprintf(o.out, "No Proxy Toolbox build is published for %s/%s.\n", runtime.GOOS, runtime.GOARCH)
		return 1
	}

	// Checked before the network: a second run in the same folder is the common
	// case, and it should not cost a request to discover.
	if _, err := os.Stat(filepath.Join(o.dir, o.asset)); err == nil {
		fmt.Fprintf(o.out, "The toolbox is already installed in this folder.\n")
		fmt.Fprintf(o.out, "If you want to update it, just open it and it will auto-update.\n")
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()

	rel, err := o.client.Latest(ctx)
	if err != nil {
		fmt.Fprintf(o.out, "Could not reach the release: %v\n", err)
		return 1
	}

	fmt.Fprintf(o.out, "Latest release : %s\n", rel.Tag)
	fmt.Fprintf(o.out, "Installing     : %s\n", o.asset)

	if err := o.client.Install(ctx, rel, o.asset, o.dir); err != nil {
		fmt.Fprintf(o.out, "\nInstall failed: %v\n", err)
		fmt.Fprintf(o.out, "Nothing was written. Run this installer again to retry.\n")
		return 1
	}

	fmt.Fprintf(o.out, "Installed to   : %s\n\n", o.dir)
	fmt.Fprintf(o.out, "Open %s to start.\n", o.asset)
	fmt.Fprintf(o.out, "It will keep itself up to date from now on.\n")
	return 0
}
