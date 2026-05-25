// Package api provides the HTTP API server for WiFi Sentinel.
// This file handles route registration.
package api

import (
	"net/http"

	"wifimonitor/internal/cloud"
	"wifimonitor/internal/collector"
	"wifimonitor/internal/config"
	"wifimonitor/internal/db"
)

// RegisterRoutes sets up all API endpoint routes on the given ServeMux.
func RegisterRoutes(mux *http.ServeMux, store *db.Store, cfg *config.Config, st *collector.SpeedTester, sm *cloud.SessionManager, fc *cloud.FirebaseClient) {
	h := NewHandlers(store, cfg, st, sm, fc)

	mux.HandleFunc("/api/status", h.HandleGetStatus)
	mux.HandleFunc("/api/history", h.HandleGetHistory)
	mux.HandleFunc("/api/aggregates", h.HandleGetAggregates)
	mux.HandleFunc("/api/config", h.HandleGetConfig)

	// Speed test endpoints
	mux.HandleFunc("/api/speedtest/run", h.HandleRunSpeedTest)
	mux.HandleFunc("/api/speedtest/status", h.HandleGetSpeedTestStatus)
	mux.HandleFunc("/api/speedtest/history", h.HandleGetSpeedTestHistory)

	// Cloud / session endpoints
	mux.HandleFunc("/api/cloud/auth", h.HandleCloudAuth)
	mux.HandleFunc("/api/cloud/status", h.HandleCloudStatus)
	mux.HandleFunc("/api/cloud/sessions", h.HandleGetSessions)
	mux.HandleFunc("/api/session/start", h.HandleSessionStart)
	mux.HandleFunc("/api/session/stop", h.HandleSessionStop)
	mux.HandleFunc("/api/session/active", h.HandleGetActiveSession)
}

