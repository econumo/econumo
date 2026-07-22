// Package web embeds the built SPA (dist/, the pnpm build output) so a
// release binary is fully self-contained. A source checkout without a
// frontend build embeds only the committed placeholder, which DistFS
// reports as "no build".
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// DistFS returns the embedded SPA rooted at dist/ and whether a real build
// is present (a placeholder-only embed has no index.html).
func DistFS() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	_, err = fs.Stat(sub, "index.html")
	return sub, err == nil
}
