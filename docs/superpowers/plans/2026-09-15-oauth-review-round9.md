# OAuth Login Review Round 9 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the round-9 findings on PR #238 (head 83c8eeb): an in-flight email-change confirmation can land after a password reset (account recovery defeated), and an unlinked provider's pending sign-in handoff stays redeemable.

**Architecture:** Both fixes reuse the round-8 shape: take the user row lock first, consume the grant row-counted in the same transaction, and make the write conditional in SQL. Email-change confirmation becomes lock → consume request (`:execrows`) → fenced narrow email UPDATE; request creation is a fenced INSERT. Unlink deletes the provider's pending handoffs, login handoffs carry the identity (issuer + subject), and redemption runs under the user row lock and re-validates the identity before minting.

**Tech Stack:** Go 1.27 (`/usr/local/go/bin/go`, `GOTOOLCHAIN=go1.27.1`), sqlc v1.30 (`~/go/bin/sqlc`).

**Spec:** `docs/superpowers/specs/2026-09-07-oauth-login-design.md` §11 (fence) — Task 2 adds one sentence on unlink.

## Global Constraints

- Branch `feature/oauth-login-fixes` (worktree `.claude/worktrees/bridge-cse_01Qx2CadepXjbi8Cte53yHaC`, HEAD 83c8eeb, already pushed as `feature/oauth-login`). One commit per task, conventional prefix, message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`; stage explicit paths; never commit `.remember/`.
- Go: `PATH=/usr/local/go/bin:$HOME/go/bin:$PATH GOTOOLCHAIN=go1.27.1 CGO_ENABLED=0 go …` from the worktree root. After editing any `internal/infra/storage/sqlc/query/{sqlite,pgsql}/*.sql`: `cd internal/infra/storage/sqlc && ~/go/bin/sqlc generate && cd -`; commit `gen/`; never hand-edit; same `-- name:` and column list on both engines (`?` vs `$N`); the `;` on the last statement line; `.sql` ASCII-only including comments.
- Lock ordering rule (from round 8): every transaction that touches `users` and a grant table takes the `users` row FIRST (`Users.LockRow` / `Repository.LockRow`), then the grant rows. Never the reverse (PostgreSQL deadlock).
- Frozen wire contract: error texts stay exactly as today (`"The confirmation code is not valid."` code `user.verification_code_invalid`; `"Sign-in link is invalid or has expired"` 401 code `oauth.handoff_invalid`; `"Invalid access token"` 401); no route/shape/golden changes.
- TDD with race reproducers: each fix has a test that FAILS on the old code (decorator hooks in the existing style: `resettingRepo`, `deactivatingRepo`, `beforeFindByID`), proven RED and recorded. PostgreSQL is available at `postgres://econumo:econumo@127.0.0.1:55433/econumo_test?sslmode=disable` (`DBTEST_ENGINE=pgsql … go test -count=1 -tags enginecompare …`) — run the touched packages on it.
- Never log emails/tokens/hashes. Comments only for the non-obvious why. Docs: `docs/regression-test-plan.md` for the user-observable outcomes, CLAUDE.md Authentication section for the unlink rule.

---

### Task 1: Email-change confirmation consumes its grant under the user lock; request creation is fenced

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/email_change*.sql` (find the file: `ls internal/infra/storage/sqlc/query/sqlite | grep -i email`) — add `ConsumeUserEmailChangeRequest :execrows` (`DELETE FROM users_email_change_requests WHERE id = ? AND user_id = ?`) and change `InsertUserEmailChangeRequest` into `InsertUserEmailChangeRequestIfGeneration :execrows` (`INSERT … SELECT … WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = ? AND u.credentials_generation = ?)`)
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/users.sql` — add `UpdateUserEmailIfGeneration :execrows` (`UPDATE users SET email = ?, email_verified = 1/TRUE, updated_at = ? WHERE id = ? AND credentials_generation = ?`)
- Regenerate: `gen/`
- Modify: `internal/user/repository.go` (`EmailChangeRequests`: `Save(ctx, r, generation) (int64, error)` replaces `Save`; add `Consume(ctx, id, userID) (int64, error)`; `Repository.ReplaceEmailIfGeneration(ctx, userID, encryptedEmail, now, generation) (int64, error)`), the repo implementations + adapters
- Modify: `internal/user/change_email.go` (`RequestEmailChange`, `issueEmailChangeCode`, `ConfirmEmailChange`)
- Test: `internal/user/change_email_integration_test.go` (+ any test double of `EmailChangeRequests`)

**Interfaces:**
- Produces: `EmailChangeRequests.Consume(ctx, id, userID vo.Id) (int64, error)`; `EmailChangeRequests.Save(ctx, r *model.EmailChangeRequest, generation int64) (int64, error)`; `Repository.ReplaceEmailIfGeneration(ctx, userID vo.Id, encryptedEmail string, now time.Time, generation int64) (int64, error)`.
- Consumes: `Repository.LockRow` (round 8), `Repository.BumpCredentialsGeneration`, `model.User.CredentialsGeneration`.

- [ ] **Step 1: Failing race test — a reset between the request read and the confirm write leaves the email alone**

In `internal/user/change_email_integration_test.go` (reuse its service builder; add an `EmailChangeRequests` decorator whose `GetByUser` runs a hook once after returning the row):

```go
func TestConfirmEmailChange_ResetBetweenRequestReadAndWriteIsRefused(t *testing.T) {
	env := newChangeEmailEnv(t) // the file's existing builder; wrap emailChangeRequests with the hook decorator
	u, code := env.requestChange(t, "attacker@x.test") // issues the pending request and returns the plain code
	sessionID := env.sessionFor(t, u)
	env.reqs.afterGetByUser = func() { env.resetPassword(t, u, "new-owner-password") } // real ResetPassword: bump + sweep
	_, err := env.svc.ConfirmEmailChange(ctx, u.ID, sessionID, model.ConfirmEmailChangeRequest{Code: code})
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.MsgCode != errs.CodeUserVerificationCodeInvalid {
		t.Fatalf("want the invalid-code error, got %v", err)
	}
	after, _ := env.repo.GetByID(ctx, u.ID)
	if got, _ := env.encode.Decode(after.Email); got != u.plainEmail {
		t.Fatalf("confirmation crossed the reclaim: email is now %q", got)
	}
}
```

Adapt helper names to the file. Run: FAIL (the email is changed).

- [ ] **Step 2: Implement**

SQL (both engines; the request table's column list as in the existing insert):

```sql
-- name: InsertUserEmailChangeRequestIfGeneration :execrows
-- A pending change is a grant to rewrite the login key; it must not be created
-- by a session a reclaim has already invalidated.
INSERT INTO users_email_change_requests (…same columns…)
SELECT …same placeholders…
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = ? AND u.credentials_generation = ?);

-- name: ConsumeUserEmailChangeRequest :execrows
DELETE FROM users_email_change_requests WHERE id = ? AND user_id = ?;
```

`users.sql`: `UpdateUserEmailIfGeneration` as above (comment: the confirm path writes only the email columns, under the generation it read after taking the row lock, so a stale aggregate can never be saved over a reclaimed account).

`ConfirmEmailChange`: keep the rate-limit + code/expiry checks on the pre-read `cr`; then

```go
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		// users first, then the grant row: the same order reclaimCredentials
		// takes, so the two can only serialize, never deadlock.
		if err := s.repo.LockRow(ctx, userID); err != nil {
			return err
		}
		n, err := s.emailChangeRequests.Consume(ctx, cr.ID, userID)
		if err != nil {
			return err
		}
		if n != 1 {
			// The reclaim (or a concurrent confirm) already took the grant.
			return invalid
		}
		u, err := s.repo.GetByID(ctx, userID) // after the lock: the generation is current
		if err != nil {
			return err
		}
		exists, err := s.repo.ExistsByEmail(ctx, cr.NewEmail) // inside the tx now
		…
		rows, err := s.repo.ReplaceEmailIfGeneration(ctx, userID, encrypted, now, u.CredentialsGeneration)
		if err != nil { return err }
		if rows != 1 { return invalid }
		updated = u; updated.UpdateEmail(encrypted, now); updated.MarkEmailVerified(now)
		return nil
	})
```

`RequestEmailChange` → `issueEmailChangeCode(ctx, u, newEmail, now)` passes `u.CredentialsGeneration` (from the row whose password was just verified) into `Save`; 0 rows → `errs.NewUnauthorized("Invalid access token")`. Update every `EmailChangeRequests` implementer/double and the `verify_email`/`admin` callers if any (`grep -rn "emailChangeRequests\.\|EmailChangeRequests" internal --include='*.go'`).

- [ ] **Step 3: Second failing test — request creation after a reclaim is refused**

```go
func TestRequestEmailChange_ResetBetweenPasswordCheckAndInsertIsRefused(t *testing.T) {
	// decorate Repository.GetByID to run ResetPassword once after returning the row
	_, err := env.svc.RequestEmailChange(ctx, u.ID, model.RequestEmailChangeRequest{Password: "old", NewEmail: "attacker@x.test"})
	var unauthorized *errs.UnauthorizedError
	if !errors.As(err, &unauthorized) { t.Fatalf("want 401, got %v", err) }
	if _, err := env.reqs.GetByUser(ctx, u.ID); err == nil { t.Fatal("a pending request was created after the reclaim") }
}
```

Run RED (before the fenced insert) then GREEN.

- [ ] **Step 4: Both engines, docs, commit**

`go test ./internal/user/... ./internal/test/apiparity/ ./internal/test/mcpparity/` and the pgsql run of `./internal/user/...`. Regression plan: in the "Password reset is a full reclaim" item add "an email change that was pending (code sent but not yet confirmed) can no longer be confirmed after the reset, even if the confirmation was already in flight".

```bash
git commit -m "fix(user): consume the email-change grant under the user lock and fence the email write

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: Unlink invalidates the provider's pending handoffs; redemption re-validates the identity under the user lock

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/oauth.sql` — add `DeleteOAuthHandoffsByUserProvider :execrows` (`DELETE FROM oauth_handoffs WHERE user_id = ? AND provider = ?`)
- Regenerate: `gen/`
- Modify: `internal/oauth/repository.go` (`Handoffs.DeleteByUserProvider`), `internal/oauth/repo/handoff*.go`
- Modify: `internal/oauth/callback.go` (`mintHandoff` gains `issuer, subject string`; login handoffs carry them), `internal/oauth/handoff.go` (`ExchangeHandoff` in a transaction: read handoff → `LockRow` → consume → identity check → mint), `internal/oauth/identities.go` (`UnlinkIdentity` also `DeleteByUserProvider` on handoffs, inside the existing transaction after the identity delete)
- Modify: `internal/oauth/repo/handoff.go` (Insert/Get already map issuer/subject/email — confirm)
- Test: `internal/oauth/service_test.go`, `internal/oauth/repo/repo_integration_test.go`, the `api/harness_test.go` fakes

**Interfaces:**
- Produces: `Handoffs.DeleteByUserProvider(ctx, userID vo.Id, provider string) (int64, error)`.
- Consumes: `Users.LockRow` (round 8), `Identities.GetByProviderSubject`, `s.tx`.

- [ ] **Step 1: Failing test — a handoff minted before the unlink cannot be redeemed after it**

```go
func TestExchangeHandoff_RefusedAfterTheProviderWasUnlinked(t *testing.T) {
	h := newHarness(t)
	u := h.users.seed(t, "p@x.test", model.AlgorithmArgon2id) // password account, so unlink is allowed
	saveIdentity(t, h, u.ID, "oidc", h.fake.IssuerURL(), h.fake.Subject, "p@x.test")
	code, flow := h.loginHandoff(t) // start-login → consent → Callback; returns the handoff code + flow secret (the file has this shape)
	if _, err := h.svc.UnlinkIdentity(ctx, u.ID, model.UnlinkIdentityRequest{Provider: "oidc"}); err != nil {
		t.Fatal(err)
	}
	_, err := h.svc.ExchangeHandoff(ctx, model.ExchangeHandoffRequest{Code: code, Flow: flow}, "ua")
	var unauthorized *errs.UnauthorizedError
	if !errors.As(err, &unauthorized) || unauthorized.Code != errs.CodeOAuthHandoffInvalid {
		t.Fatalf("handoff redeemed after unlink: %v", err)
	}
}
```

Run: FAIL (session minted).

- [ ] **Step 2: Implement**

`identities.go` `UnlinkIdentity`: after `DeleteByUserProvider` on identities, `if _, err := s.handoffs.DeleteByUserProvider(ctx, userID, req.Provider); err != nil { return err }` (comment: a handoff is a session in waiting; the unlinked provider's must go with it).

`callback.go`: `mintHandoff(ctx, st, userID, provider, issuer, subject, idToken, generation)`; set `Issuer: issuer, Subject: subject` on the login handoff (all three `mintHandoff` call sites in `login()` have `issuer` and `claims.Subject` in scope).

`handoff.go` `ExchangeHandoff`:

```go
	var res *model.LoginResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		hash := oidc.Sha256Hex(req.Code)
		peek, err := s.handoffs.Get(ctx, hash) // to learn the user before locking
		if err != nil { return err }
		if err := s.users.LockRow(ctx, peek.UserID); err != nil { return err }
		h, err := s.consumeHandoff(ctx, hash, model.OAuthHandoffKindLogin, req.Flow)
		if err != nil { return err }
		if h == nil { return invalid }
		// The identity the callback authenticated must still be linked to this
		// user: an unlink that committed after the callback (its lock serializes
		// with ours) has already deleted the handoff, and the row check covers
		// a handoff minted before this rule existed.
		id, err := s.identities.GetByProviderSubject(ctx, h.Provider, h.Issuer, h.Subject)
		if err != nil {
			if _, ok := errs.AsNotFound(err); ok { return invalid }
			return err
		}
		if !id.UserID.Equal(h.UserID) { return invalid }
		res, err = s.users.MintSession(ctx, h.UserID, userAgent, h.Provider, h.IDToken, h.Generation)
		return err
	})
```

Map NotFound from the peek to `invalid`. `consumeHandoff`'s own Get becomes redundant — refactor it to take the already-read row (or keep the double read; prefer the refactor). Update the fakes in `service_test.go`/`api/harness_test.go` (the `Handoffs` interface gained a method; `MintSession` inside a tx — the fake's `LockRow` already goes through the real repo).

- [ ] **Step 3: Repo test + docs + both engines + commit**

`repo_integration_test.go`: `DeleteByUserProvider` deletes only that provider's handoffs for that user (seed two providers, two users). Spec §11: one sentence "unlinking a provider deletes its unredeemed handoffs, and redemption re-checks the identity under the user row lock". CLAUDE.md Authentication cascade sentence: add "unlinking a provider also drops its pending sign-in handoffs". Regression plan: "Unlink a provider while a sign-in through it is mid-flight (callback done, handoff not yet exchanged): the exchange fails with the sign-in-link-invalid error." Run `go test ./internal/oauth/... ./internal/server/... ./internal/test/apiparity/ ./internal/test/mcpparity/` and the pgsql run of `./internal/oauth/...`.

```bash
git commit -m "fix(oauth): unlink drops the provider's pending handoffs; redemption re-validates the identity under the user lock

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Final gate

`make go-test`; `cd web && pnpm test && pnpm lint && npx tsc -b` (no web changes expected — confirm with `git diff --stat`); PostgreSQL: enginecompare + `DBTEST_ENGINE=pgsql` for `./internal/user/... ./internal/oauth/... ./internal/server/... ./internal/cli/...`.
