// Package benchversion exposes build metadata for the benchmark binaries.
// Override at link time with -ldflags "-X ...".
package benchversion

import "fmt"

// Build metadata injected at link time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a single-line summary of the benchmark build.
func String(name string) string {
	return fmt.Sprintf("%s %s (commit %s, built %s)", name, Version, Commit, Date)
}
