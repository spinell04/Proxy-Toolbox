package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"proxytoolbox/internal/basedir"
)

const (
	fileName             = "config.txt"
	DefaultWorkers       = 100
	DefaultDownThreshold = 3
	DefaultUpThreshold   = 2

	// Interval defaults, in different units by design. The downtime monitor
	// sleeps between individual checks; the session monitor checks the whole
	// list in parallel and sleeps between cycles, so its value is the sampling
	// period directly. It is far larger because every check costs a request to
	// a free third-party IP endpoint, and the volume scales with the list.
	DefaultMonitorIntervalMs = 1000
	DefaultSessionIntervalMs = 60000

	// Which address families the IP lookups ask for. Well-defined beats
	// permissive, and IPv4 is what most target sites see. The value is validated
	// in internal/tools, which owns the mode; config only carries the string.
	DefaultIPMode = "ipv4"

	// Auto-update is on by default: the toolbox ships as a bare binary with no
	// installer and no package manager, so nothing else would ever make a user
	// current. Off is the escape hatch for a release broken on your machine —
	// which would otherwise reinstall itself on every launch — and for a box
	// running a monitor for days that should not be restarted underneath it.
	DefaultAutoUpdate = true
)

// ─────────────────────────────────────────────────────────────────────────
// Adding or renaming a key here?  Update internal/bootstrap/config.default.txt
// in the same change.  That template is what every new install receives, and a
// key missing from it is a setting nobody can discover: it will not appear in
// their config.txt, and most users never read the docs.
//
// TestDefaultConfig_CoversEveryKeyTheParserKnows and
// TestDefaultConfig_MatchesTheBuiltInDefaults enforce both halves of this.
// ─────────────────────────────────────────────────────────────────────────

// Config holds settings from config.txt.
type Config struct {
	Workers              int
	Domain               string
	IPMode               string
	DiscordWebhook       string
	DiscordDownThreshold int
	DiscordUpThreshold   int
	MonitorIntervalMs    int
	SessionIntervalMs    int
	PingMaxLatencyMs     int
	TMMaxLatencyMs       int
	BayernMaxLatencyMs   int
	AutoUpdate           bool
}

// parseLatency returns a positive millisecond threshold, or 0 (no filter) for
// blank or invalid input.
func parseLatency(val string) int {
	n, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// parseBool reads a human-written on/off setting, falling back to def for
// anything it does not recognise.
//
// Deliberately generous about spelling: this file is edited by hand in a text
// editor with no validation and no error reporting, so "yes" and "true" and
// "1" all have to work. Anything unrecognised takes the default rather than
// reading as false — a typo must not silently turn a feature off.
func parseBool(val string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "on", "true", "yes", "1":
		return true
	case "off", "false", "no", "0":
		return false
	default:
		return def
	}
}

// Load reads config.txt and returns the parsed Config.
func Load() Config {
	cfg := Config{
		Workers:              DefaultWorkers,
		DiscordDownThreshold: DefaultDownThreshold,
		DiscordUpThreshold:   DefaultUpThreshold,
		MonitorIntervalMs:    DefaultMonitorIntervalMs,
		SessionIntervalMs:    DefaultSessionIntervalMs,
		IPMode:               DefaultIPMode,
		AutoUpdate:           DefaultAutoUpdate,
	}
	path := basedir.Path(fileName)

	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("[config] %s not found, using defaults (workers=%d)\n", fileName, DefaultWorkers)
		return cfg
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "workers":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.Workers = n
			}
		case "domain":
			cfg.Domain = val
		case "ip_mode":
			cfg.IPMode = val
		case "discord_webhook":
			cfg.DiscordWebhook = val
		case "discord_down_threshold":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.DiscordDownThreshold = n
			}
		case "discord_up_threshold":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.DiscordUpThreshold = n
			}
		case "monitor_interval_ms":
			if n := parseLatency(val); n > 0 {
				cfg.MonitorIntervalMs = n
			}
		case "session_interval_ms":
			if n := parseLatency(val); n > 0 {
				cfg.SessionIntervalMs = n
			}
		case "ping_max_latency_ms":
			cfg.PingMaxLatencyMs = parseLatency(val)
		case "tm_max_latency_ms":
			cfg.TMMaxLatencyMs = parseLatency(val)
		case "bayern_max_latency_ms":
			cfg.BayernMaxLatencyMs = parseLatency(val)
		case "auto_update":
			cfg.AutoUpdate = parseBool(val, DefaultAutoUpdate)
		}
	}
	return cfg
}

// AutoUpdateEnabled reads only the auto_update key out of config.txt.
//
// This is not redundant with Load, and merging it back into Load would
// reintroduce the bug it exists to avoid. The update check has to run before
// bootstrap.Ensure — a successful update re-execs the process, so anything
// done ahead of it would be done twice — but Ensure is also what creates
// config.txt. On a first ever run the file therefore does not exist yet, and
// Load would print "[config] config.txt not found, using defaults" before the
// very line announcing that config.txt has just been created: noise, and
// misleading, because it reads as if the user's settings had been ignored.
//
// It is the same key parsed by the same parseBool with the same default, so
// the two agree on every well-formed file. They can differ on a malformed
// one: Load scans to the end, so the last auto_update line wins, while this
// returns on the first match. Nothing but the update check should call it.
// Every failure — absent file, unreadable file, key not present — yields
// DefaultAutoUpdate, because whether the toolbox starts must never hinge on
// reading an optional setting.
func AutoUpdateEnabled() bool {
	f, err := os.Open(basedir.Path(fileName))
	if err != nil {
		return DefaultAutoUpdate
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, val, found := strings.Cut(scanner.Text(), "=")
		if !found || strings.TrimSpace(key) != "auto_update" {
			continue
		}
		return parseBool(val, DefaultAutoUpdate)
	}
	return DefaultAutoUpdate
}
