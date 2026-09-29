# CSV Compare Dashboard Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a local, read-only web dashboard that lets a user select any exported result CSVs and compare them — across runs of one tool over time, and across different tools over the same proxies.

**Architecture:** Three new layers on top of the existing tools. An *emit* layer adds a canonical proxy ID and run metadata to the CSVs the tools already write. A *parse + stats* layer (`internal/compare`) reads those CSVs into structs and computes distributions, joins and set operations with no external dependencies. A *serve* layer (`internal/dashboard`) binds `127.0.0.1` on a random port, serves an embedded single-page UI, and exposes read-only JSON endpoints over `results/`.

**Tech Stack:** Go 1.25 (module directive; 1.26 toolchain) (stdlib `encoding/csv`, `net/http`, `embed`), huh for the menu (already present), uPlot vendored as a static asset. No new Go module dependencies.

**Design doc:** `docs/plans/2026-09-19-csv-compare-dashboard-design.md`

**Conventions used throughout:**
- Run tests with `go test ./... ` from the repo root unless a narrower command is given.
- Every task ends in a commit. Commit messages use the `<type>: <description>` convention.
- Go files stay under 400 lines; split when a file grows past that.
- No mutation of input slices — return new slices.

---

## Phase 1 — Emit layer

Makes the CSVs self-describing and joinable. Touches existing tools but changes no testing behavior.

---

### Task 1: Canonical proxy ID

A proxy's identity is its full credential string. `host:port` is **not** unique: gateway-style residential pools share one host and port across hundreds of session credentials. The four accepted input formats must all normalize to one ID so the same proxy written two ways in two files joins correctly.

**Files:**
- Modify: `internal/proxy/parse.go` (add method after `URL()`, line 23)
- Test: `internal/proxy/parse_test.go`

**Step 1: Write the failing test**

Append to `internal/proxy/parse_test.go`:

```go
func TestProxyID_NormalizesAllInputFormats(t *testing.T) {
	// Every accepted input format for the same proxy must produce one ID.
	lines := []string{
		"1.2.3.4:8080:alice:secret",
		"alice:secret:1.2.3.4:8080",
		"alice:secret@1.2.3.4:8080",
		"http://alice:secret@1.2.3.4:8080",
	}
	const want = "alice:secret@1.2.3.4:8080"

	for _, line := range lines {
		p, ok := ParseLine(line)
		if !ok {
			t.Fatalf("ParseLine(%q) failed to parse", line)
		}
		if got := p.ID(); got != want {
			t.Errorf("ParseLine(%q).ID() = %q, want %q", line, got, want)
		}
	}
}

func TestProxyID_DistinguishesGatewaySessions(t *testing.T) {
	// Gateway pools share host:port and differ only by credentials.
	a, _ := ParseLine("gate.provider.com:7000:user-session-aaa:pw")
	b, _ := ParseLine("gate.provider.com:7000:user-session-bbb:pw")

	if a.ID() == b.ID() {
		t.Errorf("gateway sessions collapsed to one ID: %q", a.ID())
	}
}

func TestProxyID_LowercasesHost(t *testing.T) {
	// DNS hostnames are case-insensitive, and ID() is a cross-file join key.
	// Differing case must not split one proxy into two identities.
	upper, _ := ParseLine("Gate.Provider.COM:7000:alice:secret")
	lower, _ := ParseLine("gate.provider.com:7000:alice:secret")

	if upper.ID() != lower.ID() {
		t.Errorf("host case split one proxy into two IDs: %q vs %q", upper.ID(), lower.ID())
	}
}

func TestProxyID_PreservesCredentialCase(t *testing.T) {
	// Credentials are case-sensitive and must survive verbatim.
	p, _ := ParseLine("1.2.3.4:8080:Alice:SeCreT")

	if got := p.ID(); got != "Alice:SeCreT@1.2.3.4:8080" {
		t.Errorf("ID() = %q, want credentials unchanged", got)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/proxy/ -run TestProxyID -v
```

Expected: FAIL — `p.ID undefined (type Proxy has no field or method ID)`.

**Step 3: Write minimal implementation**

In `internal/proxy/parse.go`, add directly after the `URL()` method (line 23):

```go
// ID returns the canonical identity of a proxy: user:pass@host:port.
//
// The full credential string is the identity, not host:port. Gateway-style
// pools share a single host and port across many session credentials, so
// host:port would collapse hundreds of distinct proxies into one key.
//
// All accepted input formats normalize to this form, so the same proxy
// written differently in two files still joins. The host is lowercased
// because DNS is case-insensitive and a case difference between two files
// would otherwise split one proxy into two join keys; credentials are
// case-sensitive and kept verbatim.
//
// Credentials are assumed not to contain ":" or "@" — ParseLine's grammar
// cannot produce such fields from the colon-delimited formats.
func (p Proxy) ID() string {
	return fmt.Sprintf("%s:%s@%s:%s", p.User, p.Password, strings.ToLower(p.Host), p.Port)
}
```

`fmt` and `strings` are already imported.

**Step 4: Run test to verify it passes**

```bash
go test ./internal/proxy/ -run TestProxyID -v
```

Expected: PASS, both tests.

**Step 5: Commit**

```bash
git add internal/proxy/parse.go internal/proxy/parse_test.go
git commit -m "feat: add canonical Proxy.ID for cross-run joins"
```

---

### Task 2: Run metadata rows

A CSV currently records nothing about itself. Without `Target`, `Workers` and `Proxy file`, a timeline cannot distinguish a real degradation from a changed target or a bumped worker count — it would draw confident, false trends. These three fields are unrecoverable if not written at export time.

**Files:**
- Create: `internal/util/runmeta.go`
- Test: `internal/util/runmeta_test.go`

**Step 1: Write the failing test**

Create `internal/util/runmeta_test.go`:

```go
package util

import (
	"testing"
	"time"
)

func TestRunMeta_Rows(t *testing.T) {
	m := RunMeta{
		Tool:      "pinger",
		RunAt:     time.Date(2026, 9, 19, 14, 32, 7, 0, time.UTC),
		ProxyFile: "/Users/someone/Desktop/Proxy-Toolbox/proxyfiles/residential_de.txt",
		Target:    "https://google.com",
		Workers:   100,
	}

	got := m.Rows()
	want := [][]string{
		{"Tool", "pinger"},
		{"Run at", "2026-09-19T14:32:07Z"},
		{"Proxy file", "residential_de.txt"}, // base name only, not the user's full path
		{"Target", "https://google.com"},
		{"Workers", "100"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i][0] != want[i][0] || got[i][1] != want[i][1] {
			t.Errorf("row %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestRunMeta_RowsZeroValues(t *testing.T) {
	tests := []struct {
		name string
		meta RunMeta
		want [][]string
	}{
		{
			name: "fully zero-valued",
			meta: RunMeta{},
			want: [][]string{
				{"Tool", ""},
				{"Run at", "0001-01-01T00:00:00Z"},
				{"Proxy file", ""}, // not ".", which filepath.Base("") would return
				{"Target", ""},
				{"Workers", "0"},
			},
		},
		{
			name: "iptester has no target",
			meta: RunMeta{
				Tool:      "iptester",
				RunAt:     time.Date(2026, 9, 19, 9, 5, 0, 0, time.UTC),
				ProxyFile: "/Users/someone/Desktop/Proxy-Toolbox/proxyfiles/datacenter_us.txt",
				Workers:   20,
			},
			want: [][]string{
				{"Tool", "iptester"},
				{"Run at", "2026-09-19T09:05:00Z"},
				{"Proxy file", "datacenter_us.txt"},
				{"Target", ""},
				{"Workers", "20"},
			},
		},
		{
			name: "no workers configured",
			meta: RunMeta{
				Tool:    "pinger",
				RunAt:   time.Date(2026, 9, 19, 9, 5, 0, 0, time.UTC),
				Target:  "https://google.com",
				Workers: 0,
			},
			want: [][]string{
				{"Tool", "pinger"},
				{"Run at", "2026-09-19T09:05:00Z"},
				{"Proxy file", ""},
				{"Target", "https://google.com"},
				{"Workers", "0"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.meta.Rows()
			if len(got) != len(tt.want) {
				t.Fatalf("got %d rows, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i][0] != tt.want[i][0] || got[i][1] != tt.want[i][1] {
					t.Errorf("row %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/util/ -run TestRunMeta -v
```

Expected: FAIL — `undefined: RunMeta`.

**Step 3: Write minimal implementation**

Create `internal/util/runmeta.go`:

```go
package util

import (
	"path/filepath"
	"strconv"
	"time"
)

// RunMeta describes the run that produced a results CSV.
//
// These rows make an exported CSV self-describing. Without them the compare
// dashboard cannot tell a genuine change in proxy quality from a changed
// target, a different input file, or a different worker count.
type RunMeta struct {
	Tool      string    // "pinger", "iptester", "speedtester", "bayerntester"
	RunAt     time.Time // when the run started
	ProxyFile string    // path to the proxy file tested; only the base name is written
	Target    string    // domain or URL tested, empty where not applicable
	Workers   int       // size of the worker pool the tool actually used
}

// Rows renders the metadata as leading key/value rows for a results CSV.
//
// Only the base name of ProxyFile is written, to keep absolute paths from the
// user's machine out of exported files.
func (m RunMeta) Rows() [][]string {
	proxyFile := ""
	if m.ProxyFile != "" {
		proxyFile = filepath.Base(m.ProxyFile)
	}
	return [][]string{
		{"Tool", m.Tool},
		{"Run at", m.RunAt.UTC().Format(time.RFC3339)},
		{"Proxy file", proxyFile},
		{"Target", m.Target},
		{"Workers", strconv.Itoa(m.Workers)},
	}
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/util/ -run TestRunMeta -v
```

Expected: PASS, both tests.

**Step 5: Commit**

```bash
git add internal/util/runmeta.go internal/util/runmeta_test.go
git commit -m "feat: add RunMeta rows for self-describing result CSVs"
```

---

### Task 3: Suggested export filename

`PromptExport` takes a `defaultName` argument and ignores it entirely (`internal/util/export.go:18`). Filenames are whatever the user types, so nothing about a CSV is guessable from its name. Wire the argument up.

Enter still means skip — that existing behavior does not change. `.` accepts the suggestion.

**Files:**
- Modify: `internal/util/export.go:18-37`
- Test: `internal/util/export_test.go` (create)

**Step 1: Write the failing test**

Create `internal/util/export_test.go`:

```go
package util

import (
	"testing"
	"time"
)

func TestSuggestedFilename(t *testing.T) {
	tests := []struct {
		name string
		tool string
		at   time.Time
		want string
	}{
		{
			name: "tool name and timestamp",
			tool: "pinger",
			at:   time.Date(2026, 9, 19, 14, 32, 7, 0, time.UTC),
			want: "pinger_2026-09-19_143207.csv",
		},
		{
			name: "empty tool falls back to results",
			tool: "",
			at:   time.Date(2026, 9, 19, 14, 32, 7, 0, time.UTC),
			want: "results_2026-09-19_143207.csv",
		},
		{
			// Fails if any field loses its leading zero.
			name: "zero-padded fields",
			tool: "iptester",
			at:   time.Date(2026, 9, 19, 9, 5, 3, 0, time.UTC),
			want: "iptester_2026-09-19_090503.csv",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := suggestedFilename(tt.tool, tt.at); got != tt.want {
				t.Errorf("suggestedFilename(%q) = %q, want %q", tt.tool, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/util/ -run TestSuggestedFilename -v
```

Expected: FAIL — `undefined: suggestedFilename`.

**Step 3: Write minimal implementation**

In `internal/util/export.go`, add `"time"` to the import block, then add:

```go
// suggestedFilename builds a default export name that identifies the tool and
// the moment of the run, e.g. "pinger_2026-09-19_143207.csv".
//
// Seconds are included deliberately: the dashboard keys each run by its file
// name, and WriteCSV truncates, so a coarser stamp would let two runs of the
// same tool in the same minute silently overwrite one another.
func suggestedFilename(tool string, at time.Time) string {
	stamp := at.Format("2006-01-02_150405")
	if tool == "" {
		return "results_" + stamp + ".csv"
	}
	return tool + "_" + stamp + ".csv"
}
```

Replace the body of `PromptExport` (lines 18-37) with:

```go
func PromptExport(defaultName string) string {
	suggested := suggestedFilename(defaultName, time.Now())

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nSave results to CSV? (Enter to skip, \".\" for %s, or type filename): ", suggested)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if input == "." {
		input = suggested
	}
	if !strings.HasSuffix(strings.ToLower(input), ".csv") {
		input += ".csv"
	}
	input = filepath.Base(input) // strip any directory components (path traversal)

	dir := basedir.Path(resultsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Error creating results/ directory: %v\n", err)
		return ""
	}
	return filepath.Join(dir, input)
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/util/ -v
```

Expected: PASS, all util tests.

**Step 5: Commit**

```bash
git add internal/util/export.go internal/util/export_test.go
git commit -m "feat: suggest a tool- and time-stamped default export filename"
```

---

### Task 4: Wire pinger

Pinger writes `p.Host` — no port, no credentials. For gateway pools that collapses every proxy to one identifier, which is wrong in pinger's own CSV regardless of this feature.

**Files:**
- Modify: `internal/tools/pinger.go` — `pingResult` struct (line 21-27), all `pingResult{...}` constructions (lines 36-83), display and CSV writes (lines 194-244)

**Step 1: Change the result field**

Rename `Host` to `ProxyID` in `pingResult` (line 23) so the compiler finds every use:

```go
type pingResult struct {
	Index   int
	ProxyID string // canonical user:pass@host:port
	Latency time.Duration
	Status  int
	Err     error
}
```

**Step 2: Run the build to list every call site**

```bash
go build ./...
```

Expected: FAIL, one error per `Host:` field in `pingResult{...}` literals.

**Step 3: Fix every call site**

In `pingRawTCP` and the HTTP path, replace each `Host: p.Host` with `ProxyID: p.ID()`. There are eight such literals (lines 36, 45, 51, 55, 58, 64, 79, 83).

Note `pingRawTCP` also builds `proxyAddr := net.JoinHostPort(p.Host, p.Port)` on line 30 — that is the dial address and must **not** change.

**Step 4: Add the shared display helper**

All four tools truncate the identifier with the same `> 22` / `[:19] + "..."` snippet, sized for bare hosts. A `ProxyID` is far longer, and for gateway-style pools — hundreds of proxies sharing one host, differing only in a session credential — tail truncation leaves every row showing the same common prefix. The live terminal view would become useless precisely where the CSV is correct.

Create `internal/util/truncate.go`:

```go
package util

// TruncateID shortens a proxy ID to width characters for terminal display,
// eliding the middle rather than the tail.
//
// A proxy ID is user:pass@host:port. Tail truncation would cut the host and
// port off every row, and for gateway-style pools — where hundreds of proxies
// share one host and differ only in a session credential — it can leave every
// row showing the same common prefix. Keeping both ends preserves the
// credential prefix and the host:port, which is the most that fits.
//
// Widths below 4 leave no room to elide, so the ID is cut plainly.
func TruncateID(id string, width int) string {
	runes := []rune(id)
	if width <= 0 || len(runes) <= width {
		return id
	}
	if width < 4 {
		return string(runes[:width])
	}
	const ellipsis = "..."
	remaining := width - len(ellipsis)
	head := (remaining + 1) / 2
	tail := remaining - head
	return string(runes[:head]) + ellipsis + string(runes[len(runes)-tail:])
}
```

Operate on runes, not bytes — byte slicing would split a multi-byte character.

Test it in `internal/util/truncate_test.go`, table-driven, covering: shorter than width and exactly width (returned unchanged); longer than width (result is exactly `width` runes, keeps head and tail); width 0 and negative width (no panic); width below 4 (no panic, no stray ellipsis); `width == 4` with a longer input, the `tail == 0` boundary; a multi-byte credential (no replacement characters, still `width` runes).

The load-bearing test is the one guarding against a regression to tail truncation. Get its construction right, because the obvious version pins nothing: at width 44 a tail cut still keeps 41 characters, so two IDs that differ anywhere in their first 41 characters stay distinct under *either* strategy. The shared prefix must exceed `width - len("...")` and the IDs must differ **only at the end**:

```go
// The first 41 characters are identical, so a head-only cut would render
// these two proxies the same. Middle elision keeps the tail, so the
// differing port still shows.
const shared = "customer-acme-region-eu-west-rotating-residential-session-x:tok@gw.provider.com:"
first := shared + "7777"
second := shared + "8888"
```

Assert that the two IDs really do share more than `width - 3` leading characters (otherwise the test silently stops testing anything), that their truncations differ, and that each result ends with its own port. Verify the test is genuinely load-bearing by temporarily replacing `TruncateID`'s body with a naive tail cut and watching it fail.

Add a companion test documenting the accepted limitation: two IDs sharing head **and** tail, differing only in the elided middle, truncate to the **same** string. That is fine — the `#` column identifies rows and the CSV carries the untruncated ID — but it must be written down so nobody later "fixes" it or assumes distinctness is guaranteed.

**Step 5: Update display and CSV**

The terminal column header (line 156) becomes `"Proxy"`, widened from `%-24s` to `%-44s`:

```go
fmt.Printf("%-5s  %-44s  %-10s  %s\n", "#", "Proxy", "Latency", "Status")
```

All four tools live in `package tools`, so the widths belong in one shared file rather than four colliding per-file declarations. Create `internal/tools/display.go`:

```go
package tools

// Terminal column widths for the result tables.
//
// A proxy ID is user:pass@host:port, far wider than the bare hosts these
// tables were originally sized for, so the proxy column is wide and IDs are
// elided in the middle by util.TruncateID rather than cut at the tail.
//
// pinger, speedtester and bayerntester share one layout: an index, the proxy,
// a latency, and a status. iptester carries an extra Exit IP column, so its
// proxy column is narrower to keep the line within a standard terminal.
//
// Each table width is the rendered length of its own header plus a small
// margin, so the separator rule always spans the table.
const (
	proxyColWidth = 44
	tableWidth    = 73

	ipProxyColWidth = 36
	ipTableWidth    = 74
)
```

Tasks 5-7 use these identifiers and declare nothing of their own.

At line 194, the truncation becomes a call to the helper:

```go
display := util.TruncateID(r.ProxyID, proxyColWidth)
```

Update **every** row-rendering `fmt.Printf` in the file from `%-24s` to `%-44s` — the error row and the success row are formatted separately — so the columns still line up.

Also compute the ID once per call rather than at each return point: `pingRawTCP` calls `p.ID()` at five returns and `pingHTTP` at two. Add `id := p.ID()` at function entry and use `ProxyID: id` in each literal.

Both CSV row appends (lines 202 and 219) use `r.ProxyID` in place of `r.Host`, and the detail header (line 244) becomes:

```go
summary = append(summary, []string{"#", "Proxy", "Latency", "Status", "Error"})
```

**Step 6: Prepend the metadata rows**

Inside the `if path := util.PromptExport("pinger"); path != ""` block, the summary must start with the metadata. Replace `var summary [][]string` with:

```go
meta := util.RunMeta{
	Tool:      "pinger",
	RunAt:     start,
	ProxyFile: filePath,
	Target:    target,
	Workers:   cfg.Workers,
}
summary := meta.Rows()
```

All four are already in scope: `target` holds the resolved domain (config value or user input, `pinger.go:136-144`), `start` is the measurement start set after the prompts, and `filePath` and `cfg` come from the top of `RunPinger`.

**Step 7: Build and run a manual check**

```bash
go build ./... && go vet ./internal/tools/
```

Expected: clean.

Then run the tool against a small proxy file, export with `.`, and confirm the CSV opens with the five metadata rows and that the `Proxy` column holds `user:pass@host:port`.

**Step 8: Commit**

```bash
git add internal/tools/pinger.go internal/util/truncate.go internal/util/truncate_test.go
git commit -m "feat: emit canonical proxy ID and run metadata from pinger"
```

---

### Task 5: Wire iptester (adds a per-proxy detail table)

IP tester currently exports a summary plus a "Repeated IP / Times / Lines" table and **no per-proxy rows**. The cross-tool matrix and exit-IP enrichment both need one row per proxy, so this task adds that table alongside the existing one.

**Files:**
- Modify: `internal/tools/iptester.go` — `ipResult` struct (lines 25-31), constructions, CSV block (lines 199-219)

**Step 1: Change the result field**

```go
type ipResult struct {
	Index   int
	ProxyID string // canonical user:pass@host:port
	IP      string
	Elapsed time.Duration
	Err     error
}
```

**Step 2: Build to find call sites, then fix them**

```bash
go build ./...
```

Replace every `Host: p.Host` with `ProxyID: p.ID()` in `checkIP`.

For the display, use the `util.TruncateID` helper and the shared `ipProxyColWidth` from `display.go` rather than the old `> 22` / `[:19]` snippet at lines 136-139. iptester carries an extra 18-character Exit IP column, so its proxy column is narrower than pinger's:

```go
display := util.TruncateID(r.ProxyID, ipProxyColWidth)
```

Change the header on line 101 from `"Host"` to `"Proxy"`, update every row-rendering `fmt.Printf` in the file from `%-24s` to `%-36s`, and replace both `strings.Repeat("-", 65)` separator rules with `ipTableWidth`.

**Step 3: Prepend metadata and append the detail table**

In the export block, replace `var csvRows [][]string` (line 199) with the metadata:

```go
meta := util.RunMeta{
	Tool:      "iptester",
	RunAt:     start,
	ProxyFile: filePath,
	Target:    "", // iptester has no target; it reports exit IPs
	Workers:   cfg.Workers,
}
csvRows := meta.Rows()
```

Keep the existing summary rows and the repeated-IP table as they are. After the repeated-IP table, append a blank separator and the per-proxy detail table:

```go
// Per-proxy detail. Needed so the compare dashboard can join this run
// against runs from other tools and enrich them with exit IPs.
csvRows = append(csvRows, []string{"", ""})
csvRows = append(csvRows, []string{"#", "Proxy", "Exit IP", "Latency", "Error"})
for _, r := range results {
	errStr := ""
	if r.Err != nil {
		errStr = r.Err.Error()
	}
	latStr := ""
	if r.Err == nil {
		latStr = fmt.Sprintf("%dms", r.Elapsed.Milliseconds())
	}
	csvRows = append(csvRows, []string{
		strconv.Itoa(r.Index + 1), r.ProxyID, r.IP, latStr, errStr,
	})
}
```

Confirm the identifier holding the collected results in that scope and use it in place of `results`; `strconv` is already imported.

**Step 4: Build and verify**

```bash
go build ./... && go vet ./internal/tools/
```

Run the tool and confirm the exported CSV contains three sections: summary (with metadata), repeated IPs, and per-proxy detail.

**Step 5: Commit**

```bash
git add internal/tools/iptester.go
git commit -m "feat: emit per-proxy detail table and run metadata from iptester"
```

---

### Task 6: Wire speedtester

Writes `p.URL()`, which is `http://user:pass@host:port`. Switching to `ID()` drops the scheme so it matches the canonical form.

**Files:**
- Modify: `internal/tools/speedtester.go:46-87` (call sites), `:157` (caller), `:177-261` (CSV block)

**Step 1: Take the proxy value, not raw strings**

`testSingleProxy` takes `proxyURL` and stores it in `speedResult.Proxy`. It needs the dial URL *and* the identity, which differ — the dialer requires the `http://` scheme, the CSV must not have it.

Do **not** add two string parameters for this. Two adjacent, same-typed, similar-looking URL arguments invite a swap whose failure mode is silent: it compiles, the tests pass, and every request fails at runtime while the CSV fills with dial URLs. `pingRawTCP(index int, p proxy.Proxy, host string)` already shows the right idiom — take the value, derive both inside:

```go
func testSingleProxy(index int, p proxy.Proxy, targetURL string) speedResult {
	proxyURL := p.URL() // dial address, keeps the http:// scheme
	id := p.ID()        // canonical identity recorded in results
	...
}
```

Rename the result field to match the siblings, which both call this concept `ProxyID`:

```go
ProxyID string // canonical user:pass@host:port
```

Every `Proxy: proxyURL` becomes `ProxyID: id` (lines 56, 61, 84, 87), and every reader updates — the CSV row appends, the display call, and the `savedLines`/`passLatency` path. The compiler finds them all.

Both callers get shorter:
- `internal/tools/speedtester.go:157` → `testSingleProxy(i, proxies[i], region.URL)`
- `internal/tools/bayerntester.go:67` → `testSingleProxy(i, proxies[i], bayernURL)`

**Then delete the `r.Proxy = proxies[i].Host` override** in each tool's worker loop — one in speedtester, one at `bayerntester.go:68`. These assignments overwrite the identifier immediately after `testSingleProxy` returns it, so leaving either in place silently defeats the whole task: the CSV would still carry bare hosts. They are easy to miss because nothing about them fails to compile.

**Rename the CSV detail header's third column from `"Speed"` to `"Latency"`** in both tools:

```go
summary = append(summary, []string{"#", "Proxy", "Latency", "Status", "Error"})
```

Task 9's parser resolves latency by column name. With `"Speed"` in the header every speedtester and bayerntester row would parse as `LatencyMs: 0`, and the dashboard would render those latencies as zero with no error raised. All four tools must emit the same column name.

Leave the **terminal** header reading `"Speed"` — that is user-facing text with no contract attached.

**Step 2: Prepend metadata**

In the export block, replace `var summary [][]string` with:

```go
meta := util.RunMeta{
	Tool:      "speedtester",
	RunAt:     start,
	ProxyFile: filePath,
	Target:    region.URL,
	Workers:   workers, // the capped pool, not cfg.Workers
}
summary := meta.Rows()
```

**Step 3: Update the display**

speedtester already displays `r.Proxy`, which today holds `http://user:pass@host:port`, so it already truncates the wrong part — this is a pre-existing bug the helper fixes, not one introduced here. Replace the `> 22` / `[:19]` snippet at lines 182-185 with the Task 4 helper:

```go
display := util.TruncateID(r.Proxy, proxyColWidth)
```

`proxyColWidth` comes from the shared `display.go`; declare nothing here.

Change the header on line 140 from `"Host"` to `"Proxy"`, update all four row-rendering `fmt.Printf` calls (lines 140, 188, 194, 200) from `%-24s` to `%-44s`, and replace the bare separator-rule literals with `tableWidth`.

**Step 4: Align the export filename prefix**

`internal/tools/speedtester.go:239` currently calls `util.PromptExport("speedtest")`. Now that Task 3 made that argument the suggested filename, the prefix would read `speedtest_...csv` while the file's own `Tool` row says `speedtester`. Change it to match:

```go
if path := util.PromptExport("speedtester"); path != "" {
```

**Step 5: Build**

```bash
go build ./... && go vet ./internal/tools/
```

Expected: clean. Note this task leaves `bayerntester.go` compiling because its call site was updated in step 1; its metadata comes in Task 7.

**Step 6: Commit**

```bash
git add internal/tools/speedtester.go internal/tools/bayerntester.go
git commit -m "feat: emit canonical proxy ID and run metadata from speedtester"
```

---

### Task 7: Wire bayerntester

**Files:**
- Modify: `internal/tools/bayerntester.go:87-171`

**Step 1: Prepend metadata**

In the export block, replace `var summary [][]string` with:

```go
meta := util.RunMeta{
	Tool:      "bayerntester",
	RunAt:     start,
	ProxyFile: filePath,
	Target:    bayernURL,
	Workers:   workers, // the capped pool, not cfg.Workers
}
summary := meta.Rows()
```

Confirm a `start` time exists in scope; if not, add `start := time.Now()` before the worker pool and reuse it for the elapsed calculation rather than introducing a second clock.

**Step 2: Update the display**

Same pre-existing truncation bug as speedtester. Replace the snippet at lines 92-95 with the Task 4 helper:

```go
display := util.TruncateID(r.Proxy, proxyColWidth)
```

`proxyColWidth` comes from the shared `display.go`; declare nothing here.

Change the header on line 50 from `"Host"` to `"Proxy"`, update all four row-rendering `fmt.Printf` calls (lines 50, 98, 104, 110) from `%-24s` to `%-44s`, and replace the bare separator-rule literals with `tableWidth`.

**Step 3: Align the export filename prefix**

`internal/tools/bayerntester.go:149` currently calls `util.PromptExport("bayerntest")`. Change it to match the `Tool` value:

```go
if path := util.PromptExport("bayerntester"); path != "" {
```

**Step 4: Confirm the detail header and the override**

Task 6 already renamed the CSV detail header to `{"#", "Proxy", "Latency", "Status", "Error"}` and deleted the `r.Proxy = proxies[i].Host` override at `bayerntester.go:68`. Verify both, rather than assuming — if the override survived, this tool still exports bare hosts and the display work in Step 2 operates on the wrong value.

**Step 5: Build and verify all four tools**

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: clean, all existing tests pass.

**Step 6: Commit**

```bash
git add internal/tools/bayerntester.go
git commit -m "feat: emit run metadata from bayerntester"
```

---

## Phase 2 — Parse layer

Reads the CSVs back into structs. Pure functions, no I/O beyond reading a file.

---

### Task 8: Run types

**Files:**
- Create: `internal/compare/types.go`

**Step 1: Write the types**

```go
// Package compare parses exported result CSVs and computes comparison
// statistics across runs and across tools.
package compare

import "time"

// Outcome classifies what happened to one proxy in one run.
type Outcome string

const (
	OutcomeOK      Outcome = "ok"      // request succeeded
	OutcomeBlocked Outcome = "blocked" // reached the target, was refused (403, 429)
	OutcomeError   Outcome = "error"   // never got a usable response
)

// Meta is the self-describing header of a run, recovered from the CSV.
// Fields absent from older exports are left zero and rendered as "unknown".
type Meta struct {
	Tool      string
	RunAt     time.Time
	ProxyFile string
	Target    string
	Workers   int
}

// ProxyResult is one proxy's outcome within one run.
type ProxyResult struct {
	ProxyID   string  // canonical user:pass@host:port; the join key
	LatencyMs int     // 0 when there is no latency (errors)
	Status    int     // HTTP status, 0 when not applicable
	Outcome   Outcome
	ErrorRaw  string  // verbatim error text from the CSV
	ErrorKind string  // taxonomy bucket, see classifyError
	ExitIP    string  // iptester only, empty elsewhere
}

// Run is one parsed CSV.
type Run struct {
	File    string // base name, e.g. "pinger_2026-09-19_143207.csv"
	Meta    Meta
	Results []ProxyResult
}

// HasMeta reports whether the run carried metadata rows. Runs exported before
// metadata existed parse fine but cannot be placed on a timeline or checked
// for comparability.
func (r Run) HasMeta() bool {
	return r.Meta.Tool != "" && !r.Meta.RunAt.IsZero()
}
```

**Step 2: Build**

```bash
go build ./internal/compare/
```

Expected: clean.

**Step 3: Commit**

```bash
git add internal/compare/types.go
git commit -m "feat: add compare package run types"
```

---

### Task 9: CSV parser

The exported CSV is several tables in one file, separated by all-empty rows. The parser splits into sections, reads the leading key/value section as metadata, and identifies detail tables by their header row. Unknown sections are skipped rather than treated as errors, so the repeated-IP table and any future section cost nothing.

**Files:**
- Create: `internal/compare/parse.go`
- Create: `internal/compare/testdata/pinger_full.csv`
- Create: `internal/compare/testdata/legacy_no_meta.csv`
- Test: `internal/compare/parse_test.go`

**Step 1: Write the fixtures**

`internal/compare/testdata/pinger_full.csv`:

```csv
Summary,Value
Tool,pinger
Run at,2026-09-19T14:32:07Z
Proxy file,residential_de.txt
Target,https://google.com
Workers,100
Proxies tested,4
Successful,2
Errors,2
Total time,1.2s
Average latency,340ms
,
#,Proxy,Latency,Status,Error
1,alice:pw@1.2.3.4:8080,300ms,HTTP 200,
2,bob:pw@1.2.3.5:8080,380ms,HTTP 403,
3,carol:pw@1.2.3.6:8080,,ERROR,dial tcp 1.2.3.6:8080: i/o timeout
4,dave:pw@1.2.3.7:8080,,ERROR,proxy rejected: HTTP/1.1 407 Proxy Authentication Required
5,eve:pw@1.2.3.8:8080,90ms,HTTP 200,unexpected EOF after headers
```

Row 5 is deliberately contradictory: a success-looking status alongside a
non-empty error. Without it the `Error`-overrides-`Status` branch is exercised
only on iptester's no-Status path, because every other errored row already
carries the literal status `ERROR`, which `classifyStatus` resolves on its own.
A mutation narrowing that override to `if !hasStatus && errRaw != ""` would
otherwise pass the entire suite. The shape is real — a tool can record a status
from a partially-read response and still fail afterwards. Keep the summary
counts consistent with the rows: 5 tested, 2 successful, 3 errors, average
latency 340ms across the two OK rows.

**Tasks 10-14 note:** under the parser this fixture resolves to **1 OK, 1
blocked, 3 error**, so `okLatencies` yields exactly `[300]` and `Summarize`
reports a mean of 300 — deliberately *not* the 340ms the tool's own
`Average latency` row prints. The tool counts a 403 as a success because it saw
no Go error; the parser separates blocked from OK. The dashboard recomputes
everything from the detail rows, so the summary rows are context, not input.
Row 5 carries a non-zero latency *and* `OutcomeError`. Any stats test written
against it must use those numbers, and the row forces an explicit decision about
whether errored rows contribute to latency distributions. `okLatencies` filters
on `OutcomeOK`, so they do not.

`internal/compare/testdata/iptester_full.csv` — note the three sections and the absent Status column:

```csv
Summary,Value
Tool,iptester
Run at,2026-09-19T15:10:00Z
Proxy file,residential_de.txt
Target,
Workers,50
Proxies tested,3
Errors,1
Unique IPs,2 / 2
Total time,0.8s
,
Repeated IP,Times,Lines
9.9.9.1,2,"1, 2"
,
#,Proxy,Exit IP,Latency,Error
1,alice:pw@1.2.3.4:8080,9.9.9.1,120ms,
2,bob:pw@1.2.3.5:8080,9.9.9.1,140ms,
3,carol:pw@1.2.3.6:8080,,,dial tcp: i/o timeout
```

The repeated-IP section carries a real data row deliberately. With only a header
there, nothing exists that could leak into `Results`, so a test asserting the
section is skipped could not fail however broken the classifier was. Proxies 1
and 2 therefore share exit IP `9.9.9.1`, and `Unique IPs` reads `1 / 2` to stay
consistent with what iptester would actually write.

**Task 14 note:** this fixture has a duplicated exit IP. Subnet-grouping or
uniqueness tests needing two distinct exit IPs must use their own fixture.

`internal/compare/testdata/legacy_no_meta.csv` — an export from before Phase 1:

```csv
Summary,Value
Proxies tested,2
Successful,1
Errors,1
Total time,0.5s
,
#,Host,Latency,Status,Error
1,1.2.3.4,300ms,OK,
2,1.2.3.5,,ERROR,connection refused
```

**Step 2: Write the failing test**

Create `internal/compare/parse_test.go`:

```go
package compare

import (
	"testing"
	"time"
)

func TestParseFile_ReadsMetadata(t *testing.T) {
	run, err := ParseFile("testdata/pinger_full.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if run.Meta.Tool != "pinger" {
		t.Errorf("Tool = %q, want %q", run.Meta.Tool, "pinger")
	}
	want := time.Date(2026, 9, 19, 14, 32, 7, 0, time.UTC)
	if !run.Meta.RunAt.Equal(want) {
		t.Errorf("RunAt = %v, want %v", run.Meta.RunAt, want)
	}
	if run.Meta.Workers != 100 {
		t.Errorf("Workers = %d, want 100", run.Meta.Workers)
	}
	if run.Meta.Target != "https://google.com" {
		t.Errorf("Target = %q, want %q", run.Meta.Target, "https://google.com")
	}
}

func TestParseFile_ReadsProxyRows(t *testing.T) {
	run, _ := ParseFile("testdata/pinger_full.csv")

	if len(run.Results) != 4 {
		t.Fatalf("got %d results, want 4", len(run.Results))
	}

	first := run.Results[0]
	if first.ProxyID != "alice:pw@1.2.3.4:8080" {
		t.Errorf("ProxyID = %q", first.ProxyID)
	}
	if first.LatencyMs != 300 {
		t.Errorf("LatencyMs = %d, want 300", first.LatencyMs)
	}
	if first.Status != 200 {
		t.Errorf("Status = %d, want 200", first.Status)
	}
	if first.Outcome != OutcomeOK {
		t.Errorf("Outcome = %q, want %q", first.Outcome, OutcomeOK)
	}
}

func TestParseFile_ClassifiesOutcomes(t *testing.T) {
	run, _ := ParseFile("testdata/pinger_full.csv")

	want := []Outcome{OutcomeOK, OutcomeBlocked, OutcomeError, OutcomeError}
	for i, w := range want {
		if got := run.Results[i].Outcome; got != w {
			t.Errorf("result %d outcome = %q, want %q", i, got, w)
		}
	}
}

func TestClassifyStatus_AllToolVocabularies(t *testing.T) {
	// The four tools spell success three different ways. Any of them parsing
	// as an error would silently mis-bucket whole runs.
	tests := []struct {
		name        string
		in          string
		wantCode    int
		wantOutcome Outcome
	}{
		{"pinger bare OK", "OK", 0, OutcomeOK},
		{"pinger with code", "HTTP 200", 200, OutcomeOK},
		{"speedtester and bayerntester", "200 OK", 200, OutcomeOK},
		{"blocked", "403 BLOCKED", 403, OutcomeBlocked},
		{"rate limited", "429 BLOCKED", 429, OutcomeBlocked},
		{"explicit error", "ERROR", 0, OutcomeError},
		{"server error", "HTTP 503", 503, OutcomeError},
		{"absent", "", 0, OutcomeError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, outcome := classifyStatus(tt.in)
			if code != tt.wantCode || outcome != tt.wantOutcome {
				t.Errorf("classifyStatus(%q) = (%d, %q), want (%d, %q)",
					tt.in, code, outcome, tt.wantCode, tt.wantOutcome)
			}
		})
	}
}

func TestParseFile_IPTesterHasNoStatusColumn(t *testing.T) {
	// iptester reports exit IPs and emits no Status column. Success must be
	// inferred from an empty Error, or every successful proxy reads as failed.
	run, err := ParseFile("testdata/iptester_full.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(run.Results) != 3 {
		t.Fatalf("got %d results, want 3", len(run.Results))
	}

	want := []Outcome{OutcomeOK, OutcomeOK, OutcomeError}
	for i, w := range want {
		if got := run.Results[i].Outcome; got != w {
			t.Errorf("result %d outcome = %q, want %q", i, got, w)
		}
	}
	if run.Results[0].ExitIP != "9.9.9.1" {
		t.Errorf("ExitIP = %q, want %q", run.Results[0].ExitIP, "9.9.9.1")
	}
	if run.Results[0].LatencyMs != 120 {
		t.Errorf("LatencyMs = %d, want 120", run.Results[0].LatencyMs)
	}
}

func TestParseFile_SkipsTheRepeatedIPSection(t *testing.T) {
	// The repeated-IP table sits between the summary and the detail table and
	// must be ignored, not parsed as proxy rows.
	run, _ := ParseFile("testdata/iptester_full.csv")

	for _, r := range run.Results {
		if r.ProxyID == "" {
			t.Errorf("a repeated-IP row leaked into Results: %+v", r)
		}
	}
}

func TestParseFile_LegacyExportWithoutMetadata(t *testing.T) {
	run, err := ParseFile("testdata/legacy_no_meta.csv")
	if err != nil {
		t.Fatalf("legacy CSV must parse, got error: %v", err)
	}

	if run.HasMeta() {
		t.Error("HasMeta = true, want false for a legacy export")
	}
	if len(run.Results) != 2 {
		t.Errorf("got %d results, want 2", len(run.Results))
	}
	// The old Host column is still the identifier, even though it is not
	// a canonical ID. It simply will not join against newer runs.
	if run.Results[0].ProxyID != "1.2.3.4" {
		t.Errorf("ProxyID = %q, want %q", run.Results[0].ProxyID, "1.2.3.4")
	}
}
```

**Step 3: Run test to verify it fails**

```bash
go test ./internal/compare/ -v
```

Expected: FAIL — `undefined: ParseFile`.

**Step 4: Write the implementation**

Create `internal/compare/parse.go`:

```go
package compare

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ParseFile reads one exported results CSV.
//
// The file holds several tables separated by all-empty rows: a leading
// key/value summary, then one or more detail tables each introduced by their
// own header row. Unknown tables are skipped, so sections this parser does not
// model (the repeated-IP table, anything added later) are harmless.
func ParseFile(path string) (Run, error) {
	f, err := os.Open(path)
	if err != nil {
		return Run{}, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // rows are ragged by design
	records, err := r.ReadAll()
	if err != nil {
		return Run{}, err
	}

	run := Run{File: filepath.Base(path)}
	for _, section := range splitSections(records) {
		if len(section) == 0 {
			continue
		}
		switch {
		case isSummary(section[0]):
			run.Meta = parseMeta(section[1:])
		case isProxyDetail(section[0]):
			// Append rather than assign. Assignment silently drops all but the
			// last detail table, and it also masks a misclassified section when
			// a correct one follows — which would let isProxyDetail be broken
			// while the parser still looked right.
			run.Results = append(run.Results, parseProxyRows(section[0], section[1:])...)
		}
	}
	return run, nil
}

// splitSections breaks the record list on rows whose fields are all empty.
func splitSections(records [][]string) [][][]string {
	var sections [][][]string
	var current [][]string
	for _, rec := range records {
		if isBlank(rec) {
			if len(current) > 0 {
				sections = append(sections, current)
				current = nil
			}
			continue
		}
		current = append(current, rec)
	}
	if len(current) > 0 {
		sections = append(sections, current)
	}
	return sections
}

func isBlank(rec []string) bool {
	for _, f := range rec {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}

func isSummary(header []string) bool {
	return len(header) >= 1 && strings.EqualFold(strings.TrimSpace(header[0]), "Summary")
}

// isProxyDetail matches the per-proxy tables written by every tool. Their
// headers start with "#" followed by the identifier column.
func isProxyDetail(header []string) bool {
	if len(header) < 2 || strings.TrimSpace(header[0]) != "#" {
		return false
	}
	switch strings.TrimSpace(header[1]) {
	case "Proxy", "Host":
		return true
	}
	return false
}

func parseMeta(rows [][]string) Meta {
	var m Meta
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		key := strings.TrimSpace(row[0])
		val := strings.TrimSpace(row[1])
		switch key {
		case "Tool":
			m.Tool = val
		case "Run at":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				m.RunAt = t
			}
		case "Proxy file":
			m.ProxyFile = val
		case "Target":
			m.Target = val
		case "Workers":
			m.Workers, _ = strconv.Atoi(val)
		}
	}
	return m
}

// parseProxyRows reads a detail table. Column positions vary by tool, so
// columns are located by header name.
func parseProxyRows(header []string, rows [][]string) []ProxyResult {
	col := make(map[string]int, len(header))
	for i, name := range header {
		col[strings.TrimSpace(name)] = i
	}

	idIdx, ok := col["Proxy"]
	if !ok {
		idIdx = col["Host"] // legacy exports
	}

	get := func(row []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	// iptester's detail table has no Status column at all: a proxy succeeded if
	// it reported an exit IP and no error. Without this, classifyStatus("")
	// would mark every successful iptester row as an error and the dashboard
	// would show those runs as total failures, silently.
	_, hasStatus := col["Status"]

	results := make([]ProxyResult, 0, len(rows))
	for _, row := range rows {
		if idIdx >= len(row) {
			continue
		}
		status, outcome := 0, OutcomeOK
		if hasStatus {
			status, outcome = classifyStatus(get(row, "Status"))
		}
		errRaw := get(row, "Error")
		if errRaw != "" {
			outcome = OutcomeError
		}
		results = append(results, ProxyResult{
			ProxyID:   strings.TrimSpace(row[idIdx]),
			LatencyMs: parseLatencyMs(get(row, "Latency")),
			Status:    status,
			Outcome:   outcome,
			ErrorRaw:  errRaw,
			ErrorKind: classifyError(errRaw),
			ExitIP:    get(row, "Exit IP"),
		})
	}
	return results
}

// parseLatencyMs reads the "340ms" form the tools write. Anything else is 0.
func parseLatencyMs(s string) int {
	s = strings.TrimSuffix(strings.TrimSpace(s), "ms")
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// classifyStatus reads the status strings the tools write: "OK", "HTTP 200",
// "200 OK", "403 BLOCKED", "ERROR".
func classifyStatus(s string) (int, Outcome) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "ERROR") {
		return 0, OutcomeError
	}
	if strings.EqualFold(s, "OK") {
		return 0, OutcomeOK
	}

	code := 0
	for _, field := range strings.Fields(s) {
		if n, err := strconv.Atoi(field); err == nil && n >= 100 && n < 600 {
			code = n
			break
		}
	}
	switch {
	case code == 0:
		return 0, OutcomeError
	case code == 403 || code == 429:
		return code, OutcomeBlocked
	case code >= 200 && code < 400:
		return code, OutcomeOK
	default:
		return code, OutcomeError
	}
}
```

`classifyError` arrives in Task 12; add a temporary stub so this task compiles and is committed green:

```go
func classifyError(raw string) string { return "" }
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/compare/ -v
```

Expected: PASS, all four tests.

**Step 6: Commit**

```bash
git add internal/compare/
git commit -m "feat: parse exported result CSVs into run structs"
```

---

## Phase 3 — Stats layer

Pure functions over `[]ProxyResult` and `[]Run`. No dependencies.

---

### Task 10: Distribution statistics

The tools report an average latency, which is the least useful summary of a heavy-tailed distribution. Percentiles replace it.

**Files:**
- Create: `internal/compare/stats.go`
- Test: `internal/compare/stats_test.go`

**Step 1: Write the failing test**

```go
package compare

import (
	"math"
	"testing"
)

func TestPercentile(t *testing.T) {
	// Sorted: 10 20 30 40 50 60 70 80 90 100
	in := []int{50, 20, 90, 10, 70, 30, 100, 40, 80, 60}

	tests := []struct {
		p    float64
		want int
	}{
		{0.0, 10},
		{0.5, 50},  // nearest-rank: ceil(0.5*10) = 5th value = 50
		{0.95, 100},
		{1.0, 100},
	}

	for _, tt := range tests {
		if got := Percentile(in, tt.p); got != tt.want {
			t.Errorf("Percentile(p=%v) = %d, want %d", tt.p, got, tt.want)
		}
	}
}

func TestPercentile_EmptyInputIsZero(t *testing.T) {
	if got := Percentile(nil, 0.5); got != 0 {
		t.Errorf("Percentile(nil) = %d, want 0", got)
	}
}

func TestPercentile_DoesNotMutateInput(t *testing.T) {
	in := []int{3, 1, 2}
	Percentile(in, 0.5)

	if in[0] != 3 || in[1] != 1 || in[2] != 2 {
		t.Errorf("input mutated: %v", in)
	}
}

func TestSummarize(t *testing.T) {
	results := []ProxyResult{
		{LatencyMs: 100, Outcome: OutcomeOK},
		{LatencyMs: 200, Outcome: OutcomeOK},
		{LatencyMs: 300, Outcome: OutcomeOK},
		{LatencyMs: 0, Outcome: OutcomeBlocked},
		{LatencyMs: 0, Outcome: OutcomeError},
	}

	s := Summarize(results)

	if s.Total != 5 {
		t.Errorf("Total = %d, want 5", s.Total)
	}
	if s.OK != 3 || s.Blocked != 1 || s.Errors != 1 {
		t.Errorf("counts = ok:%d blocked:%d err:%d, want 3/1/1", s.OK, s.Blocked, s.Errors)
	}
	if s.P50 != 200 {
		t.Errorf("P50 = %d, want 200 (errors and blocks excluded)", s.P50)
	}
	if math.Abs(s.SuccessRate-0.6) > 1e-9 {
		t.Errorf("SuccessRate = %v, want 0.6", s.SuccessRate)
	}
}
```

**Step 2: Run to verify it fails**

```bash
go test ./internal/compare/ -run 'TestPercentile|TestSummarize' -v
```

Expected: FAIL — `undefined: Percentile`.

**Step 3: Implement**

Create `internal/compare/stats.go`:

```go
package compare

import (
	"math"
	"sort"
)

// Summary is the headline distribution of one run.
type Summary struct {
	Total       int     `json:"total"`
	OK          int     `json:"ok"`
	Blocked     int     `json:"blocked"`
	Errors      int     `json:"errors"`
	SuccessRate float64 `json:"successRate"`
	// LatencyCount is how many samples back the percentile fields below. It is
	// not the same as OK: okLatencies drops OK rows reporting 0ms, so a run can
	// have successes but no latency samples. Without this the serve layer
	// cannot tell "nothing to plot" from "genuinely all zero".
	LatencyCount int  `json:"latencyCount"`
	Min          int  `json:"min"`
	P25          int  `json:"p25"`
	P50          int  `json:"p50"`
	P75         int     `json:"p75"`
	P90         int     `json:"p90"`
	P95         int     `json:"p95"`
	P99         int     `json:"p99"`
	Max         int     `json:"max"`
	Mean        float64 `json:"mean"`
	StdDev      float64 `json:"stdDev"`
	IQR         int     `json:"iqr"`
}

// Percentile returns the nearest-rank percentile of values. p is in [0,1].
// The input is not modified.
func Percentile(values []int, p float64) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)

	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	rank := int(math.Ceil(p * float64(len(sorted))))
	return sorted[rank-1]
}

// okLatencies collects latencies from successful results only. Errors carry no
// latency and blocked requests measure rejection speed, not proxy speed, so
// including either would distort the distribution.
func okLatencies(results []ProxyResult) []int {
	var out []int
	for _, r := range results {
		if r.Outcome == OutcomeOK && r.LatencyMs > 0 {
			out = append(out, r.LatencyMs)
		}
	}
	return out
}

// Summarize computes the headline statistics for a set of results.
func Summarize(results []ProxyResult) Summary {
	s := Summary{Total: len(results)}
	for _, r := range results {
		switch r.Outcome {
		case OutcomeOK:
			s.OK++
		case OutcomeBlocked:
			s.Blocked++
		case OutcomeError:
			s.Errors++
		}
	}
	if s.Total > 0 {
		s.SuccessRate = float64(s.OK) / float64(s.Total)
	}

	lat := okLatencies(results)
	if len(lat) == 0 {
		return s
	}

	s.LatencyCount = len(lat)
	s.Min = Percentile(lat, 0)
	s.P25 = Percentile(lat, 0.25)
	s.P50 = Percentile(lat, 0.50)
	s.P75 = Percentile(lat, 0.75)
	s.P90 = Percentile(lat, 0.90)
	s.P95 = Percentile(lat, 0.95)
	s.P99 = Percentile(lat, 0.99)
	s.Max = Percentile(lat, 1)
	s.IQR = s.P75 - s.P25

	sum := 0
	for _, v := range lat {
		sum += v
	}
	s.Mean = float64(sum) / float64(len(lat))

	variance := 0.0
	for _, v := range lat {
		d := float64(v) - s.Mean
		variance += d * d
	}
	s.StdDev = math.Sqrt(variance / float64(len(lat)))

	return s
}
```

**Step 4: Run to verify it passes**

```bash
go test ./internal/compare/ -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/compare/stats.go internal/compare/stats_test.go
git commit -m "feat: add percentile and run summary statistics"
```

---

### Task 11: Histogram and ECDF

The ECDF overlay is the view that answers "is A faster than B" honestly — a curve to the left is faster at *every* percentile, which an average cannot show.

**Files:**
- Create: `internal/compare/distribution.go`
- Test: `internal/compare/distribution_test.go`

**Step 1: Write the failing test**

```go
package compare

import "testing"

func TestHistogram(t *testing.T) {
	values := []int{5, 15, 25, 25, 95}

	h := Histogram(values, 10)

	if len(h.Buckets) != 10 {
		t.Fatalf("got %d buckets, want 10", len(h.Buckets))
	}
	if h.Buckets[2].Count != 2 {
		t.Errorf("bucket 2 count = %d, want 2", h.Buckets[2].Count)
	}
	if h.Buckets[9].Count != 1 {
		t.Errorf("bucket 9 (max value) count = %d, want 1", h.Buckets[9].Count)
	}
}

func TestHistogram_EmptyInput(t *testing.T) {
	h := Histogram(nil, 10)

	if len(h.Buckets) != 0 {
		t.Errorf("got %d buckets for empty input, want 0", len(h.Buckets))
	}
}

func TestECDF(t *testing.T) {
	points := ECDF([]int{10, 20, 30, 40})

	if len(points) != 4 {
		t.Fatalf("got %d points, want 4", len(points))
	}
	if points[0].Value != 10 {
		t.Errorf("first value = %d, want 10 (sorted ascending)", points[0].Value)
	}
	if points[3].P != 1.0 {
		t.Errorf("last P = %v, want 1.0", points[3].P)
	}
	if points[1].P != 0.5 {
		t.Errorf("second P = %v, want 0.5", points[1].P)
	}
}
```

**Step 2: Run to verify it fails, then implement**

```bash
go test ./internal/compare/ -run 'TestHistogram|TestECDF' -v
```

Create `internal/compare/distribution.go`:

```go
package compare

import "sort"

// Bucket is one histogram bar.
type Bucket struct {
	Lo    int `json:"lo"`
	Hi    int `json:"hi"`
	Count int `json:"count"`
}

// Hist is a fixed-bucket histogram over a value range.
type Hist struct {
	Buckets []Bucket `json:"buckets"`
}

// Histogram bins values into n equal-width buckets spanning min..max.
// The maximum value falls in the last bucket rather than past the end.
//
// Two label edges are approximate by nature: the maximum sits in a bucket whose
// half-open label formally excludes it, and when every value is identical
// several buckets share a degenerate label.
func Histogram(values []int, n int) Hist {
	if len(values) == 0 || n <= 0 {
		return Hist{}
	}

	lo, hi := values[0], values[0]
	for _, v := range values {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi == lo {
		hi = lo + 1 // avoid a zero-width range
	}

	width := float64(hi-lo) / float64(n)

	// Bin against the same integer edges the labels use. Binning on the exact
	// float width while labelling with truncated ints makes the two disagree
	// on a range that does not divide evenly: a value sitting on an edge gets
	// counted in the bucket below the one whose label contains it. Counts stay
	// correct either way, so no count-based test can see the difference —
	// only the rendered labels are wrong.
	edges := make([]int, n+1)
	for i := range edges {
		edges[i] = lo + int(float64(i)*width)
	}

	buckets := make([]Bucket, n)
	for i := range buckets {
		buckets[i] = Bucket{Lo: edges[i], Hi: edges[i+1]}
	}

	for _, v := range values {
		// Largest i with edges[i] <= v.
		idx := sort.SearchInts(edges, v+1) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		buckets[idx].Count++
	}
	return Hist{Buckets: buckets}
}

// Point is one step of an empirical cumulative distribution.
type Point struct {
	Value int     `json:"value"`
	P     float64 `json:"p"`
}

// ECDF returns the empirical cumulative distribution of values, ascending.
// Overlaying two ECDFs shows which run is faster at every percentile, not
// just on average.
func ECDF(values []int) []Point {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)

	points := make([]Point, len(sorted))
	for i, v := range sorted {
		points[i] = Point{Value: v, P: float64(i+1) / float64(len(sorted))}
	}
	return points
}
```

**Step 3: Verify and commit**

```bash
go test ./internal/compare/ -v
git add internal/compare/distribution.go internal/compare/distribution_test.go
git commit -m "feat: add histogram and ECDF for latency distributions"
```

---

### Task 12: Error taxonomy

The tools store full error strings. Bucketing them turns "88 errors" into "72 timeout, 14 auth, 2 TLS", which points at a cause.

**Files:**
- Create: `internal/compare/errors.go`
- Modify: `internal/compare/parse.go` — delete the `classifyError` stub added in Task 9
- Test: `internal/compare/errors_test.go`

**Step 1: Write the failing test**

```go
package compare

import "testing"

func TestClassifyError(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"dial tcp 1.2.3.4:8080: i/o timeout", "timeout"},
		{"context deadline exceeded", "timeout"},
		{"dial tcp 1.2.3.4:8080: connect: connection refused", "conn_refused"},
		{"read tcp: connection reset by peer", "conn_reset"},
		{"remote error: tls: handshake failure", "tls"},
		{"lookup bad.host: no such host", "dns"},
		{"proxy rejected: HTTP/1.1 407 Proxy Authentication Required", "auth"},
		{"unexpected EOF", "eof"},
		{"something nobody has seen before", "other"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := classifyError(tt.raw); got != tt.want {
			t.Errorf("classifyError(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestErrorBreakdown(t *testing.T) {
	results := []ProxyResult{
		{ErrorKind: "timeout"},
		{ErrorKind: "timeout"},
		{ErrorKind: "auth"},
		{ErrorKind: ""}, // successful result, contributes nothing
	}

	got := ErrorBreakdown(results)

	if got["timeout"] != 2 {
		t.Errorf("timeout = %d, want 2", got["timeout"])
	}
	if got["auth"] != 1 {
		t.Errorf("auth = %d, want 1", got["auth"])
	}
	if _, present := got[""]; present {
		t.Error("empty kind must not appear in the breakdown")
	}
}
```

**Step 2: Run to verify it fails, then implement**

Delete the stub from `parse.go`, then create `internal/compare/errors.go`:

```go
package compare

import "strings"

// errorPatterns maps a taxonomy bucket to the substrings that identify it.
// Order matters: the first matching bucket wins, so specific patterns come
// before general ones.
var errorPatterns = []struct {
	kind    string
	substrs []string
}{
	{"auth", []string{"407", "proxy authentication", "authentication required"}},
	{"timeout", []string{"timeout", "timed out", "deadline exceeded"}},
	{"conn_refused", []string{"connection refused"}},
	{"conn_reset", []string{"connection reset", "broken pipe"}},
	{"tls", []string{"tls:", "x509", "certificate", "handshake"}},
	{"dns", []string{"no such host", "dns", "name resolution"}},
	{"eof", []string{"eof"}},
}

// classifyError buckets a raw error string into a taxonomy kind. An empty
// input returns an empty kind, meaning "this result is not an error".
func classifyError(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	for _, p := range errorPatterns {
		for _, s := range p.substrs {
			if strings.Contains(lower, s) {
				return p.kind
			}
		}
	}
	return "other"
}

// ErrorBreakdown counts results by error kind. Non-errors are excluded.
func ErrorBreakdown(results []ProxyResult) map[string]int {
	counts := make(map[string]int)
	for _, r := range results {
		if r.ErrorKind == "" {
			continue
		}
		counts[r.ErrorKind]++
	}
	return counts
}
```

**Step 3: Verify and commit**

```bash
go test ./internal/compare/ -v
git add internal/compare/errors.go internal/compare/errors_test.go internal/compare/parse.go
git commit -m "feat: classify proxy errors into a taxonomy"
```

---

### Task 13: Joins and set operations

The per-proxy join is the feature. It powers the paired scatter, the top-movers table, the cross-tool matrix, and the "OK in ping, failing TM" export.

**Files:**
- Create: `internal/compare/join.go`
- Test: `internal/compare/join_test.go`

**Step 1: Write the failing test**

```go
package compare

import "testing"

func fixtureRun(tool string, results ...ProxyResult) Run {
	return Run{File: tool + ".csv", Meta: Meta{Tool: tool}, Results: results}
}

func TestJoin_PairsByProxyID(t *testing.T) {
	a := fixtureRun("pinger",
		ProxyResult{ProxyID: "u:p@1.1.1.1:80", LatencyMs: 100, Outcome: OutcomeOK},
		ProxyResult{ProxyID: "u:p@2.2.2.2:80", LatencyMs: 200, Outcome: OutcomeOK},
		ProxyResult{ProxyID: "u:p@3.3.3.3:80", LatencyMs: 300, Outcome: OutcomeOK},
	)
	b := fixtureRun("speedtester",
		ProxyResult{ProxyID: "u:p@1.1.1.1:80", LatencyMs: 150, Outcome: OutcomeOK},
		ProxyResult{ProxyID: "u:p@2.2.2.2:80", LatencyMs: 180, Outcome: OutcomeBlocked},
	)

	j := Join([]Run{a, b})

	if j.Overlap != 2 {
		t.Errorf("Overlap = %d, want 2", j.Overlap)
	}
	if len(j.Rows) != 3 {
		t.Errorf("got %d rows, want 3 (union of both runs)", len(j.Rows))
	}

	row := j.Rows[0]
	if row.ProxyID != "u:p@1.1.1.1:80" {
		t.Fatalf("first row = %q", row.ProxyID)
	}
	if row.Cells[0].LatencyMs != 100 || row.Cells[1].LatencyMs != 150 {
		t.Errorf("cells = %v, want 100 then 150", row.Cells)
	}
}

func TestJoin_MissingCellIsMarkedAbsent(t *testing.T) {
	a := fixtureRun("pinger", ProxyResult{ProxyID: "only-in-a", Outcome: OutcomeOK})
	b := fixtureRun("speedtester")

	j := Join([]Run{a, b})

	if j.Rows[0].Cells[1].Present {
		t.Error("cell absent from run b must have Present=false")
	}
}

func TestSetDifference(t *testing.T) {
	a := fixtureRun("pinger",
		ProxyResult{ProxyID: "good", Outcome: OutcomeOK},
		ProxyResult{ProxyID: "both-ok", Outcome: OutcomeOK},
	)
	b := fixtureRun("speedtester",
		ProxyResult{ProxyID: "good", Outcome: OutcomeBlocked},
		ProxyResult{ProxyID: "both-ok", Outcome: OutcomeOK},
	)

	// Proxies that succeed in a but not in b — the actionable filter list.
	got := OKInFirstNotSecond(a, b)

	if len(got) != 1 || got[0] != "good" {
		t.Errorf("OKInFirstNotSecond = %v, want [good]", got)
	}
}
```

**Step 2: Run to verify it fails, then implement**

Create `internal/compare/join.go`:

```go
package compare

// Cell is one proxy's result within one of the joined runs.
type Cell struct {
	Present   bool    `json:"present"` // false when the proxy is absent from that run
	LatencyMs int     `json:"latencyMs"`
	Status    int     `json:"status"`
	Outcome   Outcome `json:"outcome"`
	ErrorKind string  `json:"errorKind"`
	ExitIP    string  `json:"exitIp"`
}

// JoinRow is one proxy across every joined run, in the order the runs were given.
type JoinRow struct {
	ProxyID string `json:"proxyId"`
	Cells   []Cell `json:"cells"`
}

// JoinResult is the proxy matrix: the union of proxies across runs, plus how
// many appear in all of them.
type JoinResult struct {
	Runs    []string  `json:"runs"`    // file names, matching Cells order
	Rows    []JoinRow `json:"rows"`
	Overlap int       `json:"overlap"` // proxies present in every run
}

// Join builds the proxy matrix across runs, keyed on canonical proxy ID.
//
// The row order follows first appearance across the runs in order, so output
// is deterministic. Rows are the union, not the intersection: a proxy missing
// from one run still appears, with that cell marked absent.
func Join(runs []Run) JoinResult {
	j := JoinResult{Runs: make([]string, len(runs))}
	for i, r := range runs {
		j.Runs[i] = r.File
	}

	index := make(map[string]int) // proxy ID -> row position
	for runIdx, run := range runs {
		for _, res := range run.Results {
			rowIdx, seen := index[res.ProxyID]
			if !seen {
				rowIdx = len(j.Rows)
				index[res.ProxyID] = rowIdx
				j.Rows = append(j.Rows, JoinRow{
					ProxyID: res.ProxyID,
					Cells:   make([]Cell, len(runs)),
				})
			}
			j.Rows[rowIdx].Cells[runIdx] = Cell{
				Present:   true,
				LatencyMs: res.LatencyMs,
				Status:    res.Status,
				Outcome:   res.Outcome,
				ErrorKind: res.ErrorKind,
				ExitIP:    res.ExitIP,
			}
		}
	}

	for _, row := range j.Rows {
		complete := true
		for _, c := range row.Cells {
			if !c.Present {
				complete = false
				break
			}
		}
		if complete {
			j.Overlap++
		}
	}
	return j
}

// okSet returns the IDs of proxies that succeeded in a run.
func okSet(r Run) map[string]bool {
	set := make(map[string]bool)
	for _, res := range r.Results {
		if res.Outcome == OutcomeOK {
			set[res.ProxyID] = true
		}
	}
	return set
}

// OKInFirstNotSecond returns proxies that succeed in a but not in b, in a's
// original order. This is the actionable output of a cross-tool comparison:
// "works on ping, fails on Ticketmaster".
func OKInFirstNotSecond(a, b Run) []string {
	bOK := okSet(b)

	var out []string
	seen := make(map[string]bool)
	for _, res := range a.Results {
		if res.Outcome != OutcomeOK || bOK[res.ProxyID] || seen[res.ProxyID] {
			continue
		}
		seen[res.ProxyID] = true
		out = append(out, res.ProxyID)
	}
	return out
}
```

**Step 3: Verify and commit**

```bash
go test ./internal/compare/ -v
git add internal/compare/join.go internal/compare/join_test.go
git commit -m "feat: join runs by proxy ID and compute set differences"
```

---

### Task 14: Subnet grouping and correlation

Subnet grouping exposes the single bad /24 that drags a file's average — invisible in any aggregate. Correlation answers whether a cheap ping predicts an expensive Ticketmaster request at all.

**Files:**
- Create: `internal/compare/cohort.go`
- Test: `internal/compare/cohort_test.go`

**Step 1: Write the failing test**

```go
package compare

import (
	"math"
	"testing"
)

func TestSubnetOf(t *testing.T) {
	tests := []struct{ id, want string }{
		{"user:pw@1.2.3.4:8080", "1.2.3.0/24"},
		{"user:pw@10.0.0.255:80", "10.0.0.0/24"},
		{"user:pw@gate.provider.com:7000", "gate.provider.com"}, // hostnames pass through
		{"1.2.3.4", "1.2.3.0/24"},                               // legacy bare host
		{"", ""},
	}

	for _, tt := range tests {
		if got := SubnetOf(tt.id); got != tt.want {
			t.Errorf("SubnetOf(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestGroupBySubnet(t *testing.T) {
	results := []ProxyResult{
		{ProxyID: "u:p@1.2.3.4:80", LatencyMs: 100, Outcome: OutcomeOK},
		{ProxyID: "u:p@1.2.3.9:80", LatencyMs: 300, Outcome: OutcomeOK},
		{ProxyID: "u:p@9.9.9.1:80", LatencyMs: 50, Outcome: OutcomeOK},
	}

	groups := GroupBySubnet(results)

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if groups[0].Subnet != "1.2.3.0/24" {
		t.Errorf("groups are not sorted by size: first = %q", groups[0].Subnet)
	}
	if groups[0].Summary.Total != 2 {
		t.Errorf("first group total = %d, want 2", groups[0].Summary.Total)
	}
}

func TestPearson(t *testing.T) {
	// Perfectly correlated.
	if got := Pearson([]float64{1, 2, 3}, []float64{2, 4, 6}); math.Abs(got-1.0) > 1e-9 {
		t.Errorf("Pearson(perfect) = %v, want 1.0", got)
	}
	// Mismatched lengths and degenerate input are 0, not a panic.
	if got := Pearson([]float64{1, 2}, []float64{1}); got != 0 {
		t.Errorf("Pearson(mismatched) = %v, want 0", got)
	}
	if got := Pearson([]float64{5, 5, 5}, []float64{1, 2, 3}); got != 0 {
		t.Errorf("Pearson(zero variance) = %v, want 0", got)
	}
}
```

**Step 2: Run to verify it fails, then implement**

Create `internal/compare/cohort.go`:

```go
package compare

import (
	"math"
	"net"
	"sort"
	"strings"
)

// SubnetOf returns the /24 of a proxy's host, or the hostname itself when the
// host is not an IPv4 address. Grouping by subnet exposes the single bad block
// that drags a whole file's average.
func SubnetOf(proxyID string) string {
	host := hostOf(proxyID)
	if host == "" {
		return ""
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return host // hostname-based gateway, no meaningful subnet
	}
	ip[3] = 0
	return ip.String() + "/24"
}

// hostOf extracts the host from a canonical user:pass@host:port ID, tolerating
// legacy bare-host identifiers.
func hostOf(proxyID string) string {
	s := proxyID
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h
	}
	return s
}

// SubnetGroup is one /24 (or hostname) with its own distribution.
type SubnetGroup struct {
	Subnet  string  `json:"subnet"`
	Summary Summary `json:"summary"`
}

// GroupBySubnet buckets results by /24, largest group first. Ties break on
// subnet name so the output is deterministic.
func GroupBySubnet(results []ProxyResult) []SubnetGroup {
	buckets := make(map[string][]ProxyResult)
	for _, r := range results {
		key := SubnetOf(r.ProxyID)
		buckets[key] = append(buckets[key], r)
	}

	groups := make([]SubnetGroup, 0, len(buckets))
	for subnet, rs := range buckets {
		groups = append(groups, SubnetGroup{Subnet: subnet, Summary: Summarize(rs)})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Summary.Total != groups[j].Summary.Total {
			return groups[i].Summary.Total > groups[j].Summary.Total
		}
		return groups[i].Subnet < groups[j].Subnet
	})
	return groups
}

// Pearson returns the correlation coefficient of two equal-length series.
// Mismatched lengths, empty input, or zero variance return 0 rather than NaN,
// so the dashboard can render "no relationship" instead of a broken chart.
func Pearson(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) == 0 {
		return 0
	}

	var sumX, sumY float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
	}
	meanX := sumX / float64(len(xs))
	meanY := sumY / float64(len(ys))

	var cov, varX, varY float64
	for i := range xs {
		dx := xs[i] - meanX
		dy := ys[i] - meanY
		cov += dx * dy
		varX += dx * dx
		varY += dy * dy
	}
	if varX == 0 || varY == 0 {
		return 0
	}
	return cov / math.Sqrt(varX*varY)
}
```

**Step 3: Verify and commit**

```bash
go test ./internal/compare/ -v
git add internal/compare/cohort.go internal/compare/cohort_test.go
git commit -m "feat: add subnet cohorts and Pearson correlation"
```

---

## Phase 4 — Serve layer

Local, read-only HTTP. Binds the loopback interface only.

---

### Task 15: Results directory scan

**Files:**
- Create: `internal/compare/scan.go`
- Test: `internal/compare/scan_test.go`

**Step 1: Write the failing test**

```go
package compare

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDir_ParsesEveryCSV(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile("testdata/pinger_full.csv")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.csv", "b.csv", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), src, 0644); err != nil {
			t.Fatal(err)
		}
	}

	runs, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	if len(runs) != 2 {
		t.Errorf("got %d runs, want 2 (non-CSV ignored)", len(runs))
	}
}

func TestScanDir_SkipsUnparseableFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.csv"), []byte("\"unterminated"), 0644); err != nil {
		t.Fatal(err)
	}

	runs, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("one bad file must not fail the scan: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("got %d runs, want 0", len(runs))
	}
}

func TestScanDir_MissingDirIsEmptyNotError(t *testing.T) {
	runs, err := ScanDir(filepath.Join(t.TempDir(), "does-not-exist"))

	if err != nil {
		t.Errorf("missing directory should not error: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("got %d runs, want 0", len(runs))
	}
}
```

**Step 2: Run to verify it fails, then implement**

Create `internal/compare/scan.go`:

```go
package compare

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScanDir parses every .csv in dir, newest run first.
//
// A file that fails to parse is skipped rather than failing the whole scan —
// one hand-edited CSV must not take the dashboard down. A missing directory
// yields no runs and no error, which is the state before a first export.
func ScanDir(dir string) ([]Run, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var runs []Run
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			continue
		}
		run, err := ParseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		runs = append(runs, run)
	}

	// Newest first. Runs without metadata sort last, since their zero time
	// carries no information.
	sort.SliceStable(runs, func(i, j int) bool {
		ti, tj := runs[i].Meta.RunAt, runs[j].Meta.RunAt
		if ti.IsZero() != tj.IsZero() {
			return !ti.IsZero()
		}
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return runs[i].File < runs[j].File
	})
	return runs, nil
}
```

**Step 3: Verify and commit**

```bash
go test ./internal/compare/ -v
git add internal/compare/scan.go internal/compare/scan_test.go
git commit -m "feat: scan the results directory into parsed runs"
```

---

### Task 16: HTTP server

Read-only, loopback-only, random port. The `file` parameter is reduced with `filepath.Base` so no request can escape the results directory.

**Files:**
- Create: `internal/dashboard/server.go`
- Create: `internal/dashboard/api.go`
- Test: `internal/dashboard/api_test.go`

**Step 1: Write the failing test**

```go
package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func fixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src, err := os.ReadFile("../compare/testdata/pinger_full.csv")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pinger_run.csv"), src, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestHandleRuns_ListsInventory(t *testing.T) {
	h := newAPI(fixtureDir(t))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/runs", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []runInventoryItem
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}
	if got[0].Tool != "pinger" || got[0].Summary.Total != 5 {
		t.Errorf("item = %+v", got[0])
	}
}

func TestHandleRun_RejectsPathTraversal(t *testing.T) {
	h := newAPI(fixtureDir(t))
	rec := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/run?file=../../../etc/passwd", nil)
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("path traversal must not return 200")
	}
}

func TestHandleCompare_JoinsRequestedRuns(t *testing.T) {
	h := newAPI(fixtureDir(t))
	rec := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/compare?file=pinger_run.csv&file=pinger_run.csv", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got compareResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Join.Overlap != 5 {
		t.Errorf("Overlap = %d, want 5 (a run joined with itself)", got.Join.Overlap)
	}
}

func TestAPI_RejectsNonGET(t *testing.T) {
	h := newAPI(fixtureDir(t))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405 — the dashboard is read-only", rec.Code)
	}
}
```

**Step 2: Run to verify it fails, then implement**

Create `internal/dashboard/api.go`:

```go
// Package dashboard serves a local, read-only web UI for comparing exported
// result CSVs. It binds the loopback interface only and never writes.
package dashboard

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"proxytoolbox/internal/compare"
)

// runInventoryItem is one row of the landing table.
type runInventoryItem struct {
	File      string          `json:"file"`
	Tool      string          `json:"tool"`
	RunAt     time.Time       `json:"runAt"`
	ProxyFile string          `json:"proxyFile"`
	Target    string          `json:"target"`
	Workers   int             `json:"workers"`
	HasMeta   bool            `json:"hasMeta"`
	Summary   compare.Summary `json:"summary"`
}

// runDetail is one run with everything a detail view needs.
type runDetail struct {
	runInventoryItem // carries Summary; do not redeclare it here
	Histogram compare.Hist          `json:"histogram"`
	ECDF      []compare.Point       `json:"ecdf"`
	Errors    map[string]int        `json:"errors"`
	Subnets   []compare.SubnetGroup `json:"subnets"`
	Results   []compare.ProxyResult `json:"results"`
}

// runOverview is one run inside a comparison: everything the timeline and the
// overlay charts need, and deliberately no Results.
//
// JoinResult.Rows already carries every per-proxy fact for every selected run.
// Sending Results as well would encode the same data twice — for ten 500-proxy
// runs, roughly 5000 duplicated rows — and leave the client to decide which copy
// is authoritative. A per-run drill-down is a separate /api/run call.
type runOverview struct {
	runInventoryItem
	Histogram compare.Hist          `json:"histogram"`
	ECDF      []compare.Point       `json:"ecdf"`
	Errors    map[string]int        `json:"errors"`
	Subnets   []compare.SubnetGroup `json:"subnets"`
}

// compareResponse carries everything the comparison views need for a selection.
type compareResponse struct {
	Runs []runOverview      `json:"runs"`
	Join compare.JoinResult `json:"join"`
}

func newAPI(resultsDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/runs", readOnly(func(w http.ResponseWriter, r *http.Request) {
		runs, err := compare.ScanDir(resultsDir)
		if err != nil {
			http.Error(w, "cannot read results directory", http.StatusInternalServerError)
			return
		}
		items := make([]runInventoryItem, 0, len(runs))
		for _, run := range runs {
			items = append(items, inventoryItem(run))
		}
		writeJSON(w, items)
	}))

	mux.HandleFunc("/api/run", readOnly(func(w http.ResponseWriter, r *http.Request) {
		run, ok := loadRun(resultsDir, r.URL.Query().Get("file"))
		if !ok {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		writeJSON(w, detail(run))
	}))

	mux.HandleFunc("/api/compare", readOnly(func(w http.ResponseWriter, r *http.Request) {
		names := r.URL.Query()["file"]
		if len(names) == 0 {
			http.Error(w, "no files selected", http.StatusBadRequest)
			return
		}
		var runs []compare.Run
		var overviews []runOverview
		for _, name := range names {
			run, ok := loadRun(resultsDir, name)
			if !ok {
				http.Error(w, "run not found", http.StatusNotFound) // no caller input reflected
				return
			}
			runs = append(runs, run)
			overviews = append(overviews, overview(run))
		}
		writeJSON(w, compareResponse{Runs: overviews, Join: compare.Join(runs)})
	}))

	return mux
}

// readOnly rejects anything but GET. The dashboard never mutates state.
func readOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "read-only", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

// loadRun resolves a requested file strictly inside resultsDir.
//
// Two guards do the real work, and neither is sufficient alone. filepath.Base strips
// directory components, so "../../etc/passwd" becomes "passwd". But Base does
// NOT neutralise an absolute path — Base("/etc/passwd") is "passwd" only
// because the whole path is stripped; what actually re-roots the request is
// filepath.Join(resultsDir, base). Removing either one reopens traversal, so do
// not "simplify" either away.
func loadRun(resultsDir, name string) (compare.Run, bool) {
	if name == "" {
		return compare.Run{}, false
	}
	base := filepath.Base(name)
	// Defence in depth. These already fail to parse, because Join yields a
	// directory and csv.ReadAll cannot read one — but that is an OS-specific
	// behaviour to lean on for a security property, so reject them explicitly.
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return compare.Run{}, false
	}
	run, err := compare.ParseFile(filepath.Join(resultsDir, base))
	if err != nil {
		return compare.Run{}, false
	}
	return run, true
}

func inventoryItem(run compare.Run) runInventoryItem {
	return runInventoryItem{
		File:      run.File,
		Tool:      run.Meta.Tool,
		RunAt:     run.Meta.RunAt,
		ProxyFile: run.Meta.ProxyFile,
		Target:    run.Meta.Target,
		Workers:   run.Meta.Workers,
		HasMeta:   run.HasMeta(),
		Summary:   compare.Summarize(run.Results),
	}
}

func detail(run compare.Run) runDetail {
	var lat []int
	for _, r := range run.Results {
		if r.Outcome == compare.OutcomeOK && r.LatencyMs > 0 {
			lat = append(lat, r.LatencyMs)
		}
	}
	return runDetail{
		// Summary is promoted from runInventoryItem; assigning it here would
		// not compile.
		runInventoryItem: inventoryItem(run),
		Histogram:        compare.Histogram(lat, histogramBuckets),
		ECDF:             compare.ECDF(lat),
		Errors:           compare.ErrorBreakdown(run.Results),
		Subnets:          compare.GroupBySubnet(run.Results),
		Results:          run.Results,
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "encoding failed", http.StatusInternalServerError)
	}
}
```

**Step 3: Add the server**

Create `internal/dashboard/server.go`:

```go
package dashboard

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"proxytoolbox/internal/basedir"
)

//go:embed web
var webFS embed.FS

const resultsDirName = "results"

// Serve starts the compare dashboard on a random loopback port, opens the
// browser, and blocks until the user presses Enter.
//
// The listener is bound to 127.0.0.1 explicitly: this is a local tool and must
// not be reachable from the network.
func Serve(waitForEnter func()) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("cannot start local server: %w", err)
	}

	static, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", newAPI(basedir.Path(resultsDirName)))
	mux.Handle("/", http.FileServer(http.FS(static)))

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()

	url := "http://" + ln.Addr().String()
	fmt.Printf("\nCompare dashboard running at %s\n", url)
	openBrowser(url)
	fmt.Print("Press Enter to stop the dashboard...")
	waitForEnter()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// openBrowser makes a best effort to open the dashboard. A failure is not an
// error — the URL is already printed.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, append(args, url)...).Start()
}
```

Create a placeholder so the embed directive compiles:

```bash
mkdir -p internal/dashboard/web
echo '<!doctype html><title>Compare</title><p>UI lands in Phase 5.</p>' > internal/dashboard/web/index.html
```

**Step 4: Verify and commit**

```bash
go test ./internal/dashboard/ -v && go build ./...
```

Expected: PASS, all four tests, clean build.

```bash
git add internal/dashboard/
git commit -m "feat: add read-only local dashboard server and JSON API"
```

---

### Task 17: Menu entry

**Files:**
- Modify: `main.go:16-24` (options), `:36-53` (switch)

**Step 1: Add the option**

After the Proxy Monitor option (line 23):

```go
huh.NewOption("Compare Results   — Local dashboard for comparing exported CSVs", "compare"),
```

**Step 2: Add the case**

After the `monitor` case:

```go
case "compare":
	if err := dashboard.Serve(func() { fmt.Scanln() }); err != nil {
		fmt.Println("Error:", err)
	}
```

Add `"proxytoolbox/internal/dashboard"` to the import block.

**Step 3: Verify**

```bash
go build ./... && go vet ./...
```

Run the binary, choose `Compare Results`, confirm the browser opens on a `127.0.0.1` URL and that `/api/runs` returns JSON for whatever is in `results/`.

**Step 4: Commit**

```bash
git add main.go
git commit -m "feat: add Compare Results entry to the main menu"
```

---

## Phase 5 — Web UI

Five views over the JSON API. Assets live in `internal/dashboard/web/` and are embedded.

**The API contract Phase 5 builds against, as actually shipped:**

- `GET /api/runs` → array of inventory items. `[]`, never `null`, when empty.
- `GET /api/run?file=<name>` → one run's detail: inventory fields, `summary`,
  `histogram`, `ecdf`, `errors`, `subnets`, **and** `results`, the per-proxy rows.
- `GET /api/compare?file=<a>&file=<b>...` → `{runs, join}`. Each entry in `runs`
  carries the chart data but **no `results`** — `join.rows[].cells[i]` is the
  single row-level source, indexed by `join.runs[i]`. A per-run drill-down is a
  separate `/api/run` call.
- A comparison is capped at **50 files**; stop the user at the picker rather than
  letting the request 400.
- Branch on `summary.latencyCount`, not `summary.ok`, when deciding whether a
  distribution is renderable. A successful row reporting 0ms counts in `ok` but
  contributes no sample.
- Non-GET returns 405; a non-loopback `Host` returns 403.

**The API must be addressed with relative URLs.** `fetch('/api/runs')`, never an
absolute URL. The server refuses any request whose `Host` header is not a
loopback literal, which is what stops a page the user is already viewing from
pointing a hostname it controls at `127.0.0.1` and reading proxy credentials out
of this dashboard. A hardcoded absolute URL, or a dev server proxying from
another hostname, gets a 403 rather than silently working.

**Design constraints — these are not optional.** Read `~/.claude/rules/web/design-quality.md` before writing any markup. The dashboard must not look like a default template: no uniform card grid, no flat gray-on-white with one accent. Pick a deliberate direction and carry it through — a dense, editorial analytics surface with real typographic hierarchy, intentional spacing rhythm, and designed hover and focus states. Charts are part of the design system, not an afterthought. Define every color, size and duration as a custom property in `:root`, animate only `transform` and `opacity`, and use semantic elements (`<header>`, `<main>`, `<table>`, `<fieldset>`) rather than nested `div`s.

---

### Task 18: Shell and inventory view

**Files:**
- Create: `internal/dashboard/web/index.html`
- Create: `internal/dashboard/web/app.js`
- Create: `internal/dashboard/web/styles.css`
- Create: `internal/dashboard/web/vendor/uPlot.min.js`
- Create: `internal/dashboard/web/vendor/uPlot.min.css`

**Step 1: Vendor uPlot**

Download uPlot (MIT) and place the minified JS and CSS under `web/vendor/`. Do not reference a CDN — the dashboard must work with no network. Record the version in a comment at the top of `app.js`.

**Step 2: Build the shell**

`index.html` holds a `<header>` with the title and run count, a `<main>` with an inventory `<table>`, and an empty `<section id="views">` that later tasks fill. No framework; plain modules.

**Step 3: Fetch and render the inventory**

In `app.js`:

```js
const api = (path, params) =>
  fetch(path + (params ? '?' + params : '')).then((r) => {
    if (!r.ok) throw new Error(`${path} failed: ${r.status}`);
    return r.json();
  });

async function loadInventory() {
  const runs = await api('/api/runs');
  renderInventory(runs);
}
```

Each row shows file, tool, run time, proxy file, target, workers, n, success %, p50, p95 and error rate, with a checkbox. Rows where `hasMeta` is false render the missing fields as a muted `unknown` and carry a tooltip explaining that the export predates run metadata.

Columns sort on click. Selection drives the views in the following tasks.

**Step 4: Handle the empty and error states**

An empty `results/` renders a short explanation of how to produce a CSV, not a blank page. A failed fetch renders the error, never a silent blank.

**Step 5: Verify**

```bash
go build ./... && go run .
```

Choose `Compare Results`, confirm the inventory lists every CSV with correct statistics, that sorting works, and that the page is legible at 320px, 768px and 1440px with no horizontal scroll.

**Step 6: Commit**

```bash
git add internal/dashboard/web/
git commit -m "feat: add dashboard shell and run inventory view"
```

---

### Tasks 19-22 all register through the same seam

Task 18 built `internal/dashboard/web/views/registry.js`. `app.js` knows no
view's name: it builds a `Selection` from the checked rows and hands it to the
registry, and each view module declares at import time which selections it can
render. So **no view task modifies `app.js`** — each adds its own module plus one
import line in `views/index.js`.

A view is `{ id, title, match(selection), mount(element, selection) }`, where
`selection` is `{ files, runs, tools, sameTool, withMeta, withoutMeta }`.
`sameTool` requires every run to carry a tool, so a run without metadata cannot
silently make a selection same-tool. `mount` receives an empty `<section>`, may
be async, and must return a teardown if it builds a uPlot instance or attaches a
listener — the section is discarded on the next render.

### Task 19: Timeline view

Shown when the selection covers two or more runs of the same tool.

**Files:**
- Create: `internal/dashboard/web/views/timeline.js`
- Modify: `internal/dashboard/web/views/index.js` (one import line)

**Charts:**
1. Multi-series line: success rate, p50, p95 and error rate against `runAt`. Dual axis — rates left, milliseconds right.
2. Stacked area of the error taxonomy across runs, so a shift in the error *mix* is visible and not just the count.
3. ECDF overlay, one line per run, from each run's `ecdf` array.

**Comparability banner.** Before rendering, compare `target`, `proxyFile` and `workers` across the selection. If any differ, show a prominent warning naming the field and the differing values. This is the point of the metadata: without it the chart would attribute a changed target to degraded proxies.

Runs lacking metadata cannot be placed on a time axis. Exclude them and say so in the banner rather than inventing a position.

**Verify:** export three pinger runs, select all three, confirm the trends render and that changing the config domain between runs triggers the warning.

**Commit:** `feat: add same-tool timeline comparison view`

---

### Task 20: Paired view

Shown when exactly two runs of the same tool are selected, in addition to the timeline.

**Files:**
- Create: `internal/dashboard/web/views/paired.js`
- Modify: `internal/dashboard/web/views/index.js` (one import line)

**Charts and tables, all built from `join.rows` where both cells are present:**
1. Scatter of run A latency against run B latency, one point per proxy, with a `y = x` reference line. Points above the line degraded, below improved. Color by outcome pair.
2. Delta histogram of per-proxy latency change.
3. Top movers table: largest improvements and regressions, sortable.
4. Status flips table: `OK → blocked`, `error → OK`, and so on, with counts and the proxies behind each.

State the join coverage above the charts — "312 of 500 proxies present in both runs" — since every number below it is computed on the intersection only.

**Verify:** export two pinger runs against the same file, confirm the scatter is dense along the diagonal and that the top movers match a manual check of the CSVs.

**Commit:** `feat: add paired run comparison view`

---

### Task 21: Cross-tool view

Shown when the selection spans more than one tool.

**Files:**
- Create: `internal/dashboard/web/views/crosstool.js`
- Modify: `internal/dashboard/web/views/index.js` (one import line)

**Panels:**
1. **Proxy matrix** — rows are proxies, columns are runs, cells show latency and outcome with a color scale. Filterable by outcome, sortable by any column. Virtualize the rows: a 5000-proxy matrix must not lock the page.
2. **Funnel** — proxies surviving each tool in the selected order, e.g. `500 loaded → 412 ping OK → 380 TM OK → 290 Bayern OK`.
3. **Set operations** — pick two runs and an operation ("OK in A, not OK in B"). Results download as a `.txt` proxy file, generated client-side from data already in the browser. The server stays read-only.
4. **Correlation scatter** — latency in run A against latency in run B for proxies present in both, with the Pearson coefficient labeled.
5. **Exit-IP enrichment** — when an iptester run is in the selection, add its `exitIp` as a matrix column and allow grouping any metric by exit IP or /24.

**Never merge metrics across tools into one series.** A ping is a raw CONNECT through the proxy; a Ticketmaster request is a full TLS-fingerprinted fetch against a hostile target. They are different quantities. Cross-tool comparison happens per proxy, never as a shared trend line.

**Verify:** export a pinger run, a speedtester run and an iptester run over the same proxy file. Confirm the matrix joins them, the overlap count is correct, the funnel matches the summaries, and the set-operation download contains the expected proxies.

**Commit:** `feat: add cross-tool proxy matrix and set operations`

---

### Task 22: Single run detail view

Shown when exactly one run is selected.

**Files:**
- Create: `internal/dashboard/web/views/detail.js`
- Modify: `internal/dashboard/web/views/index.js` (one import line)

**Panels, all from `/api/run`:**
1. Latency histogram from `histogram`.
2. Percentile table: min, p50, p75, p90, p95, p99, max, mean, standard deviation, IQR.
3. Per-/24 box plots from `subnets`, largest cohort first — the view that exposes one bad block dragging the average.
4. Slowest N table.
5. Error taxonomy breakdown from `errors`.

**Verify:** select one run, confirm every number matches `Summarize` output for the same CSV.

**Commit:** `feat: add single run detail view`

---

### Task 23: Documentation

**Correction to this task's original premise.** It claimed Bayern Tester and
Proxy Monitor were undocumented. They are not — the root `README.md` lists all
seven tools and `docs/tools/` carries a page for each. That claim came from a
stale note about a different file. The real scope is narrower and deeper:

**Files:**
- Create: `docs/tools/compare-results.md` — a page matching the structure of the
  six existing tool pages
- Modify: `docs/tools/README.md` — add it to the index
- Modify: `README.md` — add Compare Results to the Features table, and extend
  the Export section, which currently says only IP Uniqueness, Ping and TM
  Request prompt for a CSV
- Modify: `docs/reference/exporting-results.md` — the CSV format changed in this
  project and this page describes it

For Compare Results, cover: what it does, that it is local and read-only, that it reads `results/*.csv`, that older exports lack metadata and so cannot appear on a timeline, and that the canonical proxy ID is what makes cross-tool joins work.

**Verify:** `go build ./...` and read the README end to end against the actual menu.

**Commit:** `docs: document Compare Results and the two undocumented tools`

---

## Final verification

```bash
go build ./...
go vet ./...
go test ./... -cover
```

Manual pass:
1. Run each of the four testing tools, exporting with `.` each time.
2. Open `Compare Results`.
3. Select one run — detail view renders.
4. Select two pinger runs — timeline and paired views render.
5. Select a pinger and a speedtester run — cross-tool matrix renders with a correct overlap count.
6. Select runs against different targets — the comparability banner warns.
7. Confirm the server is unreachable from another machine on the network.

## Verification notes for anyone re-auditing the UI

- **axe-core reports phantom contrast failures immediately after `emulateMedia`.**
  `th button` carries a `color` transition, and axe samples the interpolated
  mid-flight colour — measured as low as 1.96:1 for nodes that settle at
  4.89:1 light / 5.22:1 dark. Wait ~900ms after a scheme change before running
  the audit, or chase a ghost.
- **axe cannot resolve contrast over the body's radial wash** and marks those
  nodes `incomplete` rather than failing them. Roughly 300-600 nodes, depending
  on the view. Those pairs must be measured by hand — resolve the computed
  colour to sRGB via a canvas and composite the real background stack. The
  serious contrast defect found in Task 18 was in this blind spot.
- **Pearson across tools is near zero on realistic data** — every honest pair
  in the Task 21 fixtures fell between -0.05 and 0.16. That is a finding, not a
  bug, and the view treats it as one. A deliberately-correlated fixture
  (r = 0.98) exists to prove the other branch still renders.

## Known behaviours, decided not defects

- **`Histogram` bucket labels can repeat.** `Lo`/`Hi` are int truncations of
  float edges, so when the bucket count exceeds the value range several adjacent
  buckets report identical bounds. Counts stay correct and every value is binned
  exactly once. The serve layer must not assume labels are distinct.
- **`okLatencies` excludes a genuine 0ms success.** The `LatencyMs > 0` filter is
  there to drop errored rows, which carry no latency; it also drops a real
  sub-millisecond success. Not reachable over a network proxy in practice.
- **`Join` collapses a duplicate ProxyID within one run**, last cell winning. A
  proxy file with a repeated line produces this. Collapsing is defensible for an
  identity-keyed matrix, but it is unspecified rather than chosen.
- **The tools' `Successful` count and the parser's OK count differ by design.**
  pinger counts a 403 as successful because it saw no Go error; the parser
  separates blocked from OK. The dashboard recomputes from detail rows, so the
  parser's stricter reading is the one that reaches the UI.
- **`SubnetOf` writes into the slice from `To4()`**, which is safe only because
  `net.ParseIP` allocates fresh per call.

## Out of scope

- Terminal compare view
- A `Total time` row for speedtester and bayerntester. pinger and iptester emit
  one; the other two never compute an elapsed value. No dashboard view consumes
  run duration, and the parser ignores unrecognised summary keys, so this stays
  a known, harmless asymmetry rather than new scope.
- Extracting the ~150 near-identical lines shared by `speedtester.go` and
  `bayerntester.go` (worker pool, result classification, stats, export block)
  into a common helper. Real duplication, but pre-existing and too large a
  refactor to land mid-plan. Worth a follow-up before a third tool of that
  shape appears.
- Moving `internal/tools/display.go` into `util` alongside `TruncateID`. Only
  `tools` consumes the widths today.
- Proxy Monitor, the fifth tool. It writes only `results/monitor.log` and never
  exports a CSV, so the dashboard's scan — which filters on `.csv` — ignores it.
  Its terminal `"Host"` column headers carry no contract.
- Re-running or re-measuring proxies
- Repeat sampling, timing breakdowns, significance testing
- Provider attribution
- Any remote or networked component
