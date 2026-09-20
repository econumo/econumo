# OAuth Login Review Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the 2026-09-14 security review of PR #238 (`feature/oauth-login`): make the account-reclaim fence airtight, stop trusting Google's unverified emails, make every credential-eviction path a reclaim, and fix the SPA/app regressions the review found.

**Architecture:** The `users.credentials_generation` fence stays, but the value now rides on the user row every flow already loads as its evidence (`model.User.CredentialsGeneration`), so evidence and fence come from ONE read; identity writes split into a fenced INSERT and a fenced UPDATE so a reclaimed identity can never be re-created; the reclaim itself becomes one primitive shared by `reset-password` and the CLI. SPA fixes are local to the auth/settings features.

**Tech Stack:** Go 1.27 (`/usr/local/go/bin/go`, `GOTOOLCHAIN=go1.27.1`), sqlc v1.30 (`~/go/bin/sqlc`), React 19 + vitest in `web/` (pnpm).

**Spec:** `docs/superpowers/specs/2026-09-07-oauth-login-design.md` (the review findings amend §6.2 step 4, §6.4b and §11; where the plan and the spec disagree, THIS PLAN wins — it records the review's rulings — and Task 5 edits the spec accordingly).

## Global Constraints

- Branch: `feature/oauth-login-fixes` (worktree `.claude/worktrees/bridge-cse_01Qx2CadepXjbi8Cte53yHaC`). Commit per task with a conventional prefix (`fix(user): …`, `fix(oauth): …`, `fix(web): …`, `docs: …`). End every commit message with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Go toolchain: run Go commands as `PATH=/usr/local/go/bin:$HOME/go/bin:$PATH GOTOOLCHAIN=go1.27.1 CGO_ENABLED=0 go …` from the repo root (the worktree root above).
- After editing any `internal/infra/storage/sqlc/query/{sqlite,pgsql}/*.sql`: run `cd internal/infra/storage/sqlc && ~/go/bin/sqlc generate && cd -` and commit the regenerated `gen/` files with the query. NEVER hand-edit `gen/`.
- Every query exists in BOTH `query/sqlite` (`?` placeholders) and `query/pgsql` (`$N` placeholders) with the same `-- name:` and the same column list, so the per-engine generated row types stay convertible (`userRow(row)` style whole-struct conversions in `internal/user/repo/{sqlite,pgsql}.go`).
- Semicolon rule: the `;` that ends a sqlc query goes on the LAST line of the statement, never on its own line (a lone `;` line truncates the generated SQL).
- Frozen wire contract (CLAUDE.md "Wire & data contract"): no response shape, message text, route, or datetime layout changes. `"Invalid credentials."` (401) and `"Invalid access token"` (401) are the exact strings for refused session/PAT mints.
- Goldens: if `go test ./internal/test/apiparity/` fails on a golden diff caused by an INTENDED change in this plan (Tasks 8 and 9 say so explicitly), regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/` and INSPECT the diff (`git diff --stat internal/test/apiparity/testdata`) before committing; never hand-edit a golden. Any other golden change is a bug in your task.
- TDD: write the failing test, run it and watch it fail for the expected reason, then implement. Test helpers already exist: `internal/test/dbtest` (`dbtest.New(t)`), `internal/test/fixture`, `internal/infra/oidc/oidctest` (`oidctest.New(t)` fake issuer), `internal/oauth/service_test.go` has an in-memory `fakeUsers`/`fakeIdentities` pair and `internal/oauth/api/harness_test.go` its own copy.
- Comments: only the *why* of non-obvious logic. No section dividers, no restating signatures, no references to PHP.
- Logging: never log emails, tokens, or hashes — ids only.
- Docs travel with behavior: any user-observable change updates `docs/regression-test-plan.md` in the same task; any operator-facing change updates CLAUDE.md (and `docs/oidc-setup.md` where relevant).
- Web checks: `cd web && pnpm test -- <file>` per task, and `cd web && pnpm lint && npx tsc -b` before the task's commit. The pre-existing `transaction.test.ts` Blob failure is known and NOT yours.

---

### Task 1: Generation rides on the user row; identity writes split into fenced INSERT and UPDATE

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/users.sql:1-4` (GetUserByID), `:48-51` (GetUserByEmail); delete `GetUserCredentialsGeneration` (`:30-31`)
- Modify: `internal/infra/storage/sqlc/query/pgsql/users.sql` (same three queries)
- Modify: `internal/infra/storage/sqlc/query/sqlite/oauth.sql:31-40` and `query/pgsql/oauth.sql:31-40` (replace `UpsertIdentityIfGeneration` with `InsertIdentityIfGeneration` + `UpdateIdentityIfGeneration`)
- Regenerate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}/*.go`
- Modify: `internal/model/user.go:105-120` (add `CredentialsGeneration int64`)
- Modify: `internal/user/repo/repo.go` (`userRow` gains `CredentialsGeneration int64`; `hydrate` copies it; delete `Repo.CredentialsGeneration` and the querier method)
- Modify: `internal/user/repository.go:32-35` (delete `CredentialsGeneration` from `Repository`)
- Modify: `internal/user/login.go:36-41`, `internal/user/external.go:62-67` (delete `Service.CredentialsGeneration`)
- Modify: `internal/oauth/ports.go` (delete `Users.CredentialsGeneration`), `internal/oauth/repository.go` (`SaveIfCurrent` → `InsertIfCurrent` + `UpdateIfCurrent`), `internal/oauth/repo/identity.go`, `identity_sqlite.go`, `identity_pgsql.go`
- Modify: `internal/oauth/callback.go` (steps 5/6/7, `saveLinkedIdentity`, `link`), `internal/oauth/handoff.go` (`CompleteLink`)
- Modify: `internal/server/glue_oauth_users.go` (drop the `CredentialsGeneration` adapter method)
- Test: `internal/user/repo/repo_integration_test.go` (or the existing user repo test file), `internal/oauth/repo/repo_integration_test.go`, `internal/oauth/service_test.go`, `internal/oauth/api/harness_test.go`, `internal/server/oauth_wiring_test.go`

**Interfaces:**
- Produces: `model.User.CredentialsGeneration int64` (loaded by `GetByID`/`GetByEmail`; `0` for a freshly built aggregate).
- Produces: `oauth.Identities.InsertIfCurrent(ctx, i *model.Identity, generation int64) (int64, error)` and `oauth.Identities.UpdateIfCurrent(ctx, i *model.Identity, generation int64) (int64, error)`. `SaveIfCurrent` no longer exists.
- Consumes (unchanged): `AccessTokens.InsertIfGeneration`, `Repository.BumpCredentialsGeneration`.

- [ ] **Step 1: Failing repo test — the user row carries its generation**

In `internal/user/repo/repo_integration_test.go` (create the file if the package has no integration test yet; it uses `dbtest.New(t)` and the sqlite-default engine):

```go
func TestGetByID_CarriesCredentialsGeneration(t *testing.T) {
	db := dbtest.New(t)
	txm := backend.NewTxManager(db.Raw)
	r := NewRepo(db.Engine, txm)
	u := fixture.NewUser(t, db) // whatever the fixture package exposes to create a user row; see internal/test/fixture/entities.go
	ctx := context.Background()
	if err := r.BumpCredentialsGeneration(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.BumpCredentialsGeneration(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CredentialsGeneration != 2 {
		t.Fatalf("CredentialsGeneration = %d, want 2", got.CredentialsGeneration)
	}
	byEmail, err := r.GetByEmail(ctx, u.Email)
	if err != nil {
		t.Fatal(err)
	}
	if byEmail.CredentialsGeneration != 2 {
		t.Fatalf("GetByEmail CredentialsGeneration = %d, want 2", byEmail.CredentialsGeneration)
	}
}
```

(Read `internal/test/fixture/entities.go` and an existing `*_integration_test.go` in `internal/user/repo` first and use the same user-creation helper and constructor pattern they use; the shape above is the assertion, not the setup.)

- [ ] **Step 2: Run it — expect a compile failure on `CredentialsGeneration`**

Run: `go test ./internal/user/repo/ -run TestGetByID_CarriesCredentialsGeneration`
Expected: FAIL — `got.CredentialsGeneration undefined`.

- [ ] **Step 3: Add the column to the SELECTs and the model, regenerate**

`query/sqlite/users.sql` and `query/pgsql/users.sql`, both `GetUserByID` and `GetUserByEmail`:

```sql
SELECT id, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm, access_level, access_until, timezone, email_verified, credentials_generation
FROM users
WHERE id = ?;
```

(pgsql: `WHERE id = $1;` / `WHERE lower(email) = lower($1);`). Delete the `GetUserCredentialsGeneration` query from both files. Run `sqlc generate`.

`internal/model/user.go` — add after `AccessUntil`:

```go
	// CredentialsGeneration is the account-reclaim fence as of the row read
	// that produced this aggregate. A flow that authenticates with this row
	// (password hash, linked identity, provisioned user) presents the value at
	// write time; the reclaim bumps it, so a write built on a pre-reclaim read
	// affects zero rows. Zero for an aggregate that has never been persisted.
	CredentialsGeneration int64
```

`internal/user/repo/repo.go`: add `CredentialsGeneration int64` as the LAST field of `userRow` (the generated row structs list columns in SELECT order, and the whole-struct conversions require identical field order); in `hydrate` add `CredentialsGeneration: row.CredentialsGeneration`; delete `GetUserCredentialsGeneration` from the `querier` interface, the `Repo.CredentialsGeneration` method, and its `sqlite.go`/`pgsql.go` adapter methods. Delete `CredentialsGeneration` from `user.Repository` (`repository.go`) and from `user.Service` (`external.go`).

- [ ] **Step 4: Run the repo test — pass; run the user package — fix the two callers**

Run: `go test ./internal/user/...`
Expected: `login.go:38` and `pat.go:42` no longer compile. In `login.go` delete the `generation, gerr := s.repo.CredentialsGeneration(...)` block and pass `u.CredentialsGeneration` to `createSession`. In `pat.go` TEMPORARILY read the generation via `s.repo.GetByID(ctx, userID)` and pass `u.CredentialsGeneration` (Task 3 replaces this). Re-run: PASS.

- [ ] **Step 5: Failing oauth repo test — a deleted identity is not resurrected**

In `internal/oauth/repo/repo_integration_test.go`:

```go
func TestUpdateIfCurrent_DoesNotResurrectADeletedIdentity(t *testing.T) {
	db := dbtest.New(t)
	txm := backend.NewTxManager(db.Raw)
	users := userrepo.NewRepo(db.Engine, txm)
	ids := NewIdentityRepo(db.Engine, txm)
	ctx := context.Background()
	u := seedUser(t, db) // reuse whatever helper this file already uses to create a user row
	now := time.Now().UTC().Truncate(time.Second)
	id := model.NewIdentity(ids.NextIdentity(), u.ID, "google", "https://accounts.google.com", "sub-1", "s@x.test", now)
	if n, err := ids.InsertIfCurrent(ctx, id, 0); err != nil || n != 1 {
		t.Fatalf("insert: n=%d err=%v", n, err)
	}
	if _, err := ids.DeleteByUserProvider(ctx, u.ID, "google"); err != nil {
		t.Fatal(err)
	}
	if err := users.BumpCredentialsGeneration(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	// The flow read the identity before the reclaim, then reads the bumped
	// generation; an update must still find no row.
	id.UpdateEmail("t@x.test", now)
	n, err := ids.UpdateIfCurrent(ctx, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("UpdateIfCurrent resurrected the identity: n=%d", n)
	}
	if _, err := ids.GetByUserProvider(ctx, u.ID, "google"); err == nil {
		t.Fatal("identity exists again after the reclaim")
	}
}

func TestInsertIfCurrent_RefusesAStaleGeneration(t *testing.T) {
	db := dbtest.New(t)
	txm := backend.NewTxManager(db.Raw)
	users := userrepo.NewRepo(db.Engine, txm)
	ids := NewIdentityRepo(db.Engine, txm)
	ctx := context.Background()
	u := seedUser(t, db)
	if err := users.BumpCredentialsGeneration(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	id := model.NewIdentity(ids.NextIdentity(), u.ID, "google", "https://accounts.google.com", "sub-1", "s@x.test", time.Now())
	n, err := ids.InsertIfCurrent(ctx, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("stale insert wrote %d rows", n)
	}
}
```

- [ ] **Step 6: Run — expect compile failure on `InsertIfCurrent`/`UpdateIfCurrent`**

Run: `go test ./internal/oauth/repo/ -run 'TestUpdateIfCurrent|TestInsertIfCurrent'`
Expected: FAIL to compile.

- [ ] **Step 7: Replace the upsert with a fenced INSERT and a fenced UPDATE**

`query/sqlite/oauth.sql` (replace `UpsertIdentity` and `UpsertIdentityIfGeneration`):

```sql
-- name: InsertIdentityIfGeneration :execrows
-- Same fence as InsertAccessTokenIfGeneration: a callback that resolved its
-- user before an account reclaim must not land an identity after it. A plain
-- INSERT, never an upsert — an identity the reclaim deleted must stay deleted.
INSERT INTO users_identities (id, user_id, provider, issuer, subject, email, created_at, updated_at)
SELECT ?, ?, ?, ?, ?, ?, ?, ?
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = ? AND u.credentials_generation = ?);

-- name: UpdateIdentityIfGeneration :execrows
UPDATE users_identities
SET issuer = ?, subject = ?, email = ?, updated_at = ?
WHERE id = ?
  AND EXISTS (SELECT 1 FROM users u WHERE u.id = users_identities.user_id AND u.credentials_generation = ?);
```

pgsql sibling with `$1..$10` and `$1..$6`. Run `sqlc generate`.

`internal/oauth/repository.go` — replace `SaveIfCurrent` with:

```go
	// InsertIfCurrent writes a NEW identity only while the owner's credentials
	// generation still matches the one the flow resolved them under, reporting
	// the rows written (0 = an account reclaim landed mid-flow). Never an
	// upsert: a row the reclaim deleted must not come back.
	InsertIfCurrent(ctx context.Context, i *model.Identity, generation int64) (int64, error)
	// UpdateIfCurrent rewrites issuer/subject/email of an EXISTING identity
	// under the same fence; 0 rows means the row is gone or the generation moved.
	UpdateIfCurrent(ctx context.Context, i *model.Identity, generation int64) (int64, error)
```

`internal/oauth/repo/identity.go`: implement both over the new generated queries (`InsertIdentityIfGeneration` params: the 8 columns then `ID_2: i.UserID.String(), CredentialsGeneration: generation`; `UpdateIdentityIfGeneration` params: `Issuer, Subject, Email, UpdatedAt, ID, CredentialsGeneration`). Update `identityQuerier`, `identity_sqlite.go`, `identity_pgsql.go` accordingly and delete the `UpsertIdentity` adapter.

- [ ] **Step 8: Run the oauth repo tests — PASS; then run the oauth package — fix the service**

Run: `go test ./internal/oauth/repo/`
Expected: PASS. Then `go test ./internal/oauth/...` fails to compile: fix `callback.go` and `handoff.go` as follows.

`callback.go` step 5 (existing identity):

```go
	id, err := s.identities.GetByProviderSubject(ctx, provider, issuer, claims.Subject)
	if err == nil {
		u, uerr := s.users.FindByID(ctx, id.UserID)
		if uerr != nil { … unchanged … }
		if !u.IsActive { … unchanged … }
		// u.CredentialsGeneration is the fence value read WITH the user row;
		// the identity row was read before it, so a reclaim between the two
		// reads has deleted the identity and the fenced UPDATE finds no row.
		id.UpdateEmail(email, now)
		if n, serr := s.identities.UpdateIfCurrent(ctx, id, u.CredentialsGeneration); serr != nil || n != 1 {
			logWarn(ctx, "oauth callback: identity save", orReclaimed(serr), "provider", provider)
			return s.errorURLFor(st, "provider_error")
		}
		s.mirrorEmailDrift(ctx, u, email, provider)
		return s.mintHandoff(ctx, st, u.ID, provider, tokenForSession, u.CredentialsGeneration)
	}
```

Step 6: delete the `CredentialsGeneration` call; pass `u.CredentialsGeneration` to `autoLink` and `mintHandoff`. Step 7: delete the call; pass `u.CredentialsGeneration` (0 for a fresh row) and use `InsertIfCurrent`. `saveLinkedIdentity`: the `existing` branch uses `UpdateIfCurrent`, the not-found branch `InsertIfCurrent`. Delete the `CredentialsGeneration` method from `oauth.Users` (`ports.go`).

`link()` captures the fence for the deferred write:

```go
	owner, oerr := s.users.FindByID(ctx, st.LinkUserID)
	if oerr != nil {
		logWarn(ctx, "oauth link: owner lookup", oerr, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	// … existing taken/already-linked checks …
	if ierr := s.handoffs.Insert(ctx, &model.OAuthHandoff{…, Generation: owner.CredentialsGeneration, …}); ierr != nil {
```

`handoff.go` `CompleteLink`: delete the `gen, gerr := s.users.CredentialsGeneration(...)` block and use `h.Generation` in both writes (`UpdateIfCurrent` for the existing-identity branch, `InsertIfCurrent` for the new one). Update the comment: "The fence value was captured when the callback resolved the account (h.Generation); the write is refused if a reclaim bumped it since, whatever this request's session looked like at the middleware."

Update the test fakes: `fakeUsers` in `internal/oauth/service_test.go` and `internal/oauth/api/harness_test.go` drop `CredentialsGeneration` and instead keep a per-user generation on the stored `*model.User` (`reclaim()` increments `u.CredentialsGeneration`); `fakeIdentities` implements `InsertIfCurrent` (refuse when the stored user's generation differs, refuse duplicates) and `UpdateIfCurrent` (0 rows when the id is absent or the generation differs). `internal/server/glue_oauth_users.go`: delete the adapter method.

- [ ] **Step 9: Failing service test — the reclaim between identity read and user read is fenced**

In `internal/oauth/service_test.go` (using the existing fakes; `fakeUsers.reclaim(userID)` bumps the stored user's `CredentialsGeneration` and `fakeIdentities` drops that user's rows — extend `reclaim` to do both if it does not already):

```go
func TestCallback_ExistingIdentity_ReclaimBetweenIdentityAndUserReadIsRefused(t *testing.T) {
	h := newHarness(t) // the file's existing constructor
	u := h.users.seed(t, "victim@x.test", model.AlgorithmNone)
	h.identities.add(model.NewIdentity(vo.NewId(), u.ID, "oidc", h.idp.IssuerURL(), h.idp.Subject, "squatter@x.test", h.clock.Now()))
	// The reclaim lands after GetByProviderSubject and before FindByID.
	h.users.beforeFindByID = func() { h.users.reclaim(u.ID); h.identities.deleteByUser(u.ID) }
	url := h.completeLogin(t) // start-login → fake consent → Callback, returns the redirect
	if !strings.Contains(url, "oauthError=provider_error") {
		t.Fatalf("expected provider_error redirect, got %s", url)
	}
	if got := h.handoffs.count(); got != 0 {
		t.Fatalf("a handoff was minted after the reclaim: %d", got)
	}
	if _, err := h.identities.GetByProviderSubject(context.Background(), "oidc", h.idp.IssuerURL(), h.idp.Subject); err == nil {
		t.Fatal("the reclaimed identity was resurrected")
	}
}
```

Add the `beforeFindByID func()` hook to `fakeUsers.FindByID` (called once, then cleared). Adapt helper names to what the file already has (`newHarness`, `completeLogin`, `add`, `count` are the intent — reuse existing equivalents rather than inventing parallel ones).

- [ ] **Step 10: Run — must FAIL before the callback change was in place; verify by temporarily reverting**

Run: `go test ./internal/oauth/ -run TestCallback_ExistingIdentity_ReclaimBetweenIdentityAndUserReadIsRefused`
Expected with the Step 8 code: PASS. To prove the test bites, temporarily change `UpdateIfCurrent` in the fake to an upsert and re-run: FAIL ("resurrected"). Restore. PASS.

- [ ] **Step 11: Failing wiring test — password login racing a reset does not mint a session**

In `internal/server/oauth_wiring_test.go` (next to `TestRecovery_ReclaimsAnAccountFromASquatter`, reusing its setup), assert the ordering guarantee at the repo level with a real DB, since a true race needs a hook:

```go
func TestLogin_SessionInsertIsFencedByTheGenerationReadWithTheHash(t *testing.T) {
	db := dbtest.New(t)
	txm := backend.NewTxManager(db.Raw)
	users := userrepo.NewRepo(db.Engine, txm)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, txm)
	u := fixture.NewUser(t, db)
	ctx := context.Background()
	loaded, err := users.GetByEmail(ctx, u.Email) // evidence read: hash + generation in one row
	if err != nil {
		t.Fatal(err)
	}
	if err := users.BumpCredentialsGeneration(ctx, u.ID); err != nil { // the reset commits
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour)
	n, err := tokens.InsertIfGeneration(ctx, &model.AccessToken{ID: vo.NewId(), UserID: u.ID, Kind: model.TokenKindSession,
		TokenHash: "h", CreatedAt: time.Now(), LastUsedAt: time.Now(), ExpiresAt: &exp}, loaded.CredentialsGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("session minted on a pre-reclaim read: n=%d", n)
	}
}
```

Run: `go test ./internal/server/ -run TestLogin_SessionInsertIsFenced` — Expected: PASS (it pins the contract the Login change relies on; it FAILS if someone reintroduces a separate generation read by changing `loaded.CredentialsGeneration` to a fresh `GetByID` — verify that once by hand, then restore).

- [ ] **Step 12: Whole tier green, then commit**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./internal/user/... ./internal/oauth/... ./internal/server/... ./internal/test/apiparity/ ./internal/test/mcpparity/`
Expected: all PASS, `gofmt -l` prints nothing, no golden diffs.

```bash
git add -A internal/ && git commit -m "fix(user,oauth): read the reclaim fence with the evidence, never after it

The credentials generation now rides on the user row every flow loads as
its evidence (password hash, linked identity, provisioned user), so a
reset committing between two reads can no longer slip a session, a
handoff or an identity write past the fence. Identity persistence is
split into a fenced INSERT and a fenced UPDATE: a row the reclaim deleted
is never re-created by an upsert. Link handoffs capture the generation
at callback time and present it at complete-link.

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: The legacy sha512→argon2id rehash is fenced

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/users.sql` (add `UpdateUserPasswordIfGeneration`)
- Regenerate: `gen/`
- Modify: `internal/user/repository.go` (add `UpdatePasswordIfGeneration`), `internal/user/repo/repo.go` + `sqlite.go` + `pgsql.go`
- Modify: `internal/user/login.go:76-91` (`rehashLegacyPassword`)
- Test: `internal/user/login_test.go`

**Interfaces:**
- Produces: `Repository.UpdatePasswordIfGeneration(ctx, userID vo.Id, hash, salt, algorithm string, now time.Time, generation int64) (int64, error)`.

- [ ] **Step 1: Failing test — a login racing a reset must not revert the new password**

In `internal/user/login_test.go`, find how the file builds a `Service` over a real sqlite DB with a sha512 user (there is an existing legacy-rehash test; copy its setup), then:

```go
func TestLogin_LegacyRehashDoesNotOverwriteAConcurrentReset(t *testing.T) {
	s, repo, u, plaintext := newLegacyUserService(t) // the file's existing sha512 setup helper (or inline its body)
	ctx := context.Background()
	// Simulate the reset committing after Login read its evidence: bump the fence
	// and write a fresh argon2id hash for a DIFFERENT password.
	resetHash, _ := s.hasher.Hash("new-password-after-reset")
	loaded, _ := repo.GetByID(ctx, u.ID)
	loaded.UpdatePassword(resetHash, model.AlgorithmArgon2id, time.Now())
	if err := repo.Save(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	if err := repo.BumpCredentialsGeneration(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	stale, _ := repo.GetByID(ctx, u.ID)
	stale.Password, stale.Salt, stale.Algorithm, stale.CredentialsGeneration = u.Password, u.Salt, model.AlgorithmSHA512, 0
	s.rehashLegacyPassword(ctx, stale, plaintext, time.Now())
	after, _ := repo.GetByID(ctx, u.ID)
	if after.Password != resetHash {
		t.Fatal("the legacy rehash overwrote the password written by the reset")
	}
}
```

- [ ] **Step 2: Run — FAIL ("overwrote")**

Run: `go test ./internal/user/ -run TestLogin_LegacyRehashDoesNotOverwriteAConcurrentReset`
Expected: FAIL with the message above (the current code does `repo.Save(u)` unconditionally).

- [ ] **Step 3: Add the narrow fenced UPDATE and use it**

`query/sqlite/users.sql`:

```sql
-- name: UpdateUserPasswordIfGeneration :execrows
-- The opportunistic legacy-hash upgrade writes ONLY the credential columns and
-- only under the generation the login verified the hash under, so a reset
-- committing mid-login is never overwritten by a stale aggregate save.
UPDATE users SET password = ?, salt = ?, algorithm = ?, updated_at = ?
WHERE id = ? AND credentials_generation = ?;
```

pgsql: `$1..$6`. Run `sqlc generate`. Add to `user.Repository`:

```go
	// UpdatePasswordIfGeneration rewrites only the credential columns, and only
	// while the generation still matches (see login.go rehashLegacyPassword).
	UpdatePasswordIfGeneration(ctx context.Context, userID vo.Id, hash, salt, algorithm string, now time.Time, generation int64) (int64, error)
```

Implement in `repo.go` (+ querier + both adapters). `rehashLegacyPassword` becomes:

```go
func (s *Service) rehashLegacyPassword(ctx context.Context, u *model.User, plaintext string, now time.Time) {
	if u.Algorithm == model.AlgorithmArgon2id {
		return
	}
	newHash, err := s.hasher.Hash(plaintext)
	if err != nil {
		slog.WarnContext(ctx, "legacy password rehash: hashing failed", "err", err.Error())
		return
	}
	n, err := s.repo.UpdatePasswordIfGeneration(ctx, u.ID, newHash, u.Salt, model.AlgorithmArgon2id, now, u.CredentialsGeneration)
	if err != nil {
		slog.WarnContext(ctx, "legacy password rehash: persist failed", "err", err.Error())
		return
	}
	if n == 1 {
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, now)
	}
}
```

Any test double of `user.Repository` in the package (grep `UpsertOption(` in `*_test.go`) gains the method.

- [ ] **Step 4: Run — PASS; whole package green; commit**

Run: `go test ./internal/user/...`
Expected: PASS (including the existing rehash test — the happy path still upgrades the hash).

```bash
git add -A internal/ && git commit -m "fix(user): fence the legacy password rehash on the credentials generation

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: A personal token is minted only while the presenting credential is still live

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/access_tokens.sql` (add `InsertAccessTokenIfPresenterLive`)
- Regenerate: `gen/`
- Modify: `internal/user/repository.go` (`AccessTokens.InsertIfPresenterLive`), `internal/user/repo/accesstoken.go` + `_sqlite.go` + `_pgsql.go`
- Modify: `internal/user/pat.go` (signature gains `presentingTokenID vo.Id`), `internal/user/api/pat.go:47-49`, any MCP/other caller (`grep -rn "CreatePersonalToken(" internal/ --include='*.go' | grep -v _test`)
- Test: `internal/user/token_test.go` (or the PAT test file the package already has — `grep -ln CreatePersonalToken internal/user/*_test.go`)

**Interfaces:**
- Produces: `Service.CreatePersonalToken(ctx, userID, presentingTokenID vo.Id, req model.CreatePersonalTokenRequest)`; `AccessTokens.InsertIfPresenterLive(ctx, t *model.AccessToken, presentingTokenID vo.Id) (int64, error)`.
- Consumes: `middleware.TokenIDFromCtx(ctx) (vo.Id, bool)` (exists).

- [ ] **Step 1: Failing test — a PAT minted behind a revoked session is refused**

```go
func TestCreatePersonalToken_RefusedWhenThePresentingSessionWasRevoked(t *testing.T) {
	s, tokens, u := newTokenService(t) // the file's existing sqlite-backed setup
	ctx := context.Background()
	raw, _ := s.Login(ctx, model.LoginRequest{Username: u.Email, Password: "secret"}, "ua", time.Now())
	sessionID, _, _, _ := s.Authenticate(ctx, raw.Token)
	_, sessionTokenID, _, _ := s.Authenticate(ctx, raw.Token) // Authenticate returns (userID, tokenID, level, err)
	_ = sessionID
	// The reclaim lands after the middleware authenticated this request.
	if err := s.revokeTokens(ctx, u.ID, vo.Id{}, time.Now(), model.TokenKindSession, model.TokenKindPersonal); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreatePersonalToken(ctx, u.ID, sessionTokenID, model.CreatePersonalTokenRequest{Name: "mcp"})
	var unauthorized *errs.UnauthorizedError
	if !errors.As(err, &unauthorized) || unauthorized.Msg != "Invalid access token" {
		t.Fatalf("want 401 Invalid access token, got %v", err)
	}
	rows, _ := tokens.ListByUser(ctx, u.ID, model.TokenKindPersonal)
	if len(rows) != 0 {
		t.Fatalf("a PAT row was written behind a revoked session: %d", len(rows))
	}
}
```

(Check `Authenticate`'s actual return order in `internal/user/authenticate.go:13` and use it; the intent is "the token id the middleware would put in the context".)

- [ ] **Step 2: Run — FAIL to compile (3-arg call)**

Run: `go test ./internal/user/ -run TestCreatePersonalToken_RefusedWhenThePresentingSessionWasRevoked`

- [ ] **Step 3: Implement the presenter fence**

`query/sqlite/access_tokens.sql`:

```sql
-- name: InsertAccessTokenIfPresenterLive :execrows
-- A personal token is minted only while the credential that authenticated the
-- request is still unrevoked. The reclaim revokes every token in the same
-- transaction that bumps the generation, so a request that passed the auth
-- middleware before the reclaim inserts nothing after it.
INSERT INTO access_tokens (id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token)
SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
WHERE EXISTS (SELECT 1 FROM access_tokens p WHERE p.id = ? AND p.user_id = ? AND p.revoked_at IS NULL);
```

pgsql `$1..$14`. `sqlc generate`. Add `InsertIfPresenterLive` to the `AccessTokens` interface (doc: "used for PATs; the presenting token id is the request's authenticated credential"), implement in the repo + adapters.

`pat.go`: signature `CreatePersonalToken(ctx context.Context, userID, presentingTokenID vo.Id, req model.CreatePersonalTokenRequest)`; replace the temporary `GetByID` block from Task 1 with:

```go
	n, err := s.tokens.InsertIfPresenterLive(ctx, t, presentingTokenID)
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, errs.NewUnauthorized("Invalid access token")
	}
```

`internal/user/api/pat.go`:

```go
func (h *Handlers) CreatePersonalToken(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.CreatePersonalTokenRequest) (*model.CreatePersonalTokenResult, error) {
		tokenID, _ := middleware.TokenIDFromCtx(ctx)
		return h.svc.CreatePersonalToken(ctx, userID, tokenID, req)
	})
}
```

Fix every other caller the grep finds the same way (MCP tools use `reqctx`/middleware the same way the REST edge does — look at how `revoke-session` obtains the current token id there and mirror it).

- [ ] **Step 4: Run — PASS; package + apiparity + mcpparity green; commit**

Run: `go test ./internal/user/... ./internal/test/apiparity/ ./internal/test/mcpparity/`
Expected: PASS, no golden diffs (a live session still mints a PAT exactly as before).

```bash
git add -A internal/ && git commit -m "fix(user): mint personal tokens only while the presenting credential is live

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: One reclaim primitive, used by reset-password AND the CLI

**Files:**
- Modify: `internal/user/password.go:150-208` (`ResetPassword`), `internal/user/admin.go:66-82` (`AdminChangePassword`)
- Create: `internal/user/reclaim.go`
- Modify: `internal/server/glue_user_reclaim.go` (export a constructor), `internal/cli/container.go:97-102` (wire the oauth service + reclaimer)
- Modify: `CLAUDE.md:716-720` (the cascade sentence), `docs/regression-test-plan.md` (the "Password reset is a full reclaim" item gains the CLI)
- Test: `internal/user/session_cascade_test.go:74` (`TestAdminChangePassword_RevokesAllSessionsKeepsPATs` → rewrite), `internal/server/oauth_wiring_test.go`

**Interfaces:**
- Produces: `func (s *Service) reclaimCredentials(ctx context.Context, u *model.User, provenEmail string) error` (unexported; must run inside the caller's transaction).
- Produces: `server.NewOAuthReclaimer(svc *appoauth.Service) appuser.OAuthReclaimer` (exported wrapper over the existing `oauthIdentityReclaimer`).

- [ ] **Step 1: Failing test — CLI change-password revokes PATs, bumps the fence, and reclaims identities**

Rewrite `TestAdminChangePassword_RevokesAllSessionsKeepsPATs` in `session_cascade_test.go` as:

```go
func TestAdminChangePassword_IsAFullReclaim(t *testing.T) {
	s, repo, tokens, u := newCascadeService(t) // the file's existing setup helper
	ctx := context.Background()
	seedLiveSession(t, tokens, u.ID)
	seedLivePAT(t, tokens, u.ID)
	rec := &recordingReclaimer{}
	s.SetOAuthReclaimer(rec)
	before, _ := repo.GetByID(ctx, u.ID)
	if err := s.AdminChangePassword(ctx, u.Email, "new-password-123"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{model.TokenKindSession, model.TokenKindPersonal} {
		rows, _ := tokens.ListByUser(ctx, u.ID, kind)
		for _, r := range rows {
			if r.IsLive(time.Now()) {
				t.Fatalf("%s token still live after CLI change-password", kind)
			}
		}
	}
	after, _ := repo.GetByID(ctx, u.ID)
	if after.CredentialsGeneration != before.CredentialsGeneration+1 {
		t.Fatalf("generation %d → %d, want +1", before.CredentialsGeneration, after.CredentialsGeneration)
	}
	if rec.calls != 1 || rec.userID != u.ID || rec.provenEmail != strings.ToLower(u.Email) {
		t.Fatalf("oauth reclaim not invoked with the account's email: %+v", rec)
	}
}
```

`recordingReclaimer` implements `OAuthReclaimer` recording `(userID, provenEmail)` and returning `0, 0, nil` (add it to the file if `TestResetPassword_RollsBackWhenTheIdentityReclaimFails` does not already define a usable fake — reuse if it does).

- [ ] **Step 2: Run — FAIL (PAT still live / generation unchanged / reclaimer not called)**

Run: `go test ./internal/user/ -run TestAdminChangePassword_IsAFullReclaim`

- [ ] **Step 3: Extract the primitive and call it from both paths**

Create `internal/user/reclaim.go`:

```go
package user

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// reclaimCredentials takes away every way into the account that never proved
// the mailbox, in the caller's transaction, after the caller has written the
// new password hash. It is the ONE definition of "reclaim": the reset flow
// (mailbox proven by the code) and the operator's user:change-password both
// use it, so a credential class added later (a pending grant, a linked
// identity) is swept from both paths or from neither.
func (s *Service) reclaimCredentials(ctx context.Context, u *model.User, provenEmail string) error {
	if err := s.repo.BumpCredentialsGeneration(ctx, u.ID); err != nil {
		return err
	}
	if err := s.passwordRequests.DeleteByUser(ctx, u.ID); err != nil {
		return err
	}
	if err := s.emailChangeRequests.DeleteByUser(ctx, u.ID); err != nil {
		return err
	}
	if err := s.revokeTokens(ctx, u.ID, vo.Id{}, s.clock.Now(), model.TokenKindSession, model.TokenKindPersonal); err != nil {
		return err
	}
	if s.oauthGrants == nil {
		return nil
	}
	identities, grants, err := s.oauthGrants.ReclaimAccount(ctx, u.ID, strings.ToLower(strings.TrimSpace(provenEmail)))
	if err != nil {
		return err
	}
	if identities > 0 {
		reqctx.AddLogAttr(ctx, "identities_unlinked", identities)
	}
	if grants > 0 {
		reqctx.AddLogAttr(ctx, "oauth_grants_revoked", grants)
	}
	return nil
}
```

`ResetPassword`: inside `s.tx.WithTx`, after `s.repo.Save(ctx, u)`, replace everything from the `BumpCredentialsGeneration` call to the end of the closure with `return s.reclaimCredentials(ctx, u, lowered)`. Keep the long explanatory comment above the transaction but shorten it to point at `reclaimCredentials`.

`AdminChangePassword`:

```go
	email, derr := s.encode.Decode(u.Email)
	if derr != nil {
		return derr
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, s.clock.Now())
		if err := s.repo.Save(ctx, u); err != nil {
			return err
		}
		// The operator resetting a password is evicting whoever holds the
		// account; the account's own address is the proven one.
		return s.reclaimCredentials(ctx, u, email)
	}); err != nil {
		return err
	}
	return nil
```

(Delete the trailing `revokeSessions` call; the primitive revokes everything.) If `passwordRequests`/`emailChangeRequests` can be nil in the CLI container (check `container.go:97-102` — they are non-nil there) keep the primitive nil-unsafe; if any test constructs a `Service` with nil stores and hits this path, guard with `if s.passwordRequests != nil`.

`internal/server/glue_user_reclaim.go` add:

```go
// NewOAuthReclaimer exposes the reclaim adapter to the CLI container, which
// wires the same reset cascade behind user:change-password.
func NewOAuthReclaimer(svc *appoauth.Service) appuser.OAuthReclaimer { return oauthIdentityReclaimer{oauth: svc} }
```

`internal/cli/container.go`, after `userSvc := appuser.NewService(...)`:

```go
	// No providers are needed to sweep identities and pending grants; the CLI
	// never starts a flow.
	oauthSvc := appoauth.NewService(nil, server.NewOAuthUsers(userSvc),
		oauthrepo.NewIdentityRepo(cfg.DatabaseDriver, txm), oauthrepo.NewStateRepo(cfg.DatabaseDriver, txm),
		oauthrepo.NewHandoffRepo(cfg.DatabaseDriver, txm), clk, nil, cfg.AppURL, cfg.AllowRegistration)
	userSvc.SetOAuthReclaimer(server.NewOAuthReclaimer(oauthSvc))
```

(imports: `appoauth "github.com/econumo/econumo/internal/oauth"`, `oauthrepo "github.com/econumo/econumo/internal/oauth/repo"`.)

- [ ] **Step 4: Run — PASS; then an end-to-end wiring test through the CLI container is not needed, but the server one is: add to `oauth_wiring_test.go`**

```go
func TestAdminChangePassword_UnlinksTheSquattersIdentity(t *testing.T) {
	// Same fixture as TestRecovery_ReclaimsAnAccountFromASquatter: a password
	// account whose linked identity vouches for a DIFFERENT address.
	env := newReclaimFixture(t) // extract from TestRecovery_ReclaimsAnAccountFromASquatter if it is inline
	if err := env.users.AdminChangePassword(context.Background(), env.victimEmail, "operator-set-pw-1"); err != nil {
		t.Fatal(err)
	}
	ids, _ := env.identities.ListByUser(context.Background(), env.victimID)
	if len(ids) != 0 {
		t.Fatalf("squatter identity survived CLI change-password: %+v", ids)
	}
}
```

Run: `go test ./internal/user/... ./internal/server/... ./internal/cli/...` — PASS.

- [ ] **Step 5: Docs, then commit**

CLAUDE.md lines 716-720 (the authentication cascade sentence): rewrite the CLI clause to "`reset-password` and CLI `user:change-password` are the account RECLAIM: every session, every PAT, every pending grant (reset codes, pending email change, unredeemed OAuth handoffs and link states) and every linked identity whose provider vouches for a different address are removed in the password write's transaction; `update-password` (the user changing their own password) revokes only the OTHER sessions and keeps PATs and identities". Also update the "PATs survive password changes" sentence in the Access-token section (`grep -n "PATs survive" CLAUDE.md`) to "PATs survive `update-password`; `reset-password` and `user:change-password` revoke them". Regression plan: in the "Password reset is a full reclaim" item add a final sentence "CLI `user:change-password <email> <new>` performs the same reclaim (sessions, tokens, foreign identity, pending grants)."

```bash
git add -A && git commit -m "fix(user): make CLI user:change-password the same reclaim as reset-password

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: Google's `email_verified` claim is honoured (no blanket trust)

**Files:**
- Modify: `internal/oauth/providers.go:33` (`TrustEmail: false` for Google; Apple unchanged)
- Modify: `internal/oauth/providers_test.go:25`
- Modify: `docs/superpowers/specs/2026-09-07-oauth-login-design.md:300-302`, `docs/oidc-setup.md:39-40`, `CLAUDE.md` (the "Google and Apple are fixed issuers" clause near line 476), `docs/regression-test-plan.md` (the "unverified email" item)
- Test: `internal/oauth/service_test.go`

- [ ] **Step 1: Failing test — a Google token with `email_verified:false` is rejected**

In `internal/oauth/service_test.go`, using the fake issuer as the Google slot (build the `Provider` with `ID: model.OAuthProviderGoogle` over `h.idp.Issuer(model.OAuthProviderGoogle, false)` — replicate exactly how `ProvidersFromConfig` builds Google but pointed at the fake: `TrustEmail` MUST come from `ProvidersFromConfig`'s value, so instead assert on the config builder):

```go
func TestProvidersFromConfig_GoogleDoesNotTrustUnverifiedEmail(t *testing.T) {
	cfg := config.Config{OAuthGoogleClientID: "id", OAuthGoogleClientSecret: "s"}
	ps, err := ProvidersFromConfig(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ps[0].Client.Issuer().TrustEmail {
		t.Fatal("Google slot must honour email_verified: Google documents false for unverified addresses")
	}
}
```

And in `providers_test.go:25` flip the existing assertion from `!g.TrustEmail` to `g.TrustEmail` (i.e. fail if trusted).

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/oauth/ -run 'TestProvidersFromConfig'`

- [ ] **Step 3: Flip the flag with the reason**

`providers.go` Google slot: `TrustEmail: false,` with the comment: `// Google always sends email_verified and documents false for unverified sign-in addresses; trusting the claim blindly would only ever matter in that one case.` Apple stays `TrustEmail: true` (Apple verifies every Apple ID address and its claim is a string, already parsed by `parseEmailVerified`; the review ruled to leave it).

- [ ] **Step 4: Run — PASS; docs; commit**

Run: `go test ./internal/oauth/...` — PASS.

Spec §6.2 step 4: replace "Google and Apple are configured with `TrustEmail = true`" with "Apple is configured with `TrustEmail = true` (every Apple ID address is verified); Google with `TrustEmail = false` — Google always sends `email_verified` and documents `false` for unverified addresses, so the claim is honoured (review ruling 2026-09-14)". `docs/oidc-setup.md:39-40`: "Google's email claim is used only when Google reports it as verified (`email_verified`); an unverified Google address is rejected with 'The sign-in provider has not verified this email address.'" CLAUDE.md: same one-line change where the Google/Apple trust is described. Regression plan "unverified email" item: append "(this includes a Google account whose sign-in address Google reports as unverified)".

```bash
git add -A && git commit -m "fix(oauth): honour Google's email_verified claim instead of trusting it blindly

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: Auth hot path stays narrow; unfenced write paths removed

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/access_tokens.sql` (`GetAccessTokenByHash` drops `t.provider, t.id_token`; delete `InsertAccessToken`)
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/*.sql` — delete the password-request `Delete`-by-id query (find it: `grep -rn "DeleteUserPasswordRequest\b\|name: DeletePasswordRequest" internal/infra/storage/sqlc/query`)
- Regenerate: `gen/`
- Modify: `internal/user/repo/accesstoken.go:101-109` (`tokenRowFromHashRow` leaves `Provider`/`IDToken` nil), `internal/user/repository.go:80` (delete `AccessTokens.Insert`) and `:128` (delete `PasswordRequests.Delete`), their repo methods and adapters
- Modify: test callers of `tokens.Insert` (`grep -rn "\.Insert(ctx, tok\|tokens.Insert(" internal --include='*_test.go'`) → `InsertIfGeneration(ctx, tok, 0)`

- [ ] **Step 1: Failing test — GetByHash does not load the id_token**

In `internal/user/repo/accesstoken_integration_test.go`:

```go
func TestGetByHash_DoesNotCarryTheIDToken(t *testing.T) {
	db := dbtest.New(t)
	r := NewAccessTokenRepo(db.Engine, backend.NewTxManager(db.Raw))
	u := fixture.NewUser(t, db)
	ctx := context.Background()
	idToken, provider := "eyJ.stub.token", "oidc"
	exp := time.Now().Add(time.Hour)
	tok := &model.AccessToken{ID: vo.NewId(), UserID: u.ID, Kind: model.TokenKindSession, TokenHash: "h1",
		CreatedAt: time.Now(), LastUsedAt: time.Now(), ExpiresAt: &exp, Provider: &provider, IDToken: &idToken}
	if n, err := r.InsertIfGeneration(ctx, tok, 0); err != nil || n != 1 {
		t.Fatalf("insert: %d %v", n, err)
	}
	got, _, _, err := r.GetByHash(ctx, "h1")
	if err != nil {
		t.Fatal(err)
	}
	if got.IDToken != nil {
		t.Fatal("the per-request auth path must not load id_token")
	}
	byID, _ := r.GetByID(ctx, tok.ID)
	if byID.IDToken == nil || *byID.IDToken != idToken || byID.Provider == nil {
		t.Fatal("GetByID must still carry provider and id_token for logout")
	}
}
```

- [ ] **Step 2: Run — FAIL ("must not load id_token")**

- [ ] **Step 3: Narrow the query, delete the dead methods**

Both engines' `GetAccessTokenByHash` SELECT lists drop `t.provider, t.id_token` (keep the comment). Delete `InsertAccessToken` and the password-request delete-by-id query from both engines. `sqlc generate`. `tokenRowFromHashRow` no longer sets `Provider`/`IDToken`. Delete `AccessTokens.Insert` + `PasswordRequests.Delete` from `repository.go`, their implementations, the querier entries and adapters. Update test callers to `InsertIfGeneration(ctx, tok, 0)` (generation defaults to 0 on a fresh user row, migration `20260913000000`).

- [ ] **Step 4: Run — PASS; whole Go smoke tier; commit**

Run: `go build ./... && go vet ./... && go test ./internal/user/... ./internal/test/apiparity/ ./internal/test/mcpparity/`
Expected: PASS, no golden changes (session list/logout still read provider via `GetByID`/`ListByUser`).

```bash
git add -A internal/ && git commit -m "refactor(user): keep id_token off the auth hot path, drop unfenced token insert

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: Sweep throttle on start-login; Google's bare issuer alias

**Files:**
- Modify: `internal/oauth/service.go` (add `lastSweep atomic.Int64` to `Service`), `internal/oauth/start.go:50-56`
- Modify: `internal/infra/oidc/client.go` (`Issuer.IssuerAliases []string`; `VerifyIDToken` accepts an alias), `internal/oauth/providers.go` (Google gets `IssuerAliases: []string{"accounts.google.com"}`)
- Test: `internal/oauth/service_test.go`, `internal/infra/oidc/jwt_more_test.go`

- [ ] **Step 1: Failing test — two starts inside a minute sweep once**

Add a call counter to the `fakeStates`/`fakeHandoffs` in `service_test.go` (`deleteExpiredCalls int`), then:

```go
func TestStart_SweepsExpiredRowsAtMostOncePerMinute(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 3; i++ {
		if _, err := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "oidc", Client: "web"}); err != nil {
			t.Fatal(err)
		}
	}
	if h.states.deleteExpiredCalls != 1 || h.handoffs.deleteExpiredCalls != 1 {
		t.Fatalf("sweeps: states=%d handoffs=%d, want 1/1", h.states.deleteExpiredCalls, h.handoffs.deleteExpiredCalls)
	}
	h.clock.advance(2 * time.Minute) // the harness's fixed clock helper (add one if missing)
	if _, err := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "oidc", Client: "web"}); err != nil {
		t.Fatal(err)
	}
	if h.states.deleteExpiredCalls != 2 {
		t.Fatalf("sweep did not re-arm after the interval: %d", h.states.deleteExpiredCalls)
	}
}
```

- [ ] **Step 2: Run — FAIL (3 sweeps)**

- [ ] **Step 3: Throttle**

`service.go`: add `lastSweep atomic.Int64 // unix seconds of the last expired-row sweep` to `Service` and `const sweepInterval = time.Minute`. `start.go` replace the two `DeleteExpired` calls with `s.sweepExpired(ctx, now)`:

```go
// sweepExpired purges expired states and handoffs at most once per
// sweepInterval per process: start-login is anonymous, and three write
// statements per click (two almost-always-empty DELETEs plus the insert)
// contend with real user writes on SQLite's single writer.
func (s *Service) sweepExpired(ctx context.Context, now time.Time) error {
	last := s.lastSweep.Load()
	if now.Unix()-last < int64(sweepInterval/time.Second) || !s.lastSweep.CompareAndSwap(last, now.Unix()) {
		return nil
	}
	if _, err := s.states.DeleteExpired(ctx, now); err != nil {
		return err
	}
	_, err := s.handoffs.DeleteExpired(ctx, now)
	return err
}
```

- [ ] **Step 4: Failing test — Google's bare `iss` is accepted only for the google slot**

In `internal/infra/oidc/jwt_more_test.go` (see how the file signs a token via `oidctest.Fake.SignIDToken` and verifies with a `Client` over `f.Issuer(...)`):

```go
func TestVerifyIDToken_AcceptsConfiguredIssuerAlias(t *testing.T) {
	f := oidctest.New(t)
	iss := f.Issuer("google", false)
	iss.IssuerAliases = []string{"accounts.google.com"}
	c := oidc.NewClient(iss, f.Server.Client())
	claims := f.Claims("nonce-1") // the fake's claims builder; export a wrapper if `claims` is unexported
	claims["iss"] = "accounts.google.com"
	if _, err := c.VerifyIDToken(context.Background(), f.SignIDToken(claims), "nonce-1", time.Now()); err != nil {
		t.Fatalf("alias rejected: %v", err)
	}
	plain := oidc.NewClient(f.Issuer("oidc", false), f.Server.Client())
	if _, err := plain.VerifyIDToken(context.Background(), f.SignIDToken(claims), "nonce-1", time.Now()); err == nil {
		t.Fatal("an unconfigured alias must still be rejected")
	}
}
```

Run: FAIL (compile: `IssuerAliases`).

- [ ] **Step 5: Implement the alias**

`client.go` `Issuer` gains `IssuerAliases []string // extra literal iss values accepted for this issuer (Google documents both "https://accounts.google.com" and "accounts.google.com")`. In `VerifyIDToken` replace the issuer check with:

```go
	iss := strings.TrimSuffix(cl.Iss, "/")
	if iss != strings.TrimSuffix(d.Issuer, "/") && iss != c.issuer.IssuerURL && !slices.Contains(c.issuer.IssuerAliases, iss) {
		return Claims{}, fmt.Errorf("%w: issuer %q", ErrInvalidToken, cl.Iss)
	}
```

`providers.go` Google: `IssuerAliases: []string{"accounts.google.com"},`.

- [ ] **Step 6: Run — PASS; commit**

Run: `go test ./internal/infra/oidc/... ./internal/oauth/...`

```bash
git add -A internal/ && git commit -m "fix(oauth): throttle the expired-row sweep; accept Google's bare issuer alias

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: An app-client flow with an unknown state returns to the app

**Files:**
- Modify: `internal/oauth/start.go` (state carries the client prefix), `internal/oauth/callback.go:31-39`
- Modify: `internal/test/apiparity/normalize.go:53` (`oauthParamRe`)
- Regenerate goldens (intended change: `state=` values in authorization URLs now read `<redacted>` with a `web.`/`app.` prefix)
- Test: `internal/oauth/service_test.go`

- [ ] **Step 1: Failing test**

```go
func TestCallback_UnknownStateFromTheAppReturnsToTheAppScheme(t *testing.T) {
	h := newHarness(t)
	url := h.svc.Callback(context.Background(), "oidc", CallbackInput{Code: "c", State: "app.doesnotexist"})
	if url != "econumo://oauth?error=invalid_state" {
		t.Fatalf("got %s", url)
	}
	if web := h.svc.Callback(context.Background(), "oidc", CallbackInput{Code: "c", State: "web.doesnotexist"}); !strings.HasPrefix(web, h.svc.appURL+"/login?oauthError=invalid_state") {
		t.Fatalf("got %s", web)
	}
}
```

Run: FAIL (the app case returns the web URL).

- [ ] **Step 2: Prefix the state and read the prefix on a miss**

`start.go`: after minting `state`, `state = req.Client + "." + state` (before hashing; the hash and the authorization URL both use the prefixed value). `callback.go`:

```go
	st, err := s.consumeState(ctx, provider, in.State)
	if err != nil {
		logWarn(ctx, "oauth callback: state", err, "provider", provider)
		// The row (and with it the intent) is gone; the client prefix on the
		// state value itself still says which surface to report on. It is a
		// display hint only — nothing is authorized by it.
		return s.errorURL(clientFromState(in.State), "invalid_state")
	}
```

```go
func clientFromState(state string) string {
	if strings.HasPrefix(state, model.OAuthClientApp+".") {
		return model.OAuthClientApp
	}
	return model.OAuthClientWeb
}
```

`normalize.go`: `oauthParamRe = regexp.MustCompile(`([?&](?:state|nonce|code_challenge)=)(?:web\.|app\.)?[A-Za-z0-9_-]{43}`)` and keep the replacement so the golden reads `state=<redacted>` (check what the existing replacement string is and keep it).

- [ ] **Step 3: Run, regenerate goldens, inspect, commit**

Run: `go test ./internal/oauth/... && UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff --stat internal/test/apiparity/testdata`
Expected: only oauth start scenarios change, and only in the `state=` value (the prefix is now redacted with the token). Any other diff = bug. Then `go test ./internal/test/apiparity/ ./internal/test/mcpparity/` PASS.

Regression plan: in the "📱 App: starting provider sign-in opens the in-app browser sheet" item add "an expired or already-used attempt returns to the app's login page with 'The sign-in attempt expired or was already used.' (not a web page inside the sheet)".

```bash
git add -A && git commit -m "fix(oauth): report an unknown state on the surface the flow started from

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: Providers built once; boot probe after listen; fixed fake issuer replaces the loopback normalizer

**Files:**
- Modify: `internal/server/server.go:90-101` (`Seams.OAuthProviders []appoauth.Provider`, `Seams.OAuthHTTPClient *http.Client`), `:199-201`
- Modify: `cmd/econumo/main.go:183-187`, `:229`, `:335-363`
- Modify: `internal/infra/oidc/oidctest/fake.go` (`PublicURL` + `Transport()`), `internal/test/apiparity/harness.go:94-110,132`, `internal/test/apiparity/normalize.go` (delete `fakeIssuerRe`)
- Regenerate goldens (intended: `<issuer>` becomes the literal `http://idp.example.test`)
- Modify: `CLAUDE.md` (the "serve probes every configured issuer once at boot" sentence)
- Test: `cmd/econumo/main_test.go` (if present) or `internal/server/oauth_wiring_test.go`

- [ ] **Step 1: Failing test — Build reuses injected providers**

In `internal/server/oauth_wiring_test.go`:

```go
func TestBuild_UsesInjectedProviders(t *testing.T) {
	db := dbtest.New(t)
	f := oidctest.New(t)
	cfg := baseConfig(db) // the file's existing config helper; no OIDC vars set
	injected := []appoauth.Provider{{Name: "Injected", Client: oidc.NewClient(f.Issuer("oidc", false), f.Server.Client())}}
	h := server.BuildAPI(cfg, db.Raw, server.Seams{OAuthProviders: injected})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/oauth/get-provider-list", nil))
	if !strings.Contains(rr.Body.String(), `"name":"Injected"`) {
		t.Fatalf("provider list did not come from the seam: %s", rr.Body.String())
	}
}
```

Run: FAIL to compile (`OAuthProviders`).

- [ ] **Step 2: Seams + main**

`server.go` `Seams`:

```go
	// OAuthProviders, when non-nil, are the provider clients to mount; serve
	// builds them once (so the boot probe warms the same discovery cache the
	// server uses) and tests inject fakes. nil builds them from cfg.
	OAuthProviders []appoauth.Provider
	// OAuthHTTPClient is the HTTP client for provider discovery/token/JWKS
	// calls when providers are built from cfg (nil = the default 10s client);
	// the apiparity harness maps a fixed literal issuer onto its fake.
	OAuthHTTPClient *http.Client
```

`Build`: `oauthProviders := seams.OAuthProviders; if oauthProviders == nil { oauthProviders, err = appoauth.ProvidersFromConfig(cfg, seams.OAuthHTTPClient); if err != nil { return … } }`.

`main.go`: replace the probe loop before `PORT` with building once: `providers, perr := oauth.ProvidersFromConfig(cfg, nil); if perr != nil { return perr }` (a bad Apple key still fails boot, now here). Pass `server.Seams{Updates: updates, OAuthProviders: providers}`. After the servers have started listening (find the goroutine that calls `ListenAndServe` and put this right after the `slog.Info("listening"…)` line or equivalent), run `go probeProviders(ctx, providers)`:

```go
// probeProviders fetches each provider's discovery document once, after the
// listener is up, so an unreachable issuer shows in the boot log without
// delaying startup; each provider gets its own timeout so one slow issuer
// cannot make the others report a false failure.
func probeProviders(ctx context.Context, providers []oauth.Provider) {
	for _, p := range providers {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if _, err := p.Client.Discover(pctx); err != nil {
			slog.Warn("oauth provider discovery failed at boot; sign-in through it will fail until it is reachable", "provider", p.Client.Issuer().ID, "err", err.Error())
		}
		cancel()
	}
}
```

Delete `probeResult` and `oauthProbe`; update any test of `oauthProbe` to test `probeProviders` (assert one WARN per unreachable provider by pointing at a closed `httptest` server; a reachable `oidctest.New(t)` logs nothing).

- [ ] **Step 3: Failing golden expectation — a literal issuer in the goldens**

`oidctest/fake.go`: add `PublicURL string` to `Fake`; `IssuerURL()` returns `f.PublicURL` when set, else `f.Server.URL`; every place that builds `f.Server.URL + "/…"` in `discovery` and the `iss` claim uses `f.IssuerURL()` instead. Add:

```go
// Transport routes requests for PublicURL to the in-process server, so a
// client configured with a fixed, deterministic issuer (goldens compare
// byte-for-byte across runs and engines) still talks to this fake.
func (f *Fake) Transport() http.RoundTripper {
	target := f.Server.Listener.Addr().String()
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r2 := r.Clone(r.Context())
		r2.URL.Scheme, r2.URL.Host, r2.Host = "http", target, target
		return http.DefaultTransport.RoundTrip(r2)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
```

Harness: `fakeIDP := oidctest.New(t); fakeIDP.PublicURL = "http://idp.example.test"`, `OIDCIssuerURL: fakeIDP.IssuerURL()` (now the literal), and `server.Seams{…, OAuthHTTPClient: &http.Client{Transport: fakeIDP.Transport(), Timeout: 10 * time.Second}}`. Update the harness comment (lines 88-93). Delete `fakeIssuerRe` and its use in `NormalizeGolden`.

- [ ] **Step 4: Regenerate goldens, inspect, run everything, commit**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/ && git diff internal/test/apiparity/testdata | grep '^[-+]' | grep -v '^[-+][-+]' | sort | uniq -c | sort -rn | head`
Expected: every changed line replaces `<issuer>` with `http://idp.example.test` and nothing else. Then `go test ./internal/server/... ./internal/test/... ./cmd/...` PASS. CLAUDE.md: "serve builds the provider clients once, probes each configured issuer's discovery document in the background right after the listener is up (one 5-second timeout per provider) and logs a WARN (never fails boot) when one is unreachable".

```bash
git add -A && git commit -m "fix(server): build OAuth providers once and probe issuers after listen; fixed fake issuer in goldens

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 10: "Set a password" signs the user out cleanly

**Files:**
- Modify: `web/src/features/auth/RecoveryDialog.tsx:17,34-41` (new optional `onSuccess` prop)
- Modify: `web/src/features/settings/ProfilePage.tsx:258`
- Modify: `docs/regression-test-plan.md:420-424`
- Test: `web/src/features/settings/ProfilePage.test.tsx`

- [ ] **Step 1: Failing test — after a successful reset from Settings the SPA navigates to logout**

In `ProfilePage.test.tsx` (follow the file's existing render helper and msw usage; it already renders a passwordless user for the "Set a password" row):

```tsx
it('signs out after a passwordless user sets a password (the reset revoked this session)', async () => {
  server.use(
    http.post('*/api/v1/user/remind-password', () => HttpResponse.json({ success: true, message: '', data: {} })),
    http.post('*/api/v1/user/reset-password', () => HttpResponse.json({ success: true, message: '', data: {} })),
  )
  const { navigate } = renderProfile({ hasPassword: false }) // the helper returns the mocked navigate spy the file already uses
  await userEvent.click(await screen.findByText('Set a password'))
  await userEvent.click(screen.getByRole('button', { name: /send code/i }))
  await userEvent.type(await screen.findByLabelText(/code/i), '123456')
  await userEvent.type(screen.getByLabelText(/new password/i), 'Password123!')
  await userEvent.click(screen.getByRole('button', { name: /change password|set password/i }))
  await waitFor(() => expect(navigate).toHaveBeenCalledWith('/logout'))
})
```

(Read the labels/buttons from `RecoveryDialog.tsx` and `locales/en.json` — the regex names above are intent; use the exact text.)

Run: `cd web && pnpm test -- ProfilePage` — FAIL (navigate not called).

- [ ] **Step 2: Implement**

`RecoveryDialog`: prop `onSuccess?: () => void`; in `changePassword`, after `await reset.mutateAsync(...)`: `if (onSuccess) { onSuccess() } else { onClose() }`. `ProfilePage.tsx:258`: `<RecoveryDialog open onClose={() => setRecoveryOpen(false)} onSuccess={() => navigate(RouterPage.LOGOUT)} email={user?.email} />` — the page already has `navigate` (it uses it for the logout confirm at line 264). LogoutPage then calls `logout-user` with the revoked token (a 401 it already tolerates: "best effort"), clears the persisted cache, and lands on `/login`.

- [ ] **Step 3: Run — PASS; lint; docs; commit**

Run: `cd web && pnpm test -- ProfilePage RecoveryDialog LoginPage && pnpm lint && npx tsc -b`.
Regression plan item (lines 420-424): "...after entering the code and a new password the app signs the user out (the reset ends every session, including this one) and lands on the login page; signing in with the new password works and Settings → Profile now shows 'Change password' instead of 'Set a password'."

```bash
git add -A web docs && git commit -m "fix(web): sign out after setting a password from Settings

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 11: Provider buttons — no double start in the app, hidden when the web flow cannot complete

**Files:**
- Modify: `web/src/features/auth/oauthQueries.ts` (in-flight store + `browserFinished` listener), `web/src/features/auth/ProviderButtons.tsx`
- Test: `web/src/features/auth/oauthQueries.test.tsx`, `web/src/features/auth/ProviderButtons.test.tsx`

**Interfaces:**
- Produces: `useOAuthInFlight(): boolean` and `markOAuthFlowOpen()` / `markOAuthFlowClosed()` (zustand store, module-local; the store file pattern is `web/src/app/uiStore.ts`).

- [ ] **Step 1: Failing tests**

`oauthQueries.test.tsx`:

```tsx
it('in the app, a flow stays in flight until the browser sheet finishes or the flow secret is taken', async () => {
  const listeners: Record<string, () => void> = {}
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: {
    open: vi.fn().mockResolvedValue(undefined),
    close: vi.fn().mockResolvedValue(undefined),
    addListener: vi.fn((ev: string, cb: () => void) => { listeners[ev] = cb }),
  } } }
  openAuthorizationUrl('https://idp/x')
  expect(useOAuthInFlight.getState().inFlight).toBe(true)
  listeners.browserFinished()
  expect(useOAuthInFlight.getState().inFlight).toBe(false)
  openAuthorizationUrl('https://idp/y')
  rememberOAuthFlow('f')
  takeOAuthFlow()
  expect(useOAuthInFlight.getState().inFlight).toBe(false)
})
```

`ProviderButtons.test.tsx`:

```tsx
it('keeps the buttons disabled while an app flow is in flight', async () => {
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { open: vi.fn().mockResolvedValue(undefined), addListener: vi.fn() } } }
  server.use(
    http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }] })),
    http.post('*/api/v1/oauth/start-login', () => HttpResponse.json({ success: true, message: '', data: { url: 'https://idp/google', flow: 'f1' } })),
  )
  renderButtons()
  const button = await screen.findByRole('button')
  await userEvent.click(button)
  await waitFor(() => expect(button).toBeDisabled())
  delete window.Capacitor
})

it('hides the buttons on the web when the SPA is served from a different origin than the backend', async () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: true }
  localStorage.setItem('selfHosted', JSON.stringify(true))
  localStorage.setItem('backendHost', JSON.stringify('https://money.example.org'))
  server.use(http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }] })))
  const { container } = renderButtons()
  await new Promise((r) => setTimeout(r, 20))
  expect(container.querySelector('[data-testid="provider-buttons"]')).toBeNull()
})
```

(Check `selfHosted()`/`getItem` in `web/src/lib/config.ts`/`storage.ts` for the exact storage keys and encoding — `setItem` JSON-encodes.)

Run: `cd web && pnpm test -- oauthQueries ProviderButtons` — FAIL.

- [ ] **Step 2: Implement**

`oauthQueries.ts`:

```ts
import { create } from 'zustand'

// The app leaves the WebView for the browser sheet and the start mutation
// resolves as soon as the sheet is asked to open, so `isPending` alone lets a
// second tap mint a second flow whose secret overwrites the first's. The
// sheet's own lifecycle (browserFinished) or the deep-link return (which
// takes the flow secret) is what ends the flow.
export const useOAuthInFlight = create<{ inFlight: boolean; set: (v: boolean) => void }>((set) => ({
  inFlight: false,
  set: (inFlight) => set({ inFlight }),
}))

let browserFinishedInstalled = false

export function openAuthorizationUrl(url: string): void {
  const browser = nativePlugin<BrowserPlugin>('Browser')
  if (browser) {
    if (!browserFinishedInstalled && browser.addListener) {
      browserFinishedInstalled = true
      browser.addListener('browserFinished', () => useOAuthInFlight.getState().set(false))
    }
    useOAuthInFlight.getState().set(true)
    void browser.open({ url })
    return
  }
  window.location.assign(url)
}
```

`takeOAuthFlow()` calls `useOAuthInFlight.getState().set(false)` before returning. `BrowserPlugin` in `web/src/lib/externalLinks.ts` gains `addListener?(ev: 'browserFinished', cb: () => void): unknown`. Reset `browserFinishedInstalled` in tests via an exported `__resetOAuthInFlightForTests()` only if needed (prefer re-registering harmlessly: guard with the flag, and in the test create a fresh `window.Capacitor` per test).

`ProviderButtons.tsx`: `const inFlight = useOAuthInFlight((s) => s.inFlight)`; `disabled={start.isPending || inFlight}`; and before the provider-list check:

```tsx
  // On the web the flow returns to the BACKEND's origin (ECONUMO_URL), where
  // this tab's sessionStorage flow secret does not exist; a SPA pointed at a
  // different backend cannot finish the exchange, so it must not offer it.
  if (!isNativeApp() && backendHost() !== window.location.origin) {
    return null
  }
```

- [ ] **Step 3: Run — PASS; lint; regression plan; commit**

Run: `cd web && pnpm test -- oauthQueries ProviderButtons LoginPage RegistrationPage && pnpm lint && npx tsc -b`.
Regression plan: add under the OAuth section "📱 App: tapping a provider button twice while the browser sheet is opening starts ONE flow; the buttons stay disabled until the sheet closes or the sign-in returns." and "Web: with a custom backend selected (different origin than the page), no provider buttons are shown."

```bash
git add -A web docs && git commit -m "fix(web): guard against double provider starts in the app; hide buttons off-origin

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 12: Linked-accounts toast names the configured provider; logout does not blank the page

**Files:**
- Modify: `web/src/features/settings/LinkedAccountsPage.tsx:42-67`, `web/src/features/auth/LogoutPage.tsx`
- Test: `web/src/features/settings/LinkedAccountsPage.test.tsx`, `web/src/features/auth/LogoutPage.test.tsx`

- [ ] **Step 1: Failing tests**

`LinkedAccountsPage.test.tsx` — add a case where the provider list resolves AFTER complete-link (delay the provider-list handler by 30ms with `await new Promise(r => setTimeout(r, 30))`) and the identity is `oidc` named `Authentik`; expect the toast text to contain `Authentik` (assert via the sonner mock the file already uses). `LogoutPage.test.tsx` — a provider session (`logout-user` returns `{provider: 'google', logoutUrl: ''}`) with `get-provider-list` delayed 50ms: assert `localStorage` no longer holds the token only AFTER the notice is rendered (i.e. the provider list is fetched BEFORE the token is purged), and that `window.location.assign` is never called with `/login` while the notice is shown even when the effect re-runs (render, then rerender with a changed `t` by switching `i18n.changeLanguage('de')` and back).

Run: `cd web && pnpm test -- LinkedAccountsPage LogoutPage` — FAIL.

- [ ] **Step 2: Implement**

`LinkedAccountsPage.tsx`: in `onSuccess`, resolve the name from the query cache at toast time: `const name = providerDisplayName(provider, queryClient.getQueryData<ProviderDto[]>(providersQueryKey) ?? providers.data, t)` (add `const queryClient = useQueryClient()`; import `providersQueryKey`); if it still resolves to the placeholder (`name === t('auth.oauth.provider_name.oidc')` for `oidc`), `void providers.refetch().then(r => toast.success(t(…, { provider: providerDisplayName(provider, r.data, t) })))` instead of toasting immediately. Drop `providers.data` from the effect deps.

`LogoutPage.tsx`: fetch the provider list in parallel with `logout()` (`const [res, providers] = await Promise.all([logout(), getProviderList().catch(() => undefined)])`) BEFORE `removeToken()`; use a `useRef(false)` latch so the effect body runs once; deps `[]` with `t` read through a ref (`const tRef = useRef(t); tRef.current = t`). Keep the button and notice as they are.

- [ ] **Step 3: Run — PASS; lint; commit**

Run: `cd web && pnpm test -- LinkedAccountsPage LogoutPage && pnpm lint && npx tsc -b`.

```bash
git add -A web && git commit -m "fix(web): name the configured provider in the link toast; fetch providers before purging the token on logout

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 13: Documentation and copy corrections

**Files:**
- Modify: `CLAUDE.md:469-470` (JWKS caching sentence)
- Modify: `internal/infra/mailer/identity_linked.go:10-12`
- Modify: `locales/en.json:1614`, `internal/infra/mailer/mailer_test.go:165`
- Test: `go test ./internal/infra/mailer/ ./internal/test/i18ntest/`

- [ ] **Step 1: Failing test — the English email reads naturally for Apple**

Change the `want` string in `TestIdentityLinkedEmailEnglishUnchanged` to: `"Hi Alice,\n\nYour Apple account was just linked to your Econumo account and can now be used to sign in. Apple confirmed it owns the email address on your account, so the link was made automatically.\n\nIf this wasn't you, unlink the account from Settings right away and set a password.\n\n--\nEconumo — Manage money. Together.\n"`. Run: `go test ./internal/infra/mailer/ -run TestIdentityLinkedEmailEnglishUnchanged` — FAIL.

- [ ] **Step 2: Fix the catalogue, the godoc, and CLAUDE.md**

`locales/en.json:1614`: `"body": "Hi {name},\n\nYour {provider} account was just linked ..."` (only "A {provider}" → "Your {provider}"; the other 10 catalogues keep their own grammar — placeholder parity is unchanged). `identity_linked.go:10-12`: "IdentityLinkedSender notifies the account owner when an oauth provider is auto-linked to their existing passwordless account (a verified email matching a password account is refused instead), mirroring VerifySender so the oauth feature stays free of any mail dependency." CLAUDE.md 469-470: "Discovery documents are fetched lazily and cached for the process lifetime; the JWKS is cached and re-fetched when a token names an unknown key id (at most once a minute per issuer), so a signing-key rotation needs no restart".

- [ ] **Step 3: Run — PASS; commit**

Run: `go test ./internal/infra/mailer/ ./internal/test/i18ntest/`

```bash
git add -A && git commit -m "docs: correct JWKS caching note, mailer godoc, and the linked-account email wording

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Final gate (controller runs after the last task)

- `make go-test` (smoke tier incl. coverage ≥ 80 and OpenAPI freshness — if `make swagger` changes committed docs, commit that too).
- `cd web && pnpm test && pnpm lint && npx tsc -b` (only the pre-existing `transaction.test.ts` Blob failure is tolerated).
- Optional but recommended: PostgreSQL pass — `docker run -d --name econumo-oauth-pgtest -p 127.0.0.1:55433:5432 -e POSTGRES_USER=econumo -e POSTGRES_PASSWORD=econumo -e POSTGRES_DB=econumo_test postgres:17-alpine` then `DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@127.0.0.1:55433/econumo_test?sslmode=disable' CGO_ENABLED=0 go test -tags enginecompare ./internal/test/enginecompare/ ./internal/oauth/... ./internal/user/...` and `DBTEST_ENGINE=pgsql` for the repo suites (`make test-repo-pgsql` if the compose stack is available).

## Findings deliberately NOT fixed here (rulings)

- **Concurrent unlinks can strand a passwordless user** — parked: self-inflicted double-submit, recoverable through "Forgot password" (which sets a password); a correct fix needs a transaction runner back in `oauth.Service` (removed in review round 3) or engine-specific row locking. Cost if wrong: a user locks themself out and recovers via the reset flow.
- **`mirrorEmailDrift` reads a stale aggregate** — parked: it runs only after the fenced identity UPDATE succeeded, and the mirror itself goes through `mutate` (reload-in-transaction) with a unique index on `lower(email)` catching a collision. Cost if wrong: an email mirror is skipped or logged, never a takeover.
- **`ResetPassword` saves the whole aggregate loaded before its transaction** — parked: pre-existing `Save` pattern shared by every user mutation; a concurrent `user:deactivate` racing a reset is an operator-timing edge. Cost if wrong: a deactivation is reverted by a simultaneous reset and has to be re-run.
- **Apple keeps `TrustEmail: true`** — every Apple ID address is verified by Apple before it can be used, and the claim arrives as a string handled by `parseEmailVerified`; flipping it risks breaking Apple sign-in for no attack it would prevent.
