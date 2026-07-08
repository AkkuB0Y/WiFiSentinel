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
	`
	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}

	// Additive migrations for existing databases.
	for _, stmt := range []string{
		`ALTER TABLE network_samples ADD COLUMN wifi_rssi_estimated INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE network_samples ADD COLUMN wifi_noise_available INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = s.db.Exec(stmt)
	}

	return nil
}

// InsertSample writes a single network sample to the database.
func (s *Store) InsertSample(sample NetworkSample) error {
	query := `
	INSERT INTO network_samples (timestamp, target, latency_ms, packet_loss, wifi_ssid, wifi_rssi, wifi_noise, wifi_channel, wifi_rssi_estimated, wifi_noise_available)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		boolToInt(sample.WifiRssiEstimated),
		boolToInt(sample.WifiNoiseAvailable),
	)
	return err
}

// GetLatestSample retrieves the most recent network sample.
// Returns nil if no samples exist.
func (s *Store) GetLatestSample() (*NetworkSample, error) {
	query := `
	SELECT id, timestamp, target, latency_ms, packet_loss, wifi_ssid, wifi_rssi, wifi_noise, wifi_channel, wifi_rssi_estimated, wifi_noise_available
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
	SELECT id, timestamp, target, latency_ms, packet_loss, wifi_ssid, wifi_rssi, wifi_noise, wifi_channel, wifi_rssi_estimated, wifi_noise_available
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
		var rssiEstimated, noiseAvailable int
		err := rows.Scan(
			&sample.ID, &ts, &sample.Target,
			&sample.LatencyMs, &sample.PacketLoss,
			&sample.WifiSSID, &sample.WifiRSSI, &sample.WifiNoise, &sample.WifiChannel,
			&rssiEstimated, &noiseAvailable,
		)
		if err != nil {
			return nil, err
		}
		sample.Timestamp, _ = time.Parse(time.RFC3339, ts)
		sample.WifiRssiEstimated = rssiEstimated != 0
		sample.WifiNoiseAvailable = noiseAvailable != 0
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
	var rssiEstimated, noiseAvailable int
	err := row.Scan(
		&sample.ID, &ts, &sample.Target,
		&sample.LatencyMs, &sample.PacketLoss,
		&sample.WifiSSID, &sample.WifiRSSI, &sample.WifiNoise, &sample.WifiChannel,
		&rssiEstimated, &noiseAvailable,
	)
	if err != nil {
		return nil, err
	}
	sample.Timestamp, _ = time.Parse(time.RFC3339, ts)
	sample.WifiRssiEstimated = rssiEstimated != 0
	sample.WifiNoiseAvailable = noiseAvailable != 0
	return &sample, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
