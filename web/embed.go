// Package web provides the embedded static frontend for WiFi Sentinel.
// The static files are compiled into the binary via Go's embed package.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// GetFileSystem returns an http.FileSystem rooted at the "static" subdirectory.
// This strips the "static/" prefix so files are served from the root path.
func GetFileSystem() (http.FileSystem, error) {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}
