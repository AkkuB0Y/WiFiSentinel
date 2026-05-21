// Package api provides the HTTP API server for WiFi Sentinel.
// This file handles route registration.
package api

import (
	"net/http"

	"wifimonitor/internal/config"
	"wifimonitor/internal/db"
)

// RegisterRoutes sets up all API endpoint routes on the given ServeMux.
func RegisterRoutes(mux *http.ServeMux, store *db.Store, cfg *config.Config) {
	h := NewHandlers(store, cfg)

	mux.HandleFunc("/api/status", h.HandleGetStatus)
	mux.HandleFunc("/api/history", h.HandleGetHistory)
	mux.HandleFunc("/api/aggregates", h.HandleGetAggregates)
	mux.HandleFunc("/api/config", h.HandleGetConfig)
}
