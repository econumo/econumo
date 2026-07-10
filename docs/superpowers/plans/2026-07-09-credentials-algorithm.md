# Credentials Algorithm Versioning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `users.algorithm` column (`"sha512"` legacy / `"argon2id"` new) so new users and every password write use Argon2id while existing sha512 credentials keep working (issue #64).

**Architecture:** The existing `auth.PasswordHasher` gains versioned dispatch: `Hash(plain)` always produces an Argon2id PHC string; `Verify(algorithm, stored, plain, salt)` routes to the legacy sha512×500 path or the Argon2id path by the stored algorithm value. The `model.User` aggregate carries the algorithm; all four password-writing use cases (register, update-password, reset-password, CLI change-password) write argon2id. Login only verifies.

**Tech Stack:** Go, `golang.org/x/crypto/argon2` (already in module graph), sqlc v1.30.0 for regenerated queries, sqlite + pgsql migrations.

**Spec:** `docs/superpowers/specs/2026-07-09-credentials-algorithm-design.md`

## Global Constraints

- Algorithm column values are exactly `"sha512"` and `"argon2id"`; constants live in `internal/model` (`AlgorithmSHA512`, `AlgorithmArgon2id`).
- Argon2id parameters: m=19456 KiB, t=2, p=1, 16-byte salt, 32-byte key; PHC string `$argon2id$v=19$m=19456,t=2,p=1$<b64salt>$<b64key>` with `base64.RawStdEncoding`.
- Verification parses parameters from the stored PHC string; malformed/unknown input fails closed (false), never panics.
- The wire contract is frozen: no API response changes, apiparity goldens must stay byte-identical, never hand-edit a golden.
- The legacy sha512 path, `salt` column, and its generation at registration stay untouched.
- Every commit leaves `go test ./...` green. Run commands from the worktree root.
- Commit trailer on every commit:
  `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>` and
  `Claude-Session: https://claude.ai/code/session_013GLgLN9NxYGTTbaTMY69PR`

---

### Task 1: Schema, model field, repo round-trip

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260709000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260709000000.sql`
- Modify: `internal/infra/storage/sqlc/query/sqlite/users.sql`
- Modify: `internal/infra/storage/sqlc/query/pgsql/users.sql`
- Regenerate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}` (via `sqlc generate`)
- Modify: `internal/model/user.go`
- Modify: `internal/user/repo/repo.go` (Save + hydrate)
- Modify: `internal/user/password.go`, `internal/user/admin.go` (UpdatePassword callers — signature only, still sha512 semantics in this task)
- Modify: `internal/test/fixture/entities.go` (INSERT gains the algorithm column, `'sha512'`)
- Test: `internal/user/repo/repo_integration_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `model.AlgorithmSHA512 = "sha512"`, `model.AlgorithmArgon2id = "argon2id"`; `model.User.Algorithm string`; `func (u *User) UpdatePassword(passwordHash, algorithm string, now time.Time)`; `users.algorithm` column present in both engines with default `'sha512'`; generated `sqlitegen.User`/`pgsqlgen.User` gain `Algorithm string`.

- [ ] **Step 1: Write the failing repo round-trip test**

Append to `internal/user/repo/repo_integration_test.go`:

```go
func TestUserRepo_AlgorithmRoundTrip(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()

	u := newTestUser(
		vo.MustParseId(userA), identA, "enc-email", "Alice", "https://av/a",
		"hash", "salt-a", true, fixedTime, fixedTime, nil,
	)
	u.Algorithm = model.AlgorithmArgon2id
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.GetByID(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Algorithm != model.AlgorithmArgon2id {
		t.Errorf("Algorithm = %q, want %q", got.Algorithm, model.AlgorithmArgon2id)
	}

	// A row inserted without an explicit algorithm gets the sha512 default.
	if _, err := db.Raw.Exec(db.Rebind(
		`INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, TRUE)`),
		userB, "fedcba9876543210fedcba9876543210", "e2", "Bob", "", "h2", "s2", fixedTime, fixedTime); err != nil {
		t.Fatalf("raw insert: %v", err)
	}
	legacy, err := repo.GetByID(ctx, vo.MustParseId(userB))
	if err != nil {
		t.Fatalf("GetByID legacy: %v", err)
	}
	if legacy.Algorithm != model.AlgorithmSHA512 {
		t.Errorf("legacy Algorithm = %q, want %q", legacy.Algorithm, model.AlgorithmSHA512)
	}
}
```

Note: check how other tests in this file execute raw SQL (`db.Raw.Exec` + `db.Rebind`); mirror the file's existing helper if it differs.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/user/repo/ -run TestUserRepo_AlgorithmRoundTrip -v`
Expected: FAIL — compile error: `u.Algorithm undefined` / `model.AlgorithmArgon2id undefined`.

- [ ] **Step 3: Add the migrations**

`internal/infra/storage/migrations/sqlite/20260709000000.sql`:

```sql
ALTER TABLE users ADD COLUMN algorithm VARCHAR(32) NOT NULL DEFAULT 'sha512';
```

`internal/infra/storage/migrations/pgsql/20260709000000.sql`:

```sql
ALTER TABLE users ADD COLUMN algorithm VARCHAR(32) NOT NULL DEFAULT 'sha512';
```

- [ ] **Step 4: Add `algorithm` to the user queries and regenerate sqlc**

In `internal/infra/storage/sqlc/query/sqlite/users.sql`, add `algorithm` to every column list — the two SELECTs, InsertUser, UpsertUser (both the INSERT list and the `DO UPDATE SET`). Result:

```sql
-- name: GetUserByID :one
SELECT id, identifier, email, name, avatar_url, password, salt, algorithm, created_at, updated_at, is_active
FROM users
WHERE id = ?;

-- name: GetUserByIdentifier :one
SELECT id, identifier, email, name, avatar_url, password, salt, algorithm, created_at, updated_at, is_active
FROM users
WHERE identifier = ?;

-- name: ExistsUserByIdentifier :one
SELECT EXISTS(SELECT 1 FROM users WHERE identifier = ?);

-- name: ListUserIDs :many
SELECT id FROM users;

-- name: InsertUser :exec
INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, algorithm, created_at, updated_at, is_active)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpsertUser :exec
INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, algorithm, created_at, updated_at, is_active)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    identifier = excluded.identifier,
    email      = excluded.email,
    name       = excluded.name,
    avatar_url = excluded.avatar_url,
    password   = excluded.password,
    salt       = excluded.salt,
    algorithm  = excluded.algorithm,
    updated_at = excluded.updated_at,
    is_active  = excluded.is_active;
```

Mirror in `internal/infra/storage/sqlc/query/pgsql/users.sql` with `$N` placeholders (`$1`…`$11`).

Run: `cd internal/infra/storage/sqlc && sqlc generate && cd -`
Then: `git diff --stat internal/infra/storage/sqlc/gen/` — both engines' `models.go` show `Algorithm string` on `User`, and `users.sql.go` shows it in params/scans. The field order within the generated structs must stay identical between engines (it will — same column order) or the repo's whole-struct conversion breaks.

- [ ] **Step 5: Add the model field, constants, and new UpdatePassword signature**

In `internal/model/user.go`:

```go
// After the OnboardingStarted/OnboardingCompleted consts:
// Password-hash algorithm markers stored in users.algorithm. sha512 is the
// legacy scheme (see CLAUDE.md); argon2id is written by every new hash.
const (
	AlgorithmSHA512   = "sha512"
	AlgorithmArgon2id = "argon2id"
)
```

Add the field to `User` (after `Salt`):

```go
	Salt       string // sha1(random) hex, 40 chars (unused by argon2id hashes)
	Algorithm  string // which scheme hashed Password: AlgorithmSHA512 | AlgorithmArgon2id
```

`NewUser` stamps the legacy value for now (Task 3 flips it to argon2id together with the register-path hash change, so hash and marker never disagree):

```go
		Password:   passwordHash,
		Salt:       salt,
		Algorithm:  AlgorithmSHA512,
```

Replace `UpdatePassword`:

```go
// UpdatePassword replaces the stored password hash and records which algorithm
// produced it. The caller hashes the plaintext first.
func (u *User) UpdatePassword(passwordHash, algorithm string, now time.Time) {
	u.Password = passwordHash
	u.Algorithm = algorithm
	u.UpdatedAt = now
}
```

Update the three callers to pass the (still truthful) legacy marker — this task changes no hashing behavior:
- `internal/user/password.go:43`: `u.UpdatePassword(s.hasher.Hash(req.NewPassword, u.Salt), model.AlgorithmSHA512, now)`
- `internal/user/password.go:113`: `u.UpdatePassword(s.hasher.Hash(req.Password, u.Salt), model.AlgorithmSHA512, s.clock.Now())`
- `internal/user/admin.go:67`: `u.UpdatePassword(s.hasher.Hash(newPassword, u.Salt), model.AlgorithmSHA512, s.clock.Now())`

- [ ] **Step 6: Round-trip the field in the repo**

In `internal/user/repo/repo.go`, `Save` gains one param field:

```go
	if err := r.q.UpsertUser(ctx, db, userParams{
		ID:         u.ID.String(),
		Identifier: u.Identifier,
		Email:      u.Email,
		Name:       u.Name,
		AvatarUrl:  u.AvatarURL,
		Password:   u.Password,
		Salt:       u.Salt,
		Algorithm:  u.Algorithm,
		CreatedAt:  u.CreatedAt,
		UpdatedAt:  u.UpdatedAt,
		IsActive:   u.IsActive,
	}); err != nil {
```

and `hydrate` maps it back:

```go
	return &model.User{ID: id, Identifier: row.Identifier, Email: row.Email, Name: row.Name,
		AvatarURL: row.AvatarUrl, Password: row.Password, Salt: row.Salt, Algorithm: row.Algorithm,
		IsActive: row.IsActive, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Options: opts}, nil
```

- [ ] **Step 7: Make the fixture INSERT explicit**

In `internal/test/fixture/entities.go`, the `User` INSERT names its columns; add `algorithm` with the legacy value (fixture-seeded users are sha512-hashed and now say so explicitly):

```go
	b.insert(`INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, algorithm, created_at, updated_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'sha512', ?, ?, `+active+`)`,
		id, identifier, email, u.Name, u.Avatar, password, u.Salt, now, now)
```

- [ ] **Step 8: Run the test to verify it passes, then the package suites**

Run: `go test ./internal/user/repo/ -run TestUserRepo_AlgorithmRoundTrip -v`
Expected: PASS

Run: `go test ./internal/user/... ./internal/model/... ./internal/test/... ./internal/infra/...`
Expected: PASS (apiparity goldens unchanged — nothing observable changed).

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: add users.algorithm column, model field, repo round-trip (#64)"
```

---

### Task 2: Argon2id primitives in `internal/infra/auth` (pure addition)

**Files:**
- Create: `internal/infra/auth/argon2id.go`
- Test: `internal/infra/auth/argon2id_test.go`
- Modify: `go.mod` (`golang.org/x/crypto` becomes a direct dependency via `go mod tidy`)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces (package-private, wired into the public API in Task 3): `hashArgon2id(plain string) (string, error)`, `encodeArgon2id(plain string, salt []byte, memoryKiB, time uint32, threads uint8, keyLen uint32) string`, `verifyArgon2id(stored, plain string) bool`.

- [ ] **Step 1: Write the failing tests**

Create `internal/infra/auth/argon2id_test.go`:

```go
package auth

import (
	"strings"
	"testing"
)

func TestArgon2id_HashAndVerify(t *testing.T) {
	h, err := hashArgon2id("s3cret-password")
	if err != nil {
		t.Fatalf("hashArgon2id: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("unexpected PHC prefix: %s", h)
	}
	if !verifyArgon2id(h, "s3cret-password") {
		t.Error("verify rejected the correct password")
	}
	if verifyArgon2id(h, "s3cret-passwordX") {
		t.Error("verify accepted a wrong password")
	}
	// Two hashes of the same password differ (random salt).
	h2, err := hashArgon2id("s3cret-password")
	if err != nil {
		t.Fatalf("hashArgon2id: %v", err)
	}
	if h == h2 {
		t.Error("two hashes are identical — salt is not random")
	}
}

// Parameters are read from the stored string, so a hash produced with different
// (e.g. future-tuned) parameters still verifies.
func TestArgon2id_VerifyUsesStoredParams(t *testing.T) {
	h := encodeArgon2id("pw", []byte("0123456789abcdef"), 8, 1, 1, 16)
	if !verifyArgon2id(h, "pw") {
		t.Error("verify rejected a hash with non-default params")
	}
	if verifyArgon2id(h, "pwX") {
		t.Error("verify accepted a wrong password with non-default params")
	}
}

func TestArgon2id_VerifyFailsClosed(t *testing.T) {
	valid := encodeArgon2id("pw", []byte("0123456789abcdef"), 8, 1, 1, 16)
	for name, stored := range map[string]string{
		"empty":            "",
		"not phc":          "abcdef",
		"wrong algorithm":  strings.Replace(valid, "argon2id", "argon2i", 1),
		"wrong version":    strings.Replace(valid, "v=19", "v=18", 1),
		"bad params":       "$argon2id$v=19$m=x,t=y,p=z$c2FsdA$aGFzaA",
		"bad salt b64":     "$argon2id$v=19$m=8,t=1,p=1$!!!$aGFzaA",
		"bad key b64":      "$argon2id$v=19$m=8,t=1,p=1$c2FsdA$!!!",
		"empty key":        "$argon2id$v=19$m=8,t=1,p=1$c2FsdA$",
		"missing sections": "$argon2id$v=19$m=8,t=1,p=1$c2FsdA",
		"legacy sha512":    "gLhkr35ZvvKPZeqspxjnEXharQf+bkzBhecvcxITY0IHalyVKQCVBUB1LGKcO0/EbCcbfsFhZBqEQ54rHhOohw==",
	} {
		if verifyArgon2id(stored, "pw") {
			t.Errorf("%s: verify accepted malformed input %q", name, stored)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/infra/auth/ -run TestArgon2id -v`
Expected: FAIL — compile error: `hashArgon2id`/`encodeArgon2id`/`verifyArgon2id` undefined.

- [ ] **Step 3: Implement `argon2id.go`**

```go
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP recommendation: 19 MiB memory, 2 passes, 1 lane).
// They are baked into each PHC hash string, so tuning them later only affects
// new hashes — verifyArgon2id always uses the parameters stored in the hash.
const (
	argonMemoryKiB uint32 = 19456
	argonTime      uint32 = 2
	argonThreads   uint8  = 1
	argonSaltLen          = 16
	argonKeyLen    uint32 = 32
)

func hashArgon2id(plain string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	return encodeArgon2id(plain, salt, argonMemoryKiB, argonTime, argonThreads, argonKeyLen), nil
}

// encodeArgon2id derives the key and renders the standard PHC string:
// $argon2id$v=19$m=<KiB>,t=<passes>,p=<lanes>$<b64 salt>$<b64 key>
// (raw std base64, no padding).
func encodeArgon2id(plain string, salt []byte, memoryKiB, time uint32, threads uint8, keyLen uint32) string {
	key := argon2.IDKey([]byte(plain), salt, time, memoryKiB, threads, keyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryKiB, time, threads,
		b64.EncodeToString(salt), b64.EncodeToString(key))
}

// verifyArgon2id recomputes the key with the parameters stored in the hash and
// compares constant-time. Anything malformed fails closed.
func verifyArgon2id(stored, plain string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var m, t uint32
	var p uint8
	if n, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || n != 3 || m == 0 || t == 0 || p == 0 {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return false
	}
	computed := argon2.IDKey([]byte(plain), salt, t, m, p, uint32(len(key)))
	return subtle.ConstantTimeCompare(computed, key) == 1
}
```

Then: `go mod tidy` (moves `golang.org/x/crypto` from indirect to direct).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/infra/auth/ -v`
Expected: PASS (new tests and the existing golden tests).

- [ ] **Step 5: Freeze a golden vector**

Generate one fixed-salt vector and freeze it as a test, so any future drift in the encoding breaks loudly (same spirit as `TestEncode_FixedIV`):

Run:
```bash
cat > /tmp/claude-1000/-home-dmitry-dev-econumo-econumo/d1b5ab39-d0c0-42a9-a071-7f517892aee2/scratchpad/genvec_test.go <<'EOF'
package auth

import (
	"fmt"
	"testing"
)

func TestGenerateArgon2idVector(t *testing.T) {
	fmt.Println(encodeArgon2id("s3cret-password", []byte("0123456789abcdef"), argonMemoryKiB, argonTime, argonThreads, argonKeyLen))
}
EOF
cp /tmp/claude-1000/-home-dmitry-dev-econumo-econumo/d1b5ab39-d0c0-42a9-a071-7f517892aee2/scratchpad/genvec_test.go internal/infra/auth/zz_genvec_test.go
go test ./internal/infra/auth/ -run TestGenerateArgon2idVector -v
rm internal/infra/auth/zz_genvec_test.go
```

Copy the printed PHC string into a new test appended to `argon2id_test.go` (replace `<PASTE THE PRINTED STRING HERE>` with the actual output — it will start with `$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$`):

```go
// TestArgon2id_GoldenVector freezes the exact PHC output for a fixed salt so an
// accidental change to the encoding or parameters is caught here, not by locked
// out users.
func TestArgon2id_GoldenVector(t *testing.T) {
	const golden = "<PASTE THE PRINTED STRING HERE>"
	got := encodeArgon2id("s3cret-password", []byte("0123456789abcdef"), argonMemoryKiB, argonTime, argonThreads, argonKeyLen)
	if got != golden {
		t.Errorf("encodeArgon2id drifted:\n got=%s\nwant=%s", got, golden)
	}
	if !verifyArgon2id(golden, "s3cret-password") {
		t.Error("golden vector does not verify")
	}
}
```

Run: `go test ./internal/infra/auth/ -run TestArgon2id_GoldenVector -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/infra/auth/argon2id.go internal/infra/auth/argon2id_test.go go.mod go.sum
git commit -m "feat: argon2id PHC hash/verify primitives (#64)"
```

---

### Task 3: Hasher dispatch API; register writes argon2id; login dispatches

**Files:**
- Modify: `internal/infra/auth/password.go` (API reshape)
- Modify: `internal/infra/auth/crypto_golden_test.go` (renamed methods)
- Modify: `internal/model/user.go` (NewUser stamps argon2id)
- Modify: `internal/user/register.go`, `internal/user/login.go`
- Modify: `internal/test/fixture/entities.go` (renamed legacy hash method)
- Modify: `internal/user/admin_integration_test.go` (Verify call sites)
- Test: `internal/user/api/algorithm_test.go` (new)

**Interfaces:**
- Consumes: Task 1's `model.Algorithm*` constants and `User.Algorithm`; Task 2's `hashArgon2id`/`verifyArgon2id`.
- Produces the final public hasher API used by Tasks 4–5:
  - `func (h *PasswordHasher) Hash(plain string) (string, error)` — argon2id.
  - `func (h *PasswordHasher) Verify(algorithm, hashedPassword, plainPassword, salt string) bool` — dispatch.
  - `func (h *PasswordHasher) HashSHA512(plainPassword, salt string) string` — the legacy hash (fixtures, migration tests).

- [ ] **Step 1: Write the failing end-to-end test**

Create `internal/user/api/algorithm_test.go`:

```go
package api_test

import (
	"net/http"
	"testing"
)

// TestCredentialsAlgorithm covers issue #64's core matrix over the real HTTP
// handlers: legacy (sha512) users keep logging in, registration writes
// argon2id, and both kinds of hash verify only their own password.
func TestCredentialsAlgorithm(t *testing.T) {
	h := newHarness(t)

	// The fixture-seeded user is a legacy sha512 account.
	var alg string
	if err := h.db.QueryRow(`SELECT algorithm FROM users WHERE id = ?`, seedUserID).Scan(&alg); err != nil {
		t.Fatalf("read seed algorithm: %v", err)
	}
	if alg != "sha512" {
		t.Fatalf("seed user algorithm = %q, want sha512", alg)
	}
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": seedPassword,
	}); st != http.StatusOK {
		t.Fatalf("legacy login = %d; body: %s", st, env.raw)
	}
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": "wrong-pw",
	}); st != http.StatusUnauthorized {
		t.Fatalf("legacy login with wrong password = %d, want 401", st)
	}

	// Registration creates an argon2id user.
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/register-user", "", map[string]string{
		"name": "New User", "email": "new@example.test", "password": "brand-new-pw",
	}); st != http.StatusOK {
		t.Fatalf("register = %d; body: %s", st, env.raw)
	}
	var newAlg, newHash string
	if err := h.db.QueryRow(`SELECT algorithm, password FROM users WHERE email <> '' AND id <> ? ORDER BY created_at DESC LIMIT 1`,
		seedUserID).Scan(&newAlg, &newHash); err != nil {
		t.Fatalf("read new user: %v", err)
	}
	if newAlg != "argon2id" {
		t.Errorf("new user algorithm = %q, want argon2id", newAlg)
	}
	if len(newHash) == 0 || newHash[0] != '$' {
		t.Errorf("new user hash is not a PHC string: %q", newHash)
	}

	// The argon2id user can log in; a wrong password is rejected.
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": "new@example.test", "password": "brand-new-pw",
	}); st != http.StatusOK {
		t.Fatalf("argon2id login = %d; body: %s", st, env.raw)
	}
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": "new@example.test", "password": "wrong-pw",
	}); st != http.StatusUnauthorized {
		t.Fatalf("argon2id login with wrong password = %d, want 401", st)
	}
}
```

Note: `h.do(t, method, path, token, body)` returns `(int, envelope)` where `envelope` has a `raw` field — verified in `harness_test.go:173`.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/user/api/ -run TestCredentialsAlgorithm -v`
Expected: FAIL — the new-user row has `algorithm = "sha512"` (register still writes the legacy hash).

- [ ] **Step 3: Reshape the hasher API**

In `internal/infra/auth/password.go`:
- Rename the existing `Hash` method to `HashSHA512` (same body, same doc semantics).
- Rename the existing `Verify` method to `verifySHA512` (unexported; same body).
- Add the new public API:

```go
import (
	// add to the existing imports:
	"github.com/econumo/econumo/internal/model"
)

// Hash hashes a NEW plaintext password — always with the current algorithm
// (argon2id). Legacy sha512 hashes are only ever verified, never produced,
// except through HashSHA512 (fixtures and the salt-removal migration).
func (h *PasswordHasher) Hash(plainPassword string) (string, error) {
	return hashArgon2id(plainPassword)
}

// Verify dispatches on the algorithm recorded next to the stored hash.
// Unknown algorithm values fail closed.
func (h *PasswordHasher) Verify(algorithm, hashedPassword, plainPassword, salt string) bool {
	switch algorithm {
	case model.AlgorithmSHA512:
		return h.verifySHA512(hashedPassword, plainPassword, salt)
	case model.AlgorithmArgon2id:
		return verifyArgon2id(hashedPassword, plainPassword)
	default:
		return false
	}
}
```

(`internal/infra` may import `internal/model`: archtest forbids only leaf→feature imports, and `model` is a leaf. Verify with `go test ./internal/test/archtest/` in step 6.)

Update `crypto_golden_test.go`'s password vectors to the renamed methods:

```go
	for _, tc := range v.PasswordHasher {
		got := h.HashSHA512(tc.Password, tc.Salt)
		if got != tc.Hash {
			t.Errorf("HashSHA512(%q,%q)\n got=%s\nwant=%s", tc.Password, tc.Salt, got, tc.Hash)
		}
		if !h.Verify(model.AlgorithmSHA512, tc.Hash, tc.Password, tc.Salt) {
			t.Errorf("Verify failed for password %q", tc.Password)
		}
		if h.Verify(model.AlgorithmSHA512, tc.Hash, tc.Password+"x", tc.Salt) {
			t.Errorf("Verify wrongly accepted bad password for %q", tc.Password)
		}
	}
```

(add the `model` import to the test file).

Add a dispatch test to `argon2id_test.go`:

```go
func TestPasswordHasher_VerifyDispatch(t *testing.T) {
	h := NewPasswordHasher()
	legacy := h.HashSHA512("pw", "somesalt")
	modern, err := h.Hash("pw")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !h.Verify(model.AlgorithmSHA512, legacy, "pw", "somesalt") {
		t.Error("sha512 dispatch failed")
	}
	if !h.Verify(model.AlgorithmArgon2id, modern, "pw", "") {
		t.Error("argon2id dispatch failed")
	}
	if h.Verify(model.AlgorithmArgon2id, legacy, "pw", "somesalt") {
		t.Error("argon2id verifier accepted a sha512 hash")
	}
	if h.Verify(model.AlgorithmSHA512, modern, "pw", "") {
		t.Error("sha512 verifier accepted an argon2id hash")
	}
	if h.Verify("", legacy, "pw", "somesalt") || h.Verify("md5", legacy, "pw", "somesalt") {
		t.Error("unknown algorithm must fail closed")
	}
}
```

(add the `model` import: `"github.com/econumo/econumo/internal/model"`).

- [ ] **Step 4: Flip register + login and fix the compile ripple**

`internal/model/user.go` — `NewUser` now stamps the new scheme:

```go
		Algorithm:  AlgorithmArgon2id,
```

`internal/user/register.go` `createUser` — replace the hash line:

```go
	now := s.clock.Now()
	passwordHash, herr := s.hasher.Hash(password)
	if herr != nil {
		return nil, herr
	}
```

(the `salt` generation and `NewUser(..., passwordHash, salt, now)` call stay — the column is NOT NULL and legacy verification of other users needs the code path intact).

`internal/user/login.go:26`:

```go
	if !u.IsActive || !s.hasher.Verify(u.Algorithm, u.Password, req.Password, u.Salt) {
```

Task 1 left three `UpdatePassword` callers on `s.hasher.Hash(..., u.Salt)` — that method is now `HashSHA512`. Keep this task compiling without changing their behavior (Tasks 4–5 own those flows):
- `internal/user/password.go:43` and `:113`, `internal/user/admin.go:67`: `s.hasher.Hash(x, u.Salt)` → `s.hasher.HashSHA512(x, u.Salt)`.

`internal/test/fixture/entities.go:58`: `b.hasher.Hash(u.Password, u.Salt)` → `b.hasher.HashSHA512(u.Password, u.Salt)`.

`internal/user/admin_integration_test.go` — the two `hasher.Verify(u.Password, pw, u.Salt)` assertions (lines ~60 and ~135) become `hasher.Verify(u.Algorithm, u.Password, pw, u.Salt)`.

Then sweep for stragglers: `grep -rn "hasher.Hash(\|hasher.Verify(\|\.Hash(.*Salt" internal/ | grep -v "_test.go.*encode\|encode\.Hash"` — every remaining `PasswordHasher` call must be one of the three new methods.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/user/api/ -run TestCredentialsAlgorithm -v`
Expected: PASS

Run: `go test ./internal/infra/auth/ ./internal/user/... ./internal/test/archtest/ -count=1`
Expected: PASS (golden vectors still pass — the sha512 output is unchanged; archtest accepts the auth→model import).

- [ ] **Step 6: Run the full smoke tier**

Run: `make go-test`
Expected: PASS, coverage ≥ the gate, and `git status --short internal/test/apiparity/testdata/golden/` shows NO modified goldens. If a golden changed, observable behavior changed — stop and investigate; do not regenerate.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: argon2id for new users, algorithm-dispatched login verify (#64)"
```

---

### Task 4: update-password transitions the account to argon2id

**Files:**
- Modify: `internal/user/password.go` (`UpdatePassword` use case)
- Test: `internal/user/api/algorithm_test.go` (extend)

**Interfaces:**
- Consumes: Task 3's `Hash(plain) (string, error)` and `Verify(algorithm, ...)`; `model.AlgorithmArgon2id`.
- Produces: nothing new for later tasks.

- [ ] **Step 1: Write the failing test**

Append to `internal/user/api/algorithm_test.go`:

```go
// TestUpdatePasswordTransitionsAlgorithm: a legacy user who changes their
// password moves to argon2id; a wrong old password leaves the row untouched.
func TestUpdatePasswordTransitionsAlgorithm(t *testing.T) {
	h := newHarness(t)

	st, env := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": seedPassword,
	})
	if st != http.StatusOK {
		t.Fatalf("login = %d; body: %s", st, env.raw)
	}
	token := mustUnmarshal[loginResult](t, env.raw).Token

	// Wrong old password: 400, row stays sha512.
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/update-password", token, map[string]string{
		"oldPassword": "not-the-password", "newPassword": "upgraded-pw-1",
	}); st != http.StatusBadRequest {
		t.Fatalf("update with wrong old password = %d, want 400", st)
	}
	var alg string
	if err := h.db.QueryRow(`SELECT algorithm FROM users WHERE id = ?`, seedUserID).Scan(&alg); err != nil {
		t.Fatalf("read algorithm: %v", err)
	}
	if alg != "sha512" {
		t.Fatalf("algorithm after failed update = %q, want sha512", alg)
	}

	// Correct old password: transition.
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/update-password", token, map[string]string{
		"oldPassword": seedPassword, "newPassword": "upgraded-pw-1",
	}); st != http.StatusOK {
		t.Fatalf("update-password = %d; body: %s", st, env.raw)
	}
	var hash string
	if err := h.db.QueryRow(`SELECT algorithm, password FROM users WHERE id = ?`, seedUserID).Scan(&alg, &hash); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if alg != "argon2id" {
		t.Errorf("algorithm after update = %q, want argon2id", alg)
	}
	if len(hash) == 0 || hash[0] != '$' {
		t.Errorf("hash is not a PHC string: %q", hash)
	}

	// New password logs in (through the argon2id path); old one doesn't.
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": "upgraded-pw-1",
	}); st != http.StatusOK {
		t.Fatalf("login with new password = %d, want 200", st)
	}
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": seedPassword,
	}); st != http.StatusUnauthorized {
		t.Fatalf("login with old password = %d, want 401", st)
	}
}
```

Note: `loginResult` and `mustUnmarshal` already exist in `user_endpoints_test.go` (same package). The `oldPassword`/`newPassword` field names match `model.UpdatePasswordRequest`'s JSON tags (verified in `internal/model/user_dto.go:190-192`).

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/user/api/ -run TestUpdatePasswordTransitionsAlgorithm -v`
Expected: FAIL — algorithm stays `"sha512"` after the successful update.

- [ ] **Step 3: Implement the transition**

In `internal/user/password.go`, `UpdatePassword`'s mutate closure becomes:

```go
		if !s.hasher.Verify(u.Algorithm, u.Password, req.OldPassword, u.Salt) {
			return errs.NewValidation("Password is not correct")
		}
		newHash, herr := s.hasher.Hash(req.NewPassword)
		if herr != nil {
			return herr
		}
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, now)
		return nil
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/user/... -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/user/password.go internal/user/api/algorithm_test.go
git commit -m "feat: update-password rehashes to argon2id (#64)"
```

---

### Task 5: reset-password and CLI change-password transition too

**Files:**
- Modify: `internal/user/password.go` (`ResetPassword`)
- Modify: `internal/user/admin.go` (`AdminChangePassword`)
- Test: `internal/user/api/reset_password_test.go` (extend), `internal/user/admin_integration_test.go` (extend)

**Interfaces:**
- Consumes: Task 3's hasher API.
- Produces: nothing new.

- [ ] **Step 1: Write the failing assertions**

In `internal/user/api/reset_password_test.go`, `TestRemindAndResetPassword`, right after step 4 (the successful reset), add:

```go
	// The reset rehashed the account to argon2id (issue #64).
	var alg string
	if err := h.db.QueryRow(`SELECT algorithm FROM users WHERE id = ?`, seedUserID).Scan(&alg); err != nil {
		t.Fatalf("read algorithm: %v", err)
	}
	if alg != "argon2id" {
		t.Errorf("algorithm after reset = %q, want argon2id", alg)
	}
```

In `internal/user/admin_integration_test.go`, find the change-password test (~line 120, asserting `hasher.Verify` of `"brandnew"`) and add after the existing verify assertion:

```go
	if u.Algorithm != model.AlgorithmArgon2id {
		t.Errorf("algorithm after admin change-password = %q, want %q", u.Algorithm, model.AlgorithmArgon2id)
	}
```

(add the `model` import if the file lacks it). Note: after Task 3, `newUserSvc`-created users come from `createUser` → already argon2id; the assertion still proves the *change-password path* writes argon2id because Step 3 changes it from `HashSHA512`+`AlgorithmSHA512` to the new pair — to make the test bite on a legacy row, seed the user's row back to sha512 first:

```go
	// Force the account to the legacy scheme so the test proves the transition.
	legacyHash := hasherLegacy(t, hasher, "oldpass", u.Salt)
	if _, err := db.Raw.Exec(db.Rebind(`UPDATE users SET password = ?, algorithm = 'sha512' WHERE id = ?`), legacyHash, u.ID.String()); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
```

where `hasherLegacy` is simply `hasher.HashSHA512` inlined (`legacyHash := hasher.HashSHA512("oldpass", u.Salt)`) — use the direct call, not a helper. Adjust variable names to the test's actual locals after reading it.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/user/api/ -run TestRemindAndResetPassword -v && go test ./internal/user/ -run ChangePassword -v`
Expected: FAIL — algorithm stays `"sha512"` after reset / admin change (reset seeds from the fixture's sha512 user; admin test after the forced legacy UPDATE).

- [ ] **Step 3: Implement both transitions**

`internal/user/password.go`, `ResetPassword`'s tx closure:

```go
	newHash, herr := s.hasher.Hash(req.Password)
	if herr != nil {
		return nil, herr
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, s.clock.Now())
		if serr := s.repo.Save(ctx, u); serr != nil {
			return serr
		}
		return s.passwordRequests.Delete(ctx, pr.ID)
	}); err != nil {
```

`internal/user/admin.go`, `AdminChangePassword`:

```go
	newHash, herr := s.hasher.Hash(newPassword)
	if herr != nil {
		return herr
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, s.clock.Now())
		return s.repo.Save(ctx, u)
	})
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/user/... -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/user/password.go internal/user/admin.go internal/user/api/reset_password_test.go internal/user/admin_integration_test.go
git commit -m "feat: reset-password and CLI change-password rehash to argon2id (#64)"
```

---

### Task 6: Documentation + full verification

**Files:**
- Modify: `CLAUDE.md` (Auth crypto + model notes)
- Modify: `docs/superpowers/specs/2026-07-09-credentials-algorithm-design.md` (fixture note)
- Verify: full test suites

- [ ] **Step 1: Update CLAUDE.md's frozen-contract section**

In the "Auth crypto" bullet of `CLAUDE.md`, replace the password-hash sentence with versioned wording (keep the rest of the bullet intact):

```markdown
- **Password hash**: versioned by `users.algorithm`. `sha512` (legacy, all pre-existing rows): sha512, 500 iterations, base64 (88 chars), salt merged as `password{salt}`; `digest = sha512(salted)` then 499 rounds of `sha512(digest || salted)`; verify rejects len≠88 or a `$`, constant-time. `argon2id` (every new hash: registration and all password changes): PHC string `$argon2id$v=19$m=19456,t=2,p=1$…$…` (OWASP params), salt embedded in the hash — the `salt` column persists for sha512 rows. Verification dispatches on the column; unknown values fail closed.
```

- [ ] **Step 2: Amend the spec's fixture line**

In the spec's call-site table, the fixture row said "seeds argon2id users". Replace with:

```markdown
| `internal/test/fixture` | unchanged hashing: seeds legacy `sha512` users (explicit `algorithm` column) — keeps suites fast and continuously exercises the legacy login path |
```

- [ ] **Step 3: Run the full gates**

Run: `make go-test`
Expected: PASS; coverage ≥ gate; `git status --short internal/test/apiparity/testdata/golden/` empty.

Run: `make test` (provisions PostgreSQL via compose; runs the sqlite-vs-pgsql engine-comparison suite + frontend tests)
Expected: PASS. If Docker/PostgreSQL is unavailable, run what's possible and report the gap explicitly — do not claim the engine-comparison suite passed.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md docs/superpowers/specs/2026-07-09-credentials-algorithm-design.md
git commit -m "docs: document credentials algorithm versioning (#64)"
```
