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
	ProxyID string // canonical user:pass@host:port
	IP      string
	Elapsed time.Duration
	Err     error
}

func checkIP(index int, p proxy.Proxy) ipResult {
	id := p.ID()
	parsed, err := url.Parse(p.URL())
	if err != nil {
		return ipResult{Index: index, ProxyID: id, Err: err}
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
			ProxyID: id,
			IP:      strings.TrimSpace(string(body)),
			Elapsed: time.Since(start),
		}
	}
	return ipResult{Index: index, ProxyID: id, Err: lastErr}
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
	fmt.Printf("%-5s  %-36s  %-18s  %s\n", "#", "Proxy", "Exit IP", "Latency")
	fmt.Println(strings.Repeat("-", ipTableWidth))

	ids := make([]string, len(proxies))
	for i, p := range proxies {
		ids[i] = p.ID()
	}
	// A repeated proxy asks the gateway for the same sticky session twice, so
	// firing the copies at once measures one instant rather than the rotation
	// the repeat was written to look for. See repeatgate.go.
	gate := newRepeatGate(ids, repeatDelay)
	if gate.repeated() {
		fmt.Printf("%d of %d lines are repeated proxies. Each copy after the first is checked %v behind the one before it, so this run is slower on purpose.\n\n",
			gate.repeats(), len(proxies), repeatDelay)
	}

	jobs := make(chan int, len(gate.once))
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

	// The repeats run outside the pool, so spacing them cannot starve it.
	wg.Add(1)
	go func() {
		defer wg.Done()
		gate.runRepeats(func(i int) {
			results <- checkIP(i, proxies[i])
		})
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	start := time.Now()
	for _, i := range gate.once {
		jobs <- i
	}
	close(jobs)

	ipLines := make(map[string][]int)
	errors := 0

	var ipResults []ipResult
	for r := range results {
		ipResults = append(ipResults, r)

		display := util.TruncateID(r.ProxyID, ipProxyColWidth)

		if r.Err != nil {
			fmt.Printf("%-5d  %-36s  ERROR  %s\n", r.Index+1, display, util.ShortenErr(r.Err))
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

		fmt.Printf("%-5d  %-36s  %-18s  %5dms%s\n",
			r.Index+1, display, r.IP, r.Elapsed.Milliseconds(), repeated)
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Println("\n" + strings.Repeat("-", ipTableWidth))
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

	if path := util.PromptExport("iptester", filePath); path != "" {
		// Summary rows, preceded by the run metadata that makes the export
		// self-describing.
		meta := util.RunMeta{
			Tool:      "iptester",
			RunAt:     start,
			ProxyFile: filePath,
			Target:    "", // iptester has no target; it reports exit IPs
			Workers:   cfg.Workers,
		}
		csvRows := meta.Rows()
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

		// Per-proxy detail. Needed so the compare dashboard can join this run
		// against runs from other tools and enrich them with exit IPs.
		csvRows = append(csvRows, []string{"", ""})
		csvRows = append(csvRows, []string{"#", "Proxy", "Exit IP", "Latency", "Error"})
		for _, r := range ipResults {
			errStr := ""
			latStr := ""
			if r.Err != nil {
				errStr = r.Err.Error()
			} else {
				latStr = fmt.Sprintf("%dms", r.Elapsed.Milliseconds())
			}
			csvRows = append(csvRows, []string{
				strconv.Itoa(r.Index + 1), r.ProxyID, r.IP, latStr, errStr,
			})
		}

		header := []string{"Summary", "Value"}
		if err := util.WriteCSV(path, header, csvRows); err != nil {
			fmt.Printf("Error saving: %v\n", err)
		} else {
			fmt.Printf("Saved to %s\n", path)
		}
	}

	if lines := dedupByIP(ipResults, proxies); len(lines) > 0 {
		if path := util.PromptProxyFile("unique exit IPs"); path != "" {
			if err := util.WriteLines(path, lines); err != nil {
				fmt.Printf("Error saving proxies: %v\n", err)
			} else {
				fmt.Printf("Saved %d unique-IP proxies to %s\n", len(lines), path)
			}
		}
	}
}
