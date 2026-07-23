# Retire the `users.identifier` column — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `lower(email)` the unique + lookup key for users, stop deriving `identifier` from email, and remove `User.Identifier` from the domain — keeping the physical column in place (populated with the row `id`) so no SQLite table rebuild is needed.

**Architecture:** Three functional phases plus docs. (1) Add email-based repo lookups additively so the build stays green. (2) Switch every service lookup/existence check from the md5 identifier to email. (3) Retire `identifier`: a migration adds a `lower(email)` unique index and backfills `identifier = id`; the model drops the field; the repo writes `id` into the column and stops reading it; `EncodeService.Hash` and the old identifier queries are removed. Each phase compiles and passes tests on its own.

**Tech Stack:** Go (hexagonal, per-feature packages), sqlc v1.30 (per-engine codegen: `gen/sqlite`, `gen/pgsql`), SQLite (`modernc.org/sqlite`) + PostgreSQL (`pgx`), `internal/test/dbtest` + `enginecompare`.

## Global Constraints

- **Wire contract is frozen and must not change.** `identifier` is never serialized; login/register/reset/verify behavior stays byte-identical. `apiparity` and `mcpparity` goldens MUST NOT change — if a golden diff appears, something regressed. Never hand-edit a golden.
- **Two engines, byte-identical.** Every schema/query change lands in BOTH `sqlite` and `pgsql` variants. `make test-repo-pgsql` and the `enginecompare` suite must pass.
- **sqlc is the source of truth for SQL.** After editing any `query/{sqlite,pgsql}/*.sql` or migration, regenerate with `sqlc generate` (from `internal/infra/storage/sqlc/`, or `go generate ./internal/infra/storage/sqlc/...`). A query referencing a dropped column fails generation.
- **sqlc ASCII-only in `.sql`.** No em dashes / non-ASCII in query or migration files (v1.30 sqlite codegen mangles byte offsets).
- **Coverage gate:** `make go-test` enforces `GO_COVER_MIN` (default 78).
- **Comments:** only the *why* for non-obvious/frozen-contract rationale; no restating-the-name godoc, no references to the former PHP implementation.
- **Validated fact:** sqlc generates `func (q *Queries) GetUserByEmail(ctx, lower string) (GetUserByEmailRow, error)` (sqlite) and `(bool, error)` for `ExistsUserByEmail`; the generated positional param is named `lower` (from the `lower(?)` expression) — harmless, called positionally.

---

### Task 1: Add email-based repo lookups (additive)

Add `GetUserByEmail` / `ExistsUserByEmail` alongside the existing identifier queries and repo methods. Nothing is removed yet, so the build and all existing tests stay green. New integration tests prove the case-insensitive lookup.

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/users.sql`
- Modify: `internal/infra/storage/sqlc/query/pgsql/users.sql`
- Regenerate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}/users.sql.go`, `querier.go`
- Modify: `internal/user/repository.go:15-55` (Repository interface)
- Modify: `internal/user/repo/repo.go:50-62` (querier interface), `:123-136` (methods)
- Modify: `internal/user/repo/sqlite.go`, `internal/user/repo/pgsql.go` (adapters)
- Test: `internal/user/repo/repo_integration_test.go`

**Interfaces:**
- Produces: `Repository.GetByEmail(ctx, email string) (*model.User, error)` and `Repository.ExistsByEmail(ctx, email string) (bool, error)` — case-insensitive on `email`; `GetByEmail` returns `*errs.NotFoundError` when absent. Task 2 consumes these.

- [ ] **Step 1: Write the failing integration tests**

Add to `internal/user/repo/repo_integration_test.go` (mirrors the existing `TestUserRepo_GetByIdentifier` right above; `newTestUser` args are `(id, ident, email, name, avatar, hash, salt, active, created, updated, options)`):

```go
func TestUserRepo_GetByEmail(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()
	u := newTestUser(vo.MustParseId(userA), identA, "Alice@Example.test", "Alice", "", "h", "s", true, fixedTime, fixedTime, nil)
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Lookup is case-insensitive.
	got, err := repo.GetByEmail(ctx, "alice@example.test")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.ID.String() != userA {
		t.Errorf("want %s, got %s", userA, got.ID)
	}
	_, err = repo.GetByEmail(ctx, "missing@example.test")
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound for missing email, got %v", err)
	}
}

func TestUserRepo_ExistsByEmail(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()
	u := newTestUser(vo.MustParseId(userA), identA, "Alice@Example.test", "Alice", "", "h", "s", true, fixedTime, fixedTime, nil)
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}
	exists, err := repo.ExistsByEmail(ctx, "alice@example.test")
	if err != nil || !exists {
		t.Errorf("ExistsByEmail(alice) = %v, %v; want true", exists, err)
	}
	exists, err = repo.ExistsByEmail(ctx, "nope@example.test")
	if err != nil || exists {
		t.Errorf("ExistsByEmail(nope) = %v, %v; want false", exists, err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail (compile error)**

Run: `go test ./internal/user/repo/ -run 'TestUserRepo_(GetByEmail|ExistsByEmail)'`
Expected: FAIL — `repo.GetByEmail`/`repo.ExistsByEmail` undefined.

- [ ] **Step 3: Add the sqlc queries (both engines)**

Append to `internal/infra/storage/sqlc/query/sqlite/users.sql`:

```sql
-- name: GetUserByEmail :one
SELECT id, identifier, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm, access_level, access_until, timezone, email_verified
FROM users
WHERE lower(email) = lower(?);

-- name: ExistsUserByEmail :one
SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower(?));
```

Append to `internal/infra/storage/sqlc/query/pgsql/users.sql`:

```sql
-- name: GetUserByEmail :one
SELECT id, identifier, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm, access_level, access_until, timezone, email_verified
FROM users
WHERE lower(email) = lower($1);

-- name: ExistsUserByEmail :one
SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower($1));
```

- [ ] **Step 4: Regenerate sqlc**

Run: `cd internal/infra/storage/sqlc && sqlc generate && cd -`
Expected: exit 0; new `GetUserByEmail`/`ExistsUserByEmail` in `gen/sqlite/users.sql.go` and `gen/pgsql/users.sql.go`.

- [ ] **Step 5: Add querier methods + adapters**

In `internal/user/repo/repo.go`, add to the `querier` interface (after the `ExistsUserByIdentifier` line):

```go
	GetUserByEmail(ctx context.Context, db backend.DBTX, email string) (userRow, error)
	ExistsUserByEmail(ctx context.Context, db backend.DBTX, email string) (bool, error)
```

In `internal/user/repo/sqlite.go`, add:

```go
func (sqliteQuerier) GetUserByEmail(ctx context.Context, db backend.DBTX, email string) (userRow, error) {
	row, err := sqlitegen.New(db).GetUserByEmail(ctx, email)
	return userRow(row), err
}

func (sqliteQuerier) ExistsUserByEmail(ctx context.Context, db backend.DBTX, email string) (bool, error) {
	n, err := sqlitegen.New(db).ExistsUserByEmail(ctx, email)
	return n != 0, err
}
```

In `internal/user/repo/pgsql.go`, add:

```go
func (pgsqlQuerier) GetUserByEmail(ctx context.Context, db backend.DBTX, email string) (userRow, error) {
	u, err := pgsqlgen.New(db).GetUserByEmail(ctx, email)
	return userRow(u), err
}

func (pgsqlQuerier) ExistsUserByEmail(ctx context.Context, db backend.DBTX, email string) (bool, error) {
	return pgsqlgen.New(db).ExistsUserByEmail(ctx, email)
}
```

- [ ] **Step 6: Add the Repo methods**

In `internal/user/repo/repo.go`, after `ExistsByIdentifier` (`:136`):

```go
func (r *Repo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	row, err := r.q.GetUserByEmail(ctx, r.db(ctx), email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("User not found")
		}
		return nil, err
	}
	return r.hydrate(ctx, row)
}

func (r *Repo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.q.ExistsUserByEmail(ctx, r.db(ctx), email)
}
```

- [ ] **Step 7: Add the methods to the Repository port**

In `internal/user/repository.go`, inside `type Repository interface`, after `ExistsByIdentifier` (`:28`):

```go
	// GetByEmail loads a user (with options) by email, case-insensitively.
	// Missing -> *errs.NotFoundError.
	GetByEmail(ctx context.Context, email string) (*model.User, error)

	// ExistsByEmail reports whether a user with that email exists
	// (case-insensitive). Used by registration/change-email dup-checks.
	ExistsByEmail(ctx context.Context, email string) (bool, error)
```

- [ ] **Step 8: Run the new tests (sqlite)**

Run: `go test ./internal/user/repo/ -run 'TestUserRepo_(GetByEmail|ExistsByEmail)'`
Expected: PASS.

- [ ] **Step 9: Full smoke build + vet**

Run: `go build ./... && go vet ./internal/user/...`
Expected: exit 0.

- [ ] **Step 10: Commit**

```bash
git add internal/infra/storage/sqlc internal/user/repo internal/user/repository.go
git commit -m "feat(user): add email-based repo lookups alongside identifier"
```

---

### Task 2: Switch service lookups from identifier to email

Repoint every service **lookup / existence check** at the email methods. Leave the `identifier`/`newIdentifier` locals that still feed `NewUser` / `UpdateEmail` in place (those signatures change in Task 3). Behavior is identical, so the existing suite is the test.

**Files:**
- Modify: `internal/user/login.go:24-25`
- Modify: `internal/user/register.go:49-58`
- Modify: `internal/user/admin.go:39-47` (change-email dup-check), `:158-161` (`userByEmail`)
- Modify: `internal/user/password.go:88`, `:127`
- Modify: `internal/user/verify_email.go:47`, `:131`

**Interfaces:**
- Consumes: `Repository.GetByEmail` / `ExistsByEmail` (Task 1).

- [ ] **Step 1: Switch the login lookup**

In `internal/user/login.go`, replace lines 24-25:

```go
	identifier := s.encode.Hash(strings.ToLower(req.Username))
	u, err := s.repo.GetByIdentifier(ctx, identifier)
```

with:

```go
	u, err := s.repo.GetByEmail(ctx, strings.TrimSpace(req.Username))
```

- [ ] **Step 2: Switch `userByEmail` (admin lookups)**

In `internal/user/admin.go`, replace the body of `userByEmail` (`:158-161`):

```go
func (s *Service) userByEmail(ctx context.Context, email string) (*model.User, error) {
	return s.repo.GetByEmail(ctx, strings.TrimSpace(email))
}
```

(Update the doc comment above it to say "resolves a user from a plaintext email (case-insensitive)"; drop the md5-identifier wording.)

- [ ] **Step 3: Switch the register dup-check**

In `internal/user/register.go` `createUser`, change the existence check (`:52`) from `ExistsByIdentifier(ctx, identifier)` to:

```go
	exists, err := s.repo.ExistsByEmail(ctx, loweredEmail)
```

Leave `loweredEmail` and `identifier := s.encode.Hash(loweredEmail)` (`:49-50`) as-is — `identifier` is still passed to `NewUser` at `:75` until Task 3.

- [ ] **Step 4: Switch the change-email dup-check**

In `internal/user/admin.go` `AdminChangeEmail`, the original guard compared `newIdentifier != u.Identifier` to skip the dup-check when the email is unchanged. `u.Identifier` is going away, so compare the emails directly (`u.Email` is ciphertext — decode it via the salt-free passthrough encoder). Replace the guarded block (`:37-47`) with:

```go
	currentEmail, derr := s.encode.Decode(u.Email)
	if derr != nil {
		return derr
	}
	loweredNew := strings.ToLower(strings.TrimSpace(newEmail))
	newIdentifier := s.encode.Hash(loweredNew)
	if loweredNew != strings.ToLower(currentEmail) {
		exists, eerr := s.repo.ExistsByEmail(ctx, loweredNew)
		if eerr != nil {
			return eerr
		}
		if exists {
			return &errs.ValidationError{Msg: "User already exists", MsgCode: errs.CodeUserAlreadyExists}
		}
	}
```

`newIdentifier` is still computed here because it feeds the `UpdateEmail(newIdentifier, ...)` call at `:55`, which keeps its current signature until Task 3. Leave the `encryptedEmail`/`UpdateEmail` lines (`:49-56`) unchanged in this task.

- [ ] **Step 5: Switch the password-flow lookups**

In `internal/user/password.go`, replace `s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))` at BOTH `:88` (RemindPassword) and `:127` (ResetPassword) with:

```go
	u, err := s.repo.GetByEmail(ctx, lowered)
```

(`lowered` is already `strings.ToLower(strings.TrimSpace(req.Username))` at both sites.)

- [ ] **Step 6: Switch the verify-email lookups**

In `internal/user/verify_email.go`, replace `s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))` at `:47` (ConfirmEmail) and `:131` (ResendVerificationCode / issue path) with:

```go
	u, err := s.repo.GetByEmail(ctx, lowered)
```

- [ ] **Step 7: Build + vet**

Run: `go build ./... && go vet ./internal/user/...`
Expected: exit 0. (If `strings` becomes unused in `login.go`, keep it — `TrimSpace` still uses it.)

- [ ] **Step 8: Run the user + parity suites**

Run: `go test ./internal/user/... ./internal/test/apiparity/... ./internal/test/mcpparity/...`
Expected: PASS, and NO golden changes (behavior identical).

- [ ] **Step 9: Commit**

```bash
git add internal/user
git commit -m "refactor(user): resolve users by email instead of md5 identifier"
```

---

### Task 3: Retire the identifier column

Add the migration (`lower(email)` unique index + `identifier = id` backfill), drop `User.Identifier`, make the repo write `id` into the column and stop reading it, remove `EncodeService.Hash` and the now-dead identifier queries, and update every identifier-asserting test. One atomic, behavior-preserving change.

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260722000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260722000000.sql`
- Modify: `internal/model/user.go:100-137` (field, `NewUser`), `:219-225` (`UpdateEmail`)
- Modify: `internal/user/register.go:49-75`, `internal/user/admin.go:37-56`, `internal/user/migrate.go:44-72`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/users.sql` (remove identifier queries; drop `identifier` from the two SELECT column lists)
- Regenerate: `gen/{sqlite,pgsql}`
- Modify: `internal/user/repo/repo.go` (querier, `GetByIdentifier`/`ExistsByIdentifier` removal, `Save`, `userRow`, `hydrate`), `sqlite.go`, `pgsql.go`, `internal/user/repository.go`
- Modify: `internal/infra/auth/encode.go` (remove `Hash`), `internal/infra/auth/crypto_golden_test.go` (drop Hash vectors)
- Modify tests: `internal/test/fixture/entities.go:59`, `internal/user/repo/repo_integration_test.go` (`newTestUser`, remove `TestUserRepo_GetByIdentifier`/`ExistsByIdentifier`), `internal/user/migrate_test.go`, `internal/user/admin_integration_test.go`, `internal/user/trial_integration_test.go`, `internal/user/avatar_update_integration_test.go`, `internal/user/verify_email_test.go`, `internal/model/user_test.go`, `internal/model/user_activate_test.go`, `internal/infra/storage/migrations/email_verified_backfill_test.go`

**Interfaces:**
- Produces: `model.NewUser(id vo.Id, encryptedEmail, name, avatar, passwordHash, salt string, now time.Time) *User` (no `identifier`); `(*User).UpdateEmail(encryptedEmail string, now time.Time)` (no `identifier`). `Repository` no longer has `GetByIdentifier`/`ExistsByIdentifier`. `EncodeService` no longer has `Hash`.

- [ ] **Step 1: Write the migration (both engines)**

Create `internal/infra/storage/migrations/sqlite/20260722000000.sql`:

```sql
-- The email-derived md5 identifier is retired: lower(email) is now the unique
-- key (index below), and the identifier column is kept only to satisfy its
-- pre-existing NOT NULL UNIQUE constraint without a SQLite table rebuild. Each
-- row's own id is a ready-made unique, non-null placeholder.
CREATE UNIQUE INDEX users_email_lower_uniq ON users (lower(email));
UPDATE users SET identifier = id;
```

Create `internal/infra/storage/migrations/pgsql/20260722000000.sql` with the identical two statements (same comment, same SQL — `lower(email)` and `identifier = id` are valid Postgres).

- [ ] **Step 2: Write/adjust the failing model test**

In `internal/model/user_test.go`, update the `NewUser`/`UpdateEmail` call sites to the new signatures and assert email-only behavior. Representative (adapt to the existing test names in the file):

```go
func TestUser_UpdateEmail(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	u := model.NewUser(vo.MustParseId("00000000-0000-7000-8000-000000000001"),
		"old@example.test", "Alice", "face:blue", "hash", "salt", now)
	later := now.Add(time.Hour)
	u.UpdateEmail("new@example.test", later)
	if u.Email != "new@example.test" {
		t.Errorf("email = %q, want new@example.test", u.Email)
	}
	if !u.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt not bumped")
	}
}
```

Remove any assertions on `u.Identifier` in `user_test.go` and `user_activate_test.go`.

- [ ] **Step 3: Run it to verify it fails (compile error)**

Run: `go test ./internal/model/ -run TestUser_UpdateEmail`
Expected: FAIL — too many arguments to `model.NewUser` / `UpdateEmail`.

- [ ] **Step 4: Change the model**

In `internal/model/user.go`: delete the `Identifier` field (`:102`); update the `User` doc comment (`:95-99`) to drop "or hashed (…Identifier)". Change `NewUser` (`:121-137`):

```go
// NewUser constructs a freshly-registered user. The caller (the service) has
// already encrypted the email, picked the avatar, and hashed the password.
// Options are seeded separately via SeedDefaultOptions.
func NewUser(id vo.Id, encryptedEmail, name, avatar, passwordHash, salt string, now time.Time) *User {
	return &User{
		ID:            id,
		Email:         encryptedEmail,
		Name:          name,
		Avatar:        avatar,
		Password:      passwordHash,
		Salt:          salt,
		Algorithm:     AlgorithmArgon2id,
		IsActive:      true,
		EmailVerified: true,
		AccessLevel:   AccessLevelFull,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}
```

Change `UpdateEmail` (`:219-225`):

```go
// UpdateEmail replaces the encrypted email. The identifier column is derived
// from the row id at persistence time, so it needs no update here.
func (u *User) UpdateEmail(encryptedEmail string, now time.Time) {
	u.Email = encryptedEmail
	u.UpdatedAt = now
}
```

- [ ] **Step 5: Update the service call sites**

`internal/user/register.go` `createUser`: delete `identifier := s.encode.Hash(loweredEmail)` (`:50`); keep `loweredEmail` (still used by `ExistsByEmail`). Change the `NewUser` call (`:75`) to:

```go
	u := model.NewUser(s.repo.NextIdentity(), encryptedEmail, name, avatar, passwordHash, salt, now)
```

`internal/user/admin.go` `AdminChangeEmail`: delete `newIdentifier := s.encode.Hash(loweredNew)`; change the commit (`:54-57`) to:

```go
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.UpdateEmail(encryptedEmail, s.clock.Now())
		return s.repo.Save(ctx, u)
	})
```

`internal/user/migrate.go` `MigrateRemoveDataSalt`: delete the `saltFree` encoder (`:47`) and the `newIdent` derivation (`:62`); simplify the loop body (`:55-72`) to:

```go
			plain, derr := salted.Decode(u.Email)
			if derr != nil {
				skipped++
				continue
			}
			if plain == u.Email {
				skipped++
				continue
			}
			u.UpdateEmail(plain, s.clock.Now())
			if serr := s.repo.Save(ctx, u); serr != nil {
				return serr
			}
			migrated++
```

Update the `MigrateRemoveDataSalt` doc comment to drop the identifier-re-derivation paragraph (it now only decrypts email to plaintext; the repo writes `id` into the identifier column).

- [ ] **Step 6: Update the repo (write id, stop reading identifier)**

`internal/user/repo/repo.go`:
- In `Save` (`:166-181`), change `Identifier: u.Identifier,` to `Identifier: u.ID.String(),`.
- Delete `GetByIdentifier` (`:123-132`) and `ExistsByIdentifier` (`:134-136`).
- In the `querier` interface, delete the `GetUserByIdentifier` and `ExistsUserByIdentifier` lines.
- In `userRow` (`:26-42`), delete the `Identifier string` field.
- In `hydrate` (`:241`), delete `Identifier: row.Identifier,` from the `&model.User{...}` literal.

`internal/user/repo/sqlite.go` and `pgsql.go`: delete the `GetUserByIdentifier` and `ExistsUserByIdentifier` adapter methods.

`internal/user/repository.go`: delete the `GetByIdentifier` and `ExistsByIdentifier` interface methods (`:22-28`).

- [ ] **Step 7: Update the sqlc queries + regenerate**

In `internal/infra/storage/sqlc/query/sqlite/users.sql` and `.../pgsql/users.sql`:
- Delete the `GetUserByIdentifier` and `ExistsUserByIdentifier` query blocks.
- Remove `identifier, ` from the SELECT column list of BOTH `GetUserByID` and `GetUserByEmail` (so their generated row types match the trimmed `userRow`). Leave `InsertUser` / `UpsertUser` unchanged (they still write the `identifier` column; the repo supplies `id`).

Run: `cd internal/infra/storage/sqlc && sqlc generate && cd -`
Expected: exit 0; `gen/*/users.sql.go` no longer has `GetUserByIdentifier`; `GetUserByIDRow` / `GetUserByEmailRow` no longer have an `Identifier` field.

- [ ] **Step 8: Remove `EncodeService.Hash`**

In `internal/infra/auth/encode.go`, delete the `Hash` method (`:56-59`). In `internal/infra/auth/crypto_golden_test.go`, delete the `Hash` golden vectors (the `identifier_hash` fixture field at `:25` and the `svc.Hash` assertion at `:71`); if the fixture struct field becomes unused, remove it.

- [ ] **Step 9: Update the remaining tests + fixtures**

- `internal/test/fixture/entities.go:59`: delete the line computing/assigning `Identifier` (the repo now writes `id`); if the fixture builds `model.User` via `NewUser`, drop the identifier argument.
- `internal/user/repo/repo_integration_test.go`: change `newTestUser` to drop the `ident` parameter (and remove `Identifier:` from the `model.User` it builds); update all `newTestUser(...)` call sites to drop the `identA` argument; DELETE `TestUserRepo_GetByIdentifier` and `TestUserRepo_ExistsByIdentifier` (superseded by the Task 1 email tests); remove the now-unused `identA` const if nothing else references it.
- `internal/user/migrate_test.go`: stop asserting `u.Identifier`; assert the email was decrypted to plaintext and that a subsequent `GetByEmail` finds the row.
- `internal/user/admin_integration_test.go`, `trial_integration_test.go`, `avatar_update_integration_test.go`, `verify_email_test.go`: replace every `GetByIdentifier(enc.Hash(x))` lookup with `GetByEmail(x)`; drop `enc.Hash`/identifier usages.
- `internal/infra/storage/migrations/email_verified_backfill_test.go:44,63`: the raw `INSERT INTO users (...)` still lists `identifier` (the column exists) — set its value to the row's `id` instead of an md5 (any unique non-null value works).

- [ ] **Step 10: Build, vet, and run the full Go smoke suite**

Run: `make go-test`
Expected: PASS — build + vet + gofmt + apiparity/mcpparity goldens UNCHANGED + i18n guards + coverage gate. If a golden diff appears, STOP and investigate (behavior must be identical).

- [ ] **Step 11: Run the PostgreSQL + engine-comparison suites**

Run: `make test-repo-pgsql` then `go test -tags enginecompare ./internal/test/enginecompare/...`
Expected: PASS (the `lower(email)` index + lookups behave identically on both engines). If Postgres isn't provisioned, `make test` auto-provisions it.

- [ ] **Step 12: Commit**

```bash
git add internal
git commit -m "refactor(user): retire the identifier column (lower(email) is the key)"
```

---

### Task 4: Documentation

Update the prose that describes the identifier so it matches reality (email is the key; the column is a retired `= id` placeholder), and add the upgrade note.

**Files:**
- Modify: `CLAUDE.md`
- Modify: `internal/infra/storage/sqlc/sqlc.yaml` (header comment mentioning the `Identifier` VO mapping)
- Modify: `internal/server/server.go:114`, `internal/config/config.go:24` (explanatory comments)
- Modify: `docs/run-without-docker.md` OR the README upgrade section (whichever documents `data:remove-salt`) — add the prerequisite note

- [ ] **Step 1: Update CLAUDE.md**

In the "Wire & data contract" → "Auth crypto" section, change the "User identifier" bullet to state that `lower(email)` is now the unique + lookup key (unique expression index `users_email_lower_uniq`), and that the legacy `identifier` column is retained but retired (populated with the row `id`, no longer derived from email). In the "Salt-free everywhere" bullet and the `data:remove-salt` / `ECONUMO_DATA_SALT` description, drop the "re-derives the identifier" wording — `data:remove-salt` now only decrypts email to plaintext. Keep it ASCII-clean.

- [ ] **Step 2: Update code comments**

- `internal/infra/storage/sqlc/sqlc.yaml`: in the header comment, change "string <-> domain value objects (Id, Identifier)" to just "(Id)".
- `internal/server/server.go:114` and `internal/config/config.go:24`: reword any comment that describes the md5 identifier as the lookup key to reference `lower(email)`; keep the `ECONUMO_DATA_SALT` decryption note.

- [ ] **Step 3: Add the upgrade note**

Wherever `data:remove-salt` is documented for operators, add: a still-salted instance must run `data:remove-salt` after upgrading so emails are plaintext before the `lower(email)` unique index is relied upon (salted instances are already login-broken until they migrate — no regression).

- [ ] **Step 4: Verify docs build / no broken references**

Run: `make go-lint`
Expected: exit 0 (build + vet + gofmt + OpenAPI-docs-fresh check unaffected by prose).

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md internal/infra/storage/sqlc/sqlc.yaml internal/server/server.go internal/config/config.go docs
git commit -m "docs: describe lower(email) as the user key; identifier retired"
```

---

## Notes for the implementer

- **Do not** attempt to drop the `identifier` column or its `UNIQUE` constraint — that would force a SQLite `users` table rebuild under `foreign_keys = ON`, which is the risk this design deliberately avoids.
- The three functional tasks are ordered so the build + tests stay green after each. If you must reorder, keep Task 3 atomic: the model signature change, the sqlc SELECT trim, and `userRow`/`hydrate` must land together or the package won't compile.
- Golden files (`apiparity`, `mcpparity`) must not change. A diff means a behavior regression — investigate, don't regenerate.
