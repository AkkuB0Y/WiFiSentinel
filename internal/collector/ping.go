// Package collector provides network data collection for WiFi Sentinel.
// This file defines shared ping types and parsing helpers used across all platforms.
//
// Platform-specific Ping() implementations are in:
//   - ping_unix.go    — macOS and Linux via system `ping`
//   - ping_windows.go — Windows via system `ping`
package collector

import (
	"regexp"
	"strconv"
	"strings"
)

// PingResult holds the results of a single ping operation against a target.
type PingResult struct {
	Target     string  // IP address that was pinged
	LatencyMs  float64 // average round-trip time in milliseconds
	PacketLoss float64 // packet loss percentage (0-100)
	Err        error   // non-nil if the ping command failed entirely
}

// Regular expressions for parsing macOS/Linux ping output.
// These are also used by ping_unix.go.
var (
	// Matches: "3 packets transmitted, 3 packets received, 0.0% packet loss"
	packetLossRe = regexp.MustCompile(`(\d+\.?\d*)% packet loss`)

	// Matches: "round-trip min/avg/max/stddev = 12.345/14.678/16.789/1.234 ms"
	latencyRe = regexp.MustCompile(`=\s*[\d.]+/([\d.]+)/[\d.]+/[\d.]+ ms`)
)

// parseIndividualPings extracts latency from individual ping reply lines as a fallback.
// Matches lines like: "64 bytes from 8.8.8.8: icmp_seq=0 ttl=117 time=12.345 ms"
// Used by both Unix and Windows backends.
func parseIndividualPings(output string) float64 {
	re := regexp.MustCompile(`time[=<]([\d.]+)\s*ms`)
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0
	}

	var total float64
	var count int
	for _, m := range matches {
		if len(m) > 1 {
			if v, err := strconv.ParseFloat(strings.TrimSpace(m[1]), 64); err == nil {
				total += v
				count++
			}
		}
	}

	if count == 0 {
		return 0
	}
	return total / float64(count)
}
