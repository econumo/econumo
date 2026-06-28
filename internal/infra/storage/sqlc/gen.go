// Package sqlc holds the sqlc configuration and hand-written SQL queries used to
// generate the per-engine typed query packages (gen/sqlite and gen/pgsql).
//
// This file exists only to host the go:generate directive below, so the whole
// tree regenerates with `go generate ./...` from the go/ module root. It has no
// runtime code. The generated packages live under gen/sqlite (package
// sqlitegen) and gen/pgsql (package pgsqlgen); both implement a Querier
// interface (emit_interface: true), which the repo layer depends on.
package sqlc

//go:generate sqlc generate
