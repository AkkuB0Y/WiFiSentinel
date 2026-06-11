---
name: wifi-sentinel-project
description: >
  Reference skill for WiFi Sentinel — a local network health monitoring daemon
  written in Go. Use this whenever working on this project to understand the
  architecture, file layout, conventions, and branch strategy.
---

# WiFi Sentinel — Project Reference

## Overview

WiFi Sentinel is a lightweight Go daemon that continuously monitors local network health
by measuring latency, packet loss, and WiFi signal strength. Data is stored in SQLite and
served via an embedded web dashboard.

## Project Directory

```
/Users/akshaysatish/Desktop/Desktop - AKARSH's MacBook Air/CodingStuff/wifimonitor
```

> **NOTE**: The path contains a Unicode right single quotation mark (U+2019 `'`) in
> `AKARSH's`. Use shell globs (`Desktop*`) or the Python `os` module to work around this.
> The `view_file` and `write_to_file` tools cannot handle this path directly — use shell
> commands with heredocs or Python scripts for file operations.

## Architecture

```
main.go                         # Entry point — wires everything together
internal/
  config/config.go              # Env-var configuration loader
  db/
    models.go                   # Data model structs (NetworkSample, SpeedTestSample, WebhookConfig, AlertHistoryEntry)
    db.go                       # SQLite store — migrations, CRUD, queries
  collector/
    collector.go                # Background collection loop (ping + WiFi + alert eval)
    ping.go                     # Shared ping types + regex parsers
    wifi.go                     # Shared WifiInfo type
    speedtest.go                # Speed test runner (uses speedtest-go)
    # Platform-specific files (from feat-multi-platform branch):
    # ping_unix.go, ping_windows.go, wifi_darwin.go, wifi_linux.go, wifi_windows.go, platform.go
  api/
    server.go                   # HTTP server setup with middleware (CORS, logging)
    routes.go                   # Route registration (/api/*)
    handlers.go                 # All endpoint handler functions
  cloud/
    firebase.go                 # Firebase Firestore REST API client
    session.go                  # Cloud session manager (start/stop/flush)
    buffer.go                   # Thread-safe sample buffers for cloud upload
  alerts/
    models.go                   # AlertCondition, AlertEvent types
    engine.go                   # AlertEngine — evaluates samples against webhook thresholds
    webhook.go                  # HTTP webhook dispatcher (Discord, Slack, Telegram, generic)
web/
  embed.go                      # Go embed for static files
  static/                       # Frontend (HTML/JS/CSS dashboard)
tools/
  wifi-helper.swift             # macOS CoreWLAN helper (compiled separately)
```

## Key Conventions

1. **Configuration**: All runtime config is via environment variables prefixed `SENTINEL_`.
   Dynamic config (webhooks) is stored in the SQLite database and managed via REST API.

2. **No external frameworks**: Uses only `net/http` with `ServeMux`. No router libraries.
   HTTP method dispatching is done inside handler functions.

3. **Database**: SQLite via `github.com/mattn/go-sqlite3` (CGO required).
   Schema migrations are idempotent (`CREATE TABLE IF NOT EXISTS`).

4. **Concurrency**: The collector runs in a background goroutine. Alert evaluation is
   dispatched asynchronously (`go alertEngine.Evaluate(sample)`).

5. **Module path**: `wifimonitor` (not a full GitHub path).

## Git Branch Strategy

| Branch | Purpose | Status |
|--------|---------|--------|
| `main` | Stable release | Base |
| `feat-speedtest` | Speed test feature | Merged ✅ |
| `feat-cloudstorage` | Firebase cloud sync | Merged ✅ |
| `feat-multi-platform` | Windows/Linux support | In progress |
| `feat-webhook` | Webhook alerting | In progress |

### Avoiding Merge Conflicts

`feat-multi-platform` and `feat-webhook` were designed to be orthogonal:

- **Multi-platform** touches: `collector/ping*.go`, `collector/wifi*.go`, `collector/platform.go`, `Makefile`
- **Webhook** touches: `alerts/*` (new), `config/config.go`, `db/*`, `api/*`, `collector/collector.go`
- **Only shared file**: `main.go` — changes are in different regions (multi-platform: L54-56, webhook: L70+)

## API Endpoints

### Core
- `GET /api/status` — latest network sample
- `GET /api/history?since=&limit=` — recent samples
- `GET /api/aggregates?since=&bucket=` — bucketed chart data
- `GET /api/config` — current daemon config

### Speed Test
- `POST /api/speedtest/run` — trigger manual speed test
- `GET /api/speedtest/status` — check if running
- `GET /api/speedtest/history` — speed test results

### Cloud / Sessions
- `POST /api/cloud/auth` — Firebase auth
- `GET /api/cloud/status` — cloud connection status
- `GET /api/cloud/sessions` — list sessions
- `POST /api/session/start` / `POST /api/session/stop`
- `POST /api/session/delete` / `GET /api/session/data?id=`
- `GET /api/session/active`

### Webhooks / Alerts
- `GET /api/webhooks` — list all webhook configs
- `POST /api/webhooks` — create webhook
- `PUT /api/webhooks?id=N` — update webhook
- `DELETE /api/webhooks?id=N` — delete webhook
- `POST /api/webhooks/test` — send test alert
- `GET /api/alerts/history?since=&limit=` — alert history

## Build & Run

```bash
# Build (macOS)
cd /Users/akshaysatish/Desktop/Desktop*/CodingStuff/wifimonitor
make

# Build wifi-helper (macOS only)
make wifi-helper

# Run
./sentinel

# Key env vars
SENTINEL_PING_TARGETS=8.8.8.8,1.1.1.1
SENTINEL_POLL_INTERVAL=5s
SENTINEL_HTTP_PORT=8080
SENTINEL_ALERTS_ENABLED=true
SENTINEL_ALERT_COOLDOWN=5m
SENTINEL_CLOUD_ENABLED=false
```

## Dependencies

- `github.com/mattn/go-sqlite3` — SQLite driver (CGO)
- `github.com/showwin/speedtest-go` — Speed test client
- `github.com/chelnak/ysmrr` — Terminal spinners
- `github.com/fatih/color` — Terminal colors
