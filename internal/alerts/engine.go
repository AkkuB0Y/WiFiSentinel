// Package alerts provides the alerting and webhook notification engine for WiFi Sentinel.
// This file implements the core AlertEngine that evaluates network samples against
// configured thresholds and dispatches webhook notifications.
package alerts

import (
	"fmt"
	"log"
	"sync"
	"time"

	"wifimonitor/internal/db"
)

// AlertEngine evaluates network samples against webhook alert rules and dispatches
// notifications. It maintains per-webhook, per-condition cooldown state to prevent
// notification spam.
type AlertEngine struct {
	store    *db.Store
	cooldown time.Duration

	// mu protects lastFired and activeConditions
	mu sync.Mutex

	// lastFired tracks when each (webhook_id, condition) pair last fired.
	// Key format: "webhookID:condition"
	lastFired map[string]time.Time

	// activeConditions tracks which conditions are currently active (in alert state)
	// per webhook. Used for recovery notifications.
	// Key format: "webhookID:condition"
	activeConditions map[string]bool
}

// NewAlertEngine creates a new AlertEngine.
// defaultCooldown is the minimum time between repeated alerts for the same condition
// on the same webhook (can be overridden per-webhook in the DB config).
func NewAlertEngine(store *db.Store, defaultCooldown time.Duration) *AlertEngine {
	return &AlertEngine{
		store:            store,
		cooldown:         defaultCooldown,
		lastFired:        make(map[string]time.Time),
		activeConditions: make(map[string]bool),
	}
}

// Evaluate checks a network sample against all active webhook configurations
// and dispatches alerts for any threshold violations. This method is safe for
// concurrent use and should be called from the collector's collection loop.
func (e *AlertEngine) Evaluate(sample db.NetworkSample) {
	configs, err := e.store.GetWebhookConfigs()
	if err != nil {
		log.Printf("[alerts] failed to load webhook configs: %v", err)
		return
	}

	if len(configs) == 0 {
		return
	}

	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		e.evaluateWebhook(cfg, sample)
	}
}

// evaluateWebhook checks one webhook's thresholds against the given sample.
func (e *AlertEngine) evaluateWebhook(cfg db.WebhookConfig, sample db.NetworkSample) {
	// Check connection lost (100% packet loss) — highest priority
	if cfg.ConnectionLost && sample.PacketLoss >= 100 {
		e.fireAlert(cfg, AlertEvent{
			Condition: ConditionConnectionLost,
			Severity:  SeverityCritical,
			Message:   fmt.Sprintf("Complete connectivity loss to %s — 100%% packet loss detected.", sample.Target),
			Value:     sample.PacketLoss,
			Threshold: 100,
			Timestamp: sample.Timestamp,
			Target:    sample.Target,
			WifiSSID:  sample.WifiSSID,
		})
	} else if cfg.ConnectionLost {
		e.checkRecovery(cfg, ConditionConnectionLost, sample)
	}

	// Check high latency
	if cfg.LatencyThreshold != nil && *cfg.LatencyThreshold > 0 {
		if sample.LatencyMs > *cfg.LatencyThreshold && sample.PacketLoss < 100 {
			severity := SeverityWarning
			if sample.LatencyMs > *cfg.LatencyThreshold*2 {
				severity = SeverityCritical
			}
			e.fireAlert(cfg, AlertEvent{
				Condition: ConditionLatencyHigh,
				Severity:  severity,
				Message:   fmt.Sprintf("Latency to %s is %.1f ms, exceeding the %.0f ms threshold.", sample.Target, sample.LatencyMs, *cfg.LatencyThreshold),
				Value:     sample.LatencyMs,
				Threshold: *cfg.LatencyThreshold,
				Timestamp: sample.Timestamp,
				Target:    sample.Target,
				WifiSSID:  sample.WifiSSID,
			})
		} else {
			e.checkRecovery(cfg, ConditionLatencyHigh, sample)
		}
	}

	// Check packet loss
	if cfg.PacketLossThreshold != nil && *cfg.PacketLossThreshold > 0 {
		if sample.PacketLoss > *cfg.PacketLossThreshold && sample.PacketLoss < 100 {
			severity := SeverityWarning
			if sample.PacketLoss > *cfg.PacketLossThreshold*2 {
				severity = SeverityCritical
			}
			e.fireAlert(cfg, AlertEvent{
				Condition: ConditionPacketLoss,
				Severity:  severity,
				Message:   fmt.Sprintf("Packet loss to %s is %.1f%%, exceeding the %.0f%% threshold.", sample.Target, sample.PacketLoss, *cfg.PacketLossThreshold),
				Value:     sample.PacketLoss,
				Threshold: *cfg.PacketLossThreshold,
				Timestamp: sample.Timestamp,
				Target:    sample.Target,
				WifiSSID:  sample.WifiSSID,
			})
		} else {
			e.checkRecovery(cfg, ConditionPacketLoss, sample)
		}
	}

	// Check weak signal
	if cfg.SignalThreshold != nil && *cfg.SignalThreshold < 0 {
		rssi := float64(sample.WifiRSSI)
		threshold := float64(*cfg.SignalThreshold)
		// RSSI is negative; weaker signal = more negative = lower value
		if rssi < threshold && rssi != 0 {
			severity := SeverityWarning
			if rssi < threshold-15 {
				severity = SeverityCritical
			}
			e.fireAlert(cfg, AlertEvent{
				Condition: ConditionSignalWeak,
				Severity:  severity,
				Message:   fmt.Sprintf("WiFi signal strength is %d dBm on %s, below the %d dBm threshold.", sample.WifiRSSI, sample.WifiSSID, *cfg.SignalThreshold),
				Value:     rssi,
				Threshold: threshold,
				Timestamp: sample.Timestamp,
				Target:    sample.Target,
				WifiSSID:  sample.WifiSSID,
			})
		} else {
			e.checkRecovery(cfg, ConditionSignalWeak, sample)
		}
	}
}

// fireAlert sends an alert if the cooldown period has elapsed.
func (e *AlertEngine) fireAlert(cfg db.WebhookConfig, event AlertEvent) {
	key := fmt.Sprintf("%d:%s", cfg.ID, event.Condition)
	cooldown := time.Duration(cfg.CooldownMinutes) * time.Minute
	if cooldown <= 0 {
		cooldown = e.cooldown
	}

	e.mu.Lock()
	lastFired, hasFired := e.lastFired[key]
	if hasFired && time.Since(lastFired) < cooldown {
		e.mu.Unlock()
		return // still in cooldown
	}
	e.lastFired[key] = time.Now()
	e.activeConditions[key] = true
	e.mu.Unlock()

	// Record in alert history
	entry := db.AlertHistoryEntry{
		WebhookID: cfg.ID,
		Condition: string(event.Condition),
		Severity:  event.Severity,
		Message:   event.Message,
		Value:     event.Value,
		Threshold: event.Threshold,
		FiredAt:   event.Timestamp,
		Delivered: false,
	}

	// Dispatch asynchronously so we don't block the collector
	go func() {
		err := DispatchWebhook(cfg.URL, cfg.Platform, event)
		if err != nil {
			log.Printf("[alerts] failed to dispatch %s alert to webhook %d (%s): %v",
				event.Condition, cfg.ID, cfg.Name, err)
		} else {
			entry.Delivered = true
		}

		if insertErr := e.store.InsertAlertHistory(entry); insertErr != nil {
			log.Printf("[alerts] failed to record alert history: %v", insertErr)
		}
	}()
}

// checkRecovery sends a recovery notification if the condition was previously active
// and the webhook has recovery notifications enabled.
func (e *AlertEngine) checkRecovery(cfg db.WebhookConfig, condition AlertCondition, sample db.NetworkSample) {
	if !cfg.NotifyRecovery {
		return
	}

	key := fmt.Sprintf("%d:%s", cfg.ID, condition)

	e.mu.Lock()
	wasActive := e.activeConditions[key]
	if wasActive {
		delete(e.activeConditions, key)
	}
	e.mu.Unlock()

	if !wasActive {
		return
	}

	event := AlertEvent{
		Condition: condition,
		Severity:  "info",
		Message:   fmt.Sprintf("%s has recovered and is back within normal thresholds.", condition.Label()),
		Timestamp: sample.Timestamp,
		Target:    sample.Target,
		WifiSSID:  sample.WifiSSID,
		Recovery:  true,
	}

	go func() {
		if err := DispatchWebhook(cfg.URL, cfg.Platform, event); err != nil {
			log.Printf("[alerts] failed to dispatch recovery for %s to webhook %d: %v",
				condition, cfg.ID, err)
		}
	}()
}

// SendTestAlert dispatches a test alert to validate webhook configuration.
// It bypasses cooldown checks.
func (e *AlertEngine) SendTestAlert(cfg db.WebhookConfig) error {
	event := AlertEvent{
		Condition: ConditionLatencyHigh,
		Severity:  SeverityWarning,
		Message:   "🧪 This is a test alert from WiFi Sentinel. If you see this, your webhook is configured correctly!",
		Value:     142.5,
		Threshold: 100,
		Timestamp: time.Now(),
		Target:    "8.8.8.8",
		WifiSSID:  "TestNetwork",
	}

	return DispatchWebhook(cfg.URL, cfg.Platform, event)
}
