// Package cloud provides Firebase Firestore integration for WiFi Sentinel.
// This file implements session lifecycle management — creating, tracking,
// and stopping monitoring sessions that are synced to Firestore.
package cloud

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Session represents an active or completed monitoring session.
type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Network   string    `json:"network"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Status    string    `json:"status"` // "active" | "completed"
}

// SessionManager tracks the active monitoring session and handles
// start/stop lifecycle with Firestore synchronization.
type SessionManager struct {
	mu      sync.RWMutex
	active  *Session
	client  *FirebaseClient
	buffer  *SampleBuffer
	stBuf   *SpeedTestBuffer
	stopCtx context.CancelFunc
}

// NewSessionManager creates a new session manager with the given Firebase client and buffers.
func NewSessionManager(client *FirebaseClient, buffer *SampleBuffer, stBuf *SpeedTestBuffer) *SessionManager {
	return &SessionManager{
		client: client,
		buffer: buffer,
		stBuf:  stBuf,
	}
}

// StartSession begins a new monitoring session. If a session is already active,
// it is stopped first. The session is created in Firestore if the client is authenticated.
func (sm *SessionManager) StartSession(name, network string) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Stop any existing session
	if sm.active != nil {
		sm.stopLocked()
	}

	// Generate a session ID from timestamp
	sessionID := fmt.Sprintf("session_%d", time.Now().UnixMilli())

	session := &Session{
		ID:        sessionID,
		Name:      name,
		Network:   network,
		StartedAt: time.Now(),
		Status:    "active",
	}

	// Create in Firestore if authenticated
	if sm.client != nil && sm.client.IsAuthenticated() {
		if err := sm.client.CreateSession(sessionID, name, network); err != nil {
			log.Printf("[cloud] warning: could not create session in Firestore: %v", err)
			// Continue anyway — we'll buffer locally
		}
	}

	sm.active = session

	// Start the flush goroutine
	ctx, cancel := context.WithCancel(context.Background())
	sm.stopCtx = cancel
	go sm.flushLoop(ctx, sessionID)

	log.Printf("[cloud] session started: %s (%s)", name, sessionID)
	return session, nil
}

// StopSession ends the active session and flushes remaining buffered data.
func (sm *SessionManager) StopSession() (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.active == nil {
		return nil, fmt.Errorf("no active session")
	}

	session := sm.stopLocked()
	return session, nil
}

// stopLocked stops the active session (caller must hold sm.mu).
func (sm *SessionManager) stopLocked() *Session {
	session := sm.active

	// Cancel flush loop
	if sm.stopCtx != nil {
		sm.stopCtx()
		sm.stopCtx = nil
	}

	// Final flush
	if sm.client != nil && sm.client.IsAuthenticated() {
		sm.buffer.Flush(sm.client, session.ID)
		sm.stBuf.Flush(sm.client, session.ID)

		if err := sm.client.EndSession(session.ID); err != nil {
			log.Printf("[cloud] warning: could not end session in Firestore: %v", err)
		}
	}

	session.EndedAt = time.Now()
	session.Status = "completed"
	sm.active = nil

	log.Printf("[cloud] session stopped: %s (%s)", session.Name, session.ID)
	return session
}

// GetActiveSession returns the currently active session, or nil if none.
func (sm *SessionManager) GetActiveSession() *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.active
}

// HasActiveSession returns whether a session is currently active.
func (sm *SessionManager) HasActiveSession() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.active != nil
}

// GetActiveSessionID returns the active session ID, or empty string if none.
func (sm *SessionManager) GetActiveSessionID() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.active == nil {
		return ""
	}
	return sm.active.ID
}

// flushLoop periodically flushes the sample buffer to Firestore.
func (sm *SessionManager) flushLoop(ctx context.Context, sessionID string) {
	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if sm.client != nil && sm.client.IsAuthenticated() {
				sm.buffer.Flush(sm.client, sessionID)
				sm.stBuf.Flush(sm.client, sessionID)
			}
		}
	}
}
