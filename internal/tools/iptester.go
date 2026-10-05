package tools

import (
	"fmt"
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

// ipResult carries up to two addresses, because one proxy has up to two exits
// and which one a target sees depends on the target. The pair is its identity.
type ipResult struct {
	Index   int
	ProxyID string // canonical user:pass@host:port
	IPv4    string
	IPv6    string
	// Per-family failure, kept even when the other family answered: a check
	// that returned one address of two succeeded and still has something to
	// report.
	ErrV4 error
	ErrV6 error
	// Err is set only when no requested family answered. A v4-only proxy asked
	// for both is not a broken proxy.
	Err     error
	Elapsed time.Duration
}

// ipClient builds the client a lookup runs through.
//
// Proxy is left nil for a direct line, which is what sends the request out from
// this machine and makes the exit IP the user's own.
//
// Deliberately not http.ProxyFromEnvironment: it would route through a
// corporate HTTP_PROXY where one is set, and an exit IP measured through
// somebody else's proxy is the one answer this tool must never give silently.
//
// The parse sits inside the branch because url.Parse("") returns no error — it
// yields an empty *url.URL that http.ProxyURL would hand back as a proxy with
// no host, so the error check cannot catch a direct line on its own.
func ipClient(p proxy.Proxy) (*http.Client, error) {
	transport := &http.Transport{}
	if !p.Direct {
		parsed, err := url.Parse(p.URL())
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	return &http.Client{Transport: transport, Timeout: 20 * time.Second}, nil
}

func checkIP(index int, p proxy.Proxy, mode IPMode) ipResult {
	id := p.ID()
	client, err := ipClient(p)
	if err != nil {
		return ipResult{Index: index, ProxyID: id, Err: err}
	}

	start := time.Now()
	var v4, v6 string
	var errV4, errV6 error
	if mode.wantsV4() {
		v4, errV4 = lookupIP(client, ipv4Endpoints, true)
	}
	if mode.wantsV6() {
		v6, errV6 = lookupIP(client, ipv6Endpoints, false)
	}
	r := checkOutcome(index, id, v4, v6, errV4, errV6)
	r.Elapsed = time.Since(start)
	return r
}

// checkOutcome assembles one check's result, separated from the requests so the
// rule that decides success is testable on its own.
//
// The check fails only when no requested family answered. A v4-only proxy asked
// for both is not a broken proxy, and failing the check would make every such
// proxy a permanent entry on the failure ladder.
func checkOutcome(index int, id, v4, v6 string, errV4, errV6 error) ipResult {
	r := ipResult{Index: index, ProxyID: id, IPv4: v4, IPv6: v6, ErrV4: errV4, ErrV6: errV6}
	if r.IPv4 == "" && r.IPv6 == "" {
		r.Err = familyErr(errV4, errV6)
	}
	return r
}

// familyErr combines the two families' failures onto one line. errors.Join
// separates with newlines, and both a table row and a log line are one line.
func familyErr(v4, v6 error) error {
	switch {
	case v4 != nil && v6 != nil:
		return fmt.Errorf("%s: %w; %s: %v", familyV4, v4, familyV6, v6)
	case v4 != nil:
		return v4
	default:
		return v6
	}
}

// partialErr describes a family that failed on a check that otherwise
// succeeded. Empty when nothing failed.
func (r ipResult) partialErr() string {
	var parts []string
	if r.Err == nil {
		if r.ErrV4 != nil {
			parts = append(parts, familyV4+": "+r.ErrV4.Error())
		}
		if r.ErrV6 != nil {
			parts = append(parts, familyV6+": "+r.ErrV6.Error())
		}
	}
	return strings.Join(parts, "; ")
}

func RunIPTester() {
	cfg := config.Load()
	mode := ParseIPMode(cfg.IPMode)
	fmt.Printf("[config] workers=%d, ip_mode=%s\n\n", cfg.Workers, mode)

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

	mode = promptIPMode(mode)
	fams := mode.families()
	tableWidth := ipTableWidth - ipV4ColWidth + ipColsWidth(mode)

	fmt.Printf("\nFile    : %s\n", filePath)
	fmt.Printf("Proxies : %d\n", len(proxies))
	fmt.Printf("Workers : %d\n", cfg.Workers)
	fmt.Printf("IP mode : %s\n\n", mode.Label())
	fmt.Printf("%-5s  %-36s  %s  %s\n", "#", "Proxy", ipColsHeader(mode), "Latency")
	fmt.Println(strings.Repeat("-", tableWidth))

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
				results <- checkIP(i, proxies[i], mode)
			}
		}()
	}

	// The repeats run outside the pool, so spacing them cannot starve it.
	wg.Add(1)
	go func() {
		defer wg.Done()
		gate.runRepeats(func(i int) {
			results <- checkIP(i, proxies[i], mode)
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

	// One map of address -> line numbers per tracked family. Counting across
	// families would be the bug this mode exists to fix: a v4 and a v6 address
	// from one gateway are not two distinct exits.
	seen := make([]map[string][]int, len(fams))
	for i := range seen {
		seen[i] = make(map[string][]int)
	}
	errorCount := 0

	var ipResults []ipResult
	for r := range results {
		ipResults = append(ipResults, r)

		display := util.TruncateID(r.ProxyID, ipProxyColWidth)

		if r.Err != nil {
			fmt.Printf("%-5d  %-36s  ERROR  %s\n", r.Index+1, display, util.ShortenErr(r.Err))
			errorCount++
			continue
		}

		var repeated []string
		for fi, f := range fams {
			ip := f.Get(r)
			if ip == "" {
				continue
			}
			seen[fi][ip] = append(seen[fi][ip], r.Index+1)
			lines := seen[fi][ip]
			if len(lines) > 1 {
				var prev []string
				for _, l := range lines[:len(lines)-1] {
					prev = append(prev, strconv.Itoa(l))
				}
				repeated = append(repeated, fmt.Sprintf("  *** REPEATED %s x%d (lines: %s)",
					f.Label, len(lines), strings.Join(prev, ", ")))
			}
		}

		fmt.Printf("%-5d  %-36s  %s  %5dms%s\n",
			r.Index+1, display, ipCols(mode, r), r.Elapsed.Milliseconds(),
			strings.Join(repeated, ""))
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Println("\n" + strings.Repeat("-", tableWidth))
	totalOk := len(proxies) - errorCount

	fmt.Printf("\n%-17s: %d\n", "Proxies tested", len(proxies))
	fmt.Printf("%-17s: %d\n", "Errors", errorCount)
	// One figure per family, each meaning one definite thing. The denominator is
	// how many proxies that family actually answered for, not how many checks
	// succeeded: in both mode a v4-only fleet would otherwise read as a v6 pool
	// of extraordinary diversity.
	for fi, f := range fams {
		fmt.Printf("%-17s: %d / %d\n", "Unique "+f.Label,
			len(seen[fi]), answered(seen[fi]))
	}
	fmt.Printf("%-17s: %s\n", "Total time", elapsed)

	if !anyRepeated(seen) && totalOk > 0 {
		fmt.Println("\n[OK] All proxies have unique IPs.")
	} else if totalOk > 0 {
		fmt.Println("\n[!] Repeated IPs:")
		for fi, f := range fams {
			for ip, lines := range seen[fi] {
				if len(lines) > 1 {
					var strs []string
					for _, l := range lines {
						strs = append(strs, strconv.Itoa(l))
					}
					fmt.Printf("    %-4s  %-39s  %d times  ->  lines: %s\n",
						f.Label, ip, len(lines), strings.Join(strs, ", "))
				}
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
			IPMode:    string(mode),
		}
		csvRows := meta.Rows()
		csvRows = append(csvRows, []string{"Proxies tested", strconv.Itoa(len(proxies))})
		csvRows = append(csvRows, []string{"Errors", strconv.Itoa(errorCount)})
		for fi, f := range fams {
			csvRows = append(csvRows, []string{"Unique " + f.Label,
				fmt.Sprintf("%d / %d", len(seen[fi]), answered(seen[fi]))})
		}
		csvRows = append(csvRows, []string{"Total time", elapsed.String()})
		csvRows = append(csvRows, []string{"", ""})

		// Repeated IPs section
		csvRows = append(csvRows, []string{"Repeated IP", "Family", "Times", "Lines"})
		for fi, f := range fams {
			for ip, lines := range seen[fi] {
				if len(lines) > 1 {
					var strs []string
					for _, l := range lines {
						strs = append(strs, strconv.Itoa(l))
					}
					csvRows = append(csvRows, []string{ip, f.Label, strconv.Itoa(len(lines)), strings.Join(strs, ", ")})
				}
			}
		}

		// Per-proxy detail. Needed so the compare dashboard can join this run
		// against runs from other tools and enrich them with exit IPs. The
		// address columns are named per family — an export that does not say
		// which family it measured is the ambiguity this change removes.
		csvRows = append(csvRows, []string{"", ""})
		header := []string{"#", "Proxy"}
		for _, f := range fams {
			header = append(header, "Exit "+f.Label)
		}
		header = append(header, "Latency", "Error")
		csvRows = append(csvRows, header)
		for _, r := range ipResults {
			errStr := r.partialErr()
			latStr := ""
			if r.Err != nil {
				errStr = r.Err.Error()
			} else {
				latStr = fmt.Sprintf("%dms", r.Elapsed.Milliseconds())
			}
			row := []string{strconv.Itoa(r.Index + 1), r.ProxyID}
			for _, f := range fams {
				row = append(row, f.Get(r))
			}
			csvRows = append(csvRows, append(row, latStr, errStr))
		}

		if err := util.WriteCSV(path, []string{"Summary", "Value"}, csvRows); err != nil {
			fmt.Printf("Error saving: %v\n", err)
		} else {
			fmt.Printf("Saved to %s\n", path)
		}
	}

	if lines := dedupByExitIdentity(ipResults, proxies, mode); len(lines) > 0 {
		if path := util.PromptProxyFile("unique exit IPs"); path != "" {
			if err := util.WriteLines(path, lines); err != nil {
				fmt.Printf("Error saving proxies: %v\n", err)
			} else {
				fmt.Printf("Saved %d unique-IP proxies to %s\n", len(lines), path)
			}
		}
	}
}

// answered counts the checks one family produced an address for.
func answered(seen map[string][]int) int {
	n := 0
	for _, lines := range seen {
		n += len(lines)
	}
	return n
}

// anyRepeated reports whether any family saw one address twice.
func anyRepeated(seen []map[string][]int) bool {
	for _, byIP := range seen {
		for _, lines := range byIP {
			if len(lines) > 1 {
				return true
			}
		}
	}
	return false
}
