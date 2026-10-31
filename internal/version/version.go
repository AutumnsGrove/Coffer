// Package version holds the build stamp. The Dockerfile sets it with
//
//	-ldflags "-X github.com/AutumnsGrove/Coffer/internal/version.stamp=r142.7024ab0"
//
// A stamp is r<commit count>.<short sha>. An unstamped build reports "dev". The value reaches
// /healthz, logs and the update watcher, so it is validated rather than trusted.
package version

import "regexp"

// stamp is set at link time. It is a variable (not a constant) so -X can replace it.
var stamp = ""

const maxLen = 64

var pattern = regexp.MustCompile(`^(?:dev|r[0-9]{1,6}\.[0-9a-f]{7,40})$`)

// Valid reports whether s is exactly "dev" or "r<count>.<sha>" in lowercase ASCII.
func Valid(s string) bool {
	return len(s) <= maxLen && pattern.MatchString(s)
}

// String returns the build stamp, or "dev" if it is missing or not valid. An invalid stamp is
// never echoed back.
func String() string {
	if Valid(stamp) {
		return stamp
	}
	return "dev"
}
