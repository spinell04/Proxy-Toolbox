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
	Host    string
	Latency time.Duration
	Status  int
	Err     error
}

func pingRawTCP(index int, p proxy.Proxy, host string) pingResult {
	proxyAddr := fmt.Sprintf("%s:%s", p.Host, p.Port)
	target := host + ":80"

	start := time.Now()
	conn, err := net.DialTimeout("tcp", proxyAddr, 20*time.Second)
	if err != nil {
		return pingResult{Index: index, Host: p.Host, Latency: time.Since(start), Err: fmt.Errorf("proxy connect: %w", err)}
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(20 * time.Second))

	auth := fmt.Sprintf("%s:%s", p.User, p.Password)
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Basic %s\r\n\r\n", target, target, encoded)
	if _, err = fmt.Fprint(conn, connectReq); err != nil {
		return pingResult{Index: index, Host: p.Host, Latency: time.Since(start), Err: fmt.Errorf("CONNECT send: %w", err)}
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return pingResult{Index: index, Host: p.Host, Latency: time.Since(start), Err: fmt.Errorf("CONNECT response: %w", err)}
	}
	response := string(buf[:n])
	if !strings.Contains(response, "200") {
		return pingResult{Index: index, Host: p.Host, Latency: time.Since(start), Err: fmt.Errorf("proxy rejected: %s", strings.TrimSpace(response))}
	}

	return pingResult{Index: index, Host: p.Host, Latency: time.Since(start), Status: 0}
}

func pingHTTP(index int, p proxy.Proxy, target string) pingResult {
	parsed, err := url.Parse(p.URL())
	if err != nil {
		return pingResult{Index: index, Host: p.Host, Err: fmt.Errorf("bad proxy URL: %w", err)}
	}

	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(parsed)},
		Timeout:   20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	start := time.Now()
	resp, err := client.Get(target)
	elapsed := time.Since(start)
	if err != nil {
		return pingResult{Index: index, Host: p.Host, Latency: elapsed, Err: err}
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return pingResult{Index: index, Host: p.Host, Latency: elapsed, Status: resp.StatusCode}
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
	fmt.Println("\n")

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
	fmt.Printf("%-5s  %-24s  %-10s  %s\n", "#", "Host", "Latency", "Status")
	fmt.Println(strings.Repeat("-", 55))

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

	for r := range results {
		display := r.Host
		if len(display) > 22 {
			display = display[:19] + "..."
		}

		if r.Err != nil {
			fmt.Printf("%-5d  %-24s  %-10s  ERROR  %s\n",
				r.Index+1, display, "-", util.ShortenErr(r.Err))
			csvRows = append(csvRows, []string{fmt.Sprintf("%d", r.Index+1), r.Host, "", "ERROR", r.Err.Error()})
			errors++
			continue
		}

		totalLatency += r.Latency
		success++
		status := "OK"
		if r.Status > 0 {
			status = fmt.Sprintf("HTTP %d", r.Status)
		}
		latStr := fmt.Sprintf("%dms", r.Latency.Milliseconds())
		fmt.Printf("%-5d  %-24s  %-10s  %s\n",
			r.Index+1, display, latStr, status)
		csvRows = append(csvRows, []string{fmt.Sprintf("%d", r.Index+1), r.Host, latStr, status, ""})
	}

	fmt.Println("\n" + strings.Repeat("-", 55))
	fmt.Printf("\nProxies tested   : %d\n", len(proxies))
	fmt.Printf("Successful       : %d\n", success)
	fmt.Printf("Errors           : %d\n", errors)
	fmt.Printf("Total time       : %s\n", time.Since(start).Round(time.Millisecond))
	if success > 0 {
		avg := totalLatency / time.Duration(success)
		fmt.Printf("Average latency  : %dms\n", avg.Milliseconds())
	}

	if path := util.PromptExport("pinger"); path != "" {
		elapsed := time.Since(start).Round(time.Millisecond)
		var summary [][]string
		summary = append(summary, []string{"Proxies tested", fmt.Sprintf("%d", len(proxies))})
		summary = append(summary, []string{"Successful", fmt.Sprintf("%d", success)})
		summary = append(summary, []string{"Errors", fmt.Sprintf("%d", errors)})
		summary = append(summary, []string{"Total time", elapsed.String()})
		if success > 0 {
			avg := totalLatency / time.Duration(success)
			summary = append(summary, []string{"Average latency", fmt.Sprintf("%dms", avg.Milliseconds())})
		}
		summary = append(summary, []string{"", ""})
		summary = append(summary, []string{"#", "Host", "Latency", "Status", "Error"})
		summary = append(summary, csvRows...)

		header := []string{"Summary", "Value"}
		if err := util.WriteCSV(path, header, summary); err != nil {
			fmt.Printf("Error saving: %v\n", err)
		} else {
			fmt.Printf("Saved to %s\n", path)
		}
	}
}
