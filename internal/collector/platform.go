// Package collector provides network data collection for WiFi Sentinel.
// This file implements platform detection and startup logging.
// It compiles on all platforms (no build tags).
package collector

import (
	"fmt"
	"log"
	"runtime"
)

// PlatformName returns a human-readable string identifying the current
// operating system and architecture, e.g., "macOS (arm64)", "Linux (amd64)",
// or "Windows (amd64)".
func PlatformName() string {
	osName := runtime.GOOS
	switch osName {
	case "darwin":
		osName = "macOS"
	case "linux":
		osName = "Linux"
	case "windows":
		osName = "Windows"
	}
	return fmt.Sprintf("%s (%s)", osName, runtime.GOARCH)
}

// wifiBackendName returns the name of the WiFi collection backend for the current OS.
func wifiBackendName() string {
	switch runtime.GOOS {
	case "darwin":
		return "CoreWLAN Swift helper"
	case "linux":
		return "nmcli / iwconfig"
	case "windows":
		return "netsh wlan"
	default:
		return "none (unsupported OS)"
	}
}

// pingBackendName returns the name of the ping backend for the current OS.
func pingBackendName() string {
	switch runtime.GOOS {
	case "darwin", "linux":
		return "unix ping"
	case "windows":
		return "windows ping"
	default:
		return "unknown"
	}
}

// LogPlatformInfo logs the detected platform, WiFi backend, and ping backend
// at startup. Call this from main() before starting the collector.
func LogPlatformInfo() {
	log.Printf("[platform] OS: %s", PlatformName())
	log.Printf("[platform] WiFi backend: %s", wifiBackendName())
	log.Printf("[platform] Ping backend: %s", pingBackendName())
}
