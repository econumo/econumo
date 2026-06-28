// Package migrations embeds the per-engine baseline + forward migration SQL so
// the single binary carries its schema. go:embed cannot reach outside a
// package directory, so the per-engine .sql files live alongside this package
// and it exposes them to the sqlite/pgsql backend packages.
package migrations

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed sqlite/*.sql
var sqliteFS embed.FS

//go:embed pgsql/*.sql
var pgsqlFS embed.FS

// File is one migration file: Version is the filename without extension
// (e.g. "0001_baseline"), SQL is its contents.
type File struct {
	Version string
	SQL     string
}

// SQLite returns the SQLite migrations ordered by version.
func SQLite() []File { return load(sqliteFS, "sqlite") }

// Pgsql returns the PostgreSQL migrations ordered by version.
func Pgsql() []File { return load(pgsqlFS, "pgsql") }

func load(fsys embed.FS, dir string) []File {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		panic("migrations: read " + dir + ": " + err.Error())
	}
	var out []File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := fsys.ReadFile(dir + "/" + e.Name())
		if err != nil {
			panic("migrations: read file " + e.Name() + ": " + err.Error())
		}
		out = append(out, File{
			Version: strings.TrimSuffix(e.Name(), ".sql"),
			SQL:     string(b),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}
