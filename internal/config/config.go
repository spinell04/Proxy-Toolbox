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
	fileName       = "config.txt"
	DefaultWorkers = 20
)

// Config holds settings from config.txt.
type Config struct {
	Workers int
	Domain  string
}

// Load reads config.txt and returns the parsed Config.
func Load() Config {
	cfg := Config{Workers: DefaultWorkers}
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
		}
	}
	return cfg
}
