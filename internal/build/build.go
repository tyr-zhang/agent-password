// Package build holds values injected at link time via -ldflags.
package build

var (
	Version = "1.0.0"
	Commit  = "unknown"
	Date    = "unknown"
)
