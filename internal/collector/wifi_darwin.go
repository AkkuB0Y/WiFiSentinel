//go:build darwin

// WiFi collection for macOS using a CoreWLAN-based Swift helper binary.
// The helper must be compiled separately:
//   swiftc -framework CoreWLAN -framework Foundation -O -o wifi-helper tools/wifi-helper.swift
package collector

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// helperName is the name of the CoreWLAN Swift helper binary.
const helperName = "wifi-helper"

// helperPath caches the resolved path to the wifi-helper binary.
var (
	helperPath     string
	helperPathOnce sync.Once
	helperWarned   bool
)

// resolveHelperPath finds the wifi-helper binary.
// It checks: (1) next to the running executable, (2) in the current directory.
func resolveHelperPath() string {
	// Check next to the running executable
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), helperName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Check current working directory
	if _, err := os.Stat(helperName); err == nil {
		abs, err := filepath.Abs(helperName)
		if err == nil {
			return abs
		}
		return helperName
	}

	return ""
}

// GetWifiInfo retrieves current WiFi connection details on macOS
// using the CoreWLAN-based wifi-helper binary.
func GetWifiInfo() (WifiInfo, error) {
	helperPathOnce.Do(func() {
		helperPath = resolveHelperPath()
		if helperPath != "" {
			log.Printf("[wifi] using helper: %s", helperPath)
		}
	})

	if helperPath == "" {
		if !helperWarned {
			helperWarned = true
			log.Println("[wifi] WARNING: wifi-helper binary not found. WiFi metrics will be unavailable.")
			log.Println("[wifi] Build it with: swiftc -framework CoreWLAN -framework Foundation -O -o wifi-helper tools/wifi-helper.swift")
		}
		return WifiInfo{}, nil
	}

	cmd := exec.Command(helperPath)
	output, err := cmd.Output()
	if err != nil {
		return WifiInfo{}, nil // Helper failed — WiFi may be off
	}

	info := parseHelperOutput(string(output))

	// If RSSI is non-zero but SSID is empty, macOS is redacting the network name
	// for privacy. Show a helpful status instead of appearing disconnected.
	if info.RSSI != 0 && info.SSID == "" {
		info.SSID = "Connected (Private)"
	}

	return info, nil
}

// parseHelperOutput parses the output of the wifi-helper binary.
//
// Expected format (one per line):
//
//	SSID:MyNetwork
//	RSSI:-68
//	NOISE:-97
//	CHANNEL:48
func parseHelperOutput(output string) WifiInfo {
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

		key := parts[0]
		value := parts[1]

		switch key {
		case "SSID":
			info.SSID = value
		case "RSSI":
			if v, err := strconv.Atoi(value); err == nil {
				info.RSSI = v
			}
		case "NOISE":
			if v, err := strconv.Atoi(value); err == nil {
				info.Noise = v
			}
		case "CHANNEL":
			if v, err := strconv.Atoi(value); err == nil {
				info.Channel = v
			}
		}
	}

	return info
}
