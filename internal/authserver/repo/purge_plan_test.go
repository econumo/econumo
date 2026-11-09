package repo_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/test/dbtest"
)

// SQLite matches a partial index's WHERE by expression, so `is_global = 0` in
// the index and `is_global = false` in a query silently turn every anonymous
// registration's purge into a full table scan. The queries come from the sqlc
// source so an edit to either side is caught.
func TestPurgeQueriesUseUnusedClientsIndex(t *testing.T) {
	src, err := os.ReadFile("../../infra/storage/sqlc/query/sqlite/oauth_server.sql")
	if err != nil {
		t.Fatal(err)
	}
	db := dbtest.NewSQLite(t)
	// The cap query has no range on created_at, so a scan of the partial index
	// (unused rows only) is the best it can do; the age purge must seek.
	cases := []struct {
		name   string
		args   []any
		search bool
	}{
		{"PurgeUnusedOAuthClients", []any{time.Now().UTC()}, true},
		{"PurgeExcessUnusedOAuthClients", []any{1000}, false},
	}
	for _, c := range cases {
		name := c.name
		query := sqlcQuery(t, string(src), name)
		rows, err := db.Raw.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+query, c.args...)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		for _, step := range plan {
			if strings.Contains(step, " oauth_clients") && !strings.Contains(step, "USING INDEX IDX_oauth_clients_unused") {
				t.Errorf("%s: step %q does not use IDX_oauth_clients_unused; plan %q", name, step, plan)
			}
		}
		if len(plan) == 0 {
			t.Errorf("%s: empty plan", name)
		} else if c.search && !strings.HasPrefix(plan[0], "SEARCH oauth_clients USING INDEX IDX_oauth_clients_unused") {
			t.Errorf("%s: want an index search first; plan %q", name, plan)
		}
	}
}

func sqlcQuery(t *testing.T, src, name string) string {
	t.Helper()
	_, rest, ok := strings.Cut(src, "-- name: "+name+" ")
	if !ok {
		t.Fatalf("query %s not found", name)
	}
	_, body, _ := strings.Cut(rest, "\n")
	body, _, _ = strings.Cut(body, ";")
	return body
}
