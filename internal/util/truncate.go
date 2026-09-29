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
