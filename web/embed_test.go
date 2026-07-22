package web

import (
	"io/fs"
	"testing"
)

// The committed placeholder (.gitkeep) must not read as a real SPA build —
// otherwise a source-checkout binary would serve an empty shell instead of
// falling back to the disk dist.
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

func TestSelectFS(t *testing.T) {
	// An explicitly configured directory always wins, embed or not.
	if _, label := SelectFS("/some/dir", true); label != "/some/dir" {
		t.Fatalf("explicit dir: label = %q, want /some/dir", label)
	}
	// Without an explicit dir the outcome depends on whether a build is
	// embedded (dev machines may have one); label and DistFS must agree.
	fsys, label := SelectFS("web/dist", false)
	if fsys == nil {
		t.Fatal("SelectFS returned a nil fs")
	}
	if _, ok := DistFS(); ok && label != "embedded" {
		t.Fatalf("embedded build present but label = %q", label)
	} else if !ok && label != "web/dist" {
		t.Fatalf("no embedded build but label = %q, want web/dist", label)
	}
}
