# WiFi Sentinel 📡

**A lightweight local network health monitoring daemon with a real-time web dashboard.**

WiFi Sentinel continuously monitors your network connection by measuring latency, packet loss, and WiFi signal strength, then presents the data through a sleek dark-themed dashboard.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![SQLite](https://img.shields.io/badge/SQLite-3-003B57?style=flat&logo=sqlite)
![License](https://img.shields.io/badge/License-MIT-green?style=flat)

---

## Features

- 📊 **Real-time Dashboard** — Glassmorphic dark UI with live-updating charts
- 🏓 **Latency Monitoring** — Ping multiple targets with configurable intervals (macOS, Linux, Windows)
- 📶 **WiFi Signal Tracking** — RSSI, noise level, SSID, and channel (platform-specific backends)
- 📉 **Packet Loss Detection** — Color-coded alerts for network degradation
- 💾 **Local SQLite Storage** — WAL-mode for concurrent reads/writes
- 🗑️ **Auto-Pruning** — Configurable data retention (default: 7 days)
- 🎯 **Single Binary** — Frontend embedded via Go's `embed` package
- ⚡ **Minimal Footprint** — < 1% CPU, < 20MB RAM

## Platform Support

| OS | Ping / Latency | WiFi Backend | RSSI | Noise |
|----|----------------|--------------|------|-------|
| **macOS** | system `ping` | CoreWLAN Swift helper (`wifi-helper`) | true dBm | yes |
| **Linux** | system `ping` | `nmcli` (preferred) or `iwconfig` | true dBm (`iwconfig`) or estimated (`nmcli`) | `iwconfig` only |
| **Windows** | system `ping` | `netsh wlan show interfaces` | estimated from signal % | not available |

Supported architectures: build and test on `amd64` and `arm64` for the above OSes. Other Unix-like systems are not currently supported.

The dashboard labels estimated RSSI as `~value` / `dBm (est.)` and shows `N/A` when no WiFi backend is detected.

## Architecture

```
┌─────────────────────────────────────────────┐
│            Go Binary (Single Executable)     │
│                                              │
│  ┌──────────┐    ┌──────┐    ┌───────────┐  │
│  │ Collector │───▶│SQLite│◀───│ HTTP API  │  │
│  │  Daemon   │    │  DB  │    │  Server   │  │
│  └──────────┘    └──────┘    └───────────┘  │
│       │                           │          │
│  ┌────┴────────────┐      ┌──────┴──────┐   │
│  │ ping + WiFi     │      │  Embedded   │   │
│  │ (OS-specific)   │      │  Dashboard  │   │
│  └─────────────────┘      └─────────────┘   │
└─────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- **Go 1.21+** — [Install Go](https://go.dev/dl/)
- **C Compiler** — Required by `go-sqlite3` for full SQLite support (`clang` on macOS, `gcc` on Linux, MinGW on Windows)
- **Platform tools** (for WiFi metrics):
  - **macOS**: Xcode CLI tools (for `swiftc`) to build `wifi-helper`
  - **Linux**: NetworkManager (`nmcli`) or `wireless-tools` (`iwconfig`)
  - **Windows**: `netsh` (built in)

Ping/latency monitoring works on all supported platforms without extra WiFi tooling.

### Build & Run (macOS)

```bash
git clone <your-repo-url> wifimonitor
cd wifimonitor
go mod tidy

# Builds wifi-helper + sentinel
make

./sentinel
```

### Build & Run (Linux / Windows)

Build on the target machine when possible — cross-compiling SQLite (CGO) is fragile:

```bash
go mod tidy
CGO_ENABLED=1 go build -o sentinel .
./sentinel          # Linux
sentinel.exe        # Windows
```

Makefile cross-compile helpers (`make build-linux`, `make build-windows`) are available but may require a cross-toolchain.

Then open **http://localhost:8080** in your browser.

### Run Directly (Development)

```bash
CGO_ENABLED=1 go run .
```

## Configuration

All settings are configurable via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SENTINEL_PING_TARGETS` | `8.8.8.8,1.1.1.1` | Comma-separated ping target IPs |
| `SENTINEL_POLL_INTERVAL` | `5s` | How often to collect data |
| `SENTINEL_DB_PATH` | `./sentinel.db` | Path to SQLite database file |
| `SENTINEL_HTTP_PORT` | `8080` | Dashboard HTTP port |
| `SENTINEL_RETENTION_DAYS` | `7` | Days to keep data before auto-pruning |

**Example:**

```bash
SENTINEL_PING_TARGETS="8.8.8.8,1.1.1.1,208.67.222.222" \
SENTINEL_POLL_INTERVAL="10s" \
SENTINEL_HTTP_PORT="9090" \
./sentinel
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/status` | GET | Latest network sample |
| `/api/history?since=<ISO>&limit=<N>` | GET | Recent samples (default: last 1h, limit 500) |
| `/api/aggregates?since=<ISO>&bucket=<min>` | GET | Bucketed averages for charts |
| `/api/config` | GET | Daemon configuration + platform capabilities |

`/api/config` includes a `platform` object with `wifi_supported`, `rssi_estimated`, `noise_supported`, and backend names.

## Project Structure

```
wifimonitor/
├── main.go                     # Entry point
├── Makefile                    # Build targets (macOS helper + cross-compile)
├── tools/wifi-helper.swift     # macOS CoreWLAN helper source
├── internal/
│   ├── config/config.go        # Environment-based configuration
│   ├── db/
│   │   ├── db.go               # SQLite store (CRUD, migrations, pruning)
│   │   └── models.go           # Data structs
│   ├── collector/
│   │   ├── collector.go        # Collection orchestrator
│   │   ├── ping_{unix,windows}.go
│   │   └── wifi_{darwin,linux,windows}.go
│   └── api/
│       ├── server.go           # HTTP server + middleware
│       ├── handlers.go         # API endpoint handlers
│       └── routes.go           # Route registration
└── web/
    ├── embed.go                # Static file embedding
    └── static/
        ├── index.html          # Dashboard SPA
        ├── css/style.css       # Dark glassmorphism theme
        └── js/
            ├── api.js          # API client
            ├── charts.js       # Chart.js rendering
            └── app.js          # Main application logic
```

## Dashboard

The dashboard features:
- **Status Cards** — Live latency, packet loss, signal strength, and SSID with color-coded indicators
- **Time-Series Charts** — Latency, signal strength, and packet loss over 1h/6h/24h/7d
- **Events Table** — Recent samples with color-coded status

### Signal Quality Guide

Applies to true dBm readings (macOS, Linux `iwconfig`). Estimated values on Windows/nmcli are approximate.

| RSSI (dBm) | Quality | Indicator |
|-------------|---------|-----------|
| ≥ -50 | Excellent | 🟢 Green |
| -50 to -70 | Good | 🟡 Yellow |
| < -70 | Poor | 🔴 Red |

## License

MIT
