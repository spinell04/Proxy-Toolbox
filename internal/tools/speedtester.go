package tools

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/charmbracelet/huh"

	"proxytoolbox/internal/config"
	"proxytoolbox/internal/proxy"
	"proxytoolbox/internal/util"
)

const (
	speedUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
)

var tmRegions = []struct {
	Code string
	Name string
	URL  string
}{
	{"US", "United States", "https://www.ticketmaster.com"},
	{"UK", "United Kingdom", "https://www.ticketmaster.co.uk"},
	{"ES", "Spain", "https://www.ticketmaster.es"},
	{"DE", "Germany", "https://www.ticketmaster.de"},
	{"NL", "Netherlands", "https://www.ticketmaster.nl"},
	{"CA", "Canada", "https://www.ticketmaster.ca"},
	{"MX", "Mexico", "https://www.ticketmaster.com.mx"},
}

type speedResult struct {
	Index   int
	Proxy   string
	Latency time.Duration
	Status  int
	Err     error
}

func testSingleProxy(index int, proxyURL, targetURL string) speedResult {
	opts := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(15),
		tlsclient.WithClientProfile(profiles.Chrome_133),
	}
	if proxyURL != "" {
		opts = append(opts, tlsclient.WithProxyUrl(proxyURL))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), opts...)
	if err != nil {
		return speedResult{Index: index, Proxy: proxyURL, Err: fmt.Errorf("client: %v", err)}
	}

	req, err := fhttp.NewRequest(fhttp.MethodGet, targetURL, nil)
	if err != nil {
		return speedResult{Index: index, Proxy: proxyURL, Err: fmt.Errorf("request: %v", err)}
	}
	req.Header.Set("User-Agent", speedUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-CH-UA", `"Chromium";v="133", "Not(A:Brand";v="99", "Google Chrome";v="133"`)
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header[fhttp.HeaderOrderKey] = []string{
		"accept", "accept-encoding", "accept-language",
		"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
		"sec-fetch-dest", "sec-fetch-mode", "sec-fetch-site",
		"user-agent",
	}
	req.Header[fhttp.PHeaderOrderKey] = []string{":method", ":authority", ":scheme", ":path"}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return speedResult{Index: index, Proxy: proxyURL, Latency: elapsed, Err: err}
	}
	resp.Body.Close()
	return speedResult{Index: index, Proxy: proxyURL, Latency: elapsed, Status: resp.StatusCode}
}

func RunSpeedTester() {
	var regionIdx int
	var options []huh.Option[int]
	for i, r := range tmRegions {
		options = append(options, huh.NewOption(fmt.Sprintf("%s (%s)", r.Name, r.URL), i))
	}
	err := huh.NewSelect[int]().
		Title("Select a region").
		Options(options...).
		Value(&regionIdx).
		Run()
	if err != nil {
		fmt.Println("Selection cancelled.")
		return
	}
	region := tmRegions[regionIdx]

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

	fmt.Printf("\n─────────────────────────────────────────────────────────────\n")
	fmt.Printf("  Region  : %s (%s)\n", region.Name, region.Code)
	fmt.Printf("  Target  : %s\n", region.URL)
	fmt.Printf("  Proxies : %d\n", len(proxies))
	fmt.Printf("  Workers : %d\n", cfg.Workers)
	fmt.Printf("  TLS     : Chrome 133 (tlsclient)\n")
	fmt.Printf("─────────────────────────────────────────────────────────────\n\n")

	fmt.Printf("%-5s  %-24s  %-10s  %s\n", "#", "Host", "Speed", "Status")
	fmt.Println(strings.Repeat("─", 55))

	workers := cfg.Workers
	if len(proxies) < workers {
		workers = len(proxies)
	}

	jobs := make(chan int, len(proxies))
	resultsCh := make(chan speedResult, len(proxies))

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				r := testSingleProxy(i, proxies[i].URL(), region.URL)
				r.Proxy = proxies[i].Host
				resultsCh <- r
			}
		}()
	}

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

	for r := range resultsCh {
		display := r.Proxy
		if len(display) > 22 {
			display = display[:19] + "..."
		}

		if r.Err != nil {
			fmt.Printf("%-5d  %-24s  %-10s  %s  %s\n",
				r.Index+1, display, "—", util.Red("ERROR"), util.ShortenErr(r.Err))
			csvRows = append(csvRows, []string{fmt.Sprintf("%d", r.Index+1), r.Proxy, "", "ERROR", r.Err.Error()})
			failCount++
		} else if r.Status == 403 {
			latStr := fmt.Sprintf("%dms", r.Latency.Milliseconds())
			fmt.Printf("%-5d  %-24s  %-10s  %s\n",
				r.Index+1, display, latStr, util.Yellow("403 BLOCKED"))
			csvRows = append(csvRows, []string{fmt.Sprintf("%d", r.Index+1), r.Proxy, latStr, "403 BLOCKED", ""})
			blockedCount++
		} else {
			latStr := fmt.Sprintf("%dms", r.Latency.Milliseconds())
			fmt.Printf("%-5d  %-24s  %-10s  %s\n",
				r.Index+1, display, latStr, util.Green(fmt.Sprintf("%d OK", r.Status)))
			csvRows = append(csvRows, []string{fmt.Sprintf("%d", r.Index+1), r.Proxy, latStr, fmt.Sprintf("%d OK", r.Status), ""})
			totalLatency += r.Latency
			okLatencies = append(okLatencies, r.Latency)
			okCount++
		}
	}

	fmt.Println("\n" + strings.Repeat("─", 55))
	fmt.Printf("\n  Total proxies  : %d\n", len(proxies))
	fmt.Printf("  %s        : %d\n", util.Green("Working"), okCount)
	fmt.Printf("  %s  : %d\n", util.Yellow("Blocked (403)"), blockedCount)
	fmt.Printf("  %s         : %d\n", util.Red("Failed"), failCount)

	if okCount > 0 {
		avg := totalLatency / time.Duration(okCount)

		sort.Slice(okLatencies, func(i, j int) bool { return okLatencies[i] < okLatencies[j] })
		p50 := okLatencies[len(okLatencies)/2]
		p95idx := int(float64(len(okLatencies)) * 0.95)
		if p95idx >= len(okLatencies) {
			p95idx = len(okLatencies) - 1
		}
		p95 := okLatencies[p95idx]
		fastest := okLatencies[0]
		slowest := okLatencies[len(okLatencies)-1]

		fmt.Printf("\n  ── Speed Stats (full TLS request) ──\n")
		fmt.Printf("  Average        : %dms\n", avg.Milliseconds())
		fmt.Printf("  Median (p50)   : %dms\n", p50.Milliseconds())
		fmt.Printf("  p95            : %dms\n", p95.Milliseconds())
		fmt.Printf("  Fastest        : %dms\n", fastest.Milliseconds())
		fmt.Printf("  Slowest        : %dms\n", slowest.Milliseconds())
	}

	if path := util.PromptExport("speedtest"); path != "" {
		var summary [][]string
		summary = append(summary, []string{"Total proxies", fmt.Sprintf("%d", len(proxies))})
		summary = append(summary, []string{"Working", fmt.Sprintf("%d", okCount)})
		summary = append(summary, []string{"Blocked (403)", fmt.Sprintf("%d", blockedCount)})
		summary = append(summary, []string{"Failed", fmt.Sprintf("%d", failCount)})
		if okCount > 0 {
			avg := totalLatency / time.Duration(okCount)
			p50 := okLatencies[len(okLatencies)/2]
			p95idx := int(float64(len(okLatencies)) * 0.95)
			if p95idx >= len(okLatencies) {
				p95idx = len(okLatencies) - 1
			}
			p95 := okLatencies[p95idx]
			summary = append(summary, []string{"Average", fmt.Sprintf("%dms", avg.Milliseconds())})
			summary = append(summary, []string{"Median (p50)", fmt.Sprintf("%dms", p50.Milliseconds())})
			summary = append(summary, []string{"p95", fmt.Sprintf("%dms", p95.Milliseconds())})
			summary = append(summary, []string{"Fastest", fmt.Sprintf("%dms", okLatencies[0].Milliseconds())})
			summary = append(summary, []string{"Slowest", fmt.Sprintf("%dms", okLatencies[len(okLatencies)-1].Milliseconds())})
		}
		summary = append(summary, []string{"", ""})
		summary = append(summary, []string{"#", "Host", "Speed", "Status", "Error"})
		summary = append(summary, csvRows...)

		header := []string{"Summary", "Value"}
		if err := util.WriteCSV(path, header, summary); err != nil {
			fmt.Printf("Error saving: %v\n", err)
		} else {
			fmt.Printf("Saved to %s\n", path)
		}
	}
}
