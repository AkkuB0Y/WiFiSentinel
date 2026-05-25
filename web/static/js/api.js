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
     * POST request with JSON body.
     * @param {string} endpoint - API path
     * @param {Object} body - Request body
     * @returns {Promise<Object>} Parsed JSON response
     */
    async function postRequest(endpoint, body = {}) {
        const response = await fetch(endpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
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

    /**
     * Trigger a manual speed test.
     * @returns {Promise<Object>} { status: 'started' | 'running' }
     */
    async function runSpeedTest() {
        const response = await fetch('/api/speedtest/run', { method: 'POST' });
        return response.json();
    }

    /**
     * Fetch speed test status (running state + latest result).
     * @returns {Promise<Object>} { running: boolean, latest: SpeedTestSample|null }
     */
    async function fetchSpeedTestStatus() {
        return request('/api/speedtest/status');
    }

    /**
     * Fetch historical speed test results.
     * @param {string} since - ISO 8601 timestamp
     * @param {number} limit - Max results
     * @returns {Promise<Object>} { samples: [], count: number }
     */
    async function fetchSpeedTestHistory(since, limit = 50) {
        return request('/api/speedtest/history', { since, limit });
    }

    // --- Cloud / Session API ---

    /**
     * Send Firebase Auth credentials to the backend.
     * @param {Object} authData - { id_token, user_id, email, display_name }
     * @returns {Promise<Object>} { status: 'authenticated', user_id: string }
     */
    async function cloudAuth(authData) {
        return postRequest('/api/cloud/auth', authData);
    }

    /**
     * Get cloud connection status.
     * @returns {Promise<Object>} { enabled, authenticated, active_session, project_id, api_key }
     */
    async function getCloudStatus() {
        return request('/api/cloud/status');
    }

    /**
     * Start a new cloud monitoring session.
     * @param {Object} sessionData - { name: string, network: string }
     * @returns {Promise<Object>} { status: 'started', session: Session }
     */
    async function startSession(sessionData) {
        return postRequest('/api/session/start', sessionData);
    }

    /**
     * Stop the active cloud monitoring session.
     * @returns {Promise<Object>} { status: 'stopped', session: Session }
     */
    async function stopSession() {
        return postRequest('/api/session/stop', {});
    }

    /**
     * Get the currently active session.
     * @returns {Promise<Object>} { active: boolean, session?: Session }
     */
    async function getActiveSession() {
        return request('/api/session/active');
    }

    /**
     * Get recent cloud sessions.
     * @returns {Promise<Object>} { sessions: [] }
     */
    async function getCloudSessions() {
        return request('/api/cloud/sessions');
    }

    return {
        fetchStatus,
        fetchHistory,
        fetchAggregates,
        fetchConfig,
        runSpeedTest,
        fetchSpeedTestStatus,
        fetchSpeedTestHistory,
        cloudAuth,
        getCloudStatus,
        startSession,
        stopSession,
        getActiveSession,
        getCloudSessions,
    };
})();

