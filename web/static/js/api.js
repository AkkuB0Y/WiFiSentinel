/**
 * WiFi Sentinel — API Client
 * Handles all HTTP communication with the backend API.
 */

const SentinelAPI = (() => {
    const BASE_URL = '';  // Same origin — no prefix needed

    /**
     * Generic fetch wrapper with error handling.
     * @param {string} endpoint - API path (e.g., '/api/status')
     * @param {Object} params - Query parameters
     * @returns {Promise<Object>} Parsed JSON response
     */
    async function request(endpoint, params = {}) {
        const url = new URL(endpoint, window.location.origin);
        Object.entries(params).forEach(([key, value]) => {
            if (value !== undefined && value !== null) {
                url.searchParams.set(key, value);
            }
        });

        const response = await fetch(url.toString());
        if (!response.ok) {
            throw new Error(`API error: ${response.status} ${response.statusText}`);
        }
        return response.json();
    }

    /**
     * Fetch the latest network sample.
     * @returns {Promise<Object>} Latest sample or status message
     */
    async function fetchStatus() {
        return request('/api/status');
    }

    /**
     * Fetch recent network samples.
     * @param {string} since - ISO 8601 timestamp
     * @param {number} limit - Max number of samples
     * @returns {Promise<Object>} { samples: [], count: number, since: string }
     */
    async function fetchHistory(since, limit = 500) {
        return request('/api/history', { since, limit });
    }

    /**
     * Fetch aggregated data for chart rendering.
     * @param {string} since - ISO 8601 timestamp
     * @param {number} bucket - Bucket size in minutes
     * @returns {Promise<Object>} { buckets: [], count: number, bucket_minutes: number }
     */
    async function fetchAggregates(since, bucket = 5) {
        return request('/api/aggregates', { since, bucket });
    }

    /**
     * Fetch current daemon configuration.
     * @returns {Promise<Object>} Configuration object
     */
    async function fetchConfig() {
        return request('/api/config');
    }

    return {
        fetchStatus,
        fetchHistory,
        fetchAggregates,
        fetchConfig,
    };
})();
