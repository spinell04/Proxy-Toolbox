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
	DefaultWorkers       = 20
	DefaultDownThreshold = 3
	DefaultUpThreshold   = 2
)

// Config holds settings from config.txt.
type Config struct {
	Workers              int
	Domain               string
	DiscordWebhook       string
	DiscordDownThreshold int
	DiscordUpThreshold   int
	PingMaxLatencyMs     int
	TMMaxLatencyMs       int
	BayernMaxLatencyMs   int
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

// Load reads config.txt and returns the parsed Config.
func Load() Config {
	cfg := Config{
		Workers:              DefaultWorkers,
		DiscordDownThreshold: DefaultDownThreshold,
		DiscordUpThreshold:   DefaultUpThreshold,
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
		case "ping_max_latency_ms":
			cfg.PingMaxLatencyMs = parseLatency(val)
		case "tm_max_latency_ms":
			cfg.TMMaxLatencyMs = parseLatency(val)
		case "bayern_max_latency_ms":
			cfg.BayernMaxLatencyMs = parseLatency(val)
		}
	}
	return cfg
}
