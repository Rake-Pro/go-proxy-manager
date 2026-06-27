// Package version exposes build metadata, populated at link time via -ldflags.
// Reporting the real build identity (not a hardcoded constant) is an explicit
// project goal: the NPM fork's false "vX available" came from a stale package.json.
package version

import (
	"fmt"
	"runtime"
)

// These are overridden at build time:
//
//	-X github.com/Rake-Pro/go-proxy-manager/internal/version.Version=...
//	-X github.com/Rake-Pro/go-proxy-manager/internal/version.Commit=...
//	-X github.com/Rake-Pro/go-proxy-manager/internal/version.Date=...
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info is the structured build identity surfaced by the /version endpoint.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	Go        string `json:"go"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Get returns the current build identity.
func Get() Info {
	return Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}
}

// String renders a single-line human-readable build identity.
func String() string {
	return fmt.Sprintf("go-proxy-manager %s (commit %s, built %s, %s %s/%s)",
		Version, Commit, Date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
