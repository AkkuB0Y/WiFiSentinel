//go:build windows

// Ping implementation for Windows using the system `ping` command.
//
// Windows ping uses different flags and output format:
//   - Flag: -n (count) instead of -c
//   - Flag: -w (timeout in ms) instead of -W
//   - Output: "Average = 14ms" instead of "min/avg/max/stddev"
//   - Output: "(0% loss)" instead of "0.0% packet loss"
//
// Parsing prefers per-reply latency lines (locale-independent numbers) and
// falls back to localized statistics lines when needed.
package collector

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

// Windows-specific regex patterns for parsing ping output.
var (
	// Matches: "(0% loss)" or "(100% loss)" and common localized variants.
	windowsPacketLossRes = []*regexp.Regexp{
		regexp.MustCompile(`\((\d+)%\s*loss\)`),
		regexp.MustCompile(`(?i)\((\d+)%\s*(?:loss|perte|perdidos|verlust)\)`),
	}

	// Matches: "Average = 14ms" (English locale)
	windowsLatencyRe = regexp.MustCompile(`(?i)Average\s*=\s*<?(\d+)\s*ms`)

	// Matches: "Minimum = 12ms, Maximum = 18ms, Average = 14ms"
	windowsStatsRe = regexp.MustCompile(`(?i)Minimum\s*=\s*(\d+)\s*ms.*Maximum\s*=\s*(\d+)\s*ms.*Average\s*=\s*(\d+)\s*ms`)

	// Sent/Received/Lost fallback for packet loss on localized systems.
	windowsSentRe     = regexp.MustCompile(`(?i)Sent\s*=\s*(\d+)`)
	windowsReceivedRe = regexp.MustCompile(`(?i)Received\s*=\s*(\d+)`)
	windowsLostRe     = regexp.MustCompile(`(?i)Lost\s*=\s*(\d+)`)
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

	result.PacketLoss = parseWindowsPacketLoss(outStr)
	result.LatencyMs = parseWindowsLatency(outStr)

	result.Err = nil
	return result
}

func parseWindowsPacketLoss(outStr string) float64 {
	for _, re := range windowsPacketLossRes {
		if matches := re.FindStringSubmatch(outStr); len(matches) > 1 {
			if loss, err := strconv.ParseFloat(matches[1], 64); err == nil {
				return loss
			}
		}
	}

	sent := parseWindowsPingCount(outStr, windowsSentRe)
	received := parseWindowsPingCount(outStr, windowsReceivedRe)
	lost := parseWindowsPingCount(outStr, windowsLostRe)

	if sent > 0 && lost >= 0 {
		return float64(lost) / float64(sent) * 100
	}
	if sent > 0 && received >= 0 {
		return float64(sent-received) / float64(sent) * 100
	}

	return 100
}

func parseWindowsPingCount(outStr string, re *regexp.Regexp) int {
	if matches := re.FindStringSubmatch(outStr); len(matches) > 1 {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			return n
		}
	}
	return -1
}

func parseWindowsLatency(outStr string) float64 {
	// Prefer per-reply lines — numbers are locale-independent on most Windows builds.
	if avg := parseIndividualPings(outStr); avg > 0 {
		return avg
	}

	if matches := windowsStatsRe.FindStringSubmatch(outStr); len(matches) > 3 {
		if avg, err := strconv.ParseFloat(matches[3], 64); err == nil {
			return avg
		}
	}
	if matches := windowsLatencyRe.FindStringSubmatch(outStr); len(matches) > 1 {
		if avg, err := strconv.ParseFloat(matches[1], 64); err == nil {
			return avg
		}
	}

	return 0
}
