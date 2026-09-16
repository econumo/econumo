package sqlc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A multibyte character anywhere in a query file, comments included, can make
// sqlc emit a truncated SQL constant (observed twice on this repo: the
// statement ended mid-WHERE and failed at runtime with "incomplete input"),
// so the rule in CLAUDE.md is enforced here rather than remembered.
func TestQueryFilesAreASCII(t *testing.T) {
	err := filepath.WalkDir("query", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		line := 1
		for _, c := range b {
			if c == '\n' {
				line++
				continue
			}
			if c > 0x7F {
				t.Errorf("%s:%d: non-ASCII byte 0x%02X in a query file", path, line, c)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
