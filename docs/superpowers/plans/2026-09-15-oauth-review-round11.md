# OAuth Login Review Round 11 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the two code follow-ups left by rounds 9–10 on PR #238 (head da93427): the email-verification code issuer writes its grant table without the user row lock, and `LockUserRow` is a no-op UPDATE that costs one dead tuple per login on PostgreSQL.

**Architecture:** Task 1 applies the established rule (users row first, then the grant row) to `issueVerificationCode`, and makes `ConfirmEmail`'s delete row-counted so its consume is explicit. Task 2 makes `LockUserRow` engine-specific under the engine-adapter pattern: PostgreSQL uses `SELECT id FROM users WHERE id = $1 FOR UPDATE` (a real row lock, no tuple write), SQLite keeps the no-op UPDATE; the repository contract is unchanged, including the load-bearing "a missing user silently succeeds".

**Tech Stack:** Go 1.27 (`/usr/local/go/bin/go`, `GOTOOLCHAIN=go1.27.1`), sqlc v1.30 (`~/go/bin/sqlc`).

**Spec:** `docs/superpowers/specs/2026-09-07-oauth-login-design.md` §6.4b (lock rule).

## Global Constraints

- Branch `feature/oauth-login-fixes` (HEAD da93427, pushed as `feature/oauth-login`). One commit per task, conventional prefix, message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`; stage explicit paths; never commit `.remember/`.
- Go: `PATH=/usr/local/go/bin:$HOME/go/bin:$PATH GOTOOLCHAIN=go1.27.1 CGO_ENABLED=0 go …`. After editing `internal/infra/storage/sqlc/query/{sqlite,pgsql}/*.sql`: `cd internal/infra/storage/sqlc && ~/go/bin/sqlc generate && cd -`; commit `gen/`; never hand-edit; ASCII comments; `;` on the last line. Task 2 is the one place the two engines' SQL deliberately differ — the `-- name:` stays identical and the repo method signature stays identical; the adapters absorb the difference.
- Lock ordering: users row first in every transaction.
- Frozen wire contract; no golden changes; no migrations.
- TDD with RED/GREEN evidence; PostgreSQL at `postgres://econumo:econumo@127.0.0.1:55433/econumo_test?sslmode=disable` (`DBTEST_ENGINE=pgsql … -tags enginecompare -count=1`) — run `./internal/user/...` and `./internal/oauth/...` on it.
- Docs: CLAUDE.md Authentication sentence for Task 1 (verification codes issued/consumed under the lock); the `LockUserRow` SQL comments for Task 2.

---

### Task 1: Verification codes are issued under the user row lock and consumed row-counted

**Files:**
- Modify: `internal/user/verify_email.go` (`issueVerificationCode` ~line 267: `LockRow(u.ID)` first inside its existing `WithTx`, then `DeleteByUser` + `Save`; the mail send stays OUTSIDE the transaction as today at ~line 282); `ConfirmEmail` (~59-90): replace `emailVerifications.DeleteByUser` with a row-counted `Consume(id, userID)` of the row it just compared (≠1 → the existing generic invalid-code error)
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/email_verification*.sql` (add `ConsumeUserEmailVerification :execrows` — `DELETE FROM <table> WHERE id = ? AND user_id = ?`; read the table/column names), regenerate `gen/`
- Modify: `internal/user/repository.go` (`EmailVerifications.Consume(ctx, id, userID vo.Id) (int64, error)`), `internal/user/repo/emailverification*.go` + adapters
- Modify: `internal/user/lock_order_test.go` (cases: `resend-verification-code` → `LockRow` before `DeleteByUser`/`Save`; `confirm-email` → `LockRow` before `GetByID`/`Consume`/`Save`)
- Test: `internal/user/verify_email_test.go`

- [ ] **Step 1: Failing tests** — (a) lock-order: the two new cases FAIL on the old code (no `LockRow` on the issue path; `DeleteByUser` instead of `Consume` on confirm). (b) Race: hook `EmailVerifications.GetByUser` (decorator, one-shot) so that during `ConfirmEmail`, after the row is read under the lock… — on SQLite the issue path nests on the same connection, so model the OTHER direction: hook the issue path's `DeleteByUser` to run a real `ConfirmEmail` first (nested, same connection) and assert the confirm succeeds AND the freshly issued code is NOT swept afterwards (i.e. the issue transaction, having taken the lock, re-issues after the confirm committed — verify `GetByUser` returns the new row). If the SQLite modelling cannot discriminate, use the two-pool PostgreSQL recipe (`internal/user/repo/lockrow_pgsql_test.go`) driving `ResendVerificationCode` on pool 1 and `ConfirmEmail` on pool 2. Record which and the RED output.

- [ ] **Step 2: Implement** as in Files. Keep every error text. CLAUDE.md Authentication: one clause "(verification codes, like reset and email-change codes, are issued and consumed under the user row lock)".

- [ ] **Step 3: Verify both engines, commit** — `go test ./internal/user/... ./internal/server/... ./internal/test/apiparity/ ./internal/test/mcpparity/` and the pgsql run of `./internal/user/...`; no golden changes (the verification scenarios present one valid code once).

```bash
git commit -m "fix(user): issue verification codes under the user row lock and consume them row-counted

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: `LockUserRow` is `SELECT … FOR UPDATE` on PostgreSQL

**Files:**
- Modify: `internal/infra/storage/sqlc/query/pgsql/users.sql` (`LockUserRow` becomes `-- name: LockUserRow :one` / `SELECT id FROM users WHERE id = $1 FOR UPDATE;`), `internal/infra/storage/sqlc/query/sqlite/users.sql` (stays the no-op UPDATE; comment says why the engines differ), regenerate `gen/`
- Modify: `internal/user/repo/pgsql.go` (`LockUserRow` adapter: call the `:one` and map `sql.ErrNoRows` to `nil` — a missing user must keep succeeding silently; `Repository.LockRow`'s doc in `repository.go` says two error shapes depend on it), `internal/user/repo/repo.go` (querier interface unchanged: `LockUserRow(ctx, db, id) error`)
- Test: `internal/user/repo/lockrow_pgsql_test.go` (existing two-pool test must still block/unblock on commit), plus a new case on BOTH engines: `LockRow` on a missing id returns `nil`

- [ ] **Step 1: Failing test** — `TestLockRow_MissingUserSucceedsSilently` (runs on both engines) asserting `LockRow(vo.NewId())` returns nil. Run: PASS on the old code (UPDATE of no rows is nil) — so this test guards the contract through the change rather than being RED; the RED here is different: after switching pgsql to `:one` WITHOUT the ErrNoRows mapping, this test FAILS on pgsql (`sql: no rows in result set`) — do the switch in two steps and record that RED, then add the mapping (GREEN). `lockrow_pgsql_test.go` must stay green: `FOR UPDATE` blocks the second pool exactly as the UPDATE did.

- [ ] **Step 2: Implement** — pgsql query:

```sql
-- name: LockUserRow :one
-- The row lock behind every existing-row write and credential mint (see
-- user.Repository.LockRow). SELECT ... FOR UPDATE takes the same lock the
-- no-op UPDATE did without writing a tuple version per login. The adapter
-- maps no-rows to success: a missing user must keep succeeding silently.
SELECT id FROM users WHERE id = $1 FOR UPDATE;
```

sqlite keeps `UPDATE users SET updated_at = updated_at WHERE id = ?;` with a comment that SQLite has no `FOR UPDATE` and its single-writer pool serializes regardless. Adapter (`pgsql.go`): `_, err := pgsqlgen.New(db).LockUserRow(ctx, id); if errors.Is(err, sql.ErrNoRows) { return nil }; return err`.

- [ ] **Step 3: Verify both engines, commit** — `go test ./internal/user/... ./internal/oauth/... ./internal/server/... ./internal/cli/... ./internal/test/apiparity/ ./internal/test/mcpparity/` and pgsql `./internal/user/... ./internal/oauth/...` + enginecompare. Update the PR-facing note in CLAUDE.md if it mentions the no-op UPDATE (grep `updated_at = updated_at`).

```bash
git commit -m "perf(user): LockUserRow uses SELECT ... FOR UPDATE on PostgreSQL

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

## Final gate

`make go-test`; PostgreSQL enginecompare + `./internal/user/... ./internal/oauth/... ./internal/server/... ./internal/cli/...`; no web changes expected.
