// Package web embeds the built SPA (dist/, the pnpm build output) so a
// release binary is fully self-contained. A source checkout without a
// frontend build embeds only the committed placeholder, which DistFS
// reports as "no build".
package web

import (
	"embed"
	"io/fs"
	"os"
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

// SelectFS picks the filesystem the SPA is served from: an explicitly
// configured directory always wins (dev override, separately-hosted SPA);
// otherwise the embedded build when present; otherwise the disk default —
// a source checkout with a built SPA but a placeholder-only binary. The
// returned label names the source for the boot log.
func SelectFS(dir string, explicit bool) (fs.FS, string) {
	if !explicit {
		if sub, ok := DistFS(); ok {
			return sub, "embedded"
		}
	}
	return os.DirFS(dir), dir
}
