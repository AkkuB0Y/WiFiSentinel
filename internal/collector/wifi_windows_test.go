//go:build windows

package collector

import (
	"testing"
)

func TestParseNetshOutput(t *testing.T) {
	input := `
There is 1 interface on the system:

    Name                   : Wi-Fi
    Description            : Intel(R) Wi-Fi 6 AX201 160MHz
    GUID                   : {12345678-1234-1234-1234-123456789abc}
    Physical address       : aa:bb:cc:dd:ee:ff
    State                  : connected
    SSID                   : MyHomeNetwork
    BSSID                  : 11:22:33:44:55:66
    Network type           : Infrastructure
    Radio type             : 802.11ax
    Authentication         : WPA2-Personal
    Cipher                 : CCMP
    Connection mode        : Profile
    Channel                : 149
    Receive rate (Mbps)    : 1201
    Transmit rate (Mbps)   : 1201
    Signal                 : 85%
    Profile                : MyHomeNetwork
`
	info := parseNetshOutput(input)

	if info.SSID != "MyHomeNetwork" {
		t.Errorf("SSID = %q, want %q", info.SSID, "MyHomeNetwork")
	}
	// 85/2 - 100 = -57 (integer division: 85/2 = 42, 42-100 = -58)
	expectedRSSI := (85 / 2) - 100 // = -58
	if info.RSSI != expectedRSSI {
		t.Errorf("RSSI = %d, want %d", info.RSSI, expectedRSSI)
	}
	if info.Channel != 149 {
		t.Errorf("Channel = %d, want %d", info.Channel, 149)
	}
	if info.Noise != 0 {
		t.Errorf("Noise = %d, want 0 (not available on Windows)", info.Noise)
	}
}

func TestParseNetshOutput_Disconnected(t *testing.T) {
	input := `
There is 1 interface on the system:

    Name                   : Wi-Fi
    Description            : Intel(R) Wi-Fi 6 AX201 160MHz
    GUID                   : {12345678-1234-1234-1234-123456789abc}
    Physical address       : aa:bb:cc:dd:ee:ff
    State                  : disconnected
`
	info := parseNetshOutput(input)

	if info.SSID != "" {
		t.Errorf("SSID = %q, want empty (disconnected)", info.SSID)
	}
	if info.RSSI != 0 {
		t.Errorf("RSSI = %d, want 0", info.RSSI)
	}
}

func TestParseNetshOutput_MultipleInterfaces(t *testing.T) {
	// Ensure we pick up the first SSID from the first connected interface
	input := `
There are 2 interfaces on the system:

    Name                   : Wi-Fi
    Description            : Intel(R) Wi-Fi 6 AX201 160MHz
    State                  : connected
    SSID                   : PrimaryNetwork
    Channel                : 36
    Signal                 : 90%

    Name                   : Wi-Fi 2
    Description            : USB WiFi Adapter
    State                  : connected
    SSID                   : SecondaryNetwork
    Channel                : 6
    Signal                 : 45%
`
	info := parseNetshOutput(input)

	if info.SSID != "PrimaryNetwork" {
		t.Errorf("SSID = %q, want %q (first interface)", info.SSID, "PrimaryNetwork")
	}
	if info.Channel != 36 {
		t.Errorf("Channel = %d, want %d", info.Channel, 36)
	}
}
