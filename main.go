package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"

	"proxytoolbox/internal/dashboard"
	"proxytoolbox/internal/tools"
)

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
	for {
		var choice string
		err := huh.NewSelect[string]().
			Title("Proxy Toolbox").
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
