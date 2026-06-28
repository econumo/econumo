//go:build enginecompare

// Package enginecompare holds the sqlite-vs-PostgreSQL engine comparison suite.
//
// It is compiled ONLY under the `enginecompare` build tag so the fast default
// suite (sqlite-only, no external dependency) never pulls it in. It runs at two
// levels, both asserting SQLite and PostgreSQL produce byte-identical output
// (SQLite is the reference / target engine):
//
//   - Repository level (scenarios_test.go): runs the SAME repository operation
//     against each engine and compares a deterministic snapshot. Narrow, fast,
//     good for pinpointing a specific query's divergence.
//   - API level (apiparity_test.go + apiparity_harness_test.go): stands up the
//     REAL production HTTP handler (internal/server.BuildAPI — the identical
//     router cmd/econumo serves) over each engine from an IDENTICAL seed, then
//     replays a broad catalogue of HTTP requests (every read endpoint plus a
//     write->read sequence per mutating module) and compares the raw response
//     bytes. This is the strongest parity contract: it exercises middleware,
//     JWT, the per-engine sqlc adapters, decimal/datetime handling, and envelope
//     serialization end-to-end. Server-generated UUIDv7 ids (which legitimately
//     differ per run) are redacted before comparison; everything else is strict.
//
// Together they catch divergence in the per-engine sqlc adapters (placeholder/
// type differences, datetime + decimal handling, upsert syntax, result ordering).
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
