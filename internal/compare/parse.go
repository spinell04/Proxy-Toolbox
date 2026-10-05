package compare

import (
	"encoding/csv"
	"net"
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
			// Last summary wins, deliberately asymmetric with the append used
			// for detail rows below: Meta is a single scalar record, so a
			// second Summary section replaces the first rather than merging.
			run.Meta = parseMeta(section[1:])
		case isProxyDetail(section[0]):
			// Appended, not assigned: a second detail table must never
			// silently drop the first, and appending keeps a misclassified
			// section visible instead of letting a later correct table mask
			// it. isProxyDetail is pinned directly by its own test.
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
		case "IP mode":
			m.IPMode = val
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

	// iptester's detail table has no Status column at all, so for it success is
	// inferred from an empty Error column alone (the exit IP is not consulted;
	// iptester only populates it on success anyway). Without this guard,
	// classifyStatus("") would mark every successful iptester row as an error
	// and the dashboard would show those runs as total failures, silently.
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
		exit := readExitIPs(get(row, "Exit IP"), get(row, "Exit IPv4"), get(row, "Exit IPv6"))
		results = append(results, ProxyResult{
			ProxyID:   strings.TrimSpace(row[idIdx]),
			LatencyMs: parseLatencyMs(get(row, "Latency")),
			Status:    status,
			Outcome:   outcome,
			ErrorRaw:  errRaw,
			ErrorKind: classifyError(errRaw),
			ExitIP:    exit.single(),
			ExitIPv4:  exit.V4,
			ExitIPv6:  exit.V6,
		})
	}
	return results
}

// exitIPs is one row's exit addresses.
type exitIPs struct {
	V4 string
	V6 string
}

// single is the one-address view ExitIP has always carried, v4 first: callers
// written before modes existed want an address, not a family.
func (e exitIPs) single() string {
	if e.V4 != "" {
		return e.V4
	}
	return e.V6
}

// readExitIPs reads both CSV spellings. iptester wrote one unlabelled "Exit IP"
// column before address families were selectable; it now writes "Exit IPv4"
// and/or "Exit IPv6". The legacy column is sorted by parsing it, so an old
// export's addresses land in the same fields a new one's do rather than in a
// third place every caller would have to know about.
func readExitIPs(legacy, v4, v6 string) exitIPs {
	e := exitIPs{V4: v4, V6: v6}
	if legacy == "" {
		return e
	}
	if ip := net.ParseIP(legacy); ip != nil && ip.To4() == nil {
		if e.V6 == "" {
			e.V6 = legacy
		}
		return e
	}
	// Unparseable text falls to v4, the family every pre-mode export was
	// overwhelmingly reporting. Dropping it would lose the column entirely.
	if e.V4 == "" {
		e.V4 = legacy
	}
	return e
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
//
// It assumes the code is a whitespace-delimited token, which is what every
// tool writes today. A spelling that glues the code to punctuation, such as
// "Blocked (403)", finds no code and falls through to OutcomeError; a fifth
// tool with a new vocabulary needs a case here, not just a new fixture.
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
