// Package version exposes build metadata injected at link time.
package version

import "fmt"

// Build metadata. Override with:
//
//	go build -ldflags "-X github.com/observaki/observaki/internal/version.Version=v0.1.0 \
//	  -X github.com/observaki/observaki/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/observaki/observaki/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a single-line summary of the build.
func String() string {
	return fmt.Sprintf("observaki %s (commit %s, built %s)", Version, Commit, Date)
}
