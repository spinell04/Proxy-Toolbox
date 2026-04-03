package tools

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"proxytoolbox/internal/config"
	"proxytoolbox/internal/proxy"
	"proxytoolbox/internal/util"
)

var ipEndpoints = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
}

type ipResult struct {
	Index   int
	Host    string
	IP      string
	Elapsed time.Duration
	Err     error
}

func checkIP(index int, p proxy.Proxy) ipResult {
	parsed, err := url.Parse(p.URL())
	if err != nil {
		return ipResult{Index: index, Host: p.Host, Err: err}
	}

	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(parsed)},
		Timeout:   20 * time.Second,
	}

	offset := rand.Intn(len(ipEndpoints))
	start := time.Now()
	var lastErr error

	for i := 0; i < len(ipEndpoints); i++ {
		ep := ipEndpoints[(offset+i)%len(ipEndpoints)]
		resp, err := client.Get(ep)
		if err != nil {
			lastErr = fmt.Errorf("[%s] %w", ep, err)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("[%s] read error: %w", ep, err)
			continue
		}
		return ipResult{
			Index:   index,
			Host:    p.Host,
			IP:      strings.TrimSpace(string(body)),
			Elapsed: time.Since(start),
		}
	}
	return ipResult{Index: index, Host: p.Host, Err: lastErr}
}

func RunIPTester() {
	cfg := config.Load()
	fmt.Printf("[config] workers=%d\n\n", cfg.Workers)

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

	fmt.Printf("\nFile    : %s\n", filePath)
	fmt.Printf("Proxies : %d\n", len(proxies))
	fmt.Printf("Workers : %d\n\n", cfg.Workers)
	fmt.Printf("%-5s  %-24s  %-18s  %s\n", "#", "Host", "Exit IP", "Latency")
	fmt.Println(strings.Repeat("-", 65))

	jobs := make(chan int, len(proxies))
	results := make(chan ipResult, len(proxies))

	var wg sync.WaitGroup
	for w := 0; w < cfg.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results <- checkIP(i, proxies[i])
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	start := time.Now()
	for i := range proxies {
		jobs <- i
	}
	close(jobs)

	ipLines := make(map[string][]int)
	errors := 0

	for r := range results {
		display := r.Host
		if len(display) > 22 {
			display = display[:19] + "..."
		}

		if r.Err != nil {
			fmt.Printf("%-5d  %-24s  ERROR  %s\n", r.Index+1, display, util.ShortenErr(r.Err))
			errors++
			continue
		}

		ipLines[r.IP] = append(ipLines[r.IP], r.Index+1)

		repeated := ""
		if len(ipLines[r.IP]) > 1 {
			var prev []string
			for _, l := range ipLines[r.IP][:len(ipLines[r.IP])-1] {
				prev = append(prev, strconv.Itoa(l))
			}
			repeated = fmt.Sprintf("  *** REPEATED x%d (lines: %s)",
				len(ipLines[r.IP]), strings.Join(prev, ", "))
		}

		fmt.Printf("%-5d  %-24s  %-18s  %5dms%s\n",
			r.Index+1, display, r.IP, r.Elapsed.Milliseconds(), repeated)
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Println("\n" + strings.Repeat("-", 65))
	totalOk := len(proxies) - errors
	unique := len(ipLines)

	fmt.Printf("\nProxies tested   : %d\n", len(proxies))
	fmt.Printf("Errors           : %d\n", errors)
	fmt.Printf("Unique IPs       : %d / %d\n", unique, totalOk)
	fmt.Printf("Total time       : %s\n", elapsed)

	hasRepeated := false
	for _, lines := range ipLines {
		if len(lines) > 1 {
			hasRepeated = true
			break
		}
	}

	if !hasRepeated && totalOk > 0 {
		fmt.Println("\n[OK] All proxies have unique IPs.")
	} else if totalOk > 0 {
		fmt.Println("\n[!] Repeated IPs:")
		for ip, lines := range ipLines {
			if len(lines) > 1 {
				var strs []string
				for _, l := range lines {
					strs = append(strs, strconv.Itoa(l))
				}
				fmt.Printf("    %-18s  %d times  ->  lines: %s\n",
					ip, len(lines), strings.Join(strs, ", "))
			}
		}
	}

	if path := util.PromptExport("iptester"); path != "" {
		// Summary rows
		var csvRows [][]string
		csvRows = append(csvRows, []string{"Proxies tested", strconv.Itoa(len(proxies))})
		csvRows = append(csvRows, []string{"Errors", strconv.Itoa(errors)})
		csvRows = append(csvRows, []string{"Unique IPs", fmt.Sprintf("%d / %d", unique, totalOk)})
		csvRows = append(csvRows, []string{"Total time", elapsed.String()})
		csvRows = append(csvRows, []string{"", ""})

		// Repeated IPs section
		csvRows = append(csvRows, []string{"Repeated IP", "Times", "Lines"})
		for ip, lines := range ipLines {
			if len(lines) > 1 {
				var strs []string
				for _, l := range lines {
					strs = append(strs, strconv.Itoa(l))
				}
				csvRows = append(csvRows, []string{ip, strconv.Itoa(len(lines)), strings.Join(strs, ", ")})
			}
		}

		header := []string{"Summary", "Value"}
		if err := util.WriteCSV(path, header, csvRows); err != nil {
			fmt.Printf("Error saving: %v\n", err)
		} else {
			fmt.Printf("Saved to %s\n", path)
		}
	}
}
