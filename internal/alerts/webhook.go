// Package alerts provides the alerting and webhook notification engine for WiFi Sentinel.
// This file implements the HTTP webhook dispatcher with platform-aware formatting
// for Discord, Slack, Telegram, and generic HTTP endpoints.
package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// webhookClient is a shared HTTP client with sensible timeouts for webhook delivery.
var webhookClient = &http.Client{
	Timeout: 10 * time.Second,
}

// DispatchWebhook sends an alert event to the given webhook URL.
// It auto-detects the platform from the URL and formats the payload accordingly.
// Returns an error if delivery fails after one retry.
func DispatchWebhook(webhookURL, platform string, event AlertEvent) error {
	if platform == "" || platform == "generic" {
		platform = detectPlatform(webhookURL)
	}

	var payload []byte
	var err error

	switch platform {
	case "discord":
		payload, err = formatDiscord(event)
	case "slack":
		payload, err = formatSlack(event)
	case "telegram":
		payload, err = formatTelegram(event, webhookURL)
	default:
		payload, err = formatGeneric(event)
	}

	if err != nil {
		return fmt.Errorf("failed to format %s payload: %w", platform, err)
	}

	// For Telegram, the URL needs the sendMessage path appended
	targetURL := webhookURL
	if platform == "telegram" {
		// Telegram bot URLs are like: https://api.telegram.org/bot<TOKEN>/sendMessage
		// Users might provide just the bot base URL, we ensure sendMessage is appended
		if !strings.HasSuffix(targetURL, "/sendMessage") {
			targetURL = strings.TrimRight(targetURL, "/") + "/sendMessage"
		}
	}

	// Attempt delivery with one retry
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := webhookClient.Post(targetURL, "application/json", bytes.NewReader(payload))
		if err != nil {
			if attempt == 0 {
				log.Printf("[alerts] webhook delivery attempt 1 failed for %s: %v — retrying...", platform, err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			return fmt.Errorf("webhook delivery failed after retry: %w", err)
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("[alerts] webhook delivered to %s (HTTP %d)", platform, resp.StatusCode)
			return nil
		}

		if attempt == 0 {
			log.Printf("[alerts] webhook got HTTP %d from %s — retrying...", resp.StatusCode, platform)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// detectPlatform guesses the webhook platform from the URL.
func detectPlatform(url string) string {
	switch {
	case strings.Contains(url, "discord.com/api/webhooks"):
		return "discord"
	case strings.Contains(url, "hooks.slack.com"):
		return "slack"
	case strings.Contains(url, "api.telegram.org"):
		return "telegram"
	default:
		return "generic"
	}
}

// --- Discord Formatting ---

func formatDiscord(event AlertEvent) ([]byte, error) {
	color := 0xFFA500 // orange for warning
	if event.Severity == SeverityCritical {
		color = 0xFF0000 // red for critical
	}
	if event.Recovery {
		color = 0x00FF00 // green for recovery
	}

	emoji := "⚠️"
	if event.Severity == SeverityCritical {
		emoji = "🚨"
	}
	if event.Recovery {
		emoji = "✅"
	}

	title := fmt.Sprintf("%s WiFi Sentinel — %s", emoji, event.Condition.Label())
	if event.Recovery {
		title = fmt.Sprintf("%s WiFi Sentinel — %s Recovered", emoji, event.Condition.Label())
	}

	fields := []map[string]interface{}{
		{"name": "Condition", "value": event.Condition.Label(), "inline": true},
		{"name": "Severity", "value": event.Severity, "inline": true},
		{"name": "Value", "value": formatValue(event), "inline": true},
	}

	if event.Target != "" {
		fields = append(fields, map[string]interface{}{
			"name": "Target", "value": event.Target, "inline": true,
		})
	}
	if event.WifiSSID != "" {
		fields = append(fields, map[string]interface{}{
			"name": "Network", "value": event.WifiSSID, "inline": true,
		})
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       title,
				"description": event.Message,
				"color":       color,
				"fields":      fields,
				"timestamp":   event.Timestamp.UTC().Format(time.RFC3339),
				"footer": map[string]string{
					"text": "WiFi Sentinel Alert Engine",
				},
			},
		},
	}

	return json.Marshal(payload)
}

// --- Slack Formatting ---

func formatSlack(event AlertEvent) ([]byte, error) {
	emoji := ":warning:"
	if event.Severity == SeverityCritical {
		emoji = ":rotating_light:"
	}
	if event.Recovery {
		emoji = ":white_check_mark:"
	}

	header := fmt.Sprintf("%s *WiFi Sentinel — %s*", emoji, event.Condition.Label())
	if event.Recovery {
		header = fmt.Sprintf("%s *WiFi Sentinel — %s Recovered*", emoji, event.Condition.Label())
	}

	contextParts := []string{
		fmt.Sprintf("*Severity:* %s", event.Severity),
		fmt.Sprintf("*Value:* %s", formatValue(event)),
	}
	if event.Target != "" {
		contextParts = append(contextParts, fmt.Sprintf("*Target:* %s", event.Target))
	}
	if event.WifiSSID != "" {
		contextParts = append(contextParts, fmt.Sprintf("*Network:* %s", event.WifiSSID))
	}

	payload := map[string]interface{}{
		"blocks": []map[string]interface{}{
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": header,
				},
			},
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": event.Message,
				},
			},
			{
				"type": "context",
				"elements": []map[string]string{
					{"type": "mrkdwn", "text": strings.Join(contextParts, " | ")},
				},
			},
		},
	}

	return json.Marshal(payload)
}

// --- Telegram Formatting ---

func formatTelegram(event AlertEvent, webhookURL string) ([]byte, error) {
	emoji := "⚠️"
	if event.Severity == SeverityCritical {
		emoji = "🚨"
	}
	if event.Recovery {
		emoji = "✅"
	}

	title := fmt.Sprintf("%s <b>WiFi Sentinel — %s</b>", emoji, event.Condition.Label())
	if event.Recovery {
		title = fmt.Sprintf("%s <b>WiFi Sentinel — %s Recovered</b>", emoji, event.Condition.Label())
	}

	lines := []string{
		title,
		"",
		event.Message,
		"",
		fmt.Sprintf("• <b>Severity:</b> %s", event.Severity),
		fmt.Sprintf("• <b>Value:</b> %s", formatValue(event)),
	}

	if event.Target != "" {
		lines = append(lines, fmt.Sprintf("• <b>Target:</b> %s", event.Target))
	}
	if event.WifiSSID != "" {
		lines = append(lines, fmt.Sprintf("• <b>Network:</b> %s", event.WifiSSID))
	}

	// Extract chat_id from URL query params if present, otherwise require it in the URL
	// Typical format: https://api.telegram.org/bot<TOKEN>/sendMessage?chat_id=<CHAT_ID>
	chatID := ""
	if idx := strings.Index(webhookURL, "chat_id="); idx != -1 {
		chatID = webhookURL[idx+8:]
		if ampIdx := strings.Index(chatID, "&"); ampIdx != -1 {
			chatID = chatID[:ampIdx]
		}
	}

	payload := map[string]interface{}{
		"text":       strings.Join(lines, "\n"),
		"parse_mode": "HTML",
	}

	if chatID != "" {
		payload["chat_id"] = chatID
	}

	return json.Marshal(payload)
}

// --- Generic JSON Formatting ---

func formatGeneric(event AlertEvent) ([]byte, error) {
	payload := map[string]interface{}{
		"source":    "wifi-sentinel",
		"condition": event.Condition,
		"severity":  event.Severity,
		"message":   event.Message,
		"value":     event.Value,
		"threshold": event.Threshold,
		"target":    event.Target,
		"wifi_ssid": event.WifiSSID,
		"timestamp": event.Timestamp.UTC().Format(time.RFC3339),
		"recovery":  event.Recovery,
	}
	return json.Marshal(payload)
}

// formatValue returns a human-readable string for the alert value.
func formatValue(event AlertEvent) string {
	switch event.Condition {
	case ConditionLatencyHigh:
		return fmt.Sprintf("%.1f ms (threshold: %.0f ms)", event.Value, event.Threshold)
	case ConditionPacketLoss:
		return fmt.Sprintf("%.1f%% (threshold: %.0f%%)", event.Value, event.Threshold)
	case ConditionSignalWeak:
		return fmt.Sprintf("%.0f dBm (threshold: %.0f dBm)", event.Value, event.Threshold)
	case ConditionConnectionLost:
		return "100% packet loss"
	default:
		return fmt.Sprintf("%.2f (threshold: %.2f)", event.Value, event.Threshold)
	}
}
