//go:build windows

package collector

func platformWifiCapabilities() (supported, rssiEstimated, noiseSupported bool) {
	return detectNetsh(), true, false
}
