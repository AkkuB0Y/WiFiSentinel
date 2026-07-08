//go:build linux

package collector

func platformWifiCapabilities() (supported, rssiEstimated, noiseSupported bool) {
	linuxBackendOnce.Do(func() {
		linuxBackend = detectLinuxBackend()
	})
	switch linuxBackend {
	case "nmcli":
		return true, true, false
	case "iwconfig":
		return true, false, true
	default:
		return false, false, false
	}
}
