package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/config"
)

func TestParseImportArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		path    string
		force   bool
		wantErr bool
	}{
		{"path only", []string{"db.sqlite"}, "db.sqlite", false, false},
		{"force before path", []string{"--force", "db.sqlite"}, "db.sqlite", true, false},
		{"force after path", []string{"db.sqlite", "--force"}, "db.sqlite", true, false},
		{"missing path", []string{}, "", false, true},
		{"missing path with force", []string{"--force"}, "", false, true},
		{"two paths", []string{"a.sqlite", "b.sqlite"}, "", false, true},
		{"unknown flag", []string{"--nope", "db.sqlite"}, "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, force, err := parseImportArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got path=%q force=%v", path, force)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if path != tc.path || force != tc.force {
				t.Fatalf("got (%q,%v), want (%q,%v)", path, force, tc.path, tc.force)
			}
		})
	}
}

func TestImportSQLite_RejectsNonPostgresTarget(t *testing.T) {
	c := &container{cfg: config.Config{DatabaseDriver: "sqlite"}}
	err := runImportSQLite(context.Background(), c, []string{"db.sqlite"})
	if err == nil || !strings.Contains(err.Error(), "PostgreSQL") {
		t.Fatalf("expected a PostgreSQL-required error, got %v", err)
	}
}
