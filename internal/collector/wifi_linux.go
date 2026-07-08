//go:build linux

// WiFi collection for Linux using nmcli (primary) or iwconfig (fallback).
//
// nmcli is preferred because it provides structured, machine-readable output
// and is available on most modern Linux distributions with NetworkManager.
// iwconfig is used as a fallback for minimal server/embedded environments.
package collector

import (
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var (
	linuxBackendOnce sync.Once
	linuxBackend     string // "nmcli", "iwconfig", or ""
)

// detectLinuxBackend finds the best available WiFi tool.
func detectLinuxBackend() string {
	if path, err := exec.LookPath("nmcli"); err == nil {
		log.Printf("[wifi] using nmcli backend: %s", path)
		return "nmcli"
	}
	if path, err := exec.LookPath("iwconfig"); err == nil {
		log.Printf("[wifi] using iwconfig backend: %s", path)
		return "iwconfig"
	}
	log.Println("[wifi] WARNING: neither nmcli nor iwconfig found. WiFi metrics will be unavailable.")
	log.Println("[wifi] Install NetworkManager (nmcli) or wireless-tools (iwconfig) for WiFi monitoring.")
	return ""
}

// GetWifiInfo retrieves current WiFi connection details on Linux.
// It tries nmcli first, then falls back to iwconfig.
func GetWifiInfo() (WifiInfo, error) {
	linuxBackendOnce.Do(func() {
		linuxBackend = detectLinuxBackend()
	})

	switch linuxBackend {
	case "nmcli":
		return getWifiInfoNmcli()
	case "iwconfig":
		return getWifiInfoIwconfig()
	default:
		return WifiInfo{}, nil
	}
}

// getWifiInfoNmcli uses NetworkManager's nmcli to get WiFi details
// for the currently connected WiFi device.
func getWifiInfoNmcli() (WifiInfo, error) {
	device := findConnectedWifiDevice()
	if device == "" {
		return getWifiInfoNmcliWifiList()
	}

	cmd := exec.Command("nmcli", "-t", "-f",
		"WIFI.SSID,WIFI.SIGNAL,WIFI.CHAN,WIFI.FREQ",
		"device", "show", device)
	output, err := cmd.Output()
	if err != nil {
		return getWifiInfoNmcliWifiList()
	}

	info := parseNmcliDeviceOutput(string(output))
	info.RSSIEstimated = true
	info.NoiseAvailable = false
	return info, nil
}

// findConnectedWifiDevice returns the name of the first connected WiFi interface.
func findConnectedWifiDevice() string {
	cmd := exec.Command("nmcli", "-t", "-f", "DEVICE,TYPE,STATE", "device")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		device := strings.TrimSpace(parts[0])
		typ := strings.TrimSpace(parts[1])
		state := strings.TrimSpace(parts[2])
		if typ == "wifi" && strings.Contains(state, "connected") {
			return device
		}
	}
	return ""
}

// getWifiInfoNmcliWifiList uses `nmcli device wifi list` as a secondary approach.
func getWifiInfoNmcliWifiList() (WifiInfo, error) {
	cmd := exec.Command("nmcli", "-t", "-f", "IN-USE,SSID,SIGNAL,CHAN,FREQ", "device", "wifi", "list")
	output, err := cmd.Output()
	if err != nil {
		return WifiInfo{}, nil
	}

	info := parseNmcliWifiListOutput(string(output))
	info.RSSIEstimated = true
	info.NoiseAvailable = false
	return info, nil
}

// parseNmcliDeviceOutput parses `nmcli -t -f ... device show` output.
//
// Expected format (colon-separated key:value lines across multiple devices):
//
//	GENERAL.CONNECTION:MyNetwork
//	WIFI.SSID:MyNetwork
//	WIFI.SIGNAL:72
//	WIFI.CHAN:36
//	WIFI.FREQ:5180 MHz
func parseNmcliDeviceOutput(output string) WifiInfo {
	info := WifiInfo{}

	for _, line := range strings.Split(output, "\n") {
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

		switch key {
		case "WIFI.SSID":
			if value != "" && value != "--" {
				info.SSID = value
			}
		case "WIFI.SIGNAL":
			if v, err := strconv.Atoi(value); err == nil {
				// nmcli reports signal as 0-100 quality percentage.
				info.RSSI = qualityPctToDbm(v)
			}
		case "WIFI.CHAN":
			if v, err := strconv.Atoi(value); err == nil {
				info.Channel = v
			}
		}
	}

	return info
}

// parseNmcliWifiListOutput parses `nmcli -t -f IN-USE,SSID,SIGNAL,CHAN,FREQ device wifi list`.
// Finds the line with "*" in IN-USE (the currently connected network).
//
// Expected format (colon-separated, one AP per line):
//
//	*:MyNetwork:72:36:5180 MHz
//	:OtherNetwork:45:1:2412 MHz
func parseNmcliWifiListOutput(output string) WifiInfo {
	info := WifiInfo{}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// nmcli -t uses ":" as separator; IN-USE field is "*" when connected
		// Format: IN-USE:SSID:SIGNAL:CHAN:FREQ
		// Note: SSID itself could contain ":" — use SplitN carefully
		if !strings.HasPrefix(line, "*:") {
			continue
		}

		// Remove the "*:" prefix
		rest := line[2:]
		// Split from the right for known-width fields (FREQ, CHAN, SIGNAL)
		// But it's easier to split and work backwards
		parts := strings.Split(rest, ":")
		if len(parts) < 4 {
			continue
		}

		// Last field is FREQ (e.g., "5180 MHz"), second-to-last is CHAN, etc.
		// SSID is everything up to the third-to-last separator
		freqIdx := len(parts) - 1
		chanIdx := len(parts) - 2
		signalIdx := len(parts) - 3
		ssidParts := parts[:signalIdx]

		info.SSID = strings.Join(ssidParts, ":")

		if v, err := strconv.Atoi(parts[signalIdx]); err == nil {
			info.RSSI = qualityPctToDbm(v)
		}
		if v, err := strconv.Atoi(parts[chanIdx]); err == nil {
			info.Channel = v
		}

		// Try to extract channel from frequency if channel wasn't parsed
		if info.Channel == 0 && freqIdx < len(parts) {
			info.Channel = freqToChannel(parts[freqIdx])
		}

		break // Found the connected network
	}

	return info
}

// getWifiInfoIwconfig uses the legacy iwconfig command to get WiFi details.
func getWifiInfoIwconfig() (WifiInfo, error) {
	// iwconfig without args prints all wireless interfaces
	cmd := exec.Command("iwconfig")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// iwconfig returns non-zero if no wireless extensions, but may still have output
		if len(output) == 0 {
			return WifiInfo{}, nil
		}
	}

	return finalizeIwconfigInfo(parseIwconfigOutput(string(output))), nil
}

func finalizeIwconfigInfo(info WifiInfo) WifiInfo {
	info.RSSIEstimated = false
	info.NoiseAvailable = info.RSSI != 0 || info.Noise != 0
	return info
}

// parseIwconfigOutput parses iwconfig output for WiFi details.
//
// Example iwconfig output:
//
//	wlan0     IEEE 802.11  ESSID:"MyNetwork"
//	          Mode:Managed  Frequency:5.18 GHz  Access Point: AA:BB:CC:DD:EE:FF
//	          Bit Rate=433.3 Mb/s   Tx-Power=22 dBm
//	          Link Quality=70/70  Signal level=-40 dBm
//	          Noise level=-95 dBm
func parseIwconfigOutput(output string) WifiInfo {
	info := WifiInfo{}

	// Extract ESSID
	essidRe := regexp.MustCompile(`ESSID:"([^"]*)"`)
	if matches := essidRe.FindStringSubmatch(output); len(matches) > 1 {
		info.SSID = matches[1]
	}

	// Extract Signal level in dBm
	signalRe := regexp.MustCompile(`Signal level[=:](-?\d+)\s*dBm`)
	if matches := signalRe.FindStringSubmatch(output); len(matches) > 1 {
		if v, err := strconv.Atoi(matches[1]); err == nil {
			info.RSSI = v
		}
	}

	// Extract Noise level in dBm
	noiseRe := regexp.MustCompile(`Noise level[=:](-?\d+)\s*dBm`)
	if matches := noiseRe.FindStringSubmatch(output); len(matches) > 1 {
		if v, err := strconv.Atoi(matches[1]); err == nil {
			info.Noise = v
		}
	}

	// Extract Frequency and convert to channel
	freqRe := regexp.MustCompile(`Frequency[=:](\d+\.?\d*)\s*GHz`)
	if matches := freqRe.FindStringSubmatch(output); len(matches) > 1 {
		if freq, err := strconv.ParseFloat(matches[1], 64); err == nil {
			info.Channel = ghzToChannel(freq)
		}
	}

	// Also try direct channel extraction (some iwconfig versions show it)
	if info.Channel == 0 {
		chanRe := regexp.MustCompile(`Channel[=:](\d+)`)
		if matches := chanRe.FindStringSubmatch(output); len(matches) > 1 {
			if v, err := strconv.Atoi(matches[1]); err == nil {
				info.Channel = v
			}
		}
	}

	return info
}

// ghzToChannel converts a frequency in GHz to a WiFi channel number.
func ghzToChannel(ghz float64) int {
	mhz := int(ghz * 1000)
	return freqMhzToChannel(mhz)
}

// freqToChannel extracts MHz from a frequency string like "5180 MHz" and converts to channel.
func freqToChannel(freq string) int {
	freq = strings.TrimSpace(freq)
	freq = strings.TrimSuffix(freq, " MHz")
	freq = strings.TrimSpace(freq)
	if mhz, err := strconv.Atoi(freq); err == nil {
		return freqMhzToChannel(mhz)
	}
	return 0
}

// freqMhzToChannel converts a frequency in MHz to a WiFi channel number.
// Covers 2.4 GHz (channels 1-14) and 5 GHz (channels 36-165) bands.
func freqMhzToChannel(mhz int) int {
	switch {
	case mhz >= 2412 && mhz <= 2472:
		return (mhz - 2407) / 5
	case mhz == 2484:
		return 14
	case mhz >= 5180 && mhz <= 5825:
		return (mhz - 5000) / 5
	case mhz >= 5955 && mhz <= 7115:
		// WiFi 6E (6 GHz band)
		return (mhz - 5950) / 5
	default:
		return 0
	}
}
