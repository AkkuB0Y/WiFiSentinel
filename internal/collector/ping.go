// Package collector provides network data collection for WiFi Sentinel.
// This file implements latency and packet loss measurement via the system ping command.
package collector

import (
	"fmt"
	"os/exec"
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

// Regular expressions for parsing macOS/Linux ping output
var (
	// Matches: "3 packets transmitted, 3 packets received, 0.0% packet loss"
	packetLossRe = regexp.MustCompile(`(\d+\.?\d*)% packet loss`)

	// Matches: "round-trip min/avg/max/stddev = 12.345/14.678/16.789/1.234 ms"
	latencyRe = regexp.MustCompile(`=\s*[\d.]+/([\d.]+)/[\d.]+/[\d.]+ ms`)
)

// Ping sends ICMP echo requests to the specified target and returns latency
// and packet loss metrics. It uses the system `ping` command with a 3-packet,
// 2-second timeout configuration for fast, lightweight measurements.
//
// On failure (e.g., host unreachable, command not found), it returns
// 100% packet loss with the error populated.
func Ping(target string) PingResult {
	result := PingResult{
		Target:     target,
		PacketLoss: 100, // assume total loss unless proven otherwise
	}

	// -c 3: send 3 packets
	// -W 2000: wait 2 seconds for each reply (macOS uses milliseconds)
	cmd := exec.Command("ping", "-c", "3", "-W", "2000", target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// ping returns non-zero exit code on packet loss, which is expected.
		// Only treat it as an error if we got zero output.
		if len(output) == 0 {
			result.Err = fmt.Errorf("ping %s failed: %w", target, err)
			return result
		}
	}

	outStr := string(output)

	// Parse packet loss
	if matches := packetLossRe.FindStringSubmatch(outStr); len(matches) > 1 {
		if loss, err := strconv.ParseFloat(matches[1], 64); err == nil {
			result.PacketLoss = loss
		}
	}

	// Parse average latency (only available if at least one packet was received)
	if matches := latencyRe.FindStringSubmatch(outStr); len(matches) > 1 {
		if avg, err := strconv.ParseFloat(matches[1], 64); err == nil {
			result.LatencyMs = avg
		}
	}

	// If we couldn't parse latency but had 0 loss, try alternative parsing
	if result.LatencyMs == 0 && result.PacketLoss < 100 {
		result.LatencyMs = parseIndividualPings(outStr)
	}

	result.Err = nil
	return result
}

// parseIndividualPings extracts latency from individual ping reply lines as a fallback.
// Matches lines like: "64 bytes from 8.8.8.8: icmp_seq=0 ttl=117 time=12.345 ms"
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
