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

	// The fixed columns of the session monitor's two tables. Its proxy column
	// is not here because it is sized per run from the widest id in the file —
	// that tool never truncates a proxy — so the rules are drawn as these plus
	// that width rather than from one hardcoded total.
	sessionFixedCols      = 8 + 2 + 4 + 2 + 4 + 2 + 2 + 16 + 2 + 8 + 2 + 6
	sessionStatsFixedCols = 2 + 4 + 2 + 2 + 7 + 2 + 6 + 2 + 10 + 2 + 6 + 2 + 12
)
