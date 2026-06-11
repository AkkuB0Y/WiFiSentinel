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

	"wifimonitor/internal/alerts"
	"wifimonitor/internal/cloud"
	"wifimonitor/internal/collector"
	"wifimonitor/internal/config"
	"wifimonitor/internal/db"
)

// Handlers holds dependencies for API endpoint handlers.
type Handlers struct {
	Store          *db.Store
	Config         *config.Config
	SpeedTester    *collector.SpeedTester
	SessionManager *cloud.SessionManager
	FirebaseClient *cloud.FirebaseClient
	AlertEngine    *alerts.AlertEngine
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(store *db.Store, cfg *config.Config, st *collector.SpeedTester, sm *cloud.SessionManager, fc *cloud.FirebaseClient, ae *alerts.AlertEngine) *Handlers {
	return &Handlers{Store: store, Config: cfg, SpeedTester: st, SessionManager: sm, FirebaseClient: fc, AlertEngine: ae}
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
		"ping_targets":    h.Config.PingTargets,
		"poll_interval":   h.Config.PollInterval.String(),
		"retention_days":  h.Config.RetentionDays,
		"http_port":       h.Config.HTTPPort,
		"cloud_enabled":   h.Config.CloudEnabled,
		"firebase_project": h.Config.FirebaseProjectID,
		"firebase_api_key": h.Config.FirebaseAPIKey,
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

// --- Cloud / Session Handlers ---

// HandleCloudAuth receives a Firebase Auth ID token from the frontend
// and sets up the Go cloud client with authentication.
// POST /api/cloud/auth
func (h *Handlers) HandleCloudAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	if h.FirebaseClient == nil {
		writeError(w, http.StatusServiceUnavailable, "cloud not enabled")
		return
	}

	var body struct {
		IDToken     string `json:"id_token"`
		UserID      string `json:"user_id"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.IDToken == "" || body.UserID == "" {
		writeError(w, http.StatusBadRequest, "id_token and user_id are required")
		return
	}

	// Set auth on the Firebase client
	h.FirebaseClient.SetAuth(body.IDToken, body.UserID)

	// Ensure user document exists in Firestore
	if err := h.FirebaseClient.EnsureUserDoc(body.Email, body.DisplayName); err != nil {
		log.Printf("[api] warning: could not ensure user doc: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "authenticated",
		"user_id": body.UserID,
	})
}

// HandleCloudStatus returns the current cloud connection status.
// GET /api/cloud/status
func (h *Handlers) HandleCloudStatus(w http.ResponseWriter, r *http.Request) {
	enabled := h.Config.CloudEnabled
	authenticated := false
	var activeSession interface{}

	if h.FirebaseClient != nil {
		authenticated = h.FirebaseClient.IsAuthenticated()
	}

	if h.SessionManager != nil {
		activeSession = h.SessionManager.GetActiveSession()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":        enabled,
		"authenticated":  authenticated,
		"active_session": activeSession,
		"project_id":     h.Config.FirebaseProjectID,
		"api_key":        h.Config.FirebaseAPIKey,
	})
}

// HandleSessionStart starts a new cloud monitoring session.
// POST /api/session/start
func (h *Handlers) HandleSessionStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	if h.SessionManager == nil {
		writeError(w, http.StatusServiceUnavailable, "cloud not enabled")
		return
	}

	var body struct {
		Name    string `json:"name"`
		Network string `json:"network"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Name == "" {
		body.Name = "Untitled Session"
	}

	session, err := h.SessionManager.StartSession(body.Name, body.Network)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "started",
		"session": session,
	})
}

// HandleSessionStop stops the active cloud monitoring session.
// POST /api/session/stop
func (h *Handlers) HandleSessionStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	if h.SessionManager == nil {
		writeError(w, http.StatusServiceUnavailable, "cloud not enabled")
		return
	}

	session, err := h.SessionManager.StopSession()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "stopped",
		"session": session,
	})
}

// HandleGetActiveSession returns the currently active session.
// GET /api/session/active
func (h *Handlers) HandleGetActiveSession(w http.ResponseWriter, r *http.Request) {
	if h.SessionManager == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"active": false,
		})
		return
	}

	session := h.SessionManager.GetActiveSession()
	if session == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"active": false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active":  true,
		"session": session,
	})
}

// HandleGetSessions returns the list of recent cloud sessions.
// GET /api/cloud/sessions
func (h *Handlers) HandleGetSessions(w http.ResponseWriter, r *http.Request) {
	if h.FirebaseClient == nil || !h.FirebaseClient.IsAuthenticated() {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"sessions": []interface{}{},
		})
		return
	}

	sessions, err := h.FirebaseClient.GetPreviousSessions(10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []cloud.SessionInfo{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
	})
}

// HandleSessionDelete deletes a cloud session.
// POST /api/session/delete
func (h *Handlers) HandleSessionDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	if h.FirebaseClient == nil || !h.FirebaseClient.IsAuthenticated() {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var body struct {
		SessionID string `json:"session_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	if err := h.FirebaseClient.DeleteSession(body.SessionID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "deleted",
	})
}

// HandleGetSessionData fetches historical graph data for a specific session.
// GET /api/session/data?id={session_id}
func (h *Handlers) HandleGetSessionData(w http.ResponseWriter, r *http.Request) {
	if h.FirebaseClient == nil || !h.FirebaseClient.IsAuthenticated() {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	sessionID := r.URL.Query().Get("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	samples, err := h.FirebaseClient.GetSessionData(sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch speed tests for this session
	speedTests, err := h.FirebaseClient.GetSessionSpeedTests(sessionID)
	if err != nil {
		log.Printf("[api] warning: could not fetch speed tests for session %s: %v", sessionID, err)
		speedTests = []cloud.SpeedTestData{}
	}

	// Build network sample buckets (1-to-1 mapping from raw samples)
	type bucket struct {
		BucketStart   time.Time `json:"bucket_start"`
		AvgLatency    float64   `json:"avg_latency_ms"`
		MaxLatency    float64   `json:"max_latency_ms"`
		AvgWifiRSSI   float64   `json:"avg_wifi_rssi"`
		AvgPacketLoss float64   `json:"avg_packet_loss"`
	}

	var buckets []bucket
	var totalLatency, maxLatency, totalRSSI, totalLoss float64
	for _, s := range samples {
		buckets = append(buckets, bucket{
			BucketStart:   s.Timestamp,
			AvgLatency:    s.LatencyMs,
			MaxLatency:    s.LatencyMs,
			AvgWifiRSSI:   float64(s.WifiRSSI),
			AvgPacketLoss: s.PacketLoss,
		})
		totalLatency += s.LatencyMs
		if s.LatencyMs > maxLatency {
			maxLatency = s.LatencyMs
		}
		totalRSSI += float64(s.WifiRSSI)
		totalLoss += s.PacketLoss
	}

	// Build speed test entries
	type speedEntry struct {
		Timestamp    time.Time `json:"timestamp"`
		DownloadMbps float64   `json:"download_mbps"`
		UploadMbps   float64   `json:"upload_mbps"`
		JitterMs     float64   `json:"jitter_ms"`
		LatencyMs    float64   `json:"latency_ms"`
		ServerName   string    `json:"server_name"`
	}
	var speedEntries []speedEntry
	for _, t := range speedTests {
		speedEntries = append(speedEntries, speedEntry{
			Timestamp:    t.Timestamp,
			DownloadMbps: t.DownloadMbps,
			UploadMbps:   t.UploadMbps,
			JitterMs:     t.JitterMs,
			LatencyMs:    t.LatencyMs,
			ServerName:   t.ServerName,
		})
	}

	// Compute summary stats
	sampleCount := len(samples)
	summary := map[string]interface{}{
		"sample_count":     sampleCount,
		"speed_test_count": len(speedTests),
	}
	if sampleCount > 0 {
		summary["avg_latency_ms"] = totalLatency / float64(sampleCount)
		summary["max_latency_ms"] = maxLatency
		summary["avg_wifi_rssi"] = totalRSSI / float64(sampleCount)
		summary["avg_packet_loss"] = totalLoss / float64(sampleCount)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"buckets":     buckets,
		"speed_tests": speedEntries,
		"summary":     summary,
	})
}


// --- Webhook & Alert Handlers ---

// HandleWebhooks is a multiplexer for webhook CRUD operations.
// GET    /api/webhooks        — list all webhooks
// POST   /api/webhooks        — create a new webhook
// PUT    /api/webhooks?id=N   — update a webhook
// DELETE /api/webhooks?id=N   — delete a webhook
func (h *Handlers) HandleWebhooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGetWebhooks(w, r)
	case http.MethodPost:
		h.handleCreateWebhook(w, r)
	case http.MethodPut:
		h.handleUpdateWebhook(w, r)
	case http.MethodDelete:
		h.handleDeleteWebhook(w, r)
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET, POST, PUT, DELETE only")
	}
}

func (h *Handlers) handleGetWebhooks(w http.ResponseWriter, r *http.Request) {
	configs, err := h.Store.GetWebhookConfigs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get webhooks")
		log.Printf("[api] webhooks error: %v", err)
		return
	}

	if configs == nil {
		configs = []db.WebhookConfig{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"webhooks": configs,
		"count":    len(configs),
	})
}

func (h *Handlers) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var cfg db.WebhookConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cfg.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	if cfg.Name == "" {
		cfg.Name = "My Webhook"
	}
	if cfg.Platform == "" {
		cfg.Platform = "generic"
	}
	if cfg.CooldownMinutes <= 0 {
		cfg.CooldownMinutes = 5
	}

	// Default: enabled with connection_lost alerts
	cfg.Enabled = true
	cfg.ConnectionLost = true

	id, err := h.Store.CreateWebhookConfig(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create webhook")
		log.Printf("[api] create webhook error: %v", err)
		return
	}

	cfg.ID = id
	log.Printf("[api] created webhook %d: %s (%s)", id, cfg.Name, cfg.Platform)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":  "created",
		"webhook": cfg,
	})
}

func (h *Handlers) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	var cfg db.WebhookConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Allow ID from query param or body
	if cfg.ID == 0 {
		if idStr := r.URL.Query().Get("id"); idStr != "" {
			if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
				cfg.ID = id
			}
		}
	}

	if cfg.ID == 0 {
		writeError(w, http.StatusBadRequest, "webhook id is required")
		return
	}

	if cfg.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	if err := h.Store.UpdateWebhookConfig(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update webhook")
		log.Printf("[api] update webhook error: %v", err)
		return
	}

	log.Printf("[api] updated webhook %d: %s", cfg.ID, cfg.Name)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "updated",
		"webhook": cfg,
	})
}

func (h *Handlers) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		// Try from body
		var body struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == 0 {
			writeError(w, http.StatusBadRequest, "webhook id is required")
			return
		}
		idStr = strconv.FormatInt(body.ID, 10)
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook id")
		return
	}

	if err := h.Store.DeleteWebhookConfig(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete webhook")
		log.Printf("[api] delete webhook error: %v", err)
		return
	}

	log.Printf("[api] deleted webhook %d", id)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "deleted",
	})
}

// HandleTestWebhook sends a test alert to validate a webhook configuration.
// POST /api/webhooks/test
func (h *Handlers) HandleTestWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	if h.AlertEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "alert engine not enabled")
		return
	}

	var body struct {
		WebhookID int64  `json:"webhook_id"`
		URL       string `json:"url"`
		Platform  string `json:"platform"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var cfg db.WebhookConfig

	if body.WebhookID > 0 {
		// Test an existing webhook
		existing, err := h.Store.GetWebhookConfig(body.WebhookID)
		if err != nil || existing == nil {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		cfg = *existing
	} else if body.URL != "" {
		// Test with ad-hoc URL
		cfg = db.WebhookConfig{
			URL:      body.URL,
			Platform: body.Platform,
		}
	} else {
		writeError(w, http.StatusBadRequest, "webhook_id or url is required")
		return
	}

	if err := h.AlertEngine.SendTestAlert(cfg); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "failed",
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "sent",
	})
}

// HandleGetAlertHistory returns recent alert history.
// GET /api/alerts/history?since=<ISO8601>&limit=<int>
func (h *Handlers) HandleGetAlertHistory(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-24 * time.Hour)
	limit := 100

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 1000 {
				l = 1000
			}
			limit = l
		}
	}

	entries, err := h.Store.GetAlertHistory(since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get alert history")
		log.Printf("[api] alert history error: %v", err)
		return
	}

	if entries == nil {
		entries = []db.AlertHistoryEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"alerts": entries,
		"count":  len(entries),
		"since":  since.Format(time.RFC3339),
	})
}

