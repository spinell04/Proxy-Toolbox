package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"

	"proxytoolbox/internal/tools"
)

func main() {
	for {
		var choice string
		err := huh.NewSelect[string]().
			Title("Proxy Toolbox").
			Options(
				huh.NewOption("IP Uniqueness Test — Check exit IPs, detect duplicates", "iptester"),
				huh.NewOption("Ping Test          — Ping a domain through proxies", "pinger"),
				huh.NewOption("TM Request Tester  — Test proxy speed with a full request to Ticketmaster", "speedtester"),
				huh.NewOption("Randomize File     — Shuffle proxy order in a file", "randomizer"),
				huh.NewOption("Proxy Parser       — Convert proxy format in a file", "parser"),
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
		case "pinger":
			tools.RunPinger()
		case "speedtester":
			tools.RunSpeedTester()
		case "randomizer":
			tools.RunRandomizer()
		case "parser":
			tools.RunParser()
		case "exit":
			fmt.Println("Bye.")
			return
		}

		fmt.Print("\nPress Enter to return to menu...")
		fmt.Scanln()
	}
}
