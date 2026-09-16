# OAuth Login Review Round 12 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the round-12 findings on PR #238 (head e973749): (High) a login holding the old password hash can mint a session after `update-password` committed, because the change neither bumps `credentials_generation` nor revokes sessions inside its transaction; (Low) multi-audience ID tokens are accepted without `azp` validation.

**Architecture:** `update-password` becomes a credential rotation in one transaction under the user row lock: verify the old password and hash the new one BEFORE the lock (never hash under it), then lock → narrow fenced password UPDATE (the round-7 `UpdatePasswordIfGeneration`, keyed on the generation read with the hash it verified) → bump the generation → sweep pending reset codes and email-change requests → revoke the other sessions. PATs and identities are untouched (documented contract). ID-token validation gains the OIDC Core §3.1.3.7 audience rules: a multi-audience token requires `azp` equal to the client id; any present `azp` must equal the client id.

**Tech Stack:** Go 1.27 (`/usr/local/go/bin/go`, `GOTOOLCHAIN=go1.27.1`).

**Spec:** `docs/superpowers/specs/2026-09-07-oauth-login-design.md` §6.4b (lock rule) and §5 (ID-token verification).

## Global Constraints

- Branch `feature/oauth-login-fixes` (HEAD e973749, pushed as `feature/oauth-login`). One commit per task, conventional prefix, message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`; stage explicit paths; never commit `.remember/`; never `git stash`.
- Go: `PATH=/usr/local/go/bin:$HOME/go/bin:$PATH GOTOOLCHAIN=go1.27.1 CGO_ENABLED=0 go …`. No SQL changes expected (Task 1 reuses `UpdatePasswordIfGeneration`, `BumpCredentialsGeneration`, `passwordRequests.DeleteByUser`, `emailChangeRequests.DeleteByUser`, `revokeTokens`).
- Lock ordering: users row first, then grant/token rows.
- Frozen wire contract: `"Password is not correct"` (code `user.password_incorrect`) for a wrong old password; the fenced update affecting 0 rows (the account changed under us) returns the same error value; `update-password` still keeps PATs, identities, and the presenting session; no route/shape/golden changes.
- TDD with race reproducers proven RED; PostgreSQL at `postgres://econumo:econumo@127.0.0.1:55433/econumo_test?sslmode=disable` (`DBTEST_ENGINE=pgsql … -tags enginecompare -count=1`) — run `./internal/user/...` on it.
- Docs: CLAUDE.md Authentication cascade sentence (update-password now bumps the generation and sweeps pending reset/email-change grants; still keeps PATs/identities/the current session), spec §6.4b, regression plan item; spec §5 for the `azp` rule.

---

### Task 1: `update-password` is a credential rotation in one locked transaction

**Files:**
- Modify: `internal/user/password.go:57-76` (`UpdatePassword`)
- Modify: `internal/user/lock_order_test.go` (the `update-password` case: `LockRow` before `UpdatePasswordIfGeneration`/`BumpCredentialsGeneration`/`RevokeAll`)
- Test: `internal/user/login_test.go` (or `session_cascade_test.go`; reuse `resettingRepo`-style decorators and `newUserSvcWithRepo`)
- Modify: CLAUDE.md, spec §6.4b, `docs/regression-test-plan.md`

**Interfaces:** consumes `Repository.LockRow`, `Repository.UpdatePasswordIfGeneration(ctx, userID, hash, salt, algorithm, now, generation) (int64, error)`, `Repository.BumpCredentialsGeneration`, `PasswordRequests.DeleteByUser`, `EmailChangeRequests.DeleteByUser`, `revokeTokens(ctx, userID, exceptTokenID, now, model.TokenKindSession)`.

- [ ] **Step 1: Failing race tests (both lock orders)**

(a) *Login read before the change commits:* decorate `Repository.GetByEmail` (one-shot) to run a REAL `UpdatePassword` (old → new) right after returning the row to `Login`; `Login` with the OLD password must return 401 `"Invalid credentials."` and no live session row may exist for the user afterwards. (b) *Change commits before the login read:* plain sequential — `UpdatePassword` then `Login` with the old password → 401 (this already passes; keep it as the second order). (c) *Sweep inside the transaction:* seed one other live session, one pending reset code and one pending email change; after `UpdatePassword`: the other session is revoked, the presenting session live, a PAT still live, the reset code and the email change gone, and the generation +1.

Run: (a) FAILS on the old code (a session is minted); (c) FAILS (generation unchanged, grants remain).

- [ ] **Step 2: Implement**

```go
func (s *Service) UpdatePassword(ctx context.Context, userID vo.Id, currentTokenID vo.Id, req model.UpdatePasswordRequest) (*model.UpdatePasswordResult, error) {
	incorrect := &errs.ValidationError{Msg: "Password is not correct", MsgCode: errs.CodeUserPasswordIncorrect}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	// Verify and hash before the lock: argon2 must never run under the row lock.
	if !s.hasher.Verify(u.Algorithm, u.Password, req.OldPassword, u.Salt) {
		return nil, incorrect
	}
	newHash, herr := s.hasher.Hash(req.NewPassword)
	if herr != nil {
		return nil, herr
	}
	now := s.clock.Now()
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockRow(ctx, userID); err != nil {
			return err
		}
		// The fence: the hash we verified belongs to the generation we read it
		// under; if a reclaim or another rotation moved it, this rotation
		// proved nothing and writes nothing.
		n, err := s.repo.UpdatePasswordIfGeneration(ctx, userID, newHash, u.Salt, model.AlgorithmArgon2id, now, u.CredentialsGeneration)
		if err != nil {
			return err
		}
		if n != 1 {
			return incorrect
		}
		// A rotation invalidates every flow that read the old credential:
		// in-flight logins (the bump), pending reset codes and email changes
		// (grants issued under the old password), and the other sessions.
		// The presenting session, PATs and linked identities stay (the owner
		// is acting, not recovering).
		if err := s.repo.BumpCredentialsGeneration(ctx, userID); err != nil {
			return err
		}
		if err := s.passwordRequests.DeleteByUser(ctx, userID); err != nil {
			return err
		}
		if err := s.emailChangeRequests.DeleteByUser(ctx, userID); err != nil {
			return err
		}
		return s.revokeTokens(ctx, userID, currentTokenID, now, model.TokenKindSession)
	}); err != nil {
		return nil, err
	}
	return &model.UpdatePasswordResult{}, nil
}
```

(`UpdatePasswordIfGeneration` writes password/salt/algorithm/updated_at; passing `u.Salt` keeps the column as it was — argon2id ignores it. Check the existing signature in `repository.go` and use it exactly.) Update the lock-order case.

- [ ] **Step 3: Both engines, docs, commit** — `go test ./internal/user/... ./internal/server/... ./internal/test/apiparity/ ./internal/test/mcpparity/`; pgsql `./internal/user/...`; no golden changes (apiparity's update-password scenario is a single sequential call). CLAUDE.md cascade sentence: "`update-password` (the user changing their own password) rotates the credential in one locked transaction: bumps the generation (an in-flight login with the old password mints nothing), sweeps pending reset codes and email-change requests, and revokes the OTHER sessions; it keeps the presenting session, PATs and linked identities." Spec §6.4b: same. Regression plan: "Changing the password from Settings signs out the other sessions, keeps this one and personal tokens, cancels a pending email change and any outstanding reset code, and a login with the old password that was already in flight gets 'Invalid credentials.'"

```bash
git commit -m "fix(user): update-password rotates the credential in one locked transaction (bump, sweep grants, revoke others)

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: ID-token audience rules — `azp` validation

**Files:**
- Modify: `internal/infra/oidc/jwt.go` (`idTokenClaims.Azp string \`json:"azp"\``; `audienceMatches(raw json.RawMessage, azp, clientID string) error`), `internal/infra/oidc/client.go:~268` (use it; error names the rule, never the values)
- Test: `internal/infra/oidc/jwt_test.go` (`m["aud"] = []string{"x", f.ClientID}` currently ACCEPTED — that case must now require `azp`), `jwt_more_test.go`
- Modify: spec §5 (one sentence)

- [ ] **Step 1: Failing tests** — table: single-string aud == client → OK; single aud, no azp → OK; single aud, azp == client → OK; single aud, azp == other → REJECT; multi aud containing client, no azp → REJECT; multi aud containing client, azp == client → OK; multi aud containing client, azp == other → REJECT; aud not containing client → REJECT. Run: the two multi-aud-without-azp / azp-mismatch cases FAIL on the old code (accepted).

- [ ] **Step 2: Implement** (OIDC Core §3.1.3.7 rules 3–5):

```go
// audienceMatches applies OpenID Connect Core 3.1.3.7 rules 3-5: the client id
// must be an audience; a token minted for several audiences must name this
// client as the authorized party, and any azp present must be this client.
// Otherwise a token issued for another client of the same issuer could be
// replayed here.
func audienceMatches(raw json.RawMessage, azp, clientID string) error {
	auds := audiences(raw) // []string for both the string and array forms
	if !slices.Contains(auds, clientID) {
		return fmt.Errorf("%w: audience", ErrInvalidToken)
	}
	if len(auds) > 1 && azp == "" {
		return fmt.Errorf("%w: multi-audience token without azp", ErrInvalidToken)
	}
	if azp != "" && azp != clientID {
		return fmt.Errorf("%w: azp", ErrInvalidToken)
	}
	return nil
}
```

`VerifyIDToken` calls it after the issuer check. Fix the existing test expectation (the `["x", clientID]` case now sets `azp` to `f.ClientID` to stay accepted, and a sibling case without `azp` asserts rejection). `oidctest.Fake.claims` keeps a single aud (Google/Apple/typical IdPs), so no other test changes.

- [ ] **Step 3: Verify, commit** — `go test ./internal/infra/oidc/... ./internal/oauth/... ./internal/server/... ./internal/test/apiparity/`; no golden changes. Spec §5: "Audience: the client id must be in `aud`; a multi-audience token must carry `azp` equal to the client id, and any `azp` present must equal it (Core §3.1.3.7)."

```bash
git commit -m "fix(oidc): validate azp on multi-audience ID tokens

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

## Final gate

`make go-test`; PostgreSQL `./internal/user/...` + enginecompare; no web changes expected.
