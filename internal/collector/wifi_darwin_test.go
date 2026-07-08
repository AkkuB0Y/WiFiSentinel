//go:build darwin

package collector

import (
	"testing"
)

func TestParseHelperOutput(t *testing.T) {
	input := `SSID:MyNetwork
RSSI:-68
NOISE:-97
CHANNEL:48
`
	info := parseHelperOutput(input)

	if info.SSID != "MyNetwork" {
		t.Errorf("SSID = %q, want %q", info.SSID, "MyNetwork")
	}
	if info.RSSI != -68 {
		t.Errorf("RSSI = %d, want %d", info.RSSI, -68)
	}
	if info.Noise != -97 {
		t.Errorf("Noise = %d, want %d", info.Noise, -97)
	}
	if info.Channel != 48 {
		t.Errorf("Channel = %d, want %d", info.Channel, 48)
	}
}

func TestParseHelperOutput_Empty(t *testing.T) {
	input := `SSID:
RSSI:0
NOISE:0
CHANNEL:0
`
	info := parseHelperOutput(input)

	if info.SSID != "" {
		t.Errorf("SSID = %q, want empty", info.SSID)
	}
	if info.RSSI != 0 {
		t.Errorf("RSSI = %d, want 0", info.RSSI)
	}
}

func TestParseHelperOutput_NoOutput(t *testing.T) {
	info := parseHelperOutput("")

	if info.SSID != "" {
		t.Errorf("SSID = %q, want empty", info.SSID)
	}
}

func TestParseHelperOutput_PartialOutput(t *testing.T) {
	// Only some fields present
	input := `SSID:PartialNetwork
RSSI:-55
`
	info := parseHelperOutput(input)

	if info.SSID != "PartialNetwork" {
		t.Errorf("SSID = %q, want %q", info.SSID, "PartialNetwork")
	}
	if info.RSSI != -55 {
		t.Errorf("RSSI = %d, want %d", info.RSSI, -55)
	}
	if info.Noise != 0 {
		t.Errorf("Noise = %d, want 0 (not provided)", info.Noise)
	}
	if info.Channel != 0 {
		t.Errorf("Channel = %d, want 0 (not provided)", info.Channel)
	}
}
