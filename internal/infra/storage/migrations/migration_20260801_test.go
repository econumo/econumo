package migrations_test

// Verifies the 20260801000000 migration: custom currencies move to ONE fixed
// rate on the currencies row — the LATEST dated rate row wins the backfill,
// custom rows leave currencies_rates (globals-only history from then on), and
// customs that never had a rate stay NULL.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestMigration20260801_FixedRateBackfill(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	all := toRunList(migrations.SQLite())
	var before []migrate.Migration
	for _, m := range all {
		if m.Version < "20260801000000" {
			before = append(before, m)
		}
	}
	if err := migrate.Run(ctx, db, before); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}
	seed := []string{
		`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		 VALUES ('u1', 'i1', 'a@b.c', 'A', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`,
		`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('p1', 'PTS', 'pts', '2026-01-01 00:00:00', 'u1')`,
		`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('n1', 'NOR', 'n', '2026-01-01 00:00:00', 'u1')`,
		// custom rate history: the LATEST row must win the backfill
		`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		 VALUES ('r1', 'p1', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-01-10', '5.00000000')`,
		`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		 VALUES ('r2', 'p1', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-02-10', '10.00000000')`,
		// a global rate row must survive untouched
		`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		 VALUES ('r3', 'dffc2a06-6f29-4704-8575-31709adee926', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-02-10', '1.00000000')`,
	}
	for _, s := range seed {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("seed: %v\n%s", err, s)
		}
	}
	if err := migrate.Run(ctx, db, all); err != nil {
		t.Fatalf("target migration: %v", err)
	}
	var rate sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT rate FROM currencies WHERE id = 'p1'").Scan(&rate); err != nil {
		t.Fatal(err)
	}
	// sqlite NUMERIC affinity canonicalizes "10.00000000" to "10"; pgsql keeps
	// the long form — compare the normalized decimal, like the read path does.
	if !rate.Valid || vo.NewDecimal(rate.String).String() != "10" {
		t.Fatalf("backfilled rate = %+v, want the LATEST row (normalized 10)", rate)
	}
	if err := db.QueryRowContext(ctx, "SELECT rate FROM currencies WHERE id = 'n1'").Scan(&rate); err != nil {
		t.Fatal(err)
	}
	if rate.Valid {
		t.Fatalf("no-history custom must stay NULL, got %q", rate.String)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM currencies_rates WHERE currency_id = 'p1'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("custom rate rows purged = %d rows left, err %v", n, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM currencies_rates").Scan(&n); err != nil || n != 1 {
		t.Fatalf("global rate rows must survive; total = %d, err %v", n, err)
	}
}
