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

	return info, nil
}

// parseNetshOutput parses the output of `netsh wlan show interfaces`.
//
// Example output:
//
//	There is 1 interface on the system:
//
//	    Name                   : Wi-Fi
//	    Description            : Intel(R) Wi-Fi 6 AX201
//	    GUID                   : {12345678-...}
//	    Physical address       : aa:bb:cc:dd:ee:ff
//	    State                  : connected
//	    SSID                   : MyNetwork
//	    BSSID                  : aa:bb:cc:dd:ee:ff
//	    Network type           : Infrastructure
//	    Radio type             : 802.11ax
//	    Authentication         : WPA2-Personal
//	    Cipher                 : CCMP
//	    Connection mode        : Profile
//	    Channel                : 36
//	    Receive rate (Mbps)    : 1201
//	    Transmit rate (Mbps)   : 1201
//	    Signal                 : 85%
//	    Profile                : MyNetwork
func parseNetshOutput(output string) WifiInfo {
	info := WifiInfo{}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Each line is "Key : Value" format
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		// Trim and normalize the key (handle leading whitespace in key)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Use case-insensitive matching for robustness across locales
		keyLower := strings.ToLower(key)

		switch {
		case keyLower == "ssid" && info.SSID == "":
			// Only take the first SSID (skip BSSID which comes later)
			info.SSID = value

		case keyLower == "signal":
			// Signal is reported as percentage (e.g., "85%")
			pctStr := strings.TrimSuffix(value, "%")
			pctStr = strings.TrimSpace(pctStr)
			if pct, err := strconv.Atoi(pctStr); err == nil {
				// Convert percentage to approximate dBm:
				// Microsoft uses a non-linear mapping, but linear approx is:
				// dBm ≈ (quality / 2) - 100
				info.RSSI = (pct / 2) - 100
			}

		case keyLower == "channel":
			if v, err := strconv.Atoi(value); err == nil {
				info.Channel = v
			}

		case keyLower == "radio type":
			// Log radio type for informational purposes
			log.Printf("[wifi] radio type: %s", value)
		}
	}

	// Noise is not available via netsh on Windows
	// info.Noise remains 0

	return info
}
