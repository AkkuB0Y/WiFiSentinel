// Package cloud provides Firebase Firestore integration for WiFi Sentinel.
// This file implements the Firestore REST API client for pushing monitoring data
// to the cloud. It uses direct HTTP calls rather than a heavy SDK to keep the
// binary lean and avoid CGo dependencies.
package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// FirebaseClient handles communication with the Firestore REST API.
type FirebaseClient struct {
	projectID  string
	apiKey     string
	idToken    string // Firebase Auth ID token (JWT)
	userID     string // Firebase Auth UID
	httpClient *http.Client
	mu         sync.RWMutex
}

// NewFirebaseClient creates a new Firestore REST client.
func NewFirebaseClient(projectID, apiKey string) *FirebaseClient {
	return &FirebaseClient{
		projectID: projectID,
		apiKey:    apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetAuth sets the authentication credentials obtained from the frontend.
// The ID token is a Firebase Auth JWT, and userID is the Firebase UID.
func (fc *FirebaseClient) SetAuth(idToken, userID string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.idToken = idToken
	fc.userID = userID
	log.Printf("[cloud] authenticated as user %s", userID)
}

// IsAuthenticated returns whether the client has valid auth credentials.
func (fc *FirebaseClient) IsAuthenticated() bool {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.idToken != "" && fc.userID != ""
}

// GetUserID returns the authenticated user's UID.
func (fc *FirebaseClient) GetUserID() string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.userID
}

// GetProjectID returns the Firebase project ID.
func (fc *FirebaseClient) GetProjectID() string {
	return fc.projectID
}

// GetAPIKey returns the Firebase API key.
func (fc *FirebaseClient) GetAPIKey() string {
	return fc.apiKey
}

// --- Firestore Document Types ---

// firestoreValue wraps a Go value into the Firestore REST API value format.
type firestoreValue struct {
	StringValue  *string  `json:"stringValue,omitempty"`
	DoubleValue  *float64 `json:"doubleValue,omitempty"`
	IntegerValue *string  `json:"integerValue,omitempty"`
	BoolValue    *bool    `json:"booleanValue,omitempty"`
	TimestampVal *string  `json:"timestampValue,omitempty"`
}

func stringVal(s string) firestoreValue {
	return firestoreValue{StringValue: &s}
}

func doubleVal(f float64) firestoreValue {
	return firestoreValue{DoubleValue: &f}
}

func intVal(i int) firestoreValue {
	s := fmt.Sprintf("%d", i)
	return firestoreValue{IntegerValue: &s}
}

func boolVal(b bool) firestoreValue {
	return firestoreValue{BoolValue: &b}
}

func timestampVal(t time.Time) firestoreValue {
	s := t.UTC().Format(time.RFC3339Nano)
	return firestoreValue{TimestampVal: &s}
}

// firestoreDoc represents a Firestore document for the REST API.
type firestoreDoc struct {
	Fields map[string]firestoreValue `json:"fields"`
}

// --- Firestore Operations ---

// firestoreURL builds the Firestore REST API base URL for a document path.
func (fc *FirebaseClient) firestoreURL(docPath string) string {
	return fmt.Sprintf(
		"https://firestore.googleapis.com/v1/projects/%s/databases/(default)/documents/%s",
		fc.projectID, docPath,
	)
}

// createDocument writes a new document to Firestore at the given path.
// If documentID is empty, Firestore auto-generates an ID.
func (fc *FirebaseClient) createDocument(collectionPath, documentID string, doc firestoreDoc) (string, error) {
	fc.mu.RLock()
	token := fc.idToken
	fc.mu.RUnlock()

	url := fc.firestoreURL(collectionPath)
	if documentID != "" {
		url += "?documentId=" + documentID
	}

	body, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal error: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("firestore request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("firestore error %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response to get document name (contains the auto-generated ID)
	var result struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode error: %w", err)
	}

	return result.Name, nil
}

// patchDocument updates an existing document at the given full path.
func (fc *FirebaseClient) patchDocument(docPath string, doc firestoreDoc) error {
	fc.mu.RLock()
	token := fc.idToken
	fc.mu.RUnlock()

	url := fc.firestoreURL(docPath)
	
	// Append updateMask for partial updates
	isFirst := true
	for k := range doc.Fields {
		if isFirst {
			url += "?updateMask.fieldPaths=" + k
			isFirst = false
		} else {
			url += "&updateMask.fieldPaths=" + k
		}
	}

	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("firestore request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firestore error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// --- High-Level Operations ---

// NetworkSampleData holds network sample fields for cloud upload.
type NetworkSampleData struct {
	Timestamp   time.Time
	Target      string
	LatencyMs   float64
	PacketLoss  float64
	WifiSSID    string
	WifiRSSI    int
	WifiNoise   int
	WifiChannel int
}

// SpeedTestData holds speed test fields for cloud upload.
type SpeedTestData struct {
	Timestamp    time.Time
	DownloadMbps float64
	UploadMbps   float64
	JitterMs     float64
	LatencyMs    float64
	ServerName   string
}

// EnsureUserDoc creates the user document if it doesn't exist.
func (fc *FirebaseClient) EnsureUserDoc(email, displayName string) error {
	doc := firestoreDoc{
		Fields: map[string]firestoreValue{
			"email":      stringVal(email),
			"name":       stringVal(displayName),
			"created_at": timestampVal(time.Now()),
		},
	}

	userID := fc.GetUserID()
	err := fc.patchDocument(fmt.Sprintf("users/%s", userID), doc)
	if err != nil {
		return fmt.Errorf("ensure user doc: %w", err)
	}
	return nil
}

// CreateSession creates a new monitoring session in Firestore.
// Returns the session document ID.
func (fc *FirebaseClient) CreateSession(sessionID, name, network string) error {
	userID := fc.GetUserID()
	doc := firestoreDoc{
		Fields: map[string]firestoreValue{
			"name":       stringVal(name),
			"network":    stringVal(network),
			"started_at": timestampVal(time.Now()),
			"status":     stringVal("active"),
		},
	}

	path := fmt.Sprintf("users/%s/sessions", userID)
	_, err := fc.createDocument(path, sessionID, doc)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	log.Printf("[cloud] created session %s", sessionID)
	return nil
}

// EndSession marks a session as completed in Firestore.
func (fc *FirebaseClient) EndSession(sessionID string) error {
	userID := fc.GetUserID()
	doc := firestoreDoc{
		Fields: map[string]firestoreValue{
			"ended_at": timestampVal(time.Now()),
			"status":   stringVal("completed"),
		},
	}

	path := fmt.Sprintf("users/%s/sessions/%s", userID, sessionID)
	err := fc.patchDocument(path, doc)
	if err != nil {
		return fmt.Errorf("end session: %w", err)
	}
	log.Printf("[cloud] ended session %s", sessionID)
	return nil
}

// DeleteSession removes a session document from Firestore.
func (fc *FirebaseClient) DeleteSession(sessionID string) error {
	if !fc.IsAuthenticated() {
		return fmt.Errorf("unauthenticated")
	}
	userID := fc.GetUserID()
	url := fc.firestoreURL(fmt.Sprintf("users/%s/sessions/%s", userID, sessionID))

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}

	fc.mu.RLock()
	token := fc.idToken
	fc.mu.RUnlock()
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("firestore request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firestore delete error %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("[cloud] deleted session %s", sessionID)
	return nil
}

// PushSamples writes a batch of network samples to Firestore.
// Each sample becomes a document in the session's network_samples subcollection.
func (fc *FirebaseClient) PushSamples(sessionID string, samples []NetworkSampleData) error {
	userID := fc.GetUserID()
	basePath := fmt.Sprintf("users/%s/sessions/%s/network_samples", userID, sessionID)

	for _, s := range samples {
		doc := firestoreDoc{
			Fields: map[string]firestoreValue{
				"timestamp":    timestampVal(s.Timestamp),
				"target":       stringVal(s.Target),
				"latency_ms":   doubleVal(s.LatencyMs),
				"packet_loss":  doubleVal(s.PacketLoss),
				"wifi_ssid":    stringVal(s.WifiSSID),
				"wifi_rssi":    intVal(s.WifiRSSI),
				"wifi_noise":   intVal(s.WifiNoise),
				"wifi_channel": intVal(s.WifiChannel),
			},
		}

		if _, err := fc.createDocument(basePath, "", doc); err != nil {
			return fmt.Errorf("push sample: %w", err)
		}
	}

	return nil
}

// PushSpeedTest writes a speed test result to Firestore.
func (fc *FirebaseClient) PushSpeedTest(sessionID string, data SpeedTestData) error {
	userID := fc.GetUserID()
	path := fmt.Sprintf("users/%s/sessions/%s/speed_tests", userID, sessionID)

	doc := firestoreDoc{
		Fields: map[string]firestoreValue{
			"timestamp":     timestampVal(data.Timestamp),
			"download_mbps": doubleVal(data.DownloadMbps),
			"upload_mbps":   doubleVal(data.UploadMbps),
			"jitter_ms":     doubleVal(data.JitterMs),
			"latency_ms":    doubleVal(data.LatencyMs),
			"server_name":   stringVal(data.ServerName),
		},
	}

	_, err := fc.createDocument(path, "", doc)
	if err != nil {
		return fmt.Errorf("push speed test: %w", err)
	}
	return nil
}

// SessionInfo represents basic details of a cloud session.
type SessionInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Network   string    `json:"network"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Status    string    `json:"status"`
}

// GetPreviousSessions fetches the latest sessions for the authenticated user.
func (fc *FirebaseClient) GetPreviousSessions(limit int) ([]SessionInfo, error) {
	if !fc.IsAuthenticated() {
		return nil, fmt.Errorf("unauthenticated")
	}

	fc.mu.RLock()
	token := fc.idToken
	userID := fc.userID
	fc.mu.RUnlock()

	url := fmt.Sprintf(
		"https://firestore.googleapis.com/v1/projects/%s/databases/(default)/documents/users/%s/sessions?pageSize=%d&orderBy=started_at%%20desc",
		fc.projectID, userID, limit,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return []SessionInfo{}, nil
		}
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("firestore error %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[cloud] raw firestore sessions response: %s", string(respBody))

	var result struct {
		Documents []struct {
			Name   string `json:"name"`
			Fields map[string]struct {
				StringVal    *string `json:"stringValue,omitempty"`
				TimestampVal *string `json:"timestampValue,omitempty"`
			} `json:"fields"`
		} `json:"documents"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	var sessions []SessionInfo
	for _, doc := range result.Documents {
		// Extract document ID from name path: projects/.../sessions/{session_id}
		parts := strings.Split(doc.Name, "/")
		id := parts[len(parts)-1]

		name := "Untitled Session"
		if f, ok := doc.Fields["name"]; ok && f.StringVal != nil {
			name = *f.StringVal
		}

		network := ""
		if f, ok := doc.Fields["network"]; ok && f.StringVal != nil {
			network = *f.StringVal
		}

		status := ""
		if f, ok := doc.Fields["status"]; ok && f.StringVal != nil {
			status = *f.StringVal
		}

		var startedAt time.Time
		if f, ok := doc.Fields["started_at"]; ok && f.TimestampVal != nil {
			startedAt, _ = time.Parse(time.RFC3339Nano, *f.TimestampVal)
		}

		var endedAt time.Time
		if f, ok := doc.Fields["ended_at"]; ok && f.TimestampVal != nil {
			endedAt, _ = time.Parse(time.RFC3339Nano, *f.TimestampVal)
		}

		sessions = append(sessions, SessionInfo{
			ID:        id,
			Name:      name,
			Network:   network,
			StartedAt: startedAt,
			EndedAt:   endedAt,
			Status:    status,
		})
	}

	return sessions, nil
}

// GetSessionData fetches the network samples for a specific session.
func (fc *FirebaseClient) GetSessionData(sessionID string) ([]NetworkSampleData, error) {
	if !fc.IsAuthenticated() {
		return nil, fmt.Errorf("unauthenticated")
	}

	fc.mu.RLock()
	token := fc.idToken
	userID := fc.userID
	fc.mu.RUnlock()

	// Fetch up to 2000 samples for the session, ordered by timestamp
	url := fmt.Sprintf(
		"https://firestore.googleapis.com/v1/projects/%s/databases/(default)/documents/users/%s/sessions/%s/network_samples?pageSize=2000&orderBy=timestamp",
		fc.projectID, userID, sessionID,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return []NetworkSampleData{}, nil
		}
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("firestore error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Documents []struct {
			Fields map[string]struct {
				StringVal    *string  `json:"stringValue,omitempty"`
				DoubleVal    *float64 `json:"doubleValue,omitempty"`
				IntVal       *string  `json:"integerValue,omitempty"`
				TimestampVal *string  `json:"timestampValue,omitempty"`
			} `json:"fields"`
		} `json:"documents"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var samples []NetworkSampleData
	for _, doc := range result.Documents {
		var s NetworkSampleData

		if f, ok := doc.Fields["timestamp"]; ok && f.TimestampVal != nil {
			s.Timestamp, _ = time.Parse(time.RFC3339Nano, *f.TimestampVal)
		}
		if f, ok := doc.Fields["target"]; ok && f.StringVal != nil {
			s.Target = *f.StringVal
		}
		if f, ok := doc.Fields["latency_ms"]; ok && f.DoubleVal != nil {
			s.LatencyMs = *f.DoubleVal
		}
		if f, ok := doc.Fields["packet_loss"]; ok && f.DoubleVal != nil {
			s.PacketLoss = *f.DoubleVal
		}
		if f, ok := doc.Fields["wifi_ssid"]; ok && f.StringVal != nil {
			s.WifiSSID = *f.StringVal
		}

		// Firestore integerValue is actually represented as a string in JSON
		if f, ok := doc.Fields["wifi_rssi"]; ok && f.IntVal != nil {
			fmt.Sscanf(*f.IntVal, "%d", &s.WifiRSSI)
		}
		if f, ok := doc.Fields["wifi_noise"]; ok && f.IntVal != nil {
			fmt.Sscanf(*f.IntVal, "%d", &s.WifiNoise)
		}
		if f, ok := doc.Fields["wifi_channel"]; ok && f.IntVal != nil {
			fmt.Sscanf(*f.IntVal, "%d", &s.WifiChannel)
		}

		samples = append(samples, s)
	}

	return samples, nil
}

// GetSessionSpeedTests fetches speed test results for a specific session.
func (fc *FirebaseClient) GetSessionSpeedTests(sessionID string) ([]SpeedTestData, error) {
	if !fc.IsAuthenticated() {
		return nil, fmt.Errorf("unauthenticated")
	}

	fc.mu.RLock()
	token := fc.idToken
	userID := fc.userID
	fc.mu.RUnlock()

	url := fmt.Sprintf(
		"https://firestore.googleapis.com/v1/projects/%s/databases/(default)/documents/users/%s/sessions/%s/speed_tests?pageSize=500&orderBy=timestamp",
		fc.projectID, userID, sessionID,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return []SpeedTestData{}, nil
		}
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("firestore error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Documents []struct {
			Fields map[string]struct {
				StringVal    *string  `json:"stringValue,omitempty"`
				DoubleVal    *float64 `json:"doubleValue,omitempty"`
				TimestampVal *string  `json:"timestampValue,omitempty"`
			} `json:"fields"`
		} `json:"documents"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tests []SpeedTestData
	for _, doc := range result.Documents {
		var t SpeedTestData

		if f, ok := doc.Fields["timestamp"]; ok && f.TimestampVal != nil {
			t.Timestamp, _ = time.Parse(time.RFC3339Nano, *f.TimestampVal)
		}
		if f, ok := doc.Fields["download_mbps"]; ok && f.DoubleVal != nil {
			t.DownloadMbps = *f.DoubleVal
		}
		if f, ok := doc.Fields["upload_mbps"]; ok && f.DoubleVal != nil {
			t.UploadMbps = *f.DoubleVal
		}
		if f, ok := doc.Fields["jitter_ms"]; ok && f.DoubleVal != nil {
			t.JitterMs = *f.DoubleVal
		}
		if f, ok := doc.Fields["latency_ms"]; ok && f.DoubleVal != nil {
			t.LatencyMs = *f.DoubleVal
		}
		if f, ok := doc.Fields["server_name"]; ok && f.StringVal != nil {
			t.ServerName = *f.StringVal
		}

		tests = append(tests, t)
	}

	return tests, nil
}
