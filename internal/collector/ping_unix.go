//go:build darwin || linux

// Ping implementation for macOS and Linux using the system `ping` command.
//
// macOS and Linux ping have slightly different timeout flag semantics:
//   - macOS: -W takes milliseconds
//   - Linux: -W takes seconds
//
// pingTimeoutFlag() selects the correct -W value at runtime via GOOS.
package collector

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
)

// pingTimeoutFlag returns the appropriate -W flag value for the current OS.
// macOS ping -W expects milliseconds; Linux ping -W expects seconds.
func pingTimeoutFlag() string {
	if runtime.GOOS == "darwin" {
		return "2000" // 2 seconds in milliseconds
	}
	return "2" // 2 seconds
}

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
	// -W <timeout>: wait for each reply (OS-dependent units)
	cmd := exec.Command("ping", "-c", "3", "-W", pingTimeoutFlag(), target)
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
