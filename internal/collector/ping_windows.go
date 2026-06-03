//go:build windows

// Ping implementation for Windows using the system `ping` command.
//
// Windows ping uses different flags and output format:
//   - Flag: -n (count) instead of -c
//   - Flag: -w (timeout in ms) instead of -W
//   - Output: "Average = 14ms" instead of "min/avg/max/stddev"
//   - Output: "(0% loss)" instead of "0.0% packet loss"
package collector

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

// Windows-specific regex patterns for parsing ping output.
var (
	// Matches: "(0% loss)" or "(100% loss)"
	windowsPacketLossRe = regexp.MustCompile(`\((\d+)%\s*loss\)`)

	// Matches: "Average = 14ms" (English locale)
	// Also handles: "Average = <1ms"
	windowsLatencyRe = regexp.MustCompile(`(?i)Average\s*=\s*<?(\d+)\s*ms`)

	// Matches: "Minimum = 12ms, Maximum = 18ms, Average = 14ms"
	windowsStatsRe = regexp.MustCompile(`(?i)Minimum\s*=\s*(\d+)\s*ms.*Maximum\s*=\s*(\d+)\s*ms.*Average\s*=\s*(\d+)\s*ms`)
)

// Ping sends ICMP echo requests to the specified target on Windows and returns
// latency and packet loss metrics.
//
// Uses `ping -n 3 -w 2000 <target>` for fast, lightweight measurements.
func Ping(target string) PingResult {
	result := PingResult{
		Target:     target,
		PacketLoss: 100, // assume total loss unless proven otherwise
	}

	// -n 3: send 3 packets
	// -w 2000: wait 2 seconds for each reply (milliseconds)
	cmd := exec.Command("ping", "-n", "3", "-w", "2000", target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) == 0 {
			result.Err = fmt.Errorf("ping %s failed: %w", target, err)
			return result
		}
	}

	outStr := string(output)

	// Parse packet loss
	if matches := windowsPacketLossRe.FindStringSubmatch(outStr); len(matches) > 1 {
		if loss, err := strconv.ParseFloat(matches[1], 64); err == nil {
			result.PacketLoss = loss
		}
	}

	// Parse average latency from statistics line
	if matches := windowsStatsRe.FindStringSubmatch(outStr); len(matches) > 3 {
		if avg, err := strconv.ParseFloat(matches[3], 64); err == nil {
			result.LatencyMs = avg
		}
	} else if matches := windowsLatencyRe.FindStringSubmatch(outStr); len(matches) > 1 {
		if avg, err := strconv.ParseFloat(matches[1], 64); err == nil {
			result.LatencyMs = avg
		}
	}

	// Fallback: parse individual reply lines
	// Windows format: "Reply from 8.8.8.8: bytes=32 time=14ms TTL=117"
	if result.LatencyMs == 0 && result.PacketLoss < 100 {
		result.LatencyMs = parseIndividualPings(outStr)
	}

	result.Err = nil
	return result
}
