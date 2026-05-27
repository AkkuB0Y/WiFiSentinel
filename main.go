// WiFi Sentinel — Local Network Health Monitor
//
// A lightweight daemon that continuously monitors your local network health
// by measuring latency, packet loss, and WiFi signal strength. Data is stored
// in a local SQLite database and served via an embedded web dashboard.
//
// Usage:
//
//	go build -o sentinel ./
//	./sentinel
//
// Environment variables:
//
//	SENTINEL_PING_TARGETS    Comma-separated ping target IPs (default: "8.8.8.8,1.1.1.1")
//	SENTINEL_POLL_INTERVAL   Polling interval duration (default: "5s")
//	SENTINEL_DB_PATH         SQLite database path (default: "./sentinel.db")
//	SENTINEL_HTTP_PORT       HTTP server port (default: 8080)
//	SENTINEL_RETENTION_DAYS  Days to retain data (default: 7)
//	SENTINEL_CLOUD_ENABLED   Enable Firebase cloud sync (default: false)
//	SENTINEL_FIREBASE_PROJECT Firebase project ID
//	SENTINEL_FIREBASE_API_KEY Firebase Web API key
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wifimonitor/internal/api"
	"wifimonitor/internal/cloud"
	"wifimonitor/internal/collector"
	"wifimonitor/internal/config"
	"wifimonitor/internal/db"
	"wifimonitor/web"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[sentinel] ")

	// Banner
	fmt.Println(`
  ╦ ╦╦╔═╗╦  ╔═╗╔═╗╔╗╔╔╦╗╦╔╗╔╔═╗╦  
  ║║║║╠╣ ║  ╚═╗║╣ ║║║ ║ ║║║║║╣ ║  
  ╚╩╝╩╚  ╩  ╚═╝╚═╝╝╚╝ ╩ ╩╝╚╝╚═╝╩═╝
  Local Network Health Monitor v1.0
	`)

	// Load configuration
	cfg := config.LoadConfig()
	log.Printf("config: targets=%v interval=%s port=%d retention=%dd db=%s speedtest=%s cloud=%v",
		cfg.PingTargets, cfg.PollInterval, cfg.HTTPPort, cfg.RetentionDays, cfg.DBPath, cfg.SpeedTestInterval, cfg.CloudEnabled)

	// Initialize database
	store, err := db.NewStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer store.Close()
	log.Println("database initialized")

	// Get embedded static filesystem
	staticFS, err := web.GetFileSystem()
	if err != nil {
		log.Fatalf("failed to load embedded frontend: %v", err)
	}

	// --- Cloud / Firebase Setup ---
	var firebaseClient *cloud.FirebaseClient
	var sessionMgr *cloud.SessionManager
	var sampleBuffer *cloud.SampleBuffer
	var speedTestBuffer *cloud.SpeedTestBuffer

	if cfg.CloudEnabled {
		if cfg.FirebaseProjectID == "" {
			log.Println("[cloud] WARNING: cloud enabled but SENTINEL_FIREBASE_PROJECT not set")
		}
		firebaseClient = cloud.NewFirebaseClient(cfg.FirebaseProjectID, cfg.FirebaseAPIKey)
		sampleBuffer = cloud.NewSampleBuffer()
		speedTestBuffer = cloud.NewSpeedTestBuffer()
		sessionMgr = cloud.NewSessionManager(firebaseClient, sampleBuffer, speedTestBuffer)
		log.Printf("[cloud] Firebase cloud mode enabled — project: %s", cfg.FirebaseProjectID)
	} else {
		log.Println("[cloud] cloud mode disabled — local-only operation")
	}

	// Create speed tester with DB storage callback
	speedTester := collector.NewSpeedTester(func(result collector.SpeedTestResult) {
		if result.Err != nil {
			return
		}
		sample := db.SpeedTestSample{
			Timestamp:    result.Timestamp,
			DownloadMbps: result.DownloadMbps,
			UploadMbps:   result.UploadMbps,
			JitterMs:     result.JitterMs,
			LatencyMs:    result.LatencyMs,
			ServerName:   result.ServerName,
			ServerID:     result.ServerID,
		}
		if err := store.InsertSpeedTest(sample); err != nil {
			log.Printf("[speedtest] failed to store result: %v", err)
		}

		// Also buffer for cloud upload if session is active
		if speedTestBuffer != nil && sessionMgr != nil && sessionMgr.HasActiveSession() {
			speedTestBuffer.Add(cloud.SpeedTestData{
				Timestamp:    result.Timestamp,
				DownloadMbps: result.DownloadMbps,
				UploadMbps:   result.UploadMbps,
				JitterMs:     result.JitterMs,
				LatencyMs:    result.LatencyMs,
				ServerName:   result.ServerName,
			})
		}
	})

	// Create HTTP server
	server := api.NewServer(cfg, store, staticFS, speedTester, sessionMgr, firebaseClient)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start collector in background goroutine
	coll := collector.NewCollector(cfg, store, sampleBuffer, sessionMgr)
	go coll.Start(ctx)

	// Start automatic speed test scheduler if interval > 0
	if cfg.SpeedTestInterval > 0 {
		go func() {
			log.Printf("[speedtest] auto-scheduler enabled — running every %s", cfg.SpeedTestInterval)
			// Run initial speed test after a short delay (let collector start first)
			initialDelay := time.NewTimer(10 * time.Second)
			select {
			case <-ctx.Done():
				initialDelay.Stop()
				return
			case <-initialDelay.C:
			}

			// Run the first test
			testCtx, testCancel := context.WithTimeout(ctx, 120*time.Second)
			speedTester.RunTest(testCtx)
			testCancel()

			// Schedule subsequent tests
			ticker := time.NewTicker(cfg.SpeedTestInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					testCtx, testCancel := context.WithTimeout(ctx, 120*time.Second)
					speedTester.RunTest(testCtx)
					testCancel()
				}
			}
		}()
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("received signal: %v — shutting down...", sig)

		// Stop any active cloud session gracefully
		if sessionMgr != nil && sessionMgr.HasActiveSession() {
			log.Println("[cloud] stopping active session before shutdown...")
			sessionMgr.StopSession()
		}

		cancel() // Stop collector

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	// Start HTTP server
	log.Printf("dashboard available at http://localhost:%d", cfg.HTTPPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}

	log.Println("WiFi Sentinel stopped.")
}

