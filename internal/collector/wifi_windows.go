//go:build windows

// WiFi collection for Windows using `netsh wlan show interfaces`.
//
// Note: Windows does not expose noise floor via netsh, so WifiInfo.Noise
// will always be 0 on this platform.
package collector

import (
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

var (
	windowsWifiWarned bool
	windowsWifiOnce   sync.Once
	windowsHasNetsh   bool
)

// detectNetsh checks whether netsh is available (it should be on all Windows versions).
func detectNetsh() bool {
	if _, err := exec.LookPath("netsh"); err != nil {
		log.Println("[wifi] WARNING: netsh not found. WiFi metrics will be unavailable.")
		return false
	}
	return true
}

// GetWifiInfo retrieves current WiFi connection details on Windows
// using `netsh wlan show interfaces`.
func GetWifiInfo() (WifiInfo, error) {
	windowsWifiOnce.Do(func() {
		windowsHasNetsh = detectNetsh()
	})

	if !windowsHasNetsh {
		return WifiInfo{}, nil
	}

	cmd := exec.Command("netsh", "wlan", "show", "interfaces")
	output, err := cmd.Output()
	if err != nil {
		if !windowsWifiWarned {
			windowsWifiWarned = true
			log.Printf("[wifi] netsh wlan failed: %v", err)
		}
		return WifiInfo{}, nil
	}

	info := parseNetshOutput(string(output))
	info.RSSIEstimated = true
	info.NoiseAvailable = false
	return info, nil
}

// parseNetshOutput parses the output of `netsh wlan show interfaces`.
// When multiple interfaces are present, returns the connected interface
// with the strongest signal.
func parseNetshOutput(output string) WifiInfo {
	var best WifiInfo
	bestSignalPct := -1

	for _, block := range splitNetshInterfaceBlocks(output) {
		info, signalPct, connected := parseNetshInterfaceBlock(block)
		if !connected {
			continue
		}
		if signalPct > bestSignalPct || (signalPct == bestSignalPct && info.SSID != "" && best.SSID == "") {
			best = info
			bestSignalPct = signalPct
		}
	}

	return best
}

// splitNetshInterfaceBlocks splits netsh output into per-interface blocks.
func splitNetshInterfaceBlocks(output string) []string {
	var blocks []string
	var current strings.Builder

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if isNetshInterfaceHeader(trimmed) && current.Len() > 0 {
			blocks = append(blocks, current.String())
			current.Reset()
		}
		if trimmed != "" {
			current.WriteString(line)
			current.WriteByte('\n')
		}
	}

	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}
	return blocks
}

func isNetshInterfaceHeader(line string) bool {
	key, _, ok := strings.Cut(line, ":")
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(key), "Name")
}

// parseNetshInterfaceBlock parses one netsh interface block.
// Returns the parsed info, raw signal percentage, and whether the interface is connected.
func parseNetshInterfaceBlock(block string) (WifiInfo, int, bool) {
	info := WifiInfo{}
	signalPct := 0
	connected := false

	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		keyLower := strings.ToLower(key)

		switch {
		case keyLower == "ssid" && info.SSID == "":
			info.SSID = value

		case keyLower == "state":
			connected = strings.Contains(strings.ToLower(value), "connected") &&
				!strings.Contains(strings.ToLower(value), "disconnected")

		case keyLower == "signal":
			pctStr := strings.TrimSuffix(value, "%")
			pctStr = strings.TrimSpace(pctStr)
			if pct, err := strconv.Atoi(pctStr); err == nil {
				signalPct = pct
				info.RSSI = qualityPctToDbm(pct)
			}

		case keyLower == "channel":
			if v, err := strconv.Atoi(value); err == nil {
				info.Channel = v
			}
		}
	}

	return info, signalPct, connected
}
