// Package build holds version metadata injected at link time via -ldflags.
package build

var (
	// Version is the semantic version (git tag) of the build.
	Version = "dev"
	// Commit is the short git SHA the build was produced from.
	Commit = "none"
	// Date is the RFC3339 build timestamp.
	Date = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return Version + " (commit " + Commit + ", built " + Date + ")"
}
