package collector

import (
	"runtime"
	"strings"
	"testing"
)

func TestPlatformName(t *testing.T) {
	name := PlatformName()

	// Should contain the architecture
	if !strings.Contains(name, runtime.GOARCH) {
		t.Errorf("PlatformName() = %q, should contain arch %q", name, runtime.GOARCH)
	}

	// Should contain a human-readable OS name
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(name, "macOS") {
			t.Errorf("PlatformName() = %q, should contain 'macOS' on darwin", name)
		}
	case "linux":
		if !strings.Contains(name, "Linux") {
			t.Errorf("PlatformName() = %q, should contain 'Linux'", name)
		}
	case "windows":
		if !strings.Contains(name, "Windows") {
			t.Errorf("PlatformName() = %q, should contain 'Windows'", name)
		}
	}
}

func TestParseIndividualPings(t *testing.T) {
	// Unix-style ping reply lines
	input := `64 bytes from 8.8.8.8: icmp_seq=0 ttl=117 time=12.345 ms
64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=14.678 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=117 time=16.789 ms`

	avg := parseIndividualPings(input)
	expected := (12.345 + 14.678 + 16.789) / 3

	if avg < expected-0.01 || avg > expected+0.01 {
		t.Errorf("parseIndividualPings() = %f, want ~%f", avg, expected)
	}
}

func TestParseIndividualPings_WindowsStyle(t *testing.T) {
	// Windows-style ping reply lines
	input := `Reply from 8.8.8.8: bytes=32 time=14ms TTL=117
Reply from 8.8.8.8: bytes=32 time=15ms TTL=117
Reply from 8.8.8.8: bytes=32 time=13ms TTL=117`

	avg := parseIndividualPings(input)
	expected := (14.0 + 15.0 + 13.0) / 3

	if avg < expected-0.01 || avg > expected+0.01 {
		t.Errorf("parseIndividualPings() = %f, want ~%f", avg, expected)
	}
}

func TestParseIndividualPings_Empty(t *testing.T) {
	avg := parseIndividualPings("")
	if avg != 0 {
		t.Errorf("parseIndividualPings('') = %f, want 0", avg)
	}
}
