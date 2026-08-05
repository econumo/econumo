// Package compat holds the version-compatibility floors of the mobile-app
// handshake in one file shared by both stacks: the Go backend embeds it, the
// SPA imports the same versions.json via a Vite JSON import, so the two can
// never drift. The embed directive must live in this directory: go:embed
// cannot reference parent paths.
package compat

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
)

//go:embed versions.json
var raw []byte

var versions = func() (v struct {
	MinServerVersion string `json:"minServerVersion"`
	MinAppVersion    string `json:"minAppVersion"`
}) {
	if err := json.Unmarshal(raw, &v); err != nil {
		panic(fmt.Sprintf("compat: malformed versions.json: %v", err))
	}
	semver := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	if !semver.MatchString(v.MinServerVersion) || !semver.MatchString(v.MinAppVersion) {
		panic(fmt.Sprintf("compat: versions must be strict vX.Y.Z, got %+v", v))
	}
	return v
}()

// MinAppVersion is the oldest mobile-app build this backend accepts; served
// to clients as MIN_APP_VERSION in econumo-config.js, and an older app
// hard-blocks itself against this server. Bump it only when a release breaks
// compatibility with older app builds.
var MinAppVersion = versions.MinAppVersion

// MinServerVersion is the oldest server the bundled app can talk to. It is
// consumed by the SPA straight from versions.json (MIN_SERVER_VERSION in
// web/src/lib/appConfig.ts); it is exported here so the Go suite guards the
// value's format alongside MinAppVersion.
var MinServerVersion = versions.MinServerVersion
