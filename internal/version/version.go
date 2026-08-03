// Package version carries the build-stamped binary version. Release builds
// overwrite the default at link time:
//
//	go build -ldflags "-X github.com/econumo/econumo/internal/version.Version=v1.2.3"
package version

// Version is "dev" unless stamped by -ldflags at build time.
var Version = "dev"

// MinAppVersion is the oldest mobile-app version this backend accepts; it is
// served to clients as MIN_APP_VERSION in econumo-config.js, and an older app
// hard-blocks itself against this server. Bump it only when a release breaks
// compatibility with older app builds.
const MinAppVersion = "v1.0.0"
