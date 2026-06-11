// Package alerts provides the alerting and webhook notification engine for WiFi Sentinel.
// This file defines alert types, conditions, and event structures.
package alerts

import "time"

// AlertCondition identifies what kind of network degradation triggered an alert.
type AlertCondition string

const (
	// ConditionLatencyHigh fires when average latency exceeds the configured threshold.
	ConditionLatencyHigh AlertCondition = "latency_high"

	// ConditionPacketLoss fires when packet loss exceeds the configured threshold.
	ConditionPacketLoss AlertCondition = "packet_loss"

	// ConditionSignalWeak fires when WiFi RSSI drops below the configured threshold.
	ConditionSignalWeak AlertCondition = "signal_weak"

	// ConditionConnectionLost fires on 100% packet loss (total connectivity failure).
	ConditionConnectionLost AlertCondition = "connection_lost"
)

// Severity levels for alerts.
const (
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// AlertEvent represents a single alert that was triggered by the evaluation engine.
// It carries enough context for the webhook dispatcher to format a rich notification.
type AlertEvent struct {
	Condition AlertCondition `json:"condition"`
	Severity  string         `json:"severity"`
	Message   string         `json:"message"`
	Value     float64        `json:"value"`     // the measured value that triggered the alert
	Threshold float64        `json:"threshold"` // the threshold that was exceeded
	Timestamp time.Time      `json:"timestamp"`
	Target    string         `json:"target"`    // ping target that triggered it
	WifiSSID  string         `json:"wifi_ssid"` // network context
	Recovery  bool           `json:"recovery"`  // true if this is a recovery ("all clear") event
}

// conditionLabels provides human-readable names for conditions.
var conditionLabels = map[AlertCondition]string{
	ConditionLatencyHigh:    "High Latency",
	ConditionPacketLoss:     "Packet Loss",
	ConditionSignalWeak:     "Weak Signal",
	ConditionConnectionLost: "Connection Lost",
}

// ConditionLabel returns a human-readable label for the condition.
func (c AlertCondition) Label() string {
	if label, ok := conditionLabels[c]; ok {
		return label
	}
	return string(c)
}
