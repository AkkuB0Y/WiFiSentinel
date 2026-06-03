// Package collector provides network data collection for WiFi Sentinel.
// This file defines the shared WifiInfo type used across all platforms.
//
// Platform-specific GetWifiInfo() implementations are in:
//   - wifi_darwin.go  — macOS via CoreWLAN Swift helper
//   - wifi_linux.go   — Linux via nmcli (primary) or iwconfig (fallback)
//   - wifi_windows.go — Windows via netsh wlan show interfaces
package collector

// WifiInfo holds WiFi connection details retrieved from the system.
// All platform backends populate this same struct; some fields may be
// unavailable on certain platforms (e.g., Noise is not reported on Windows).
type WifiInfo struct {
	SSID    string // connected network name
	RSSI    int    // signal strength in dBm (negative, closer to 0 = stronger)
	Noise   int    // noise level in dBm (negative); 0 if unavailable
	Channel int    // WiFi channel number
}
