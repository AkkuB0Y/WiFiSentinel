// Package collector provides network data collection for WiFi Sentinel.
// This file implements internet speed testing using the speedtest-go library.
package collector

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

// SpeedTestResult holds the results of a single speed test run.
type SpeedTestResult struct {
	Timestamp    time.Time
	DownloadMbps float64
	UploadMbps   float64
	JitterMs     float64
	LatencyMs    float64
	ServerName   string
	ServerID     string
	Err          error
}

// SpeedTester manages speed test execution with mutex protection
// to prevent concurrent tests.
type SpeedTester struct {
	mu        sync.Mutex
	running   bool
	lastRun   time.Time
	onResult  func(SpeedTestResult) // callback when a test completes
}

// NewSpeedTester creates a new SpeedTester with the given result callback.
func NewSpeedTester(onResult func(SpeedTestResult)) *SpeedTester {
	return &SpeedTester{
		onResult: onResult,
	}
}

// IsRunning returns whether a speed test is currently in progress.
func (st *SpeedTester) IsRunning() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.running
}

// RunTest executes a full speed test (download + upload).
// Returns immediately if a test is already running.
// The test runs in the calling goroutine — use `go RunTest()` for async execution.
func (st *SpeedTester) RunTest(ctx context.Context) (*SpeedTestResult, error) {
	st.mu.Lock()
	if st.running {
		st.mu.Unlock()
		return nil, fmt.Errorf("speed test already in progress")
	}
	st.running = true
	st.mu.Unlock()

	defer func() {
		st.mu.Lock()
		st.running = false
		st.lastRun = time.Now()
		st.mu.Unlock()
	}()

	result := st.executeTest(ctx)

	if st.onResult != nil {
		st.onResult(result)
	}

	return &result, result.Err
}

// executeTest performs the actual speed test using speedtest-go.
func (st *SpeedTester) executeTest(ctx context.Context) SpeedTestResult {
	result := SpeedTestResult{
		Timestamp: time.Now(),
	}

	log.Println("[speedtest] starting speed test...")

	client := speedtest.New()

	// Fetch server list and pick the best one
	serverList, err := client.FetchServers()
	if err != nil || len(serverList) == 0 {
		result.Err = fmt.Errorf("failed to fetch speed test servers: %w", err)
		log.Printf("[speedtest] server fetch error: %v", result.Err)
		return result
	}

	// Pick the best (closest/fastest) server
	targets, err := serverList.FindServer([]int{})
	if err != nil || len(targets) == 0 {
		result.Err = fmt.Errorf("failed to find suitable server: %w", err)
		log.Printf("[speedtest] server selection error: %v", result.Err)
		return result
	}

	server := targets[0]
	result.ServerName = server.Name + " - " + server.Sponsor
	result.ServerID = server.ID

	log.Printf("[speedtest] using server: %s (%s)", result.ServerName, server.Host)

	// Test latency
	err = server.PingTestContext(ctx, nil)
	if err != nil {
		result.Err = fmt.Errorf("ping test failed: %w", err)
		log.Printf("[speedtest] ping error: %v", err)
		return result
	}
	result.LatencyMs = float64(server.Latency.Milliseconds())
	result.JitterMs = float64(server.Jitter.Milliseconds())

	// Download test
	err = server.DownloadTestContext(ctx)
	if err != nil {
		result.Err = fmt.Errorf("download test failed: %w", err)
		log.Printf("[speedtest] download error: %v", err)
		return result
	}
	result.DownloadMbps = server.DLSpeed.Mbps()

	log.Printf("[speedtest] download: %.1f Mbps", result.DownloadMbps)

	// Upload test
	err = server.UploadTestContext(ctx)
	if err != nil {
		result.Err = fmt.Errorf("upload test failed: %w", err)
		log.Printf("[speedtest] upload error: %v", err)
		return result
	}
	result.UploadMbps = server.ULSpeed.Mbps()

	log.Printf("[speedtest] upload: %.1f Mbps", result.UploadMbps)
	log.Printf("[speedtest] complete — ↓%.1f Mbps ↑%.1f Mbps latency: %.0fms jitter: %.0fms",
		result.DownloadMbps, result.UploadMbps, result.LatencyMs, result.JitterMs)

	return result
}
