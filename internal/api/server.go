// Package api provides the HTTP API server for WiFi Sentinel.
// This file sets up the HTTP server with middleware.
package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"wifimonitor/internal/cloud"
	"wifimonitor/internal/collector"
	"wifimonitor/internal/config"
	"wifimonitor/internal/db"
)

// NewServer creates and configures an HTTP server with all routes and middleware.
// It serves both the API endpoints and the embedded static frontend.
func NewServer(cfg *config.Config, store *db.Store, staticFS http.FileSystem, st *collector.SpeedTester, sm *cloud.SessionManager, fc *cloud.FirebaseClient) *http.Server {
	mux := http.NewServeMux()

	// Register API routes
	RegisterRoutes(mux, store, cfg, st, sm, fc)

	// Serve embedded static frontend
	if staticFS != nil {
		fileServer := http.FileServer(staticFS)
		mux.Handle("/", fileServer)
	}

	// Wrap with middleware
	handler := loggingMiddleware(corsMiddleware(mux))

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// corsMiddleware adds CORS headers for local development.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs each request with method, path, status, and duration.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		// Only log API requests to avoid flooding logs with static file requests
		if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
			log.Printf("[api] %s %s → %d (%s)",
				r.Method, r.URL.Path, wrapped.statusCode, time.Since(start).Round(time.Microsecond))
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
