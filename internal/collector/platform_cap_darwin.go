//go:build darwin

package collector

func platformWifiCapabilities() (supported, rssiEstimated, noiseSupported bool) {
	return resolveHelperPath() != "", false, true
}
