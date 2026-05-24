/**
 * WiFi Sentinel — Main Application
 * Initializes the dashboard, manages polling, and updates the UI.
 */

const SentinelApp = (() => {
    // Polling configuration
    const POLL_INTERVAL = 5000; // 5 seconds
    let pollTimer = null;
    let isConnected = false;
    let currentRange = '1h';
    let consecutiveErrors = 0;
    let speedTestRunning = false;
    let speedTestPollTimer = null;

    // Time range → { since offset, bucket size in minutes }
    const RANGE_CONFIG = {
        '1h':  { offsetMs: 60 * 60 * 1000,          bucket: 1  },
        '6h':  { offsetMs: 6 * 60 * 60 * 1000,      bucket: 5  },
        '24h': { offsetMs: 24 * 60 * 60 * 1000,     bucket: 15 },
        '7d':  { offsetMs: 7 * 24 * 60 * 60 * 1000, bucket: 60 },
    };

    /**
     * Initialize the application.
     */
    function init() {
        // Initialize charts
        SentinelCharts.init();

        // Set up time range buttons
        setupTimeRangeSelector();

        // Set up speed test controls
        setupSpeedTest();

        // Load config for footer
        loadConfig();

        // Start polling
        poll();
        pollTimer = setInterval(poll, POLL_INTERVAL);
    }

    /**
     * Set up time range selector buttons.
     */
    function setupTimeRangeSelector() {
        const buttons = document.querySelectorAll('.range-btn');
        buttons.forEach(btn => {
            btn.addEventListener('click', () => {
                buttons.forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                currentRange = btn.dataset.range;
                // Immediately fetch new data for the selected range
                fetchAndUpdateCharts();
            });
        });
    }

    /**
     * Main poll function — fetches status and chart data.
     */
    async function poll() {
        try {
            // Fetch current status and chart data in parallel
            const [status, chartData] = await Promise.all([
                SentinelAPI.fetchStatus(),
                fetchChartData(),
            ]);

            // Update connection state
            setConnected(true);
            consecutiveErrors = 0;

            // Update status cards
            updateStatusCards(status);

            // Update charts
            if (chartData && chartData.buckets) {
                SentinelCharts.updateCharts(chartData.buckets);
            }

            // Update events table
            await updateEventsTable();

            // Update speed test status
            await updateSpeedTestStatus();

            // Update last-updated timestamp
            document.getElementById('last-updated').textContent = 
                new Date().toLocaleTimeString();

        } catch (err) {
            consecutiveErrors++;
            console.error('[sentinel] poll error:', err);

            if (consecutiveErrors >= 3) {
                setConnected(false);
            }
        }
    }

    /**
     * Fetch chart aggregate data based on current range selection.
     */
    async function fetchChartData() {
        const config = RANGE_CONFIG[currentRange];
        const since = new Date(Date.now() - config.offsetMs).toISOString();
        return SentinelAPI.fetchAggregates(since, config.bucket);
    }

    /**
     * Fetch and update only chart data (used when changing time range).
     */
    async function fetchAndUpdateCharts() {
        try {
            const data = await fetchChartData();
            if (data && data.buckets) {
                SentinelCharts.updateCharts(data.buckets);
            }
        } catch (err) {
            console.error('[sentinel] chart update error:', err);
        }
    }

    /**
     * Update status cards with latest sample data.
     */
    function updateStatusCards(data) {
        if (!data || data.status === 'waiting') {
            return; // No data yet
        }

        // Latency
        const latVal = document.getElementById('val-latency');
        const latInd = document.getElementById('indicator-latency');
        const latency = data.latency_ms;
        latVal.textContent = latency > 0 ? latency.toFixed(1) : '—';
        latVal.classList.add('value-flash');
        setTimeout(() => latVal.classList.remove('value-flash'), 400);
        
        if (latency <= 0) {
            latInd.className = 'card-indicator';
        } else if (latency < 50) {
            latInd.className = 'card-indicator good';
        } else if (latency < 100) {
            latInd.className = 'card-indicator warn';
        } else {
            latInd.className = 'card-indicator bad';
        }

        // Packet loss
        const lossVal = document.getElementById('val-loss');
        const lossInd = document.getElementById('indicator-loss');
        const loss = data.packet_loss;
        lossVal.textContent = loss.toFixed(1);
        lossVal.classList.add('value-flash');
        setTimeout(() => lossVal.classList.remove('value-flash'), 400);

        if (loss === 0) {
            lossInd.className = 'card-indicator good';
        } else if (loss < 10) {
            lossInd.className = 'card-indicator warn';
        } else {
            lossInd.className = 'card-indicator bad';
        }

        // Signal strength (RSSI)
        const sigVal = document.getElementById('val-signal');
        const sigInd = document.getElementById('indicator-signal');
        const rssi = data.wifi_rssi;
        sigVal.textContent = rssi !== 0 ? rssi : '—';
        sigVal.classList.add('value-flash');
        setTimeout(() => sigVal.classList.remove('value-flash'), 400);

        if (rssi === 0) {
            sigInd.className = 'card-indicator';
        } else if (rssi >= -50) {
            sigInd.className = 'card-indicator good';
        } else if (rssi >= -70) {
            sigInd.className = 'card-indicator warn';
        } else {
            sigInd.className = 'card-indicator bad';
        }

        // Network info
        const netVal = document.getElementById('val-network');
        const chVal = document.getElementById('val-channel');
        const netInd = document.getElementById('indicator-network');
        // Determine connection status: SSID may be empty due to macOS privacy
        // redaction even when connected (RSSI non-zero).
        const ssid = data.wifi_ssid;
        const isConnected = ssid || rssi !== 0;
        if (ssid) {
            netVal.textContent = ssid;
        } else if (rssi !== 0) {
            netVal.textContent = 'Connected (Private)';
        } else {
            netVal.textContent = 'Not Connected';
        }
        chVal.textContent = data.wifi_channel > 0 ? `Channel ${data.wifi_channel}` : '—';
        netInd.className = isConnected ? 'card-indicator good' : 'card-indicator';
    }

    /**
     * Update the recent events table.
     */
    async function updateEventsTable() {
        try {
            const since = new Date(Date.now() - 5 * 60 * 1000).toISOString(); // Last 5 minutes
            const data = await SentinelAPI.fetchHistory(since, 20);

            const tbody = document.getElementById('events-tbody');
            const countEl = document.getElementById('event-count');

            if (!data.samples || data.samples.length === 0) {
                tbody.innerHTML = '<tr><td colspan="7" class="empty-state">Waiting for data...</td></tr>';
                countEl.textContent = '0 samples';
                return;
            }

            countEl.textContent = `${data.count} samples`;

            const rows = data.samples.slice(0, 20).map(s => {
                const time = new Date(s.timestamp).toLocaleTimeString();
                const latClass = getStatusClass(s.latency_ms, 50, 100);
                const lossClass = getLossClass(s.packet_loss);
                const rssiClass = getRSSIClass(s.wifi_rssi);

                return `<tr>
                    <td>${time}</td>
                    <td>${s.target}</td>
                    <td class="${latClass}">${s.latency_ms > 0 ? s.latency_ms.toFixed(1) + ' ms' : '—'}</td>
                    <td class="${lossClass}">${s.packet_loss.toFixed(1)}%</td>
                    <td>${s.wifi_ssid || '—'}</td>
                    <td class="${rssiClass}">${s.wifi_rssi !== 0 ? s.wifi_rssi + ' dBm' : '—'}</td>
                    <td>${s.wifi_channel > 0 ? s.wifi_channel : '—'}</td>
                </tr>`;
            }).join('');

            tbody.innerHTML = rows;
        } catch (err) {
            console.error('[sentinel] events table error:', err);
        }
    }

    /**
     * Set connection status indicator.
     */
    function setConnected(connected) {
        if (connected === isConnected) return;
        isConnected = connected;

        const badge = document.getElementById('connection-status');
        const text = badge.querySelector('.status-text');

        if (connected) {
            badge.classList.remove('disconnected');
            text.textContent = 'Connected';
        } else {
            badge.classList.add('disconnected');
            text.textContent = 'Disconnected';
        }
    }

    /**
     * Load and display daemon config in the footer.
     */
    async function loadConfig() {
        try {
            const cfg = await SentinelAPI.fetchConfig();
            document.getElementById('footer-targets').textContent = 
                `Targets: ${cfg.ping_targets.join(', ')}`;
            document.getElementById('footer-interval').textContent = 
                `Poll: ${cfg.poll_interval}`;
        } catch (err) {
            // Config display is non-critical
            console.warn('[sentinel] could not load config:', err);
        }
    }

    // --- Helper functions for status coloring ---

    function getStatusClass(value, warnThresh, badThresh) {
        if (value <= 0) return '';
        if (value < warnThresh) return 'status-good';
        if (value < badThresh) return 'status-warn';
        return 'status-bad';
    }

    function getLossClass(loss) {
        if (loss === 0) return 'status-good';
        if (loss < 10) return 'status-warn';
        return 'status-bad';
    }

    function getRSSIClass(rssi) {
        if (rssi === 0) return '';
        if (rssi >= -50) return 'status-good';
        if (rssi >= -70) return 'status-warn';
        return 'status-bad';
    }

    // --- Speed Test Functions ---

    /**
     * Set up speed test button handlers.
     */
    function setupSpeedTest() {
        const runBtn = document.getElementById('speedtest-run-btn');
        const expandBtn = document.getElementById('speedtest-expand-btn');
        const historyPanel = document.getElementById('speedtest-history');

        if (runBtn) {
            runBtn.addEventListener('click', handleRunSpeedTest);
        }

        if (expandBtn && historyPanel) {
            expandBtn.addEventListener('click', () => {
                const expanded = historyPanel.classList.toggle('expanded');
                expandBtn.classList.toggle('expanded', expanded);
                if (expanded) {
                    loadSpeedTestHistory();
                }
            });
        }
    }

    /**
     * Handle the Run Test button click.
     */
    async function handleRunSpeedTest() {
        const btn = document.getElementById('speedtest-run-btn');
        const progress = document.getElementById('speedtest-progress');
        const progressBar = document.getElementById('speedtest-progress-bar');
        const progressText = document.getElementById('speedtest-progress-text');

        try {
            const resp = await SentinelAPI.runSpeedTest();

            if (resp.status === 'started' || resp.status === 'running') {
                // Show progress state
                btn.classList.add('running');
                btn.querySelector('span').textContent = 'Running...';
                progress.classList.add('active');
                progressText.textContent = 'Measuring download & upload speeds...';
                speedTestRunning = true;

                // Reset progress bar animation
                progressBar.style.animation = 'none';
                progressBar.offsetHeight; // force reflow
                progressBar.style.animation = '';

                // Poll more frequently while test is running
                startSpeedTestPoll();
            }
        } catch (err) {
            console.error('[sentinel] speed test trigger error:', err);
        }
    }

    /**
     * Poll speed test status every 2s while a test is running.
     */
    function startSpeedTestPoll() {
        if (speedTestPollTimer) clearInterval(speedTestPollTimer);
        speedTestPollTimer = setInterval(async () => {
            try {
                const status = await SentinelAPI.fetchSpeedTestStatus();
                if (!status.running && speedTestRunning) {
                    // Test just completed
                    speedTestRunning = false;
                    clearInterval(speedTestPollTimer);
                    speedTestPollTimer = null;

                    // Reset UI
                    const btn = document.getElementById('speedtest-run-btn');
                    const progress = document.getElementById('speedtest-progress');
                    btn.classList.remove('running');
                    btn.querySelector('span').textContent = 'Run Test';
                    progress.classList.remove('active');

                    // Update with new results
                    updateSpeedTestDisplay(status.latest);
                    loadSpeedTestHistory();
                }
            } catch (err) {
                console.error('[sentinel] speed test poll error:', err);
            }
        }, 2000);
    }

    /**
     * Update speed test status during regular polling.
     */
    async function updateSpeedTestStatus() {
        try {
            const status = await SentinelAPI.fetchSpeedTestStatus();

            // Update running state
            if (status.running && !speedTestRunning) {
                speedTestRunning = true;
                const btn = document.getElementById('speedtest-run-btn');
                const progress = document.getElementById('speedtest-progress');
                btn.classList.add('running');
                btn.querySelector('span').textContent = 'Running...';
                progress.classList.add('active');
                startSpeedTestPoll();
            }

            // Update display with latest result
            if (status.latest) {
                updateSpeedTestDisplay(status.latest);
            }
        } catch (err) {
            // Speed test status is non-critical
            console.warn('[sentinel] speed test status error:', err);
        }
    }

    /**
     * Update speed test bar with latest result data.
     */
    function updateSpeedTestDisplay(data) {
        if (!data) return;

        document.getElementById('speedtest-download').textContent = data.download_mbps.toFixed(1);
        document.getElementById('speedtest-upload').textContent = data.upload_mbps.toFixed(1);
        document.getElementById('speedtest-jitter').textContent = data.jitter_ms.toFixed(1);
        document.getElementById('speedtest-ago').textContent = timeAgo(new Date(data.timestamp));
    }

    /**
     * Load and display speed test history chart.
     */
    async function loadSpeedTestHistory() {
        try {
            const since = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString();
            const data = await SentinelAPI.fetchSpeedTestHistory(since, 50);
            if (data && data.samples) {
                SentinelCharts.updateSpeedTestChart(data.samples);
            }
        } catch (err) {
            console.error('[sentinel] speed test history error:', err);
        }
    }

    /**
     * Format a date as a human-readable "time ago" string.
     */
    function timeAgo(date) {
        const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
        if (seconds < 60) return 'just now';
        const minutes = Math.floor(seconds / 60);
        if (minutes < 60) return `${minutes} min ago`;
        const hours = Math.floor(minutes / 60);
        if (hours < 24) return `${hours}h ago`;
        const days = Math.floor(hours / 24);
        return `${days}d ago`;
    }

    return { init };
})();

// Start the app when DOM is ready
document.addEventListener('DOMContentLoaded', SentinelApp.init);
