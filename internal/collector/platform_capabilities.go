// Package collector — platform capability detection (all platforms).
package collector

// PlatformCapabilities describes runtime platform support for the dashboard.
type PlatformCapabilities struct {
	Platform       string `json:"platform"`
	WifiBackend    string `json:"wifi_backend"`
	PingBackend    string `json:"ping_backend"`
	WifiSupported  bool   `json:"wifi_supported"`
	RssiEstimated  bool   `json:"rssi_estimated"`
	NoiseSupported bool   `json:"noise_supported"`
}

// PlatformCapabilities returns support flags for the current OS and detected backends.
func GetPlatformCapabilities() PlatformCapabilities {
	supported, rssiEstimated, noiseSupported := platformWifiCapabilities()
	return PlatformCapabilities{
		Platform:       PlatformName(),
		WifiBackend:    wifiBackendName(),
		PingBackend:    pingBackendName(),
		WifiSupported:  supported,
		RssiEstimated:  rssiEstimated,
		NoiseSupported: noiseSupported,
	}
}
