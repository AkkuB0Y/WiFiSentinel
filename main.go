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
	log.Printf("config: targets=%v interval=%s port=%d retention=%dd db=%s",
		cfg.PingTargets, cfg.PollInterval, cfg.HTTPPort, cfg.RetentionDays, cfg.DBPath)

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

	// Create HTTP server
	server := api.NewServer(cfg, store, staticFS)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start collector in background goroutine
	coll := collector.NewCollector(cfg, store)
	go coll.Start(ctx)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("received signal: %v — shutting down...", sig)
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
