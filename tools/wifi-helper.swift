// WiFi Helper — CoreWLAN-based WiFi info tool for WiFi Sentinel
//
// Replaces the deprecated macOS `airport` CLI utility.
// Outputs WiFi connection details in a simple KEY:VALUE format
// that the Go collector can parse quickly.
//
// Build:
//   swiftc -framework CoreWLAN -framework Foundation -O -o wifi-helper tools/wifi-helper.swift
//
// Output format (one per line):
//   SSID:<network name or empty>
//   RSSI:<signal strength in dBm>
//   NOISE:<noise level in dBm>
//   CHANNEL:<channel number>

import Foundation
import CoreWLAN

let client = CWWiFiClient.shared()
guard let iface = client.interface() else {
    // No WiFi interface found
    print("SSID:")
    print("RSSI:0")
    print("NOISE:0")
    print("CHANNEL:0")
    exit(0)
}

let ssid = iface.ssid() ?? ""
let rssi = iface.rssiValue()
let noise = iface.noiseMeasurement()
let channel = iface.wlanChannel()?.channelNumber ?? 0

print("SSID:\(ssid)")
print("RSSI:\(rssi)")
print("NOISE:\(noise)")
print("CHANNEL:\(channel)")
