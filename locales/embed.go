// Package locales embeds the per-language translation catalogues shared by the
// SPA (via Vite JSON imports) and the Go binary. The embed directive must live
// in this directory: go:embed cannot reference parent paths.
package locales

import "embed"

//go:embed *.json
var FS embed.FS
