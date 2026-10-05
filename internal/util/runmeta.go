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
	Workers   int       // concurrency used
	// IPMode is which address families the exit-IP lookups asked for:
	// "ipv4", "ipv6" or "both". Empty where not applicable, which is every
	// tool but iptester, and every export made before modes existed.
	IPMode string
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
		{"IP mode", m.IPMode},
	}
}
