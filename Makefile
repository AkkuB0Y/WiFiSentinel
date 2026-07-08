# WiFi Sentinel — Build targets
#
# Usage:
#   make              Build macOS wifi-helper + sentinel binary
#   make sentinel     Build sentinel only (Go binary for current OS)
#   make wifi-helper  Build the macOS CoreWLAN helper
#   make build-linux  Cross-compile for Linux (amd64)
#   make build-windows Cross-compile for Windows (amd64)
#   make build-all    Build for all platforms
#   make test         Run all unit tests
#   make clean        Remove compiled binaries

.PHONY: all clean sentinel build-linux build-windows build-all test

# Default: build everything for macOS (includes Swift wifi-helper)
all: wifi-helper sentinel

wifi-helper: tools/wifi-helper.swift
	swiftc -framework CoreWLAN -framework Foundation -O -o wifi-helper tools/wifi-helper.swift

sentinel: wifi-helper
	go build -o sentinel .

# Cross-compilation targets (no wifi-helper needed — each OS has its own backend)
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-linux-musl-gcc go build -o sentinel-linux . 2>/dev/null || \
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags "sqlite_omit_load_extension" -o sentinel-linux . 2>/dev/null || \
	echo "NOTE: Linux cross-compile requires CGO for SQLite. Use 'go build' on the target Linux system."

build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -o sentinel-windows.exe . 2>/dev/null || \
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags "sqlite_omit_load_extension" -o sentinel-windows.exe . 2>/dev/null || \
	echo "NOTE: Windows cross-compile requires CGO for SQLite. Use 'go build' on the target Windows system."

build-all: all build-linux build-windows

test:
	go test -v ./internal/collector/...

clean:
	rm -f sentinel sentinel-linux sentinel-windows.exe wifi-helper
