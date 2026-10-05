package tools

import (
	"fmt"
	"strings"
)

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

	// Address column widths. A v4 address is at most 15 characters and a v6
	// address 39, so both mode is far wider than one line of either — which is
	// the price of asking two questions per check.
	ipV4ColWidth = 18
	ipV6ColWidth = 39

	// The fixed columns of the session monitor's two tables. Neither the proxy
	// column nor the address columns are here: the proxy column is sized per
	// run from the widest id in the file (that tool never truncates a proxy)
	// and the address columns depend on the IP mode. Rules are drawn as these
	// plus both, rather than from one hardcoded total.
	sessionFixedCols      = 8 + 2 + 4 + 2 + 4 + 2 + 2 + 2 + 8 + 2 + 6
	sessionStatsFixedCols = 2 + 4 + 2 + 2 + 7 + 2 + 6 + 2 + 10 + 2 + 6 + 2 + 2
)

// ipColsWidth is the rendered width of the address columns a mode shows,
// including the gap between the two of them in both mode.
func ipColsWidth(mode IPMode) int {
	w := 0
	for i, f := range mode.families() {
		if i > 0 {
			w += 2
		}
		w += f.Width
	}
	return w
}

// ipColsHeader and ipCols keep the address columns' layout in one place: both
// mode adds a second one, and the header, every row and the rule have to agree.
func ipColsHeader(mode IPMode) string {
	cells := make([]string, 0, 2)
	for _, f := range mode.families() {
		cells = append(cells, fmt.Sprintf("%-*s", f.Width, "Exit "+f.Label))
	}
	return strings.Join(cells, "  ")
}

// ipCols renders one row's addresses. An unanswered family shows "-": it is
// unknown for this check, not known to be absent.
func ipCols(mode IPMode, r ipResult) string {
	cells := make([]string, 0, 2)
	for _, f := range mode.families() {
		v := f.Get(r)
		if v == "" {
			v = "-"
		}
		cells = append(cells, fmt.Sprintf("%-*s", f.Width, v))
	}
	return strings.Join(cells, "  ")
}

// currentHeader labels the session statistics table's address columns.
func currentHeader(mode IPMode) string {
	cells := make([]string, 0, 2)
	for _, f := range mode.families() {
		cells = append(cells, fmt.Sprintf("%-*s", f.Width, "Current "+f.Label))
	}
	return strings.Join(cells, "  ")
}
