package web

import (
	"io/fs"
	"testing"
)

// The committed placeholder (.gitkeep) must not read as a real SPA build —
// otherwise a source-checkout binary would report an embedded build and serve
// an empty shell.
func TestDistFS_PlaceholderIsNotABuild(t *testing.T) {
	sub, ok := DistFS()
	if sub == nil {
		t.Fatal("DistFS returned a nil fs")
	}
	if _, err := fs.Stat(sub, "index.html"); err == nil {
		// A dev machine with a built SPA embeds the real thing; the placeholder
		// assertion only applies to a frontend-free checkout (CI smoke).
		if !ok {
			t.Fatal("index.html embedded but DistFS reported no build")
		}
		t.Skip("real SPA build present in web/dist")
	}
	if ok {
		t.Fatal("DistFS reported a real build for a placeholder-only embed")
	}
}
