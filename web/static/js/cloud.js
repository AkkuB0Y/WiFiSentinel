/**
 * WiFi Sentinel — Cloud Module
 * Handles Firebase Auth (Google Sign-In) globally and manages
 * the Cloud Console session state and past sessions history feed.
 */

const SentinelCloud = (() => {
    let firebaseApp = null;
    let firebaseAuth = null;
    let isInitialized = false;
    let currentUser = null;
    let cloudConfig = null;
    let activeSession = null;
    let tokenRefreshTimer = null;

    /**
     * Initialize the cloud module.
     * Fetches cloud config from the backend and sets up Firebase if enabled.
     */
    async function init() {
        try {
            cloudConfig = await SentinelAPI.getCloudStatus();

            if (!cloudConfig.enabled) {
                hideCloudBar();
                return;
            }

            showCloudBar();

            // Load Firebase SDK from CDN
            await loadFirebaseSDK();

            // Initialize Firebase
            const config = {
                apiKey: cloudConfig.api_key,
                authDomain: `${cloudConfig.project_id}.firebaseapp.com`,
                projectId: cloudConfig.project_id,
            };

            // Firebase compat SDK uses firebase.initializeApp
            if (typeof firebase !== 'undefined') {
                firebaseApp = firebase.initializeApp(config);
                firebaseAuth = firebase.auth();

                // Listen for auth state changes
                firebaseAuth.onAuthStateChanged(handleAuthStateChange);
                isInitialized = true;
            }

            // Check for active session
            updateSessionState();

        } catch (err) {
            console.error('[cloud] initialization error:', err);
        }
    }

    /**
     * Load Firebase Auth SDK from CDN.
     */
    function loadFirebaseSDK() {
        return new Promise((resolve, reject) => {
            // Check if already loaded
            if (typeof firebase !== 'undefined' && firebase.auth) {
                resolve();
                return;
            }

            // Load Firebase App
            const appScript = document.createElement('script');
            appScript.src = 'https://www.gstatic.com/firebasejs/10.12.0/firebase-app-compat.js';
            appScript.onload = () => {
                // Load Firebase Auth
                const authScript = document.createElement('script');
                authScript.src = 'https://www.gstatic.com/firebasejs/10.12.0/firebase-auth-compat.js';
                authScript.onload = resolve;
                authScript.onerror = reject;
                document.head.appendChild(authScript);
            };
            appScript.onerror = reject;
            document.head.appendChild(appScript);
        });
    }

    /**
     * Handle Firebase Auth state changes.
     */
    async function handleAuthStateChange(user) {
        currentUser = user;

        // Render header auth state on whatever page is open
        renderHeaderAuth(user);

        if (user) {
            // User signed in — send token to backend
            try {
                const idToken = await user.getIdToken(true);
                await SentinelAPI.cloudAuth({
                    id_token: idToken,
                    user_id: user.uid,
                    email: user.email || '',
                    display_name: user.displayName || '',
                });

                // Set up periodic token refresh (every 50 minutes)
                if (tokenRefreshTimer) clearInterval(tokenRefreshTimer);
                tokenRefreshTimer = setInterval(async () => {
                    try {
                        const freshToken = await user.getIdToken(true);
                        await SentinelAPI.cloudAuth({
                            id_token: freshToken,
                            user_id: user.uid,
                            email: user.email || '',
                            display_name: user.displayName || '',
                        });
                    } catch (err) {
                        console.error('[cloud] token refresh error:', err);
                    }
                }, 50 * 60 * 1000);

                // Update Central UI states if on cloud.html page
                if (isCloudConsolePage()) {
                    if (activeSession) {
                        showRecordingState(activeSession);
                    } else {
                        showAuthenticatedState(user);
                    }
                }
            } catch (err) {
                console.error('[cloud] auth backend error:', err);
                if (isCloudConsolePage()) showUnauthenticatedState();
            }
        } else {
            // User signed out
            if (tokenRefreshTimer) {
                clearInterval(tokenRefreshTimer);
                tokenRefreshTimer = null;
            }
            if (isCloudConsolePage()) {
                showUnauthenticatedState();
            }
        }
    }

    /**
     * Sign in with Google popup.
     */
    async function signIn() {
        if (!firebaseAuth) return;

        try {
            const provider = new firebase.auth.GoogleAuthProvider();
            await firebaseAuth.signInWithPopup(provider);
        } catch (err) {
            if (err.code !== 'auth/popup-closed-by-user') {
                console.error('[cloud] sign-in error:', err);
            }
        }
    }

    /**
     * Sign out.
     */
    async function signOut() {
        if (!firebaseAuth) return;

        // Stop active session first
        if (activeSession) {
            await stopSession();
        }

        try {
            await firebaseAuth.signOut();
            // Redirect back to main dashboard on sign out if on cloud console
            if (isCloudConsolePage()) {
                window.location.href = 'index.html';
            }
        } catch (err) {
            console.error('[cloud] sign-out error:', err);
        }
    }

    /**
     * Start a new cloud monitoring session.
     */
    async function startSession() {
        const nameInput = document.getElementById('cloud-session-name');
        const name = nameInput ? nameInput.value.trim() : '';

        // Query active SSID from backend to assign to session network metadata
        let networkSSID = 'Local Network';
        try {
            const status = await SentinelAPI.fetchStatus();
            if (status && status.wifi_ssid) {
                networkSSID = status.wifi_ssid;
            }
        } catch (err) {
            // Non-critical fallback
        }

        try {
            const result = await SentinelAPI.startSession({
                name: name || 'Untitled Session',
                network: networkSSID,
            });

            if (result.session) {
                activeSession = result.session;
                showRecordingState(activeSession);
            }
        } catch (err) {
            console.error('[cloud] start session error:', err);
        }
    }

    /**
     * Stop the active cloud monitoring session.
     */
    async function stopSession() {
        try {
            await SentinelAPI.stopSession();
            activeSession = null;
            showAuthenticatedState(currentUser);
        } catch (err) {
            console.error('[cloud] stop session error:', err);
        }
    }

    /**
     * Update session state from the backend.
     */
    async function updateSessionState() {
        try {
            const status = await SentinelAPI.getCloudStatus();
            if (status.active_session) {
                activeSession = status.active_session;
                if (isCloudConsolePage()) {
                    showRecordingState(activeSession);
                }
            } else if (currentUser && isCloudConsolePage()) {
                showAuthenticatedState(currentUser);
            }
        } catch (err) {
            // Non-critical
        }
    }

    // --- Helper Functions ---

    function isCloudConsolePage() {
        // Returns true if the central cloud-content section exists (specific to cloud.html)
        return document.getElementById('cloud-content') !== null;
    }

    function hideCloudBar() {
        const bar = document.getElementById('cloud-session-section');
        if (bar) bar.style.display = 'none';
        
        const headerAuth = document.getElementById('header-auth-section');
        if (headerAuth) headerAuth.style.display = 'none';
    }

    function showCloudBar() {
        const bar = document.getElementById('cloud-session-section');
        if (bar) bar.style.display = '';

        const headerAuth = document.getElementById('header-auth-section');
        if (headerAuth) headerAuth.style.display = '';
    }

    function updateStatusBadge(state) {
        const badge = document.getElementById('cloud-status-badge');
        const dot = badge ? badge.querySelector('.cloud-status-dot') : null;
        const text = document.getElementById('cloud-status-text');

        if (!badge || !dot || !text) return;

        badge.className = 'cloud-status-badge ' + state;
        if (state === 'unauth') {
            text.textContent = 'Unauthenticated';
        } else if (state === 'auth') {
            text.textContent = 'Connected';
        } else if (state === 'recording') {
            text.textContent = 'Recording';
        }
    }

    // --- Global Header Auth Rendering ---

    function renderHeaderAuth(user) {
        const container = document.getElementById('header-auth-section');
        if (!container) return;

        if (!user) {
            container.innerHTML = `
                <button class="header-signin-btn" id="header-signin-btn">
                    <svg width="14" height="14" viewBox="0 0 24 24"><path fill="currentColor" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"/><path fill="currentColor" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/><path fill="currentColor" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/><path fill="currentColor" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/></svg>
                    <span>Sign In</span>
                </button>
            `;
            document.getElementById('header-signin-btn').addEventListener('click', signIn);
        } else {
            const displayName = user.displayName || user.email || 'User';
            const photoURL = user.photoURL;
            const avatarHTML = photoURL
                ? `<img class="header-profile-avatar" src="${photoURL}" alt="" width="28" height="28" referrerpolicy="no-referrer">`
                : `<div class="header-profile-avatar-placeholder">${displayName.charAt(0).toUpperCase()}</div>`;

            // Render profile trigger and drop-down menu
            container.innerHTML = `
                <div class="header-profile-trigger" id="header-profile-trigger" title="Account Menu">
                    ${avatarHTML}
                </div>
                <div class="auth-dropdown" id="auth-dropdown">
                    <div class="auth-dropdown-user-info">
                        <span class="auth-dropdown-user-name">${displayName}</span>
                        <span class="auth-dropdown-user-email">${user.email || ''}</span>
                    </div>
                    <div class="auth-dropdown-links">
                        <a href="cloud.html" class="auth-dropdown-link">
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z"/>
                            </svg>
                            <span>Cloud Console</span>
                        </a>
                        <a href="index.html" class="auth-dropdown-link">
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <rect x="3" y="3" width="7" height="9"/>
                                <rect x="14" y="3" width="7" height="5"/>
                                <rect x="14" y="12" width="7" height="9"/>
                                <rect x="3" y="16" width="7" height="5"/>
                            </svg>
                            <span>Dashboard</span>
                        </a>
                    </div>
                    <button class="auth-dropdown-signout-btn" id="header-signout-btn">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
                            <polyline points="16 17 21 12 16 7"/>
                            <line x1="21" y1="12" x2="9" y2="12"/>
                        </svg>
                        <span>Sign Out</span>
                    </button>
                </div>
            `;

            // Toggle dropdown logic
            const trigger = document.getElementById('header-profile-trigger');
            const dropdown = document.getElementById('auth-dropdown');

            trigger.addEventListener('click', (e) => {
                e.stopPropagation();
                dropdown.classList.toggle('open');
                trigger.classList.toggle('active');
            });

            document.addEventListener('click', () => {
                if (dropdown.classList.contains('open')) {
                    dropdown.classList.remove('open');
                    trigger.classList.remove('active');
                }
            });

            dropdown.addEventListener('click', (e) => {
                e.stopPropagation(); // prevent closing when clicking inside
            });

            document.getElementById('header-signout-btn').addEventListener('click', signOut);
        }
    }

    // --- Dedicated Page Central UI Renderers ---

    function showUnauthenticatedState() {
        updateStatusBadge('unauth');
        const content = document.getElementById('cloud-content');
        if (!content) return;

        content.innerHTML = `
            <div class="cloud-unauth-grid">
                <div class="cloud-benefits-panel">
                    <p class="cloud-info-desc">WiFi Sentinel Cloud allows you to stream and store your network health metrics securely to Google Firestore.</p>
                    <ul class="cloud-benefits-list">
                        <li>
                            <span class="benefit-icon">☁️</span>
                            <div class="benefit-desc">
                                <strong>Real-Time Syncing</strong>
                                <span>Streams latency and speed tests directly to your cloud project.</span>
                            </div>
                        </li>
                        <li>
                            <span class="benefit-icon">📊</span>
                            <div class="benefit-desc">
                                <strong>Multi-Device Dashboard</strong>
                                <span>Track network tests from multiple hosts simultaneously in one database.</span>
                            </div>
                        </li>
                        <li>
                            <span class="benefit-icon">💾</span>
                            <div class="benefit-desc">
                                <strong>Offline Auto-Buffer</strong>
                                <span>No local data loss during outages. Automatically flushes once connection resumes.</span>
                            </div>
                        </li>
                    </ul>
                </div>
                <div class="cloud-login-panel">
                    <div class="login-wrapper">
                        <div class="login-decor">
                            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                                <path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z" stroke-linecap="round" stroke-linejoin="round"/>
                            </svg>
                        </div>
                        <h3>Opt-In Cloud Tracking</h3>
                        <p>Authenticate with Google to start recording secure sessions to Firestore.</p>
                        <button class="cloud-signin-btn" id="cloud-signin-btn-main">
                            <svg width="18" height="18" viewBox="0 0 24 24"><path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"/><path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/><path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/><path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/></svg>
                            <span>Sign in with Google</span>
                        </button>
                    </div>
                </div>
            </div>
        `;

        document.getElementById('cloud-signin-btn-main').addEventListener('click', signIn);
    }

    function showAuthenticatedState(user) {
        updateStatusBadge('auth');
        const content = document.getElementById('cloud-content');
        if (!content) return;

        const displayName = user.displayName || user.email || 'User';
        const photoURL = user.photoURL;
        const avatarHTML = photoURL
            ? `<img class="cloud-avatar" src="${photoURL}" alt="" width="32" height="32" referrerpolicy="no-referrer">`
            : `<div class="cloud-avatar-placeholder">${displayName.charAt(0).toUpperCase()}</div>`;

        content.innerHTML = `
            <div class="cloud-auth-grid">
                <div class="cloud-session-creation-panel">
                    <div class="profile-header-row">
                        <div class="cloud-user-info">
                            ${avatarHTML}
                            <div class="user-meta-details">
                                <span class="cloud-user-name">${displayName}</span>
                                <span class="cloud-user-email">${user.email || ''}</span>
                            </div>
                        </div>
                    </div>

                    <div class="session-creator-box">
                        <h3>Create Monitoring Session</h3>
                        <p class="creator-box-desc">Start a tracking session to segment and index network records inside Firestore.</p>
                        <div class="session-form-controls">
                            <input type="text" class="cloud-session-input" id="cloud-session-name" 
                                   placeholder="Session name (e.g. Home Wi-Fi, Office Desk)" maxlength="50">
                            <button class="cloud-start-btn" id="cloud-start-btn">
                                <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>
                                <span>Start Session</span>
                            </button>
                        </div>
                    </div>
                </div>

                <div class="cloud-previous-sessions-section">
                    <h3>Recent Cloud Sessions</h3>
                    <div class="cloud-previous-sessions-list" id="cloud-previous-sessions-list">
                        <div class="cloud-sessions-loading">
                            <svg class="spinner" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <circle cx="12" cy="12" r="10" stroke-dasharray="32" stroke-dashoffset="8"/>
                            </svg>
                            <span>Loading past sessions...</span>
                        </div>
                    </div>
                </div>
            </div>
        `;

        document.getElementById('cloud-start-btn').addEventListener('click', startSession);

        // Enter key starts session
        document.getElementById('cloud-session-name').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') startSession();
        });

        // Load past sessions from Firestore
        loadPreviousSessionsList();
    }

    function showRecordingState(session) {
        updateStatusBadge('recording');
        const content = document.getElementById('cloud-content');
        if (!content) return;

        const elapsed = session.started_at ? getElapsed(new Date(session.started_at)) : '0:00';

        content.innerHTML = `
            <div class="cloud-auth-grid">
                <div class="cloud-recording-panel">
                    <div class="recording-card-header">
                        <div class="recording-label-badge">
                            <span class="cloud-recording-dot"></span>
                            <span>LIVE RECORDING</span>
                        </div>
                        <div class="cloud-session-elapsed" id="cloud-elapsed">${elapsed}</div>
                    </div>

                    <div class="recording-dashboard">
                        <h2 class="active-session-title">${session.name || 'Untitled Session'}</h2>
                        <div class="session-stat-grid">
                            <div class="session-stat-card">
                                <span class="stat-label">Session ID</span>
                                <span class="stat-val font-mono" title="${session.id}">${session.id}</span>
                            </div>
                            <div class="session-stat-card">
                                <span class="stat-label">Network SSID</span>
                                <span class="stat-val" id="recording-ssid">${session.network || 'Local Network'}</span>
                            </div>
                            <div class="session-stat-card">
                                <span class="stat-label">Sync Status</span>
                                <span class="stat-val status-syncing">
                                    <svg class="spinner" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                        <circle cx="12" cy="12" r="10" stroke-dasharray="32" stroke-dashoffset="8"/>
                                    </svg>
                                    <span>Syncing Live</span>
                                </span>
                            </div>
                        </div>

                        <button class="cloud-stop-btn" id="cloud-stop-btn">
                            <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><rect x="4" y="4" width="16" height="16" rx="2"/></svg>
                            <span>Stop Monitoring & Save</span>
                        </button>
                    </div>
                </div>

                <div class="cloud-previous-sessions-section">
                    <h3>Recent Cloud Sessions</h3>
                    <div class="cloud-previous-sessions-list" id="cloud-previous-sessions-list">
                        <div class="cloud-sessions-loading">
                            <svg class="spinner" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <circle cx="12" cy="12" r="10" stroke-dasharray="32" stroke-dashoffset="8"/>
                            </svg>
                            <span>Loading past sessions...</span>
                        </div>
                    </div>
                </div>
            </div>
        `;

        document.getElementById('cloud-stop-btn').addEventListener('click', stopSession);

        // Start elapsed timer
        startElapsedTimer(session.started_at);

        // Load past sessions
        loadPreviousSessionsList();
    }

    async function loadPreviousSessionsList() {
        const listContainer = document.getElementById('cloud-previous-sessions-list');
        if (!listContainer) return;

        try {
            const data = await SentinelAPI.getCloudSessions();
            const sessions = data.sessions || [];

            // Exclude currently active session from history list
            const completedSessions = sessions.filter(s => !activeSession || s.id !== activeSession.id);

            if (completedSessions.length === 0) {
                listContainer.innerHTML = `
                    <div class="cloud-sessions-empty">
                        <span class="empty-icon">📂</span>
                        <span class="empty-text">No completed sessions found.</span>
                    </div>
                `;
                return;
            }

            listContainer.innerHTML = completedSessions.map(session => {
                const startedAt = new Date(session.started_at);
                const dateStr = startedAt.toLocaleDateString(undefined, {
                    month: 'short',
                    day: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit'
                });
                
                let durationStr = '—';
                if (session.ended_at) {
                    const endedAt = new Date(session.ended_at);
                    const diff = Math.floor((endedAt.getTime() - startedAt.getTime()) / 1000);
                    if (diff < 60) {
                        durationStr = `${diff}s`;
                    } else {
                        const mins = Math.floor(diff / 60);
                        const secs = diff % 60;
                        durationStr = `${mins}m ${secs}s`;
                    }
                }

                const networkLabel = session.network ? `on <strong>${session.network}</strong>` : 'Local Network';

                return `
                    <div class="cloud-session-item">
                        <div class="cloud-session-info-left">
                            <span class="session-item-name" title="${session.name}">${session.name}</span>
                            <span class="session-item-date">${dateStr} · ${networkLabel}</span>
                        </div>
                        <div class="cloud-session-info-right">
                            <span class="session-item-duration">${durationStr}</span>
                            <span class="session-item-badge">Saved</span>
                        </div>
                    </div>
                `;
            }).join('');

        } catch (err) {
            console.error('[cloud] error fetching previous sessions:', err);
            listContainer.innerHTML = `
                <div class="cloud-sessions-error">
                    <span class="error-icon">⚠️</span>
                    <span class="error-text">Failed to load previous sessions. Check Firestore Rules.</span>
                </div>
            `;
        }
    }

    let elapsedTimer = null;
    function startElapsedTimer(startedAt) {
        if (elapsedTimer) clearInterval(elapsedTimer);
        const startTime = new Date(startedAt);
        elapsedTimer = setInterval(() => {
            const el = document.getElementById('cloud-elapsed');
            if (el) {
                el.textContent = getElapsed(startTime);
            }
        }, 1000);
    }

    function getElapsed(startTime) {
        const diff = Math.floor((Date.now() - startTime.getTime()) / 1000);
        const hrs = Math.floor(diff / 3600);
        const mins = Math.floor((diff % 3600) / 60);
        const secs = diff % 60;
        if (hrs > 0) {
            return `${hrs}:${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;
        }
        return `${mins}:${String(secs).padStart(2, '0')}`;
    }

    return { init, updateSessionState };
})();

// Automatically initialize module on page load
document.addEventListener('DOMContentLoaded', () => {
    // Only auto-init if app.js is not loaded (app.js handles init on index.html)
    // cloud.html loads cloud.js directly, so it needs DOMContentLoaded auto-init!
    if (window.location.pathname.includes('cloud.html')) {
        SentinelCloud.init();
    }
});
