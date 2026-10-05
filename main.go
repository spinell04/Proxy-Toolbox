package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"

	"proxytoolbox/internal/basedir"
	"proxytoolbox/internal/bootstrap"
	"proxytoolbox/internal/config"
	"proxytoolbox/internal/dashboard"
	"proxytoolbox/internal/tools"
	"proxytoolbox/internal/update"
)

// version is set at link time by .github/workflows/release.yml:
//
//	-ldflags "-X main.version=v1.2.3"
//
// The "dev" default is what makes `go run .` and local builds safe: a binary
// with this value never auto-updates, so a developer's working copy is never
// replaced by the published release. See internal/update.IsRelease.
var version = "dev"

// siteRequestMenu shows the Site Request Tester submenu and reports whether a
// tool ran. Both entries make a full request to one fixed site, which is what
// separates them from Ping Test: they measure a real page fetch, not a connect.
func siteRequestMenu() bool {
	var choice string
	err := huh.NewSelect[string]().
		Title("Site Request Tester").
		Options(
			huh.NewOption("Ticketmaster — Full request to ticketmaster.de", "speedtester"),
			huh.NewOption("Bayern       — Full request to fcbayern.com/de/tickets", "bayern"),
			huh.NewOption("Back", "back"),
		).
		Value(&choice).
		Run()

	if err != nil {
		fmt.Println("Bye.")
		os.Exit(0)
	}

	switch choice {
	case "speedtester":
		tools.RunSpeedTester()
	case "bayern":
		tools.RunBayernTester()
	default:
		return false
	}
	return true
}

// monitorMenu shows the Monitor submenu and reports whether a tool ran.
func monitorMenu() bool {
	var choice string
	err := huh.NewSelect[string]().
		Title("Monitor").
		Options(
			huh.NewOption("Downtime monitor — Continuous reachability checks with alerts", "downtime"),
			huh.NewOption("Session monitor  — Alert when a proxy's exit IP changes", "session"),
			huh.NewOption("Back", "back"),
		).
		Value(&choice).
		Run()

	if err != nil {
		fmt.Println("Bye.")
		os.Exit(0)
	}

	switch choice {
	case "downtime":
		tools.RunMonitor()
	case "session":
		tools.RunSessionMonitor()
	default:
		return false
	}
	return true
}

func main() {
	// Before bootstrap, and before the first menu: an update replaces this
	// binary and re-execs, so anything done first would be done twice. It
	// returns normally when there is nothing to do and never fails fatally —
	// see internal/update.MaybeUpdate.
	//
	// The flag comes from config.AutoUpdateEnabled rather than Load because
	// config.txt does not exist yet on a first run — bootstrap below is what
	// creates it — and Load would announce that as a problem. The doc comment
	// on AutoUpdateEnabled has the rest.
	update.MaybeUpdate(version, config.AutoUpdateEnabled())

	// The binary ships on its own and is dropped into an empty folder, so the
	// layout it expects has to appear before the first menu. Reported rather
	// than silent: files turning up next to the executable should be something
	// the user was told about, not something they discover later.
	if created, err := bootstrap.Ensure(); err != nil {
		fmt.Println("Setup error:", err)
		os.Exit(1)
	} else if len(created) > 0 {
		// Name the directory rather than saying "next to the binary": under
		// `go run` the binary lives in Go's build cache and these land in the
		// working directory instead, so that phrasing would point at the
		// wrong place in exactly the case a developer is looking.
		fmt.Printf("Created %s in %s\n\n", strings.Join(created, ", "), basedir.Root)
	}

	for {
		var choice string
		err := huh.NewSelect[string]().
			Title("Proxy Toolbox "+version).
			Options(
				huh.NewOption("IP Uniqueness Test — Check exit IPs, detect duplicates", "iptester"),
				huh.NewOption("Site Request Test  — Full page requests to Ticketmaster or Bayern", "siterequest"),
				huh.NewOption("Monitor            — Downtime and session monitoring", "monitor"),
				huh.NewOption("Ping Test          — Ping a domain through proxies", "pinger"),
				huh.NewOption("Randomize File     — Shuffle proxy order in a file", "randomizer"),
				huh.NewOption("Proxy Parser       — Convert proxy format in a file", "parser"),
				huh.NewOption("Compare Results    — Local dashboard for comparing exported CSVs", "compare"),
				huh.NewOption("Exit", "exit"),
			).
			Value(&choice).
			Run()

		if err != nil {
			// User pressed Ctrl+C or terminal closed
			fmt.Println("Bye.")
			os.Exit(0)
		}

		switch choice {
		case "iptester":
			tools.RunIPTester()
		case "siterequest":
			if !siteRequestMenu() {
				// Back: the top-level menu redraws, so skip the pause prompt.
				continue
			}
		case "monitor":
			if !monitorMenu() {
				continue
			}
		case "pinger":
			tools.RunPinger()
		case "randomizer":
			tools.RunRandomizer()
		case "parser":
			tools.RunParser()
		case "compare":
			if err := dashboard.Serve(func() { fmt.Scanln() }); err != nil {
				fmt.Println("Error:", err)
			}
		case "exit":
			fmt.Println("Bye.")
			return
		}

		fmt.Print("\nPress Enter to return to menu...")
		fmt.Scanln()
	}
}
