package apiparity

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/web/middleware"
)

// The consent page promises a read token changes nothing, so every table is
// compared around its requests, the refused ones included. The token's own
// last_used_at is bookkeeping and the one column allowed to move.
func TestReadTokenWritesNothing(t *testing.T) {
	db := dbtest.NewSQLite(t)
	h := NewHarness(t, db)
	before := dumpTables(t, db)

	call := func(method, path string, want int) {
		t.Helper()
		req, err := http.NewRequest(method, h.URL()+path, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+ReadToken)
		req.Header.Set("X-Timezone", "Pacific/Auckland")
		req.Header.Set("Accept-Language", "de")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("%s %s = %d, want %d", method, path, res.StatusCode, want)
		}
	}
	for path := range middleware.ReadScopePaths {
		call(http.MethodGet, path, http.StatusOK)
	}
	call(http.MethodGet, "/api/v1/budget/get-budget-list", http.StatusUnauthorized)
	call(http.MethodPost, "/api/v1/user/update-name", http.StatusUnauthorized)
	call(http.MethodPost, "/mcp", http.StatusUnauthorized)

	after := dumpTables(t, db)
	for table, rows := range before {
		if after[table] != rows {
			t.Errorf("table %s changed:\nbefore: %s\nafter:  %s", table, rows, after[table])
		}
	}
}

func dumpTables(t *testing.T, db *dbtest.DB) map[string]string {
	t.Helper()
	names, err := db.Raw.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for names.Next() {
		var name string
		if err := names.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	names.Close()

	dump := make(map[string]string, len(tables))
	for _, table := range tables {
		rows, err := db.Raw.Query(`SELECT * FROM "` + table + `" ORDER BY 1, 2`)
		if err != nil {
			rows, err = db.Raw.Query(`SELECT * FROM "` + table + `" ORDER BY 1`)
		}
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			for i, col := range cols {
				if table == "access_tokens" && col == "last_used_at" {
					continue
				}
				fmt.Fprintf(&b, "%s=%v ", col, vals[i])
			}
			b.WriteByte('\n')
		}
		rows.Close()
		dump[table] = b.String()
	}
	return dump
}
