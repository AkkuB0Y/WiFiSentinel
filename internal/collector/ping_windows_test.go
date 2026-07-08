//go:build windows

package collector

import (
	"testing"
)

func TestWindowsPingParsing(t *testing.T) {
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

	loss := parseWindowsPacketLoss(input)
	if loss != 0 {
		t.Errorf("packet loss = %f, want 0", loss)
	}

	latency := parseWindowsLatency(input)
	if latency < 13.9 || latency > 14.1 {
		t.Errorf("average latency = %f, want ~14", latency)
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

	loss := parseWindowsPacketLoss(input)
	if loss != 33 {
		t.Errorf("packet loss = %f, want 33", loss)
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

	loss := parseWindowsPacketLoss(input)
	if loss != 100 {
		t.Errorf("packet loss = %f, want 100", loss)
	}

	if latency := parseWindowsLatency(input); latency != 0 {
		t.Errorf("latency = %f, want 0 with 100%% loss", latency)
	}
}

func TestWindowsPingParsing_FrenchReplyLines(t *testing.T) {
	input := `
Envoi d'une requête 'Ping'  8.8.8.8 avec 32 octets de données :
Réponse de 8.8.8.8 : octets=32 temps=12 ms TTL=117
Réponse de 8.8.8.8 : octets=32 temps=14 ms TTL=117
Réponse de 8.8.8.8 : octets=32 temps=16 ms TTL=117
`

	latency := parseWindowsLatency(input)
	if latency < 13.9 || latency > 14.1 {
		t.Errorf("average latency = %f, want ~14 from French reply lines", latency)
	}
}
