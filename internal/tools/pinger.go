package tools

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"proxytoolbox/internal/config"
	"proxytoolbox/internal/proxy"
	"proxytoolbox/internal/util"
)

type pingResult struct {
	Index   int
	ProxyID string // canonical user:pass@host:port
	Latency time.Duration
	Status  int
	Err     error
}

// rawPingPort is what the Ping Test's bare-host mode means by "reach this
// host": a TCP connect to its HTTP port.
const rawPingPort = "80"

// rawPingTarget composes the address pingRawTCP dials.
//
// Split out as a pure function so the port can be asserted without a network.
// Reading it back off a dial error does not work: an unresolvable name fails at
// the DNS lookup before the port is used, and "127.0.0.1:8080" contains
// "127.0.0.1:80" as a substring, so a Contains check silently accepts the wrong
// port. That mutant survived until this function existed.
func rawPingTarget(host string) string { return host + ":" + rawPingPort }

// pingRawTCP measures a TCP reach to host's HTTP port.
//
// A thin wrapper over pingRawTCPTo, which takes a full address so a test can
// point it at a listener on an ephemeral port rather than port 80 of a real
// machine.
func pingRawTCP(index int, p proxy.Proxy, host string) pingResult {
	return pingRawTCPTo(index, p, rawPingTarget(host))
}

func pingRawTCPTo(index int, p proxy.Proxy, target string) pingResult {
	id := p.ID()

	// A direct line has no proxy to CONNECT through, so this is a plain TCP
	// connect to the target. It measures one hop where the proxied path below
	// measures two — which is the comparison a baseline is for, not a flaw.
	if p.Direct {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", target, 20*time.Second)
		if err != nil {
			// Returned bare rather than wrapped as "proxy connect": there is no
			// proxy here, and naming one would misattribute the failure.
			return pingResult{Index: index, ProxyID: id, Latency: time.Since(start), Err: err}
		}
		conn.Close()
		return pingResult{Index: index, ProxyID: id, Latency: time.Since(start), Status: 0}
	}

	proxyAddr := net.JoinHostPort(p.Host, p.Port)

	start := time.Now()
	conn, err := net.DialTimeout("tcp", proxyAddr, 20*time.Second)
	if err != nil {
		return pingResult{Index: index, ProxyID: id, Latency: time.Since(start), Err: fmt.Errorf("proxy connect: %w", err)}
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(20 * time.Second))

	auth := fmt.Sprintf("%s:%s", p.User, p.Password)
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Basic %s\r\n\r\n", target, target, encoded)
	if _, err = fmt.Fprint(conn, connectReq); err != nil {
		return pingResult{Index: index, ProxyID: id, Latency: time.Since(start), Err: fmt.Errorf("CONNECT send: %w", err)}
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return pingResult{Index: index, ProxyID: id, Latency: time.Since(start), Err: fmt.Errorf("CONNECT response: %w", err)}
	}
	response := string(buf[:n])
	if !strings.Contains(response, "200") {
		return pingResult{Index: index, ProxyID: id, Latency: time.Since(start), Err: fmt.Errorf("proxy rejected: %s", strings.TrimSpace(response))}
	}

	return pingResult{Index: index, ProxyID: id, Latency: time.Since(start), Status: 0}
}

func pingHTTP(index int, p proxy.Proxy, target string) pingResult {
	id := p.ID()

	// Proxy is left nil for a direct line, which is what makes the request go
	// out from this machine.
	//
	// Deliberately not http.ProxyFromEnvironment: that would route through a
	// corporate HTTP_PROXY where one is set, and a "direct" baseline measured
	// through somebody's proxy is a lie the user has no way to spot.
	//
	// The parse lives inside the branch because url.Parse("") returns no error
	// — it yields an empty *url.URL that http.ProxyURL would hand back as a
	// proxy with no host, so the error check below cannot catch a direct line.
	transport := &http.Transport{}
	if !p.Direct {
		parsed, err := url.Parse(p.URL())
		if err != nil {
			return pingResult{Index: index, ProxyID: id, Err: fmt.Errorf("bad proxy URL: %w", err)}
		}
		transport.Proxy = http.ProxyURL(parsed)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	start := time.Now()
	resp, err := client.Get(target)
	elapsed := time.Since(start)
	if err != nil {
		return pingResult{Index: index, ProxyID: id, Latency: elapsed, Err: err}
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return pingResult{Index: index, ProxyID: id, Latency: elapsed, Status: resp.StatusCode}
}

func pingProxy(index int, p proxy.Proxy, target string) pingResult {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return pingHTTP(index, p, target)
	}
	return pingRawTCP(index, p, target)
}

func RunPinger() {
	cfg := config.Load()
	fmt.Printf("[config] workers=%d", cfg.Workers)
	if cfg.Domain != "" {
		fmt.Printf(", domain=%s", cfg.Domain)
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

	if cfg.Domain != "" {
		fmt.Printf("\nDomain from config.txt: %s\n", cfg.Domain)
		fmt.Print("Press Enter to use it or type another: ")
	} else {
		fmt.Print("\nDomain to ping (e.g. google.com or https://google.com): ")
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

	mode := "TCP connect (no HTTP, no TLS)"
	if strings.HasPrefix(target, "https://") {
		mode = "HTTPS (CONNECT + TLS + request)"
	} else if strings.HasPrefix(target, "http://") {
		mode = "HTTP (full request)"
	}

	fmt.Printf("\nFile    : %s\n", filePath)
	fmt.Printf("Proxies : %d\n", len(proxies))
	fmt.Printf("Target  : %s\n", target)
	fmt.Printf("Mode    : %s\n", mode)
	fmt.Printf("Workers : %d\n\n", cfg.Workers)
	fmt.Printf("%-5s  %-44s  %-10s  %s\n", "#", "Proxy", "Latency", "Status")
	fmt.Println(strings.Repeat("-", tableWidth))

	jobs := make(chan int, len(proxies))
	results := make(chan pingResult, len(proxies))

	var wg sync.WaitGroup
	for w := 0; w < cfg.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results <- pingProxy(i, proxies[i], target)
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

	var totalLatency time.Duration
	errors := 0
	success := 0
	var csvRows [][]string
	var savedLines []string
	maxMs := cfg.PingMaxLatencyMs

	for r := range results {
		display := util.TruncateID(r.ProxyID, proxyColWidth)

		if r.Err != nil {
			fmt.Printf("%-5d  %-44s  %-10s  ERROR  %s\n",
				r.Index+1, display, "-", util.ShortenErr(r.Err))
			csvRows = append(csvRows, []string{fmt.Sprintf("%d", r.Index+1), r.ProxyID, "", "ERROR", r.Err.Error()})
			errors++
			continue
		}

		totalLatency += r.Latency
		success++
		if passLatency(r.Latency.Milliseconds(), maxMs) {
			savedLines = append(savedLines, proxies[r.Index].Raw)
		}
		status := "OK"
		if r.Status > 0 {
			status = fmt.Sprintf("HTTP %d", r.Status)
		}
		latStr := fmt.Sprintf("%dms", r.Latency.Milliseconds())
		fmt.Printf("%-5d  %-44s  %-10s  %s\n",
			r.Index+1, display, latStr, status)
		csvRows = append(csvRows, []string{fmt.Sprintf("%d", r.Index+1), r.ProxyID, latStr, status, ""})
	}

	fmt.Println("\n" + strings.Repeat("-", tableWidth))
	fmt.Printf("\nProxies tested   : %d\n", len(proxies))
	fmt.Printf("Successful       : %d\n", success)
	fmt.Printf("Errors           : %d\n", errors)
	fmt.Printf("Total time       : %s\n", time.Since(start).Round(time.Millisecond))
	if success > 0 {
		avg := totalLatency / time.Duration(success)
		fmt.Printf("Average latency  : %dms\n", avg.Milliseconds())
	}

	if path := util.PromptExport("pinger", filePath); path != "" {
		elapsed := time.Since(start).Round(time.Millisecond)
		meta := util.RunMeta{
			Tool:      "pinger",
			RunAt:     start,
			ProxyFile: filePath,
			Target:    target,
			Workers:   cfg.Workers,
		}
		summary := meta.Rows()
		summary = append(summary, []string{"Proxies tested", fmt.Sprintf("%d", len(proxies))})
		summary = append(summary, []string{"Successful", fmt.Sprintf("%d", success)})
		summary = append(summary, []string{"Errors", fmt.Sprintf("%d", errors)})
		summary = append(summary, []string{"Total time", elapsed.String()})
		if success > 0 {
			avg := totalLatency / time.Duration(success)
			summary = append(summary, []string{"Average latency", fmt.Sprintf("%dms", avg.Milliseconds())})
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
