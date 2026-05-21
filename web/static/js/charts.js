/**
 * WiFi Sentinel — Chart Manager
 * Creates and manages Chart.js time-series visualizations.
 */

const SentinelCharts = (() => {
    // Chart.js global defaults for our dark theme
    Chart.defaults.color = '#94a3b8';
    Chart.defaults.font.family = "'Inter', sans-serif";
    Chart.defaults.font.size = 11;
    Chart.defaults.plugins.legend.display = false;
    Chart.defaults.responsive = true;
    Chart.defaults.maintainAspectRatio = false;
    Chart.defaults.animation.duration = 400;
    Chart.defaults.animation.easing = 'easeOutQuart';

    const gridColor = 'rgba(255, 255, 255, 0.04)';
    const tickColor = '#64748b';

    let latencyChart = null;
    let signalChart = null;
    let lossChart = null;

    /**
     * Shared axis configuration for all time-series charts.
     */
    function timeScaleConfig() {
        return {
            type: 'time',
            time: {
                tooltipFormat: 'HH:mm:ss',
                displayFormats: {
                    second: 'HH:mm:ss',
                    minute: 'HH:mm',
                    hour: 'HH:mm',
                    day: 'MMM d',
                },
            },
            grid: {
                color: gridColor,
                drawBorder: false,
            },
            ticks: {
                color: tickColor,
                maxRotation: 0,
                autoSkipPadding: 20,
                font: { size: 10, family: "'JetBrains Mono', monospace" },
            },
        };
    }

    function yAxisConfig(label, suggestedMin, suggestedMax) {
        return {
            grid: {
                color: gridColor,
                drawBorder: false,
            },
            ticks: {
                color: tickColor,
                font: { size: 10, family: "'JetBrains Mono', monospace" },
                padding: 8,
            },
            suggestedMin,
            suggestedMax,
            title: {
                display: false,
            },
        };
    }

    function tooltipConfig() {
        return {
            backgroundColor: 'rgba(15, 23, 42, 0.95)',
            titleColor: '#f1f5f9',
            bodyColor: '#94a3b8',
            borderColor: 'rgba(255, 255, 255, 0.1)',
            borderWidth: 1,
            cornerRadius: 8,
            padding: 10,
            titleFont: { weight: '600', size: 12 },
            bodyFont: { size: 11, family: "'JetBrains Mono', monospace" },
            displayColors: true,
            boxWidth: 8,
            boxHeight: 8,
            boxPadding: 4,
        };
    }

    /**
     * Create the latency time-series chart.
     */
    function createLatencyChart(canvasId) {
        const ctx = document.getElementById(canvasId).getContext('2d');
        
        // Create gradient fill
        const gradient = ctx.createLinearGradient(0, 0, 0, 220);
        gradient.addColorStop(0, 'rgba(56, 189, 248, 0.2)');
        gradient.addColorStop(1, 'rgba(56, 189, 248, 0)');

        latencyChart = new Chart(ctx, {
            type: 'line',
            data: {
                datasets: [
                    {
                        label: 'Avg Latency',
                        data: [],
                        borderColor: '#38bdf8',
                        backgroundColor: gradient,
                        borderWidth: 2,
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0,
                        pointHoverRadius: 4,
                        pointHoverBackgroundColor: '#38bdf8',
                    },
                    {
                        label: 'Max Latency',
                        data: [],
                        borderColor: 'rgba(239, 68, 68, 0.4)',
                        borderWidth: 1,
                        borderDash: [4, 4],
                        fill: false,
                        tension: 0.3,
                        pointRadius: 0,
                        pointHoverRadius: 3,
                    },
                ],
            },
            options: {
                scales: {
                    x: timeScaleConfig(),
                    y: yAxisConfig('ms', 0, undefined),
                },
                plugins: {
                    tooltip: tooltipConfig(),
                },
                interaction: {
                    intersect: false,
                    mode: 'index',
                },
            },
        });

        return latencyChart;
    }

    /**
     * Create the WiFi signal strength chart.
     */
    function createSignalChart(canvasId) {
        const ctx = document.getElementById(canvasId).getContext('2d');

        const gradient = ctx.createLinearGradient(0, 0, 0, 220);
        gradient.addColorStop(0, 'rgba(34, 197, 94, 0.2)');
        gradient.addColorStop(1, 'rgba(34, 197, 94, 0)');

        signalChart = new Chart(ctx, {
            type: 'line',
            data: {
                datasets: [
                    {
                        label: 'RSSI',
                        data: [],
                        borderColor: '#22c55e',
                        backgroundColor: gradient,
                        borderWidth: 2,
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0,
                        pointHoverRadius: 4,
                        pointHoverBackgroundColor: '#22c55e',
                    },
                ],
            },
            options: {
                scales: {
                    x: timeScaleConfig(),
                    y: yAxisConfig('dBm', -90, -20),
                },
                plugins: {
                    tooltip: tooltipConfig(),
                },
                interaction: {
                    intersect: false,
                    mode: 'index',
                },
            },
        });

        return signalChart;
    }

    /**
     * Create the packet loss bar chart.
     */
    function createPacketLossChart(canvasId) {
        const ctx = document.getElementById(canvasId).getContext('2d');

        lossChart = new Chart(ctx, {
            type: 'bar',
            data: {
                datasets: [
                    {
                        label: 'Packet Loss',
                        data: [],
                        backgroundColor: (context) => {
                            const value = context.raw?.y || 0;
                            if (value >= 50) return 'rgba(239, 68, 68, 0.7)';
                            if (value >= 10) return 'rgba(245, 158, 11, 0.7)';
                            if (value > 0) return 'rgba(245, 158, 11, 0.4)';
                            return 'rgba(34, 197, 94, 0.3)';
                        },
                        borderRadius: 3,
                        borderSkipped: false,
                        maxBarThickness: 12,
                    },
                ],
            },
            options: {
                scales: {
                    x: timeScaleConfig(),
                    y: {
                        ...yAxisConfig('%', 0, 100),
                        max: 100,
                    },
                },
                plugins: {
                    tooltip: tooltipConfig(),
                },
            },
        });

        return lossChart;
    }

    /**
     * Update all charts with new aggregate data.
     * @param {Array} buckets - Array of AggregateBucket objects from the API
     */
    function updateCharts(buckets) {
        if (!buckets || buckets.length === 0) return;

        const latencyAvg = [];
        const latencyMax = [];
        const signalData = [];
        const lossData = [];

        buckets.forEach(b => {
            const t = new Date(b.bucket_start);
            latencyAvg.push({ x: t, y: Math.round(b.avg_latency_ms * 100) / 100 });
            latencyMax.push({ x: t, y: Math.round(b.max_latency_ms * 100) / 100 });
            signalData.push({ x: t, y: Math.round(b.avg_wifi_rssi) });
            lossData.push({ x: t, y: Math.round(b.avg_packet_loss * 100) / 100 });
        });

        if (latencyChart) {
            latencyChart.data.datasets[0].data = latencyAvg;
            latencyChart.data.datasets[1].data = latencyMax;
            latencyChart.update('none');  // Skip animation for smooth updates
        }

        if (signalChart) {
            signalChart.data.datasets[0].data = signalData;
            signalChart.update('none');
        }

        if (lossChart) {
            lossChart.data.datasets[0].data = lossData;
            lossChart.update('none');
        }

        // Update chart badges
        updateBadges(buckets);
    }

    /**
     * Update the small badge values next to chart titles.
     */
    function updateBadges(buckets) {
        if (buckets.length === 0) return;

        const avgLat = buckets.reduce((s, b) => s + b.avg_latency_ms, 0) / buckets.length;
        const avgSig = buckets.reduce((s, b) => s + b.avg_wifi_rssi, 0) / buckets.length;
        const avgLoss = buckets.reduce((s, b) => s + b.avg_packet_loss, 0) / buckets.length;

        const latBadge = document.getElementById('chart-badge-latency');
        const sigBadge = document.getElementById('chart-badge-signal');
        const lossBadge = document.getElementById('chart-badge-loss');

        if (latBadge) latBadge.textContent = `avg: ${avgLat.toFixed(1)} ms`;
        if (sigBadge) sigBadge.textContent = `avg: ${Math.round(avgSig)} dBm`;
        if (lossBadge) lossBadge.textContent = `avg: ${avgLoss.toFixed(1)}%`;
    }

    /**
     * Initialize all charts.
     */
    function init() {
        createLatencyChart('chart-latency');
        createSignalChart('chart-signal');
        createPacketLossChart('chart-loss');
    }

    /**
     * Destroy all charts (for cleanup).
     */
    function destroy() {
        if (latencyChart) { latencyChart.destroy(); latencyChart = null; }
        if (signalChart) { signalChart.destroy(); signalChart = null; }
        if (lossChart) { lossChart.destroy(); lossChart = null; }
    }

    return {
        init,
        destroy,
        updateCharts,
    };
})();
