package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"proxytoolbox/internal/basedir"
	"proxytoolbox/internal/config"
	"proxytoolbox/internal/proxy"
	"proxytoolbox/internal/util"
)

type proxyStats struct {
	Host          string
	TotalChecks   int
	Failures      int
	TotalLatency  time.Duration
	CurrentStreak int
	LongestStreak int
	SuccessStreak int
	AlertedDown   bool
	DownSince     time.Time
	FailureTimes  []time.Time
}

type alertAction int

const (
	alertNone alertAction = iota
	alertDown
	alertUp
)

// evaluateAlert mutates s based on the latest check outcome and returns
// whether a DOWN or UP transition just occurred. Pure w.r.t. external state.
func evaluateAlert(s *proxyStats, success bool, downThreshold, upThreshold int, now time.Time) alertAction {
	if success {
		s.SuccessStreak++
		s.CurrentStreak = 0
		if s.AlertedDown && s.SuccessStreak >= upThreshold {
			s.AlertedDown = false
			return alertUp
		}
		return alertNone
	}
	s.CurrentStreak++
	s.SuccessStreak = 0
	if !s.AlertedDown && s.CurrentStreak >= downThreshold {
		s.AlertedDown = true
		s.DownSince = now
		return alertDown
	}
	return alertNone
}

type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type discordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color"`
	Fields      []discordEmbedField `json:"fields,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

type discordWebhookPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

const (
	colorDown = 0xE74C3C
	colorUp   = 0x2ECC71
	maxErrLen = 300
)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func buildDownEmbed(p proxy.Proxy, target string, err error, consecutive, cycle int) discordEmbed {
	return discordEmbed{
		Title:     "Proxy DOWN",
		Color:     colorDown,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields: []discordEmbedField{
			{Name: "Proxy", Value: fmt.Sprintf("`%s:%s`", p.Host, p.Port), Inline: true},
			{Name: "Target", Value: fmt.Sprintf("`%s`", target), Inline: true},
			{Name: "Consecutive Failures", Value: strconv.Itoa(consecutive), Inline: true},
			{Name: "Cycle", Value: strconv.Itoa(cycle), Inline: true},
			{Name: "Error", Value: truncate(err.Error(), maxErrLen)},
		},
	}
}

func buildUpEmbed(p proxy.Proxy, target string, downtime time.Duration, cycle int) discordEmbed {
	return discordEmbed{
		Title:     "Proxy RECOVERED",
		Color:     colorUp,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields: []discordEmbedField{
			{Name: "Proxy", Value: fmt.Sprintf("`%s:%s`", p.Host, p.Port), Inline: true},
			{Name: "Target", Value: fmt.Sprintf("`%s`", target), Inline: true},
			{Name: "Downtime", Value: downtime.Round(time.Second).String(), Inline: true},
			{Name: "Cycle", Value: strconv.Itoa(cycle), Inline: true},
		},
	}
}

func sendDiscordEmbed(webhookURL string, embed discordEmbed) {
	if webhookURL == "" {
		return
	}
	body, err := json.Marshal(discordWebhookPayload{Embeds: []discordEmbed{embed}})
	if err != nil {
		fmt.Printf("  [webhook encode error: %s]\n", util.ShortenErr(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		fmt.Printf("  [webhook error: %s]\n", util.ShortenErr(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("  [webhook error: %s]\n", util.ShortenErr(err))
		return
	}
	resp.Body.Close()
}

func printMonitorStats(stats []proxyStats, startTime time.Time, cycles int) {
	elapsed := time.Since(startTime).Round(time.Second)
	totalChecks := 0
	totalFailures := 0
	for _, s := range stats {
		totalChecks += s.TotalChecks
		totalFailures += s.Failures
	}

	fmt.Println("\n" + strings.Repeat("=", 75))
	fmt.Printf("\n  Monitoring ran for: %s  |  Cycles: %d  |  Total checks: %d\n\n",
		elapsed, cycles, totalChecks)

	fmt.Printf("  %-4s  %-24s  %-7s  %-6s  %-9s  %-12s  %s\n",
		"#", "Host", "Checks", "Fails", "Success%", "Avg Latency", "Max Streak")
	fmt.Printf("  %s\n", strings.Repeat("-", 71))

	for i, s := range stats {
		display := s.Host
		if len(display) > 22 {
			display = display[:19] + "..."
		}

		var successPct float64
		if s.TotalChecks > 0 {
			successPct = float64(s.TotalChecks-s.Failures) / float64(s.TotalChecks) * 100
		}

		var pctStr string
		switch {
		case successPct == 100:
			pctStr = util.Green(fmt.Sprintf("%.1f%%", successPct))
		case successPct >= 90:
			pctStr = util.Yellow(fmt.Sprintf("%.1f%%", successPct))
		default:
			pctStr = util.Red(fmt.Sprintf("%.1f%%", successPct))
		}

		avgLatency := "-"
		successes := s.TotalChecks - s.Failures
		if successes > 0 {
			avg := s.TotalLatency / time.Duration(successes)
			avgLatency = fmt.Sprintf("%dms", avg.Milliseconds())
		}

		fmt.Printf("  %-4d  %-24s  %-7d  %-6d  %-9s  %-12s  %d\n",
			i+1, display, s.TotalChecks, s.Failures, pctStr, avgLatency, s.LongestStreak)
	}

	// Failure timeline
	if totalFailures == 0 {
		fmt.Printf("\n  No failures recorded.\n")
		fmt.Println(strings.Repeat("=", 75))
		return
	}

	var allTimes []time.Time
	for _, s := range stats {
		allTimes = append(allTimes, s.FailureTimes...)
	}

	if len(allTimes) == 0 {
		fmt.Println(strings.Repeat("=", 75))
		return
	}

	minT := allTimes[0]
	maxT := allTimes[0]
	for _, t := range allTimes {
		if t.Before(minT) {
			minT = t
		}
		if t.After(maxT) {
			maxT = t
		}
	}

	bucketStart := minT.Truncate(time.Minute)
	bucketEnd := maxT.Truncate(time.Minute).Add(time.Minute)
	numBuckets := int(bucketEnd.Sub(bucketStart) / time.Minute)
	if numBuckets > 60 {
		numBuckets = 60 // cap display at 60 rows
	}

	buckets := make([]int, numBuckets)
	maxCount := 0
	for _, t := range allTimes {
		idx := int(t.Sub(bucketStart) / time.Minute)
		if idx >= 0 && idx < numBuckets {
			buckets[idx]++
			if buckets[idx] > maxCount {
				maxCount = buckets[idx]
			}
		}
	}

	fmt.Printf("\n  Failure Timeline (1-min buckets)\n")
	maxBarWidth := 30
	for i, count := range buckets {
		ts := bucketStart.Add(time.Duration(i) * time.Minute).Format("15:04")
		barLen := 0
		if maxCount > 0 && count > 0 {
			barLen = count * maxBarWidth / maxCount
			if barLen == 0 {
				barLen = 1
			}
		}
		bar := strings.Repeat("\u2588", barLen)
		if count > 0 {
			fmt.Printf("  %s  %s  %d\n", ts, util.Red(bar), count)
		} else {
			fmt.Printf("  %s\n", ts)
		}
	}

	fmt.Println(strings.Repeat("=", 75))
}

func RunMonitor() {
	cfg := config.Load()
	fmt.Printf("[config] domain=%s", cfg.Domain)
	if cfg.DiscordWebhook != "" {
		fmt.Print(", discord_webhook=configured")
	}
	fmt.Print("\n\n")

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

	// Prompt for domain
	if cfg.Domain != "" {
		fmt.Printf("Domain from config.txt: %s\n", cfg.Domain)
		fmt.Print("Press Enter to use it or type another: ")
	} else {
		fmt.Print("Domain to ping (e.g. google.com or https://google.com): ")
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var target string
	if input == "" && cfg.Domain != "" {
		target = cfg.Domain
	} else if input != "" {
		target = input
	} else {
		fmt.Println("Error: no domain specified.")
		return
	}

	// Prompt for interval
	fmt.Print("Interval between pings in ms (default 1000): ")
	intervalInput, _ := reader.ReadString('\n')
	intervalInput = strings.TrimSpace(intervalInput)
	intervalMs := 1000
	if intervalInput != "" {
		if n, err := strconv.Atoi(intervalInput); err == nil && n > 0 {
			intervalMs = n
		}
	}
	interval := time.Duration(intervalMs) * time.Millisecond

	// Open log file
	resultsDir := basedir.Path("results")
	os.MkdirAll(resultsDir, 0755)
	logPath := basedir.Path("results/monitor.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Warning: could not open log file: %v\n", err)
		logFile = nil
	}
	if logFile != nil {
		defer logFile.Close()
	}

	// Banner
	webhookStatus := "disabled"
	if cfg.DiscordWebhook != "" {
		webhookStatus = "enabled"
	}
	fmt.Printf("\n")
	fmt.Printf("File     : %s\n", filePath)
	fmt.Printf("Proxies  : %d\n", len(proxies))
	fmt.Printf("Target   : %s\n", target)
	fmt.Printf("Interval : %dms\n", intervalMs)
	fmt.Printf("Webhook  : %s\n", webhookStatus)
	fmt.Printf("Log file : %s\n", logPath)
	fmt.Printf("\nPress Ctrl+C to stop and show statistics.\n\n")

	fmt.Printf("%-12s  %-4s  %-4s  %-24s  %-10s  %s\n",
		"Time", "Cyc", "#", "Host", "Latency", "Status")
	fmt.Println(strings.Repeat("-", 70))

	// Signal handling
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Init stats
	stats := make([]proxyStats, len(proxies))
	for i, p := range proxies {
		stats[i].Host = fmt.Sprintf("%s:%s", p.Host, p.Port)
	}

	var webhookWG sync.WaitGroup
	var fleet proxyStats

	startTime := time.Now()
	cycle := 0

	for {
		cycle++
		for i, p := range proxies {
			if ctx.Err() != nil {
				goto done
			}

			result := pingProxy(i, p, target)
			stats[i].TotalChecks++
			now := time.Now()
			ts := now.Format("15:04:05")

			display := p.Host
			if len(display) > 22 {
				display = display[:19] + "..."
			}

			if result.Err != nil {
				stats[i].Failures++
				stats[i].CurrentStreak++
				stats[i].SuccessStreak = 0
				if stats[i].CurrentStreak > stats[i].LongestStreak {
					stats[i].LongestStreak = stats[i].CurrentStreak
				}
				stats[i].FailureTimes = append(stats[i].FailureTimes, now)

				action := evaluateAlert(&fleet, false, cfg.DiscordDownThreshold, cfg.DiscordUpThreshold, now)

				fmt.Printf("%s  %-4s  %-4s  %-24s  %-10s  %s\n",
					ts,
					fmt.Sprintf("C%d", cycle),
					fmt.Sprintf("#%d", i+1),
					display,
					"-",
					util.Red("FAIL  "+util.ShortenErr(result.Err)))

				// Log to file
				if logFile != nil {
					fmt.Fprintf(logFile, "%s  FAIL  %s:%s  -> %s  %s\n",
						now.Format("2006-01-02 15:04:05"), p.Host, p.Port, target, result.Err.Error())
				}

				if action == alertDown {
					webhookWG.Add(1)
					embed := buildDownEmbed(p, target, result.Err, fleet.CurrentStreak, cycle)
					go func() {
						defer webhookWG.Done()
						sendDiscordEmbed(cfg.DiscordWebhook, embed)
					}()
				}
			} else {
				stats[i].TotalLatency += result.Latency
				stats[i].CurrentStreak = 0
				stats[i].SuccessStreak++

				downSince := fleet.DownSince
				action := evaluateAlert(&fleet, true, cfg.DiscordDownThreshold, cfg.DiscordUpThreshold, now)

				latStr := fmt.Sprintf("%dms", result.Latency.Milliseconds())
				status := "OK"
				if result.Status > 0 {
					status = fmt.Sprintf("HTTP %d", result.Status)
				}

				fmt.Printf("%s  %-4s  %-4s  %-24s  %-10s  %s\n",
					ts,
					fmt.Sprintf("C%d", cycle),
					fmt.Sprintf("#%d", i+1),
					display,
					latStr,
					util.Green(status))

				if action == alertUp {
					webhookWG.Add(1)
					embed := buildUpEmbed(p, target, now.Sub(downSince), cycle)
					go func() {
						defer webhookWG.Done()
						sendDiscordEmbed(cfg.DiscordWebhook, embed)
					}()
				}
			}

			// Sleep with cancellation
			select {
			case <-time.After(interval):
			case <-ctx.Done():
				goto done
			}
		}
	}

done:
	signal.Stop(sigCh)
	webhookWG.Wait()
	printMonitorStats(stats, startTime, cycle-1)
}
