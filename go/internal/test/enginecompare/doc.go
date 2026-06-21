//go:build enginecompare

// Package enginecompare holds the sqlite-vs-PostgreSQL engine comparison suite.
//
// It is compiled ONLY under the `enginecompare` build tag so the fast default
// suite (sqlite-only, no external dependency) never pulls it in. Each test runs
// the SAME sequence of repository operations against a freshly-migrated SQLite
// DB and a freshly-migrated PostgreSQL DB, then asserts the two produce
// byte-identical results — catching divergence in the per-engine sqlc adapters
// (placeholder/type differences, datetime + decimal handling, upsert syntax).
//
// PostgreSQL is provided via DATABASE_TEST_PGSQL_URL (see dbtest.NewPostgres);
// when that env var is unset every test SKIPS, so:
//
//	go test -tags enginecompare ./internal/enginecompare/...                # skips pg (sqlite half still runs the scenarios)
//	DATABASE_TEST_PGSQL_URL=postgres://econumo:econumo@localhost:5432/econumo_test?sslmode=disable \
//	  go test -tags enginecompare ./internal/enginecompare/...              # full comparison
//
// CI runs the second form against a Postgres service container (see the Makefile
// `go-test-engines` target and the GitHub Actions workflow).
package enginecompare
