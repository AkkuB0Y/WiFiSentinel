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
                if (isCloudConsolePage()) {
                    // On the dedicated cloud page, show a "not enabled" message
                    showCloudDisabledState();
                } else {
                    hideCloudBar();
                }
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
            SentinelAPI.clearAuthToken();
            if (isCloudConsolePage()) {
                showUnauthenticatedState();
            } else {
                renderHeaderAuth(user);
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

    function showCloudDisabledState() {
        updateStatusBadge('unauth');
        const content = document.getElementById('cloud-content');
        if (!content) return;

        content.innerHTML = `
            <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 16px; padding: 60px 20px; text-align: center;">
                <svg width="56" height="56" viewBox="0 0 24 24" fill="none" stroke="#64748b" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="opacity: 0.5;">
                    <path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z"/>
                    <line x1="4" y1="4" x2="20" y2="20" stroke="#ef4444" stroke-width="2"/>
                </svg>
                <h2 style="margin: 0; font-size: 1.15rem; font-weight: 600; color: #f1f5f9;">Cloud Mode Not Enabled</h2>
                <p style="margin: 0; max-width: 420px; font-size: 0.82rem; color: #94a3b8; line-height: 1.6;">
                    The WiFi Sentinel daemon is running in <strong style="color:#f1f5f9;">local-only mode</strong>. To enable cloud tracking, set the following environment variables and restart:
                </p>
                <div style="background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.06); border-radius: 8px; padding: 16px 20px; font-family: 'JetBrains Mono', monospace; font-size: 0.72rem; color: #94a3b8; text-align: left; line-height: 1.8; max-width: 440px; width: 100%;">
                    <span style="color:#64748b;"># In your start.sh or shell:</span><br>
                    <span style="color:#22c55e;">export</span> SENTINEL_CLOUD_ENABLED=<span style="color:#38bdf8;">true</span><br>
                    <span style="color:#22c55e;">export</span> SENTINEL_FIREBASE_PROJECT=<span style="color:#38bdf8;">"your-project-id"</span><br>
                    <span style="color:#22c55e;">export</span> SENTINEL_FIREBASE_API_KEY=<span style="color:#38bdf8;">"your-api-key"</span>
                </div>
                <a href="index.html" style="margin-top: 8px; display: inline-flex; align-items: center; gap: 6px; padding: 8px 16px; border: 1px solid rgba(56, 189, 248, 0.2); background: rgba(56, 189, 248, 0.06); color: #38bdf8; border-radius: 8px; font-size: 0.75rem; font-weight: 600; text-decoration: none; transition: all 0.2s ease;">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg>
                    Back to Dashboard
                </a>
            </div>
        `;
    }

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
                            <span class="benefit-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z"/></svg></span>
                            <div class="benefit-desc">
                                <strong>Real-Time Syncing</strong>
                                <span>Streams latency and speed tests directly to your cloud project.</span>
                            </div>
                        </li>
                        <li>
                            <span class="benefit-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg></span>
                            <div class="benefit-desc">
                                <strong>Multi-Device Dashboard</strong>
                                <span>Track network tests from multiple hosts simultaneously in one database.</span>
                            </div>
                        </li>
                        <li>
                            <span class="benefit-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/></svg></span>
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
            <div class="cloud-auth-layout">
                <!-- Left: Session Creation + Tracking Info -->
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

                    <!-- What Gets Tracked -->
                    <div class="tracking-explainer">
                        <h3 class="tracking-explainer-title">
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
                            What Gets Tracked
                        </h3>
                        <div class="tracking-metrics-grid">
                            <div class="tracking-metric-card">
                                <div class="tracking-metric-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg></div>
                                <div class="tracking-metric-info">
                                    <span class="tracking-metric-name">Network Latency</span>
                                    <span class="tracking-metric-desc">Ping RTT to targets (ms)</span>
                                </div>
                            </div>
                            <div class="tracking-metric-card">
                                <div class="tracking-metric-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2.69l5.66 5.66a8 8 0 1 1-11.31 0z"/></svg></div>
                                <div class="tracking-metric-info">
                                    <span class="tracking-metric-name">Packet Loss</span>
                                    <span class="tracking-metric-desc">% failed pings per interval</span>
                                </div>
                            </div>
                            <div class="tracking-metric-card">
                                <div class="tracking-metric-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12.55a11 11 0 0 1 14.08 0"/><path d="M8.53 16.11a6 6 0 0 1 6.95 0"/><line x1="12" y1="20" x2="12.01" y2="20"/></svg></div>
                                <div class="tracking-metric-info">
                                    <span class="tracking-metric-name">WiFi Signal (RSSI)</span>
                                    <span class="tracking-metric-desc">Signal strength in dBm</span>
                                </div>
                            </div>
                            <div class="tracking-metric-card">
                                <div class="tracking-metric-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="2"/><path d="M16.24 7.76a6 6 0 0 1 0 8.49"/><path d="M7.76 16.24a6 6 0 0 1 0-8.49"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14"/><path d="M4.93 19.07a10 10 0 0 1 0-14.14"/></svg></div>
                                <div class="tracking-metric-info">
                                    <span class="tracking-metric-name">WiFi Noise & Channel</span>
                                    <span class="tracking-metric-desc">Noise floor (dBm) + channel</span>
                                </div>
                            </div>
                            <div class="tracking-metric-card">
                                <div class="tracking-metric-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg></div>
                                <div class="tracking-metric-info">
                                    <span class="tracking-metric-name">Network SSID</span>
                                    <span class="tracking-metric-desc">Connected network name</span>
                                </div>
                            </div>
                            <div class="tracking-metric-card">
                                <div class="tracking-metric-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2v4"/><path d="m19.07 4.93-2.83 2.83"/><path d="M20 12h-4"/><path d="m19.07 19.07-2.83-2.83"/><path d="M12 20v-4"/><path d="m4.93 19.07 2.83-2.83"/><path d="M4 12h4"/><path d="m4.93 4.93 2.83 2.83"/><circle cx="12" cy="12" r="3"/></svg></div>
                                <div class="tracking-metric-info">
                                    <span class="tracking-metric-name">Speed Tests</span>
                                    <span class="tracking-metric-desc">DL/UL Mbps, jitter, latency</span>
                                </div>
                            </div>
                        </div>
                        <p class="tracking-explainer-note">Samples collected every <strong>5 seconds</strong> (configurable). Speed tests captured when triggered during an active session.</p>
                    </div>
                </div>

                <!-- Right: Previous Sessions -->
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
            <div class="cloud-auth-layout">
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

                        <!-- Live Metrics -->
                        <div class="recording-live-metrics" id="recording-live-metrics">
                            <h4 class="live-metrics-title">
                                <span class="live-dot"></span>
                                Live Capture Preview
                            </h4>
                            <div class="live-metrics-grid" id="live-metrics-grid">
                                <div class="live-metric-card">
                                    <span class="live-metric-label">Latency</span>
                                    <span class="live-metric-value" id="live-latency">—</span>
                                    <span class="live-metric-unit">ms</span>
                                </div>
                                <div class="live-metric-card">
                                    <span class="live-metric-label">Loss</span>
                                    <span class="live-metric-value" id="live-loss">—</span>
                                    <span class="live-metric-unit">%</span>
                                </div>
                                <div class="live-metric-card">
                                    <span class="live-metric-label">Signal</span>
                                    <span class="live-metric-value" id="live-rssi">—</span>
                                    <span class="live-metric-unit">dBm</span>
                                </div>
                                <div class="live-metric-card">
                                    <span class="live-metric-label">SSID</span>
                                    <span class="live-metric-value live-metric-text" id="live-ssid">—</span>
                                </div>
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

        // Start live metrics polling
        startLiveMetricsPolling();

        // Load past sessions
        loadPreviousSessionsList();
    }

    // --- Live Metrics Polling During Recording ---
    let liveMetricsTimer = null;

    function startLiveMetricsPolling() {
        if (liveMetricsTimer) clearInterval(liveMetricsTimer);
        updateLiveMetrics(); // immediate first fetch
        liveMetricsTimer = setInterval(updateLiveMetrics, 5000);
    }

    function stopLiveMetricsPolling() {
        if (liveMetricsTimer) {
            clearInterval(liveMetricsTimer);
            liveMetricsTimer = null;
        }
    }

    async function updateLiveMetrics() {
        try {
            const status = await SentinelAPI.fetchStatus();
            if (!status || status.status === 'waiting') return;

            const latEl = document.getElementById('live-latency');
            const lossEl = document.getElementById('live-loss');
            const rssiEl = document.getElementById('live-rssi');
            const ssidEl = document.getElementById('live-ssid');

            if (latEl) latEl.textContent = status.latency_ms != null ? status.latency_ms.toFixed(1) : '—';
            if (lossEl) lossEl.textContent = status.packet_loss != null ? status.packet_loss.toFixed(0) : '—';
            if (rssiEl) rssiEl.textContent = status.wifi_rssi != null ? status.wifi_rssi : '—';
            if (ssidEl) ssidEl.textContent = status.wifi_ssid || '—';
        } catch (err) {
            // Non-critical
        }
    }

    // --- Session List Rendering ---

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
                        <span class="empty-icon"><svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h5l2 2h11v10a2 2 0 0 1-2 2H3z"/><path d="M3 6a2 2 0 0 1 2-2h4l2 2"/></svg></span>
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

                // Escape session name for use in onclick attribute
                const safeName = (session.name || 'Untitled').replace(/'/g, "\\'").replace(/"/g, '&quot;');
                const safeTime = startedAt.toLocaleString().replace(/'/g, "\\'").replace(/"/g, '&quot;');

                return `
                    <div class="cloud-session-item" onclick="SentinelCloud.openSessionViewer('${session.id}', '${safeName}', '${safeTime}')">
                        <div class="cloud-session-info-left">
                            <span class="session-item-name" title="${session.name}">${session.name}</span>
                            <span class="session-item-date">${dateStr} · ${networkLabel}</span>
                        </div>
                        <div class="cloud-session-info-right">
                            <span class="session-item-duration">${durationStr}</span>
                            <span class="session-item-badge session-view-badge" title="View Session">
                                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-7 11-7 11 7 11 7-4 7-11 7S1 12 1 12z"/><circle cx="12" cy="12" r="3"/></svg>
                                View
                            </span>
                            <span class="session-item-badge session-delete-badge" title="Delete Session" onclick="SentinelCloud.handleDeleteSession(event, '${session.id}')">
                                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/></svg>
                                Delete
                            </span>
                        </div>
                    </div>
                `;
            }).join('');

        } catch (err) {
            console.error('[cloud] error fetching previous sessions:', err);
            listContainer.innerHTML = `
                <div class="cloud-sessions-error">
                    <span class="error-icon"><svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg></span>
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

    async function handleDeleteSession(event, sessionId) {
        event.stopPropagation();
        if (!confirm("Are you sure you want to delete this session? This action cannot be undone.")) {
            return;
        }

        try {
            await SentinelAPI.deleteSession(sessionId);
            loadPreviousSessionsList();
        } catch (err) {
            console.error('[cloud] error deleting session:', err);
            alert('Failed to delete session. Please try again.');
        }
    }

    // --- Session Viewer Modal ---

    async function openSessionViewer(sessionId, sessionName, sessionTime) {
        const modal = document.getElementById('session-viewer-modal');
        const nameEl = document.getElementById('viewer-session-name');
        const timeEl = document.getElementById('viewer-session-time');
        const loadingEl = document.getElementById('viewer-loading');
        const chartsEl = document.getElementById('viewer-charts');
        const summaryEl = document.getElementById('viewer-summary');

        nameEl.textContent = sessionName;
        timeEl.textContent = sessionTime;
        
        loadingEl.style.display = 'block';
        chartsEl.style.display = 'none';
        if (summaryEl) summaryEl.style.display = 'none';
        modal.style.display = 'flex';

        try {
            const data = await SentinelAPI.getSessionData(sessionId);
            
            if (data && data.buckets && data.buckets.length > 0) {
                // Show summary stats
                if (summaryEl && data.summary) {
                    renderSessionSummary(summaryEl, data.summary);
                    summaryEl.style.display = 'grid';
                }

                // Initialize charts if not already done
                SentinelCharts.init();
                SentinelCharts.updateCharts(data.buckets);

                // Update speed test chart if data exists
                if (data.speed_tests && data.speed_tests.length > 0) {
                    SentinelCharts.updateSpeedTestChart(data.speed_tests);
                    const stContainer = document.getElementById('viewer-speedtest-section');
                    if (stContainer) stContainer.style.display = 'block';
                }

                loadingEl.style.display = 'none';
                chartsEl.style.display = 'block';
            } else {
                loadingEl.innerHTML = `
                    <div class="viewer-empty-state">
                        <span style="font-size: 2rem;"><svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5 5h14a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2z"/></svg></span>
                        <h3>No Data Recorded</h3>
                        <p>This session has no network samples. The session may have been too short to capture data, or the data may have failed to sync to Firestore.</p>
                    </div>
                `;
            }
        } catch (err) {
            console.error('[cloud] error loading session data:', err);
            loadingEl.innerHTML = `
                <div class="viewer-empty-state error">
                    <span style="font-size: 2rem;"><svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg></span>
                    <h3>Failed to Load Data</h3>
                    <p>${err.message || 'An error occurred while fetching session data from Firestore. Check your authentication and try again.'}</p>
                </div>
            `;
        }
    }

    function renderSessionSummary(container, summary) {
        const avgLat = summary.avg_latency_ms != null ? summary.avg_latency_ms.toFixed(1) : '—';
        const maxLat = summary.max_latency_ms != null ? summary.max_latency_ms.toFixed(1) : '—';
        const avgSig = summary.avg_wifi_rssi != null ? Math.round(summary.avg_wifi_rssi) : '—';
        const avgLoss = summary.avg_packet_loss != null ? summary.avg_packet_loss.toFixed(1) : '—';
        const samples = summary.sample_count || 0;
        const speedTests = summary.speed_test_count || 0;

        container.innerHTML = `
            <div class="summary-stat-card">
                <span class="summary-stat-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg></span>
                <div class="summary-stat-data">
                    <span class="summary-stat-value">${avgLat}<span class="summary-stat-unit">ms</span></span>
                    <span class="summary-stat-label">Avg Latency</span>
                </div>
            </div>
            <div class="summary-stat-card">
                <span class="summary-stat-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 6 13.5 15.5 8.5 10.5 1 18"/><polyline points="17 6 23 6 23 12"/></svg></span>
                <div class="summary-stat-data">
                    <span class="summary-stat-value">${maxLat}<span class="summary-stat-unit">ms</span></span>
                    <span class="summary-stat-label">Max Latency</span>
                </div>
            </div>
            <div class="summary-stat-card">
                <span class="summary-stat-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12.55a11 11 0 0 1 14.08 0"/><path d="M8.53 16.11a6 6 0 0 1 6.95 0"/><line x1="12" y1="20" x2="12.01" y2="20"/></svg></span>
                <div class="summary-stat-data">
                    <span class="summary-stat-value">${avgSig}<span class="summary-stat-unit">dBm</span></span>
                    <span class="summary-stat-label">Avg Signal</span>
                </div>
            </div>
            <div class="summary-stat-card">
                <span class="summary-stat-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2.69l5.66 5.66a8 8 0 1 1-11.31 0z"/></svg></span>
                <div class="summary-stat-data">
                    <span class="summary-stat-value">${avgLoss}<span class="summary-stat-unit">%</span></span>
                    <span class="summary-stat-label">Avg Loss</span>
                </div>
            </div>
            <div class="summary-stat-card">
                <span class="summary-stat-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg></span>
                <div class="summary-stat-data">
                    <span class="summary-stat-value">${samples}</span>
                    <span class="summary-stat-label">Samples</span>
                </div>
            </div>
            <div class="summary-stat-card">
                <span class="summary-stat-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2v4"/><path d="m19.07 4.93-2.83 2.83"/><path d="M20 12h-4"/><path d="m19.07 19.07-2.83-2.83"/><path d="M12 20v-4"/><path d="m4.93 19.07 2.83-2.83"/><path d="M4 12h4"/><path d="m4.93 4.93 2.83 2.83"/><circle cx="12" cy="12" r="3"/></svg></span>
                <div class="summary-stat-data">
                    <span class="summary-stat-value">${speedTests}</span>
                    <span class="summary-stat-label">Speed Tests</span>
                </div>
            </div>
        `;
    }

    function closeSessionViewer() {
        const modal = document.getElementById('session-viewer-modal');
        modal.style.display = 'none';
        stopLiveMetricsPolling();
        SentinelCharts.destroy();
    }

    return { init, updateSessionState, handleDeleteSession, openSessionViewer, closeSessionViewer };
})();

// Automatically initialize module on page load
document.addEventListener('DOMContentLoaded', () => {
    // Only auto-init if app.js is not loaded (app.js handles init on index.html)
    // cloud.html loads cloud.js directly, so it needs DOMContentLoaded auto-init!
    if (window.location.pathname.includes('cloud.html')) {
        SentinelCloud.init();
    }
});
