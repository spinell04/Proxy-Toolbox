package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"proxytoolbox/internal/basedir"
	"proxytoolbox/internal/config"
	"proxytoolbox/internal/proxy"
	"proxytoolbox/internal/util"
)

func printSessionStats(stats []sessionStats, mode IPMode, startTime time.Time, cycles int) {
	elapsed := time.Since(startTime).Round(time.Second)
	fams := mode.families()
	totalChecks, totalFailures, totalRotations := 0, 0, 0
	// One set, not one per family: a v4 and a v6 string can never collide, and
	// the figure answers "how many distinct exits did this fleet show".
	fleetIPs := make(map[string]struct{})
	for i := range stats {
		s := &stats[i]
		totalChecks += s.TotalChecks
		totalFailures += s.Failures
		totalRotations += s.Rotations
		for _, f := range fams {
			for ip := range s.state(f.Label).Seen {
				fleetIPs[ip] = struct{}{}
			}
		}
	}

	ids := make([]string, len(stats))
	for i, s := range stats {
		ids[i] = s.ProxyID
	}
	proxyCol := sessionProxyCol(ids)
	// Named statsWidth, not tableWidth: a local of that name reads fine but
	// every use *above* its declaration silently resolves to the package
	// constant instead, which is how the opening and closing rules of this
	// block came to be different lengths.
	statsWidth := sessionStatsFixedCols + proxyCol + ipColsWidth(mode)

	fmt.Println("\n" + strings.Repeat("=", statsWidth))
	fmt.Printf("\n  Session monitoring ran for: %s  |  Cycles: %d  |  Total checks: %d\n\n",
		elapsed, cycles, totalChecks)

	// "IPs" is the distinct addresses this proxy showed across every tracked
	// family, summed: in both mode a proxy with one v4 exit and three v6 exits
	// showed four.
	fmt.Printf("  %-4s  %-*s  %-7s  %-6s  %-10s  %-6s  %s\n",
		"#", proxyCol, "Proxy", "Checks", "Fails", "Rotations", "IPs",
		strings.TrimRight(currentHeader(mode), " "))
	fmt.Printf("  %s\n", strings.Repeat("-", statsWidth-2))

	for i := range stats {
		s := &stats[i]
		cells := make([]string, 0, len(fams))
		distinct := 0
		for _, f := range fams {
			st := s.state(f.Label)
			distinct += len(st.Seen)
			cur := st.Current
			if cur == "" {
				cur = "-"
			}
			cells = append(cells, fmt.Sprintf("%-*s", f.Width, cur))
		}
		// Trailing pad trimmed: these are the last columns on the line.
		current := strings.TrimRight(strings.Join(cells, "  "), " ")

		rotStr := strconv.Itoa(s.Rotations)
		if s.Rotations > 0 {
			rotStr = util.Yellow(rotStr)
		}
		failStr := strconv.Itoa(s.Failures)
		if s.Failures > 0 {
			failStr = util.Red(failStr)
		}

		fmt.Printf("  %-4d  %-*s  %-7d  %-6s  %-10s  %-6d  %s\n",
			i+1, proxyCol, s.ProxyID,
			s.TotalChecks, failStr, rotStr, distinct, current)
	}

	fmt.Printf("\n  Fleet totals: %d rotations  |  %d distinct exit IPs  |  %d failed checks\n",
		totalRotations, len(fleetIPs), totalFailures)
	fmt.Println(strings.Repeat("=", statsWidth))
}

// sessionProxyCol sizes the proxy column to the widest id actually present.
//
// The session monitor never truncates a proxy. Gateway proxies differ only by
// the session id buried in the middle of the username, which is exactly what
// middle-elision removes — two proxies from one pool would render identically,
// in the one tool whose whole job is telling you which proxy did something.
// A wide table is the smaller cost.
func sessionProxyCol(ids []string) int {
	w := len("Proxy")
	for _, id := range ids {
		if n := utf8.RuneCountInString(id); n > w {
			w = n
		}
	}
	return w
}

// RunSessionMonitor watches each proxy's exit IP and alerts when it changes.
func RunSessionMonitor() {
	cfg := config.Load()
	if cfg.DiscordWebhook != "" {
		fmt.Print("[config] discord_webhook=configured\n")
	}
	fmt.Print("\n")

	reader := bufio.NewReader(os.Stdin)

	filePath, err := proxy.SelectFile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	raw, err := proxy.LoadRawLines(filePath)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	var proxies []proxy.Proxy
	for _, line := range raw {
		if p, ok := proxy.ParseLine(line); ok {
			proxies = append(proxies, p)
		}
	}
	if len(proxies) == 0 {
		fmt.Println("No valid proxies found.")
		return
	}

	mode := promptIPMode(ParseIPMode(cfg.IPMode))

	intervalMs := cfg.SessionIntervalMs
	fmt.Printf("Interval between checks in ms (default %d): ", intervalMs)
	intervalInput, _ := reader.ReadString('\n')
	if intervalInput = strings.TrimSpace(intervalInput); intervalInput != "" {
		if n, err := strconv.Atoi(intervalInput); err == nil && n > 0 {
			intervalMs = n
		}
	}
	interval := time.Duration(intervalMs) * time.Millisecond

	resultsDir := basedir.Path("results")
	os.MkdirAll(resultsDir, 0755)
	logPath := basedir.Path("results/session-monitor.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Warning: could not open log file: %v\n", err)
		logFile = nil
	}
	if logFile != nil {
		defer logFile.Close()
	}

	webhookStatus := "disabled"
	if cfg.DiscordWebhook != "" {
		webhookStatus = "enabled"
	}

	// One cycle checks every proxy in parallel, so the interval *is* the
	// sampling period: unlike the downtime monitor, a long list does not push
	// a proxy's next check further out.
	// Lookups, not checks: both mode asks each proxy twice per check, so the
	// figure the free endpoints see is double.
	perMin := float64(len(proxies)*mode.lookupsPerCheck()) * float64(time.Minute) / float64(interval)

	fmt.Printf("\n")
	fmt.Printf("File     : %s\n", filePath)
	fmt.Printf("Proxies  : %d\n", len(proxies))
	fmt.Printf("Workers  : %d\n", cfg.Workers)
	fmt.Printf("Interval : %dms per cycle\n", intervalMs)
	fmt.Printf("IP mode  : %s\n", mode.Label())
	fmt.Printf("Coverage : every proxy checked every ~%s\n", formatSamplingPeriod(interval))
	// The IP endpoints are free third-party services. This is the figure that
	// decides whether they start refusing, so it is on the banner rather than
	// left for the user to work out from the other two lines.
	fmt.Printf("Load     : ~%.0f IP lookups per minute\n", perMin)
	fmt.Printf("Webhook  : %s\n", webhookStatus)
	fmt.Printf("Log file : %s\n", logPath)
	fmt.Printf("\nPress Ctrl+C to stop and show statistics.\n\n")

	liveIDs := make([]string, len(proxies))
	for i, p := range proxies {
		liveIDs[i] = p.ID()
	}
	proxyCol := sessionProxyCol(liveIDs)
	liveWidth := sessionFixedCols + proxyCol + ipColsWidth(mode)
	fmt.Printf("%-8s  %-4s  %-4s  %-*s  %s  %-8s  %s\n",
		"Time", "Cyc", "#", proxyCol, "Proxy", ipColsHeader(mode), "Latency", "Status")
	fmt.Println(strings.Repeat("-", liveWidth))

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	stats := make([]sessionStats, len(proxies))
	for i, p := range proxies {
		stats[i].ProxyID = p.ID()
	}

	var webhookWG sync.WaitGroup
	send := func(embed discordEmbed) {
		webhookWG.Add(1)
		go func() {
			defer webhookWG.Done()
			sendDiscordEmbed(cfg.DiscordWebhook, embed)
		}()
	}

	startTime := time.Now()
	cycle := 0

	// Workers only perform the request. Every mutation of stats, every line
	// printed, every log write and every webhook stays on this goroutine, so
	// the per-proxy state machine never needs a lock and the output cannot
	// interleave mid-row.
	for {
		cycle++
		cycleStart := time.Now()

		jobs := make(chan int, len(proxies))
		results := make(chan ipResult, len(proxies))

		workers := cfg.Workers
		if workers > len(proxies) {
			workers = len(proxies)
		}
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					results <- checkIP(i, proxies[i], mode)
				}
			}()
		}
		for i := range proxies {
			jobs <- i
		}
		close(jobs)
		go func() {
			wg.Wait()
			close(results)
		}()

		for result := range results {
			i := result.Index
			p := proxies[i]
			now := time.Now()
			st := &stats[i]

			// Snapshot the failure streak this check is about to end.
			failingSince, lastErr := st.FailingSince, st.LastErr

			out := evaluateSession(st, result, mode, now)
			rotations := out.rotations()

			ts := now.Format("15:04:05")
			// Never truncated: see sessionProxyCol.
			display := st.ProxyID

			if result.Err != nil {
				fmt.Printf("%-8s  %-4s  %-4s  %-*s  %s  %-8s  %s\n",
					ts, fmt.Sprintf("C%d", cycle), fmt.Sprintf("#%d", i+1),
					proxyCol, display, ipCols(mode, ipResult{}), "-",
					util.Red("FAIL  "+util.ShortenErr(result.Err)))

				if logFile != nil {
					fmt.Fprintf(logFile, "%s  FAIL  %s  %s\n",
						now.Format("2006-01-02 15:04:05"), st.ProxyID, result.Err.Error())
				}
			} else {
				status := util.Green("OK")
				if len(rotations) > 0 {
					var parts []string
					for _, c := range rotations {
						parts = append(parts, fmt.Sprintf("%s %s -> %s", c.Family, c.PrevIP, c.NewIP))
					}
					status = util.Yellow("*** ROTATED  " + strings.Join(parts, "  |  "))
				}

				fmt.Printf("%-8s  %-4s  %-4s  %-*s  %s  %-8s  %s\n",
					ts, fmt.Sprintf("C%d", cycle), fmt.Sprintf("#%d", i+1),
					proxyCol, display, ipCols(mode, result),
					fmt.Sprintf("%dms", result.Elapsed.Milliseconds()), status)

				if logFile != nil {
					for _, c := range rotations {
						fmt.Fprintf(logFile, "%s  ROTATED  %s  %s  %s -> %s  held %s\n",
							now.Format("2006-01-02 15:04:05"), st.ProxyID,
							c.Family, c.PrevIP, c.NewIP, c.Held.Round(time.Second))
					}
				}
				// A family that answered nothing is unknown for this check, not
				// gone: see ipState.observe. Nothing is printed and nothing is
				// alerted, which is why this loop reads only rotations.
				if logFile != nil && result.partialErr() != "" {
					fmt.Fprintf(logFile, "%s  PARTIAL  %s  %s\n",
						now.Format("2006-01-02 15:04:05"), st.ProxyID, result.partialErr())
				}
			}

			switch out.Health {
			case healthFailAlert:
				send(buildSessionFailEmbed(p, i+1, result.Err, st.FailStreak, cycle))
			case healthRecovered:
				send(buildSessionRecoveredEmbed(p, i+1, now.Sub(failingSince), lastErr, cycle))
			}
			for _, c := range rotations {
				send(buildRotationEmbed(p, i+1, c, cycle))
			}
		}

		if ctx.Err() != nil {
			goto done
		}

		// Cadence is measured from the start of the cycle, not its end, so the
		// sampling period stays the interval instead of drifting by however
		// long the checks took. A cycle that overruns says so and starts the
		// next one immediately rather than queueing up behind itself.
		wait := interval - time.Since(cycleStart)
		if wait <= 0 {
			fmt.Printf("%s  cycle %d took %s, longer than the %s interval — running back to back\n",
				time.Now().Format("15:04:05"), cycle,
				time.Since(cycleStart).Round(time.Millisecond), interval)
			continue
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			goto done
		}
	}

done:
	signal.Stop(sigCh)
	webhookWG.Wait()
	printSessionStats(stats, mode, startTime, cycle-1)
}
