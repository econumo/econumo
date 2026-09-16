# OAuth Login Review Round 10 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the round-10 finding on PR #238 (head e867560): password-reset codes are not atomically consumed — `ResetPassword` validates the request before the user lock and never re-checks or row-count-consumes it after, so a replaced or already-used code still resets the password, and two concurrent resets with one code both succeed.

**Architecture:** Same idiom as rounds 8–9: resolve the user, take the user row lock, then load AND row-count consume the matching password-request row inside that transaction before writing the password; `RemindPassword` replaces the code under the same lock so issuance and consumption have one defined order (users row first, then the grant row).

**Tech Stack:** Go 1.27 (`/usr/local/go/bin/go`, `GOTOOLCHAIN=go1.27.1`), sqlc v1.30 (`~/go/bin/sqlc`).

**Spec:** `docs/superpowers/specs/2026-09-07-oauth-login-design.md` §6.4b (the lock rule) — add one sentence on reset-code consumption.

## Global Constraints

- Branch `feature/oauth-login-fixes` (HEAD e867560, pushed as `feature/oauth-login`). One commit, conventional prefix, message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`; stage explicit paths; never commit `.remember/`.
- Go: `PATH=/usr/local/go/bin:$HOME/go/bin:$PATH GOTOOLCHAIN=go1.27.1 CGO_ENABLED=0 go …`. After editing `internal/infra/storage/sqlc/query/{sqlite,pgsql}/password_request*.sql`: `cd internal/infra/storage/sqlc && ~/go/bin/sqlc generate && cd -`; commit `gen/`; never hand-edit; same `-- name:` both engines (`?` vs `$N`); `;` on the last line; ASCII comments.
- Lock ordering: users row FIRST, then grant rows, in every transaction.
- Frozen wire contract: the refusal is the existing reset error (`"Reset password error"`, code `user.reset_password_error` — read the exact value in `password.go`); the expired-code error stays `"The code is expired"`; no route/shape/golden changes.
- TDD: race reproducers proven RED on the old code; PostgreSQL available at `postgres://econumo:econumo@127.0.0.1:55433/econumo_test?sslmode=disable` (`DBTEST_ENGINE=pgsql … -tags enginecompare -count=1`) — run `./internal/user/...` on it too.
- Docs: CLAUDE.md Authentication (reset consumes its code under the lock), spec §6.4b sentence, regression plan item ("a reset code works exactly once, even when two requests present it at the same time; issuing a new code invalidates an in-flight reset with the old one").

---

### Task 1: Reset codes are consumed row-counted under the user lock; remind replaces them under the same lock

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/password_request*.sql` — add `ConsumeUserPasswordRequest :execrows` (`DELETE FROM users_password_requests WHERE id = ? AND user_id = ?` — check the exact table name in the existing queries)
- Regenerate: `gen/`
- Modify: `internal/user/repository.go` (`PasswordRequests.Consume(ctx, id, userID vo.Id) (int64, error)`), `internal/user/repo/passwordrequest*.go` (+ adapters)
- Modify: `internal/user/password.go` (`ResetPassword`, `RemindPassword`)
- Modify: `internal/user/lock_order_test.go` (the `reset-password` case now also asserts `Consume` after `LockRow`; add a `remind-password` case asserting `LockRow` precedes `DeleteByUser`/`Save`)
- Test: `internal/user/change_email_integration_test.go` style hooks (or `login_test.go` / `session_cascade_test.go` — wherever the reset race helpers already live)

**Interfaces:**
- Produces: `PasswordRequests.Consume(ctx, id, userID vo.Id) (int64, error)`.
- Consumes: `Repository.LockRow`, `PasswordRequests.GetByUserAndCode`, `reclaimCredentials`.

- [ ] **Step 1: Failing race tests**

(a) *Replaced code still resets:* hook `PasswordRequests.GetByUserAndCode` (decorator in the existing style) to run a REAL `RemindPassword` (which replaces the code with a new one) once after returning the row; then `ResetPassword` with the OLD code must return the reset error and the password must be unchanged.

(b) *Two concurrent resets with one code:* hook the same read so the second call's `GetByUserAndCode` runs a real `ResetPassword` with the same code and password "first"; the outer call (password "second") must be refused and the stored hash must verify "first", not "second". (On SQLite the nested reset runs on the same connection — that models "the other reset committed before our lock"; RED on the old code because the outer call still saves "second".)

Run: both FAIL on the old code (record the output).

- [ ] **Step 2: Implement**

`ResetPassword`: keep the pre-tx lookup by email, the pre-tx `GetByUserAndCode` + expiry check (cheap early errors, unchanged wire), and the argon2 hash BEFORE the transaction (never hash under the lock). Inside `WithTx`: `LockRow(u.ID)` → `GetByID` (re-read) → the existing proven-address re-check → `pr2, err := s.passwordRequests.GetByUserAndCode(ctx, u.ID, hashedCode)` (NotFound → the reset error: the code was replaced or already used) → `if pr2.IsExpired(now) → the expired error` → `n, err := s.passwordRequests.Consume(ctx, pr2.ID, u.ID)`; `n != 1` → the reset error → `Save` → `reclaimCredentials` (its `passwordRequests.DeleteByUser` now sweeps only OTHER codes; keep it). Comment (why): the code is the evidence; it is re-read and consumed under the lock so a concurrent reset or a fresh code issued after this request started cannot both succeed.

`RemindPassword`: inside its existing `WithTx`, `LockRow(u.ID)` as the first statement, then `DeleteByUser` + `Save` (comment: issuance and consumption serialize on the same lock, so "issue B" and "consume A" have one order).

- [ ] **Step 3: Both engines, docs, commit**

`go build ./... && go vet ./... && gofmt -l .`; `go test ./internal/user/... ./internal/oauth/... ./internal/server/... ./internal/cli/... ./internal/test/apiparity/ ./internal/test/mcpparity/`; `DBTEST_ENGINE=pgsql … go test -count=1 -tags enginecompare ./internal/user/...`. No golden changes (the apiparity reset scenarios present one valid code once). Docs per Global Constraints.

```bash
git commit -m "fix(user): consume the reset code row-counted under the user lock; remind replaces it under the same lock

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

## Final gate

`make go-test`; PostgreSQL `./internal/user/... ./internal/oauth/...` + enginecompare; no web changes expected.
