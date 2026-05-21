// Package config provides configuration management for WiFi Sentinel.
// Configuration is loaded from environment variables with sensible defaults.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for WiFi Sentinel.
type Config struct {
	// PingTargets is a list of IP addresses to ping for latency/loss measurement.
	PingTargets []string

	// PollInterval is how often the collector gathers network samples.
	PollInterval time.Duration

	// DBPath is the filesystem path to the SQLite database file.
	DBPath string

	// HTTPPort is the port the HTTP API server listens on.
	HTTPPort int

	// RetentionDays is how many days of data to keep before pruning.
	RetentionDays int
}

// LoadConfig reads configuration from environment variables, falling back
// to sensible defaults for local network monitoring.
//
// Environment variables:
//   - SENTINEL_PING_TARGETS: comma-separated IPs (default: "8.8.8.8,1.1.1.1")
//   - SENTINEL_POLL_INTERVAL: duration string (default: "5s")
//   - SENTINEL_DB_PATH: path to SQLite DB (default: "./sentinel.db")
//   - SENTINEL_HTTP_PORT: HTTP port (default: 8080)
//   - SENTINEL_RETENTION_DAYS: days to retain data (default: 7)
func LoadConfig() *Config {
	cfg := &Config{
		PingTargets:   []string{"8.8.8.8", "1.1.1.1"},
		PollInterval:  5 * time.Second,
		DBPath:        "./sentinel.db",
		HTTPPort:      8080,
		RetentionDays: 7,
	}

	if targets := os.Getenv("SENTINEL_PING_TARGETS"); targets != "" {
		parts := strings.Split(targets, ",")
		cleaned := make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				cleaned = append(cleaned, t)
			}
		}
		if len(cleaned) > 0 {
			cfg.PingTargets = cleaned
		}
	}

	if interval := os.Getenv("SENTINEL_POLL_INTERVAL"); interval != "" {
		if d, err := time.ParseDuration(interval); err == nil && d > 0 {
			cfg.PollInterval = d
		}
	}

	if dbPath := os.Getenv("SENTINEL_DB_PATH"); dbPath != "" {
		cfg.DBPath = dbPath
	}

	if port := os.Getenv("SENTINEL_HTTP_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil && p > 0 && p < 65536 {
			cfg.HTTPPort = p
		}
	}

	if days := os.Getenv("SENTINEL_RETENTION_DAYS"); days != "" {
		if d, err := strconv.Atoi(days); err == nil && d > 0 {
			cfg.RetentionDays = d
		}
	}

	return cfg
}
