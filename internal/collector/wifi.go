// Package collector provides network data collection for WiFi Sentinel.
// This file implements WiFi signal strength monitoring via the macOS airport CLI utility.
package collector

import (
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// WifiInfo holds WiFi connection details retrieved from the system.
type WifiInfo struct {
	SSID    string // connected network name
	RSSI    int    // signal strength in dBm (negative, closer to 0 = stronger)
	Noise   int    // noise level in dBm (negative)
	Channel int    // WiFi channel number
}

// airportPath is the macOS airport CLI utility location.
const airportPath = "/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport"

// GetWifiInfo retrieves current WiFi connection details from the system.
// On macOS, it uses the airport CLI utility. On other platforms, it returns
// a zero-value WifiInfo with no error (WiFi monitoring is macOS-only).
func GetWifiInfo() (WifiInfo, error) {
	if runtime.GOOS != "darwin" {
		return WifiInfo{}, nil
	}

	cmd := exec.Command(airportPath, "-I")
	output, err := cmd.Output()
	if err != nil {
		return WifiInfo{}, nil // WiFi may be off — not an error
	}

	return parseAirportOutput(string(output)), nil
}

// parseAirportOutput parses the output of `airport -I` into a WifiInfo struct.
//
// Example airport -I output:
//
//	agrCtlRSSI: -54
//	agrExtRSSI: 0
//	agrCtlNoise: -86
//	agrExtNoise: 0
//	state: running
//	op mode: station
//	lastTxRate: 234
//	maxRate: 300
//	lastAssocStatus: 0
//	802.11 auth: open
//	link auth: wpa2-psk
//	BSSID: aa:bb:cc:dd:ee:ff
//	SSID: MyNetwork
//	MCS: 5
//	channel: 6
func parseAirportOutput(output string) WifiInfo {
	info := WifiInfo{}
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "SSID":
			info.SSID = value
		case "agrCtlRSSI":
			if v, err := strconv.Atoi(value); err == nil {
				info.RSSI = v
			}
		case "agrCtlNoise":
			if v, err := strconv.Atoi(value); err == nil {
				info.Noise = v
			}
		case "channel":
			// Channel can be "6" or "6,1" (primary,secondary)
			ch := value
			if idx := strings.Index(ch, ","); idx != -1 {
				ch = ch[:idx]
			}
			// Strip any non-numeric characters
			re := regexp.MustCompile(`\d+`)
			if m := re.FindString(ch); m != "" {
				if v, err := strconv.Atoi(m); err == nil {
					info.Channel = v
				}
			}
		}
	}

	return info
}
