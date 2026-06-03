//go:build windows

package collector

import (
	"testing"
)

func TestWindowsPingParsing(t *testing.T) {
	// Typical Windows ping output
	input := `
Pinging 8.8.8.8 with 32 bytes of data:
Reply from 8.8.8.8: bytes=32 time=14ms TTL=117
Reply from 8.8.8.8: bytes=32 time=15ms TTL=117
Reply from 8.8.8.8: bytes=32 time=13ms TTL=117

Ping statistics for 8.8.8.8:
    Packets: Sent = 3, Received = 3, Lost = 0 (0% loss),
Approximate round trip times in milli-seconds:
    Minimum = 13ms, Maximum = 15ms, Average = 14ms
`
	// Test packet loss parsing
	if matches := windowsPacketLossRe.FindStringSubmatch(input); len(matches) > 1 {
		if matches[1] != "0" {
			t.Errorf("packet loss = %q, want %q", matches[1], "0")
		}
	} else {
		t.Error("failed to parse packet loss from Windows ping output")
	}

	// Test latency parsing
	if matches := windowsStatsRe.FindStringSubmatch(input); len(matches) > 3 {
		if matches[3] != "14" {
			t.Errorf("average latency = %q, want %q", matches[3], "14")
		}
	} else {
		t.Error("failed to parse latency statistics from Windows ping output")
	}
}

func TestWindowsPingParsing_PartialLoss(t *testing.T) {
	input := `
Pinging 10.0.0.1 with 32 bytes of data:
Reply from 10.0.0.1: bytes=32 time=5ms TTL=64
Request timed out.
Reply from 10.0.0.1: bytes=32 time=8ms TTL=64

Ping statistics for 10.0.0.1:
    Packets: Sent = 3, Received = 2, Lost = 1 (33% loss),
Approximate round trip times in milli-seconds:
    Minimum = 5ms, Maximum = 8ms, Average = 6ms
`
	if matches := windowsPacketLossRe.FindStringSubmatch(input); len(matches) > 1 {
		if matches[1] != "33" {
			t.Errorf("packet loss = %q, want %q", matches[1], "33")
		}
	} else {
		t.Error("failed to parse packet loss")
	}
}

func TestWindowsPingParsing_TotalLoss(t *testing.T) {
	input := `
Pinging 192.168.99.99 with 32 bytes of data:
Request timed out.
Request timed out.
Request timed out.

Ping statistics for 192.168.99.99:
    Packets: Sent = 3, Received = 0, Lost = 3 (100% loss),
`
	if matches := windowsPacketLossRe.FindStringSubmatch(input); len(matches) > 1 {
		if matches[1] != "100" {
			t.Errorf("packet loss = %q, want %q", matches[1], "100")
		}
	} else {
		t.Error("failed to parse 100% packet loss")
	}

	// No stats line should exist for total loss
	if matches := windowsStatsRe.FindStringSubmatch(input); len(matches) > 0 {
		t.Error("should not have latency stats with 100% loss")
	}
}
