// Package api provides the HTTP API server for WiFi Sentinel.
// This file contains the endpoint handler functions.
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"wifimonitor/internal/collector"
	"wifimonitor/internal/config"
	"wifimonitor/internal/db"
)

// Handlers holds dependencies for API endpoint handlers.
type Handlers struct {
	Store       *db.Store
	Config      *config.Config
	SpeedTester *collector.SpeedTester
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(store *db.Store, cfg *config.Config, st *collector.SpeedTester) *Handlers {
	return &Handlers{Store: store, Config: cfg, SpeedTester: st}
}

// HandleGetStatus returns the latest network sample as JSON.
// GET /api/status
func (h *Handlers) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	sample, err := h.Store.GetLatestSample()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get status")
		log.Printf("[api] status error: %v", err)
		return
	}

	if sample == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "waiting",
			"message": "No data collected yet. The collector is starting up.",
		})
		return
	}

	writeJSON(w, http.StatusOK, sample)
}

// HandleGetHistory returns recent network samples as JSON.
// GET /api/history?since=<ISO8601>&limit=<int>
//
// Query parameters:
//   - since: ISO 8601 timestamp (default: 1 hour ago)
//   - limit: max number of samples (default: 500, max: 5000)
func (h *Handlers) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-time.Hour)
	limit := 500

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 5000 {
				l = 5000
			}
			limit = l
		}
	}

	samples, err := h.Store.GetSamples(since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get history")
		log.Printf("[api] history error: %v", err)
		return
	}

	if samples == nil {
		samples = []db.NetworkSample{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"samples": samples,
		"count":   len(samples),
		"since":   since.Format(time.RFC3339),
	})
}

// HandleGetAggregates returns bucketed aggregate data for chart rendering.
// GET /api/aggregates?since=<ISO8601>&bucket=<minutes>
//
// Query parameters:
//   - since: ISO 8601 timestamp (default: 1 hour ago)
//   - bucket: bucket size in minutes (default: 5, min: 1, max: 60)
func (h *Handlers) HandleGetAggregates(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-time.Hour)
	bucket := 5

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	if bucketStr := r.URL.Query().Get("bucket"); bucketStr != "" {
		if b, err := strconv.Atoi(bucketStr); err == nil && b >= 1 && b <= 60 {
			bucket = b
		}
	}

	aggregates, err := h.Store.GetAggregates(since, bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get aggregates")
		log.Printf("[api] aggregates error: %v", err)
		return
	}

	if aggregates == nil {
		aggregates = []db.AggregateBucket{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"buckets":        aggregates,
		"count":          len(aggregates),
		"since":          since.Format(time.RFC3339),
		"bucket_minutes": bucket,
	})
}

// HandleGetConfig returns the current daemon configuration.
// GET /api/config
func (h *Handlers) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ping_targets":   h.Config.PingTargets,
		"poll_interval":  h.Config.PollInterval.String(),
		"retention_days": h.Config.RetentionDays,
		"http_port":      h.Config.HTTPPort,
	})
}

// writeJSON serializes data to JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[api] json encode error: %v", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// --- Speed Test Handlers ---

// HandleRunSpeedTest triggers a manual speed test.
// POST /api/speedtest/run
func (h *Handlers) HandleRunSpeedTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	if h.SpeedTester == nil {
		writeError(w, http.StatusServiceUnavailable, "speed test not available")
		return
	}

	if h.SpeedTester.IsRunning() {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"status":  "running",
			"message": "A speed test is already in progress",
		})
		return
	}

	// Run the test asynchronously
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		if _, err := h.SpeedTester.RunTest(ctx); err != nil {
			log.Printf("[api] speed test error: %v", err)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":  "started",
		"message": "Speed test started",
	})
}

// HandleGetSpeedTestStatus returns whether a test is running and the latest result.
// GET /api/speedtest/status
func (h *Handlers) HandleGetSpeedTestStatus(w http.ResponseWriter, r *http.Request) {
	running := false
	if h.SpeedTester != nil {
		running = h.SpeedTester.IsRunning()
	}

	latest, err := h.Store.GetLatestSpeedTest()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get speed test status")
		log.Printf("[api] speedtest status error: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"running": running,
		"latest":  latest,
	})
}

// HandleGetSpeedTestHistory returns historical speed test results.
// GET /api/speedtest/history?since=<ISO8601>&limit=<int>
func (h *Handlers) HandleGetSpeedTestHistory(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-7 * 24 * time.Hour) // Default: last 7 days
	limit := 100

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 500 {
				l = 500
			}
			limit = l
		}
	}

	samples, err := h.Store.GetSpeedTestHistory(since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get speed test history")
		log.Printf("[api] speedtest history error: %v", err)
		return
	}

	if samples == nil {
		samples = []db.SpeedTestSample{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"samples": samples,
		"count":   len(samples),
		"since":   since.Format(time.RFC3339),
	})
}
