//go:build !darwin && !linux && !windows

package collector

func platformWifiCapabilities() (supported, rssiEstimated, noiseSupported bool) {
	return false, false, false
}
