// Package version exposes build metadata baked in at link time.
package version

var (
	// Version is the semver build tag, set via -ldflags.
	Version = "0.0.0-dev"
	// Commit is the short git SHA, set via -ldflags.
	Commit = "unknown"
	// Date is the build timestamp (RFC3339, UTC), set via -ldflags.
	Date = "unknown"
)

// Info returns build metadata.
func Info() (string, string, string) {
	return Version, Commit, Date
}
