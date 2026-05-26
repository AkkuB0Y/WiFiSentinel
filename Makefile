# WiFi Sentinel — Build targets
#
# Usage:
#   make          Build both wifi-helper and sentinel
#   make clean    Remove compiled binaries

.PHONY: all clean sentinel

all: wifi-helper sentinel

wifi-helper: tools/wifi-helper.swift
	swiftc -framework CoreWLAN -framework Foundation -O -o wifi-helper tools/wifi-helper.swift

sentinel: wifi-helper
	go build -o sentinel .

clean:
	rm -f sentinel wifi-helper
