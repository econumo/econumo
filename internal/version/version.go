// Package version carries the build-stamped binary version. Release builds
// overwrite the defaults at link time:
//
//	go build -ldflags "-X github.com/econumo/econumo/internal/version.Version=v1.2.3 -X github.com/econumo/econumo/internal/version.Commit=<sha>"
package version

import "runtime/debug"

// Version is "dev" unless stamped by -ldflags at build time.
var Version = "dev"

// Commit is the source revision, stamped by -ldflags. Empty falls back to the
// revision the go toolchain records when it builds inside a git checkout.
var Commit = ""

// Ref identifies the exact build, "" when unknown. Every dev build reports the
// same Version, so the SPA keys its caches on Version plus Ref: a dev deploy
// must not restore what an older build cached.
func Ref() string {
	if Commit != "" {
		return Commit
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}
