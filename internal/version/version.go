// Package version carries the build-stamped binary version. Release builds
// overwrite the default at link time:
//
//	go build -ldflags "-X github.com/econumo/econumo/internal/version.Version=v1.2.3"
package version

// Version is "dev" unless stamped by -ldflags at build time.
var Version = "dev"
