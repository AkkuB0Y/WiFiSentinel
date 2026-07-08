// Package db provides data models for WiFi Sentinel.
package db

import "time"

// NetworkSample represents a single point-in-time measurement of network health.
type NetworkSample struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Target      string    `json:"target"`       // ping target IP address
	LatencyMs   float64   `json:"latency_ms"`   // round-trip time in milliseconds
	PacketLoss  float64   `json:"packet_loss"`  // packet loss percentage (0-100)
	WifiSSID           string    `json:"wifi_ssid"`              // connected WiFi network name
	WifiRSSI           int       `json:"wifi_rssi"`              // signal strength in dBm (negative)
	WifiNoise          int       `json:"wifi_noise"`             // noise level in dBm (negative)
	WifiChannel        int       `json:"wifi_channel"`           // WiFi channel number
	WifiRssiEstimated  bool      `json:"wifi_rssi_estimated"`    // true when RSSI is converted from quality %
	WifiNoiseAvailable bool      `json:"wifi_noise_available"`   // true when noise was read from the OS
}

// AggregateBucket represents aggregated network metrics over a time bucket.
// Used for chart data to reduce the number of data points over long time ranges.
type AggregateBucket struct {
	BucketStart    time.Time `json:"bucket_start"`
	AvgLatencyMs   float64   `json:"avg_latency_ms"`
	MinLatencyMs   float64   `json:"min_latency_ms"`
	MaxLatencyMs   float64   `json:"max_latency_ms"`
	AvgPacketLoss  float64   `json:"avg_packet_loss"`
	AvgWifiRSSI    float64   `json:"avg_wifi_rssi"`
	MinWifiRSSI    float64   `json:"min_wifi_rssi"`
	MaxWifiRSSI    float64   `json:"max_wifi_rssi"`
	SampleCount    int       `json:"sample_count"`
}

// SpeedTestSample represents a single speed test measurement.
// Speed tests run on a separate, less frequent schedule than network samples.
type SpeedTestSample struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	DownloadMbps float64   `json:"download_mbps"`
	UploadMbps   float64   `json:"upload_mbps"`
	JitterMs     float64   `json:"jitter_ms"`
	LatencyMs    float64   `json:"latency_ms"`
	ServerName   string    `json:"server_name"`
	ServerID     string    `json:"server_id"`
}

// WebhookConfig represents a user-configured webhook for alert notifications.
// Threshold fields use pointers so that nil = "disabled" vs 0 = "alert at zero".
type WebhookConfig struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	URL                 string   `json:"url"`
	Platform            string   `json:"platform"`             // discord, slack, telegram, generic
	Enabled             bool     `json:"enabled"`
	LatencyThreshold    *float64 `json:"latency_threshold"`    // ms, nil = disabled
	PacketLossThreshold *float64 `json:"packet_loss_threshold"` // %, nil = disabled
	SignalThreshold     *int     `json:"signal_threshold"`     // dBm, nil = disabled
	ConnectionLost      bool     `json:"connection_lost"`      // alert on 100% loss
	CooldownMinutes     int      `json:"cooldown_minutes"`
	NotifyRecovery      bool     `json:"notify_recovery"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// AlertHistoryEntry records a single alert that was fired.
type AlertHistoryEntry struct {
	ID        int64     `json:"id"`
	WebhookID int64     `json:"webhook_id"`
	Condition string    `json:"condition"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	FiredAt   time.Time `json:"fired_at"`
	Delivered bool      `json:"delivered"`
}

