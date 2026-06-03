//go:build linux

package collector

import (
	"testing"
)

func TestParseNmcliDeviceOutput(t *testing.T) {
	input := `GENERAL.DEVICE:wlan0
GENERAL.TYPE:wifi
GENERAL.HWADDR:AA:BB:CC:DD:EE:FF
GENERAL.STATE:100 (connected)
GENERAL.CONNECTION:MyHomeNetwork
WIFI.SSID:MyHomeNetwork
WIFI.MODE:Infra
WIFI.FREQ:5180 MHz
WIFI.SIGNAL:72
WIFI.SECURITY:WPA2
WIFI.CHAN:36
`
	info := parseNmcliDeviceOutput(input)

	if info.SSID != "MyHomeNetwork" {
		t.Errorf("SSID = %q, want %q", info.SSID, "MyHomeNetwork")
	}
	// 72/2 - 100 = -64
	if info.RSSI != -64 {
		t.Errorf("RSSI = %d, want %d", info.RSSI, -64)
	}
	if info.Channel != 36 {
		t.Errorf("Channel = %d, want %d", info.Channel, 36)
	}
}

func TestParseNmcliDeviceOutput_NoWifi(t *testing.T) {
	input := `GENERAL.DEVICE:eth0
GENERAL.TYPE:ethernet
GENERAL.HWADDR:AA:BB:CC:DD:EE:FF
GENERAL.STATE:100 (connected)
GENERAL.CONNECTION:Wired connection 1
`
	info := parseNmcliDeviceOutput(input)

	if info.SSID != "" {
		t.Errorf("SSID = %q, want empty", info.SSID)
	}
	if info.RSSI != 0 {
		t.Errorf("RSSI = %d, want 0", info.RSSI)
	}
}

func TestParseNmcliWifiListOutput(t *testing.T) {
	input := `:OtherNetwork:45:1:2412 MHz
*:MyNetwork:82:36:5180 MHz
:ThirdNetwork:30:11:2462 MHz
`
	info := parseNmcliWifiListOutput(input)

	if info.SSID != "MyNetwork" {
		t.Errorf("SSID = %q, want %q", info.SSID, "MyNetwork")
	}
	// 82/2 - 100 = -59
	if info.RSSI != -59 {
		t.Errorf("RSSI = %d, want %d", info.RSSI, -59)
	}
	if info.Channel != 36 {
		t.Errorf("Channel = %d, want %d", info.Channel, 36)
	}
}

func TestParseIwconfigOutput(t *testing.T) {
	input := `wlan0     IEEE 802.11  ESSID:"CoffeeShopWifi"
          Mode:Managed  Frequency:5.18 GHz  Access Point: AA:BB:CC:DD:EE:FF
          Bit Rate=433.3 Mb/s   Tx-Power=22 dBm
          Retry short limit:7   RTS thr:off   Fragment thr:off
          Power Management:on
          Link Quality=70/70  Signal level=-40 dBm
          Noise level=-95 dBm
          Rx invalid nwid:0  Rx invalid crypt:0  Rx invalid frag:0
          Tx excessive retries:0  Invalid misc:0   Missed beacon:0
`
	info := parseIwconfigOutput(input)

	if info.SSID != "CoffeeShopWifi" {
		t.Errorf("SSID = %q, want %q", info.SSID, "CoffeeShopWifi")
	}
	if info.RSSI != -40 {
		t.Errorf("RSSI = %d, want %d", info.RSSI, -40)
	}
	if info.Noise != -95 {
		t.Errorf("Noise = %d, want %d", info.Noise, -95)
	}
	if info.Channel != 36 {
		t.Errorf("Channel = %d, want %d (from 5.18 GHz)", info.Channel, 36)
	}
}

func TestParseIwconfigOutput_Disconnected(t *testing.T) {
	input := `wlan0     IEEE 802.11  ESSID:off/any
          Mode:Managed  Access Point: Not-Associated
          Tx-Power=22 dBm
          Retry short limit:7   RTS thr:off   Fragment thr:off
          Power Management:on
`
	info := parseIwconfigOutput(input)

	// "off/any" is not a quoted ESSID, so it shouldn't match
	if info.SSID != "" {
		t.Errorf("SSID = %q, want empty (disconnected)", info.SSID)
	}
	if info.RSSI != 0 {
		t.Errorf("RSSI = %d, want 0", info.RSSI)
	}
}

func TestFreqMhzToChannel(t *testing.T) {
	tests := []struct {
		mhz  int
		want int
	}{
		{2412, 1},
		{2437, 6},
		{2462, 11},
		{2484, 14},
		{5180, 36},
		{5240, 48},
		{5745, 149},
		{5825, 165},
		{5955, 1},   // WiFi 6E
		{6115, 33},  // WiFi 6E
		{1000, 0},   // out of range
	}

	for _, tc := range tests {
		got := freqMhzToChannel(tc.mhz)
		if got != tc.want {
			t.Errorf("freqMhzToChannel(%d) = %d, want %d", tc.mhz, got, tc.want)
		}
	}
}

func TestGhzToChannel(t *testing.T) {
	tests := []struct {
		ghz  float64
		want int
	}{
		{2.412, 1},
		{2.437, 6},
		{5.180, 36},
		{5.745, 149},
	}

	for _, tc := range tests {
		got := ghzToChannel(tc.ghz)
		if got != tc.want {
			t.Errorf("ghzToChannel(%.3f) = %d, want %d", tc.ghz, got, tc.want)
		}
	}
}
