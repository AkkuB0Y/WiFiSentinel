// Package db provides SQLite-backed storage for WiFi Sentinel.
// It handles database initialization, migrations, CRUD operations, and data pruning.
package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Store wraps a SQLite database connection and provides methods
// for inserting, querying, and pruning network samples.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) a SQLite database at the given path and
// runs initial migrations. The database is configured with WAL mode
// and tuned pragmas for low-overhead writes.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&cache=shared")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Connection pool settings for lightweight usage
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return store, nil
}

// migrate creates the necessary tables and indexes if they don't exist.
func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS network_samples (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp    DATETIME NOT NULL DEFAULT (datetime('now')),
		target       TEXT NOT NULL,
		latency_ms   REAL NOT NULL DEFAULT 0,
		packet_loss  REAL NOT NULL DEFAULT 0,
		wifi_ssid    TEXT NOT NULL DEFAULT '',
		wifi_rssi    INTEGER NOT NULL DEFAULT 0,
		wifi_noise   INTEGER NOT NULL DEFAULT 0,
		wifi_channel INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_samples_timestamp ON network_samples(timestamp);
	CREATE INDEX IF NOT EXISTS idx_samples_target ON network_samples(target);

	CREATE TABLE IF NOT EXISTS speed_test_samples (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp     DATETIME NOT NULL DEFAULT (datetime('now')),
		download_mbps REAL NOT NULL DEFAULT 0,
		upload_mbps   REAL NOT NULL DEFAULT 0,
		jitter_ms     REAL NOT NULL DEFAULT 0,
		latency_ms    REAL NOT NULL DEFAULT 0,
		server_name   TEXT NOT NULL DEFAULT '',
		server_id     TEXT NOT NULL DEFAULT ''
	);

	CREATE INDEX IF NOT EXISTS idx_speedtest_timestamp ON speed_test_samples(timestamp);

	CREATE TABLE IF NOT EXISTS webhook_configs (
		id                    INTEGER PRIMARY KEY AUTOINCREMENT,
		name                  TEXT NOT NULL DEFAULT 'My Webhook',
		url                   TEXT NOT NULL,
		platform              TEXT NOT NULL DEFAULT 'generic',
		enabled               INTEGER NOT NULL DEFAULT 1,
		latency_threshold     REAL DEFAULT NULL,
		packet_loss_threshold REAL DEFAULT NULL,
		signal_threshold      INTEGER DEFAULT NULL,
		connection_lost       INTEGER NOT NULL DEFAULT 1,
		cooldown_minutes      INTEGER NOT NULL DEFAULT 5,
		notify_recovery       INTEGER NOT NULL DEFAULT 0,
		created_at            DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at            DATETIME NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS alert_history (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		webhook_id    INTEGER NOT NULL,
		condition     TEXT NOT NULL,
		severity      TEXT NOT NULL,
		message       TEXT NOT NULL,
		value         REAL NOT NULL,
		threshold     REAL NOT NULL,
		fired_at      DATETIME NOT NULL DEFAULT (datetime('now')),
		delivered     INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (webhook_id) REFERENCES webhook_configs(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_alert_history_fired ON alert_history(fired_at);
	CREATE INDEX IF NOT EXISTS idx_alert_history_webhook ON alert_history(webhook_id);
	`
	_, err := s.db.Exec(schema)
	return err
}

// InsertSample writes a single network sample to the database.
func (s *Store) InsertSample(sample NetworkSample) error {
	query := `
	INSERT INTO network_samples (timestamp, target, latency_ms, packet_loss, wifi_ssid, wifi_rssi, wifi_noise, wifi_channel)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		sample.Timestamp.UTC().Format(time.RFC3339),
		sample.Target,
		sample.LatencyMs,
		sample.PacketLoss,
		sample.WifiSSID,
		sample.WifiRSSI,
		sample.WifiNoise,
		sample.WifiChannel,
	)
	return err
}

// GetLatestSample retrieves the most recent network sample.
// Returns nil if no samples exist.
func (s *Store) GetLatestSample() (*NetworkSample, error) {
	query := `
	SELECT id, timestamp, target, latency_ms, packet_loss, wifi_ssid, wifi_rssi, wifi_noise, wifi_channel
	FROM network_samples
	ORDER BY timestamp DESC
	LIMIT 1
	`
	row := s.db.QueryRow(query)
	sample, err := scanSample(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sample, nil
}

// GetSamples retrieves network samples since the given time, up to the specified limit.
// Results are ordered newest-first.
func (s *Store) GetSamples(since time.Time, limit int) ([]NetworkSample, error) {
	query := `
	SELECT id, timestamp, target, latency_ms, packet_loss, wifi_ssid, wifi_rssi, wifi_noise, wifi_channel
	FROM network_samples
	WHERE timestamp >= ?
	ORDER BY timestamp DESC
	LIMIT ?
	`
	rows, err := s.db.Query(query, since.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []NetworkSample
	for rows.Next() {
		var sample NetworkSample
		var ts string
		err := rows.Scan(
			&sample.ID, &ts, &sample.Target,
			&sample.LatencyMs, &sample.PacketLoss,
			&sample.WifiSSID, &sample.WifiRSSI, &sample.WifiNoise, &sample.WifiChannel,
		)
		if err != nil {
			return nil, err
		}
		sample.Timestamp, _ = time.Parse(time.RFC3339, ts)
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

// GetAggregates returns aggregated network metrics in time buckets.
// Each bucket spans `bucketMinutes` minutes, providing avg/min/max values for charts.
func (s *Store) GetAggregates(since time.Time, bucketMinutes int) ([]AggregateBucket, error) {
	query := `
	SELECT
		strftime('%Y-%m-%dT%H:', timestamp) || 
			printf('%02d', (CAST(strftime('%M', timestamp) AS INTEGER) / ?) * ?) || 
			':00Z' as bucket_start,
		AVG(latency_ms)  as avg_latency,
		MIN(latency_ms)  as min_latency,
		MAX(latency_ms)  as max_latency,
		AVG(packet_loss) as avg_packet_loss,
		AVG(wifi_rssi)   as avg_rssi,
		MIN(wifi_rssi)   as min_rssi,
		MAX(wifi_rssi)   as max_rssi,
		COUNT(*)         as sample_count
	FROM network_samples
	WHERE timestamp >= ?
	GROUP BY bucket_start
	ORDER BY bucket_start ASC
	`
	rows, err := s.db.Query(query, bucketMinutes, bucketMinutes, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buckets []AggregateBucket
	for rows.Next() {
		var b AggregateBucket
		var ts string
		err := rows.Scan(
			&ts,
			&b.AvgLatencyMs, &b.MinLatencyMs, &b.MaxLatencyMs,
			&b.AvgPacketLoss,
			&b.AvgWifiRSSI, &b.MinWifiRSSI, &b.MaxWifiRSSI,
			&b.SampleCount,
		)
		if err != nil {
			return nil, err
		}
		b.BucketStart, _ = time.Parse(time.RFC3339, ts)
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

// Prune deletes all samples older than the given time.
// Returns the number of rows deleted.
func (s *Store) Prune(olderThan time.Time) (int64, error) {
	result, err := s.db.Exec(
		"DELETE FROM network_samples WHERE timestamp < ?",
		olderThan.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Speed Test Methods ---

// InsertSpeedTest writes a single speed test result to the database.
func (s *Store) InsertSpeedTest(sample SpeedTestSample) error {
	query := `
	INSERT INTO speed_test_samples (timestamp, download_mbps, upload_mbps, jitter_ms, latency_ms, server_name, server_id)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		sample.Timestamp.UTC().Format(time.RFC3339),
		sample.DownloadMbps,
		sample.UploadMbps,
		sample.JitterMs,
		sample.LatencyMs,
		sample.ServerName,
		sample.ServerID,
	)
	return err
}

// GetLatestSpeedTest retrieves the most recent speed test result.
// Returns nil if no speed tests have been run.
func (s *Store) GetLatestSpeedTest() (*SpeedTestSample, error) {
	query := `
	SELECT id, timestamp, download_mbps, upload_mbps, jitter_ms, latency_ms, server_name, server_id
	FROM speed_test_samples
	ORDER BY timestamp DESC
	LIMIT 1
	`
	row := s.db.QueryRow(query)
	var sample SpeedTestSample
	var ts string
	err := row.Scan(
		&sample.ID, &ts,
		&sample.DownloadMbps, &sample.UploadMbps,
		&sample.JitterMs, &sample.LatencyMs,
		&sample.ServerName, &sample.ServerID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sample.Timestamp, _ = time.Parse(time.RFC3339, ts)
	return &sample, nil
}

// GetSpeedTestHistory retrieves speed test results since the given time.
func (s *Store) GetSpeedTestHistory(since time.Time, limit int) ([]SpeedTestSample, error) {
	query := `
	SELECT id, timestamp, download_mbps, upload_mbps, jitter_ms, latency_ms, server_name, server_id
	FROM speed_test_samples
	WHERE timestamp >= ?
	ORDER BY timestamp DESC
	LIMIT ?
	`
	rows, err := s.db.Query(query, since.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []SpeedTestSample
	for rows.Next() {
		var sample SpeedTestSample
		var ts string
		err := rows.Scan(
			&sample.ID, &ts,
			&sample.DownloadMbps, &sample.UploadMbps,
			&sample.JitterMs, &sample.LatencyMs,
			&sample.ServerName, &sample.ServerID,
		)
		if err != nil {
			return nil, err
		}
		sample.Timestamp, _ = time.Parse(time.RFC3339, ts)
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

// --- Helpers ---

// scanSample scans a single row into a NetworkSample.
type scannable interface {
	Scan(dest ...interface{}) error
}

func scanSample(row scannable) (*NetworkSample, error) {
	var sample NetworkSample
	var ts string
	err := row.Scan(
		&sample.ID, &ts, &sample.Target,
		&sample.LatencyMs, &sample.PacketLoss,
		&sample.WifiSSID, &sample.WifiRSSI, &sample.WifiNoise, &sample.WifiChannel,
	)
	if err != nil {
		return nil, err
	}
	sample.Timestamp, _ = time.Parse(time.RFC3339, ts)
	return &sample, nil
}


// --- Webhook Config Methods ---

// GetWebhookConfigs returns all webhook configurations.
func (s *Store) GetWebhookConfigs() ([]WebhookConfig, error) {
	query := `
	SELECT id, name, url, platform, enabled, latency_threshold, packet_loss_threshold,
	       signal_threshold, connection_lost, cooldown_minutes, notify_recovery, created_at, updated_at
	FROM webhook_configs
	ORDER BY created_at ASC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []WebhookConfig
	for rows.Next() {
		var cfg WebhookConfig
		var enabled, connLost, notifyRecovery int
		var createdAt, updatedAt string
		var latencyThresh, lossThresh sql.NullFloat64
		var signalThresh sql.NullInt64

		err := rows.Scan(
			&cfg.ID, &cfg.Name, &cfg.URL, &cfg.Platform,
			&enabled, &latencyThresh, &lossThresh,
			&signalThresh, &connLost, &cfg.CooldownMinutes,
			&notifyRecovery, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		cfg.Enabled = enabled == 1
		cfg.ConnectionLost = connLost == 1
		cfg.NotifyRecovery = notifyRecovery == 1
		if latencyThresh.Valid {
			v := latencyThresh.Float64
			cfg.LatencyThreshold = &v
		}
		if lossThresh.Valid {
			v := lossThresh.Float64
			cfg.PacketLossThreshold = &v
		}
		if signalThresh.Valid {
			v := int(signalThresh.Int64)
			cfg.SignalThreshold = &v
		}
		cfg.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		cfg.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// GetWebhookConfig returns a single webhook configuration by ID.
func (s *Store) GetWebhookConfig(id int64) (*WebhookConfig, error) {
	query := `
	SELECT id, name, url, platform, enabled, latency_threshold, packet_loss_threshold,
	       signal_threshold, connection_lost, cooldown_minutes, notify_recovery, created_at, updated_at
	FROM webhook_configs WHERE id = ?
	`
	var cfg WebhookConfig
	var enabled, connLost, notifyRecovery int
	var createdAt, updatedAt string
	var latencyThresh, lossThresh sql.NullFloat64
	var signalThresh sql.NullInt64

	err := s.db.QueryRow(query, id).Scan(
		&cfg.ID, &cfg.Name, &cfg.URL, &cfg.Platform,
		&enabled, &latencyThresh, &lossThresh,
		&signalThresh, &connLost, &cfg.CooldownMinutes,
		&notifyRecovery, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	cfg.Enabled = enabled == 1
	cfg.ConnectionLost = connLost == 1
	cfg.NotifyRecovery = notifyRecovery == 1
	if latencyThresh.Valid {
		v := latencyThresh.Float64
		cfg.LatencyThreshold = &v
	}
	if lossThresh.Valid {
		v := lossThresh.Float64
		cfg.PacketLossThreshold = &v
	}
	if signalThresh.Valid {
		v := int(signalThresh.Int64)
		cfg.SignalThreshold = &v
	}
	cfg.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	cfg.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &cfg, nil
}

// CreateWebhookConfig inserts a new webhook configuration and returns its ID.
func (s *Store) CreateWebhookConfig(cfg WebhookConfig) (int64, error) {
	boolToInt := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}

	query := `
	INSERT INTO webhook_configs (name, url, platform, enabled, latency_threshold, packet_loss_threshold,
	                             signal_threshold, connection_lost, cooldown_minutes, notify_recovery)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query,
		cfg.Name, cfg.URL, cfg.Platform,
		boolToInt(cfg.Enabled),
		cfg.LatencyThreshold, cfg.PacketLossThreshold, cfg.SignalThreshold,
		boolToInt(cfg.ConnectionLost),
		cfg.CooldownMinutes,
		boolToInt(cfg.NotifyRecovery),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateWebhookConfig updates an existing webhook configuration.
func (s *Store) UpdateWebhookConfig(cfg WebhookConfig) error {
	boolToInt := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}

	query := `
	UPDATE webhook_configs
	SET name = ?, url = ?, platform = ?, enabled = ?, latency_threshold = ?,
	    packet_loss_threshold = ?, signal_threshold = ?, connection_lost = ?,
	    cooldown_minutes = ?, notify_recovery = ?, updated_at = datetime('now')
	WHERE id = ?
	`
	_, err := s.db.Exec(query,
		cfg.Name, cfg.URL, cfg.Platform,
		boolToInt(cfg.Enabled),
		cfg.LatencyThreshold, cfg.PacketLossThreshold, cfg.SignalThreshold,
		boolToInt(cfg.ConnectionLost),
		cfg.CooldownMinutes,
		boolToInt(cfg.NotifyRecovery),
		cfg.ID,
	)
	return err
}

// DeleteWebhookConfig removes a webhook configuration by ID.
func (s *Store) DeleteWebhookConfig(id int64) error {
	_, err := s.db.Exec("DELETE FROM webhook_configs WHERE id = ?", id)
	return err
}

// --- Alert History Methods ---

// InsertAlertHistory records an alert that was fired.
func (s *Store) InsertAlertHistory(entry AlertHistoryEntry) error {
	delivered := 0
	if entry.Delivered {
		delivered = 1
	}

	query := `
	INSERT INTO alert_history (webhook_id, condition, severity, message, value, threshold, fired_at, delivered)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		entry.WebhookID, entry.Condition, entry.Severity, entry.Message,
		entry.Value, entry.Threshold,
		entry.FiredAt.UTC().Format(time.RFC3339),
		delivered,
	)
	return err
}

// GetAlertHistory retrieves recent alert history entries.
func (s *Store) GetAlertHistory(since time.Time, limit int) ([]AlertHistoryEntry, error) {
	query := `
	SELECT id, webhook_id, condition, severity, message, value, threshold, fired_at, delivered
	FROM alert_history
	WHERE fired_at >= ?
	ORDER BY fired_at DESC
	LIMIT ?
	`
	rows, err := s.db.Query(query, since.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AlertHistoryEntry
	for rows.Next() {
		var e AlertHistoryEntry
		var ts string
		var delivered int

		err := rows.Scan(
			&e.ID, &e.WebhookID, &e.Condition, &e.Severity,
			&e.Message, &e.Value, &e.Threshold, &ts, &delivered,
		)
		if err != nil {
			return nil, err
		}
		e.FiredAt, _ = time.Parse(time.RFC3339, ts)
		e.Delivered = delivered == 1
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// PruneAlertHistory deletes alert history entries older than the given time.
func (s *Store) PruneAlertHistory(olderThan time.Time) (int64, error) {
	result, err := s.db.Exec(
		"DELETE FROM alert_history WHERE fired_at < ?",
		olderThan.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

