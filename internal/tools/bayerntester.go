package tools

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"proxytoolbox/internal/config"
	"proxytoolbox/internal/proxy"
	"proxytoolbox/internal/util"
)

const bayernURL = "https://fcbayern.com/de/tickets"

func RunBayernTester() {
	cfg := config.Load()

	filePath, err := proxy.SelectFile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	raw, err := proxy.LoadRawLines(filePath)
	if err != nil {
		fmt.Printf("Error loading proxies: %v\n", err)
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

	workers := cfg.Workers
	if len(proxies) < workers {
		workers = len(proxies)
	}

	fmt.Printf("\n─────────────────────────────────────────────────────────────\n")
	fmt.Printf("  Target  : %s\n", bayernURL)
	fmt.Printf("  Proxies : %d\n", len(proxies))
	fmt.Printf("  Workers : %d\n", workers)
	fmt.Printf("  TLS     : Chrome 133 (tlsclient)\n")
	fmt.Printf("─────────────────────────────────────────────────────────────\n\n")

	fmt.Printf("%-5s  %-44s  %-10s  %s\n", "#", "Proxy", "Speed", "Status")
	fmt.Println(strings.Repeat("─", tableWidth))

	jobs := make(chan int, len(proxies))
	resultsCh := make(chan speedResult, len(proxies))

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				resultsCh <- testSingleProxy(i, proxies[i], bayernURL)
			}
		}()
	}

	start := time.Now()
	for i := range proxies {
		jobs <- i
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var totalLatency time.Duration
	var okCount, failCount, blockedCount int
	var okLatencies []time.Duration
	var csvRows [][]string
	var savedLines []string
	maxMs := cfg.BayernMaxLatencyMs

	for r := range resultsCh {
		display := util.TruncateID(r.ProxyID, proxyColWidth)

		if r.Err != nil {
			fmt.Printf("%-5d  %-44s  %-10s  %s  %s\n",
				r.Index+1, display, "—", util.Red("ERROR"), util.ShortenErr(r.Err))
			csvRows = append(csvRows, []string{fmt.Sprintf("%d", r.Index+1), r.ProxyID, "", "ERROR", r.Err.Error()})
			failCount++
		} else if r.Status == 403 {
			latStr := fmt.Sprintf("%dms", r.Latency.Milliseconds())
			fmt.Printf("%-5d  %-44s  %-10s  %s\n",
				r.Index+1, display, latStr, util.Yellow("403 BLOCKED"))
			csvRows = append(csvRows, []string{fmt.Sprintf("%d", r.Index+1), r.ProxyID, latStr, "403 BLOCKED", ""})
			blockedCount++
		} else {
			latStr := fmt.Sprintf("%dms", r.Latency.Milliseconds())
			fmt.Printf("%-5d  %-44s  %-10s  %s\n",
				r.Index+1, display, latStr, util.Green(fmt.Sprintf("%d OK", r.Status)))
			csvRows = append(csvRows, []string{fmt.Sprintf("%d", r.Index+1), r.ProxyID, latStr, fmt.Sprintf("%d OK", r.Status), ""})
			totalLatency += r.Latency
			okLatencies = append(okLatencies, r.Latency)
			okCount++
			if passLatency(r.Latency.Milliseconds(), maxMs) {
				savedLines = append(savedLines, proxies[r.Index].Raw)
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("─", tableWidth))
	fmt.Printf("\n  Total proxies  : %d\n", len(proxies))
	fmt.Printf("  %s        : %d\n", util.Green("Working"), okCount)
	fmt.Printf("  %s  : %d\n", util.Yellow("Blocked (403)"), blockedCount)
	fmt.Printf("  %s         : %d\n", util.Red("Failed"), failCount)

	// Computed once here and reused by the CSV export below, so the terminal
	// summary and the exported file can never report different figures.
	// util.Percentile sorts its own copy, so okLatencies needs no sorting.
	var avg, p50, p95, fastest, slowest time.Duration
	if okCount > 0 {
		avg = totalLatency / time.Duration(okCount)
		p50 = util.Percentile(okLatencies, 0.50)
		p95 = util.Percentile(okLatencies, 0.95)
		fastest = util.Percentile(okLatencies, 0)
		slowest = util.Percentile(okLatencies, 1)

		fmt.Printf("\n  ── Speed Stats (full TLS request) ──\n")
		fmt.Printf("  Average        : %dms\n", avg.Milliseconds())
		fmt.Printf("  Median (p50)   : %dms\n", p50.Milliseconds())
		fmt.Printf("  p95            : %dms\n", p95.Milliseconds())
		fmt.Printf("  Fastest        : %dms\n", fastest.Milliseconds())
		fmt.Printf("  Slowest        : %dms\n", slowest.Milliseconds())
	}

	if path := util.PromptExport("bayerntester", filePath); path != "" {
		meta := util.RunMeta{
			Tool:      "bayerntester",
			RunAt:     start,
			ProxyFile: filePath,
			Target:    bayernURL,
			Workers:   workers,
		}
		summary := meta.Rows()
		summary = append(summary, []string{"Total proxies", fmt.Sprintf("%d", len(proxies))})
		summary = append(summary, []string{"Working", fmt.Sprintf("%d", okCount)})
		summary = append(summary, []string{"Blocked (403)", fmt.Sprintf("%d", blockedCount)})
		summary = append(summary, []string{"Failed", fmt.Sprintf("%d", failCount)})
		if okCount > 0 {
			summary = append(summary, []string{"Average", fmt.Sprintf("%dms", avg.Milliseconds())})
			summary = append(summary, []string{"Median (p50)", fmt.Sprintf("%dms", p50.Milliseconds())})
			summary = append(summary, []string{"p95", fmt.Sprintf("%dms", p95.Milliseconds())})
			summary = append(summary, []string{"Fastest", fmt.Sprintf("%dms", fastest.Milliseconds())})
			summary = append(summary, []string{"Slowest", fmt.Sprintf("%dms", slowest.Milliseconds())})
		}
		summary = append(summary, []string{"", ""})
		summary = append(summary, []string{"#", "Proxy", "Latency", "Status", "Error"})
		summary = append(summary, csvRows...)

		header := []string{"Summary", "Value"}
		if err := util.WriteCSV(path, header, summary); err != nil {
			fmt.Printf("Error saving: %v\n", err)
		} else {
			fmt.Printf("Saved to %s\n", path)
		}
	}

	if len(savedLines) > 0 {
		label := "all working proxies"
		if maxMs > 0 {
			label = fmt.Sprintf("latency < %dms", maxMs)
		}
		if path := util.PromptProxyFile(label); path != "" {
			if err := util.WriteLines(path, savedLines); err != nil {
				fmt.Printf("Error saving proxies: %v\n", err)
			} else {
				fmt.Printf("Saved %d proxies to %s\n", len(savedLines), path)
			}
		}
	}
}
