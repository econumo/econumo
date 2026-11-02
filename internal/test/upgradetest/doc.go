//go:build upgrade

// Package upgradetest upgrades a database written by a PUBLISHED release binary
// to the binary built from this checkout, then checks that nothing the old
// release stored was lost or broken.
//
// The rest of the suite always starts from an empty database and applies every
// migration in version order, so it cannot see a migration that only misbehaves
// on a database some earlier release left behind — a migration released with a
// version below one an earlier release had already applied runs AFTER it there
// and in the opposite order on a fresh install. That is how the access_tokens
// repair (20261031000010-12) became necessary: a v1.5.x database lost two
// columns on the way to v1.6.0 and could no longer mint sessions.
//
// For every source release (the newest patch of each minor line that publishes
// linux binaries, or ECONUMO_UPGRADE_FROM) and every engine (SQLite always,
// PostgreSQL when DATABASE_TEST_PGSQL_URL answers):
//
//  1. the release binary, downloaded from GitHub Releases and checked against
//     its SHA256SUMS, boots on an empty database and gets realistic data
//     through its own HTTP API;
//  2. the reads that data feeds are recorded, together with the migration
//     versions the release applied;
//  3. the current binary boots on the same database (and once more, so a
//     second boot over a migrated database is covered too) and must:
//     - leave the schema identical to a fresh install's,
//     - apply no migration numbered below the release's newest one,
//     - keep the release's sessions and personal access tokens valid and
//     still log users in,
//     - return every value the release returned for the same reads
//     (additive fields and rows are allowed),
//     - hold the exact balances and budget figures the seed implies,
//     - still accept writes on the migrated rows.
//
// Run it with `make test-upgrade` (linux only: releases publish linux binaries).
// Downloads are cached in ECONUMO_UPGRADE_CACHE (default: the user cache dir).
package upgradetest
