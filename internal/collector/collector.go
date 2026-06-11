// Package collector provides network data collection for WiFi Sentinel.
// This file implements the main collection orchestrator that runs as a background daemon.
package collector

import (
	"context"
	"log"
	"sync"
	"time"

	"wifimonitor/internal/alerts"
	"wifimonitor/internal/cloud"
	"wifimonitor/internal/config"
	"wifimonitor/internal/db"
)

// Collector orchestrates periodic network health data collection.
// It pings configured targets, gathers WiFi info, and stores results in the database.
type Collector struct {
	cfg         *config.Config
	store       *db.Store
	ticks       uint64 // counts ticks for periodic maintenance
	cloudBuf    *cloud.SampleBuffer
	sessionMgr  *cloud.SessionManager
	alertEngine *alerts.AlertEngine
}

// NewCollector creates a new Collector with the given configuration and database store.
// cloudBuf, sessionMgr, and alertEngine may be nil when their respective features are disabled.
func NewCollector(cfg *config.Config, store *db.Store, cloudBuf *cloud.SampleBuffer, sessionMgr *cloud.SessionManager, alertEngine *alerts.AlertEngine) *Collector {
	return &Collector{
		cfg:         cfg,
		store:       store,
		cloudBuf:    cloudBuf,
		sessionMgr:  sessionMgr,
		alertEngine: alertEngine,
	}
}

// Start begins the collection loop, running at the configured poll interval.
// It blocks until the context is cancelled. This should be called in a goroutine.
//
// Each tick:
//  1. Collects WiFi info (macOS only)
//  2. Pings all configured targets concurrently
//  3. Stores results in the database
//  4. Every 100 ticks, prunes old data based on retention policy
func (c *Collector) Start(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.PollInterval)
	defer ticker.Stop()

	log.Printf("[collector] started — polling every %s, targets: %v", c.cfg.PollInterval, c.cfg.PingTargets)

	// Run an initial collection immediately
	c.collect(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("[collector] stopping...")
			return
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

// collect performs a single round of data collection.
func (c *Collector) collect(ctx context.Context) {
	c.ticks++
	now := time.Now()

	// 1. Get WiFi info (fast, single call)
	wifi, err := GetWifiInfo()
	if err != nil {
		log.Printf("[collector] wifi info error: %v", err)
	}

	// 2. Ping all targets concurrently
	results := c.pingAll(ctx)

	// 3. Store each result as a sample
	for _, pr := range results {
		sample := db.NetworkSample{
			Timestamp:   now,
			Target:      pr.Target,
			LatencyMs:   pr.LatencyMs,
			PacketLoss:  pr.PacketLoss,
			WifiSSID:    wifi.SSID,
			WifiRSSI:    wifi.RSSI,
			WifiNoise:   wifi.Noise,
			WifiChannel: wifi.Channel,
		}

		if err := c.store.InsertSample(sample); err != nil {
			log.Printf("[collector] failed to insert sample for %s: %v", pr.Target, err)
		}

		// Push to cloud buffer if session is active
		if c.cloudBuf != nil && c.sessionMgr != nil && c.sessionMgr.HasActiveSession() {
			c.cloudBuf.Add(cloud.NetworkSampleData{
				Timestamp:   now,
				Target:      pr.Target,
				LatencyMs:   pr.LatencyMs,
				PacketLoss:  pr.PacketLoss,
				WifiSSID:    wifi.SSID,
				WifiRSSI:    wifi.RSSI,
				WifiNoise:   wifi.Noise,
				WifiChannel: wifi.Channel,
			})
		}

		// Evaluate alert rules against this sample (non-blocking)
		if c.alertEngine != nil {
			go c.alertEngine.Evaluate(sample)
		}

		if pr.Err != nil {
			log.Printf("[collector] ping %s error: %v", pr.Target, pr.Err)
		}
	}

	// 4. Periodic maintenance: prune old data every 100 ticks
	if c.ticks%100 == 0 {
		cutoff := time.Now().AddDate(0, 0, -c.cfg.RetentionDays)
		deleted, err := c.store.Prune(cutoff)
		if err != nil {
			log.Printf("[collector] prune error: %v", err)
		} else if deleted > 0 {
			log.Printf("[collector] pruned %d old samples", deleted)
		}
	}
}

// pingAll pings all configured targets concurrently and returns the results.
func (c *Collector) pingAll(ctx context.Context) []PingResult {
	var wg sync.WaitGroup
	results := make([]PingResult, len(c.cfg.PingTargets))

	for i, target := range c.cfg.PingTargets {
		wg.Add(1)
		go func(idx int, t string) {
			defer wg.Done()
			// Check context before starting ping
			select {
			case <-ctx.Done():
				results[idx] = PingResult{Target: t, PacketLoss: 100}
				return
			default:
			}
			results[idx] = Ping(t)
		}(i, target)
	}

	wg.Wait()
	return results
}

