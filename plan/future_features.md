# Future Features Plan

This document outlines the roadmap for future features and improvements to WiFi Sentinel, organized by priority and impact.

## 1. Huge Features to Make it Stand Out 🚀
These are the "killer features" that would elevate this from a personal utility to a widely adopted tool:

*   ~~**Multi-Platform WiFi Support:**~~ **Done** — macOS (`wifi-helper`/CoreWLAN), Windows (`netsh`), Linux (`nmcli` / `iwconfig`).
*   **Automated Speed Tests:** Add a feature to run automated download/upload speed tests on a set schedule (e.g., every 6 hours) and plot bandwidth degradation over time.
*   **Alerting & Notification Engine:** Integrate webhook support so users can easily plug in Discord, Slack, Telegram, or even simple email alerts when the network degrades or drops completely.
*   **Multi-Node / Distributed Monitoring:** Allow users to run lightweight "agent" binaries on their Raspberry Pi, desktop, and laptop, all reporting back to a central Sentinel instance to map out WiFi dead zones.
*   **Local Network Discovery (ARP Scanning):** Add a feature that occasionally scans the local subnet to track how many devices are connected to the network, as spikes in connected devices often correlate with drops in performance.

## 2. Quality of Life Changes 🛠️
These make the tool feel polished, premium, and easy to deploy for less technical users:

*   **Docker Containerization:** Publish a lightweight Alpine/Scratch-based Docker image for homelab users who prefer deploying via Docker Compose instead of standalone binaries.
*   **In-UI Configuration Dashboard:** Build a settings page in the frontend where users can add/remove ping targets, adjust intervals, and change retention policies dynamically, rather than relying entirely on environment variables.
*   **WebSockets / Server-Sent Events (SSE):** Upgrade the frontend to use WebSockets or SSE for real-time dashboard updates. This makes the dashboard feel infinitely more responsive while reducing server load from HTTP polling.
*   **Data Export/Import:** Allow power users to export their historical data as a CSV or JSON file for custom analysis.
*   **Theme Toggle (Light/Dark):** Add a simple CSS variable toggle for a light mode theme to complement the existing dark UI.

## 3. Internal Code Restructures for Efficiency 🏗️
These architectural changes will make the codebase highly maintainable, testable, and easier for open-source contributors to work on:

*   **Interface-Driven Collector Pattern:** Abstract data collection logic into a `MetricCollector` interface. This allows for easily plugging in different implementations (`IcmpPinger`, `HttpPinger`, `MacWifiCollector`) and simplifies unit testing via mocking.
*   **Dependency Injection (DI):** Refactor API handlers so they don't rely on global variables for the Database or Cloud buffers. Pass an `AppContainer` or `Server` struct into handlers that holds references to the DB, Config, and Cloud Client.
*   **Graceful Shutdowns:** Ensure the Go daemon intercepts the interrupt signal (`os.Signal`) to cleanly stop accepting new API requests, flush the remaining SQLite WAL, and flush any pending cloud buffers before fully exiting.
*   **Query Optimization & Caching:** For aggregation endpoints, introduce a background worker that pre-calculates hourly/daily averages and saves them into a separate `aggregations` table, or use an in-memory cache for the most requested time ranges to prevent heavy on-the-fly computation.
