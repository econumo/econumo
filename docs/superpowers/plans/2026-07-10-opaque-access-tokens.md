# Opaque Access Tokens (Sessions + PATs) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace stateless JWTs with DB-backed opaque tokens: revocable login sessions with a sliding 30-day TTL, personal access tokens (PATs), a sessions/tokens UI in the profile's new Security section, and full removal of the JWT machinery.

**Architecture:** A new `access_tokens` table stores SHA-256 hashes of opaque tokens (`eco_ses_*` sessions, `eco_pat_*` PATs). The `user` feature owns the entity, repository, and all use cases (authenticate, session CRUD, PAT CRUD, revocation cascades); the auth middleware swaps its JWT verifier for a `TokenAuthenticator` backed by the user service. All expiry/validity comparison happens in Go (never in SQL) to stay engine-agnostic, following the password-request precedent.

**Tech Stack:** Go stdlib (`crypto/rand`, `crypto/sha256`, `encoding/base64`), sqlc v1.30 for both engines, React 19 + TanStack Query + vitest for the SPA.

**Spec:** `docs/superpowers/specs/2026-07-10-opaque-tokens-sessions-design.md`

## Global Constraints

- Token format: `eco_ses_` / `eco_pat_` prefix + `base64.RawURLEncoding` of 32 random bytes (43 chars, alphabet `[A-Za-z0-9_-]`). DB stores only `hex(sha256(full token))`.
- Constants (no new env vars): session TTL `30 * 24 * time.Hour`, touch throttle `5 * time.Minute`, dead-row retention `30 * 24 * time.Hour`.
- 401 messages change to exactly `"Access token not found"` (missing/malformed header) and `"Invalid access token"` (unknown/expired/revoked). Everything else about the error envelope is frozen.
- Login's raw `{token, user}` top-level response shape is frozen; only the token contents change. `logout-user`'s frozen `{"result":"test"}` quirk stays.
- Datetimes on the wire and in DB use `datetime.Layout` (`"2006-01-02 15:04:05"`). Nullable JSON fields (`expiresAt`) are `null`, not `""`.
- Validity/expiry comparisons happen in Go, NEVER in SQL (engine date-format differences — see the password_request.sql precedent).
- Never hand-edit a golden. Regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` and inspect the diff — every golden change in this project is an intended behavior change from this plan.
- Every commit leaves `make go-test` green (build + vet + gofmt + docs-fresh + sqlite tests + coverage ≥72). Frontend tasks keep `make web-test` and `make web-lint` green.
- Commit trailer on every commit:
  `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>` and
  `Claude-Session: https://claude.ai/code/session_01H7DuYzcxc78qDyGB8aT2Wv`

---

### Task 1: Schema + sqlc queries (both engines)

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260710000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260710000000.sql`
- Create: `internal/infra/storage/sqlc/query/sqlite/access_tokens.sql`
- Create: `internal/infra/storage/sqlc/query/pgsql/access_tokens.sql`
- Regenerate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}` (via `sqlc generate`)

**Interfaces:**
- Consumes: nothing new.
- Produces: table `access_tokens`; generated `sqlitegen.AccessToken` / `pgsqlgen.AccessToken` structs (field-identical: `ID, UserID, Kind, TokenHash string; Name, UserAgent *string; CreatedAt, LastUsedAt time.Time; ExpiresAt, RevokedAt *time.Time`) and methods `InsertAccessToken`, `GetAccessTokenByHash`, `UpdateAccessToken`, `ListAccessTokensByUser`, `DeleteAccessToken`.

- [ ] **Step 1: Write the sqlite migration**

`internal/infra/storage/migrations/sqlite/20260710000000.sql`:

```sql
-- Opaque access tokens: login sessions and personal access tokens (PATs).
-- Only the sha256 hash of a token is ever stored; validity/expiry is
-- evaluated in Go (not SQL) to avoid engine date-format differences.
CREATE TABLE access_tokens
(
    id           TEXT NOT NULL
    , user_id      TEXT NOT NULL
    , kind         TEXT NOT NULL
    , token_hash   TEXT NOT NULL
    , name         TEXT DEFAULT NULL
    , user_agent   TEXT DEFAULT NULL
    , created_at   DATETIME NOT NULL
    , last_used_at DATETIME NOT NULL
    , expires_at   DATETIME DEFAULT NULL
    , revoked_at   DATETIME DEFAULT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX UNIQ_access_tokens_token_hash ON access_tokens (token_hash);
CREATE INDEX IDX_access_tokens_user_id ON access_tokens (user_id);
```

- [ ] **Step 2: Write the pgsql migration**

`internal/infra/storage/migrations/pgsql/20260710000000.sql`:

```sql
-- Opaque access tokens: login sessions and personal access tokens (PATs).
-- Only the sha256 hash of a token is ever stored; validity/expiry is
-- evaluated in Go (not SQL) to avoid engine date-format differences.
CREATE TABLE access_tokens
(
    id           TEXT NOT NULL
    , user_id      TEXT NOT NULL
    , kind         TEXT NOT NULL
    , token_hash   TEXT NOT NULL
    , name         TEXT DEFAULT NULL
    , user_agent   TEXT DEFAULT NULL
    , created_at   TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , last_used_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expires_at   TIMESTAMP(0) WITHOUT TIME ZONE DEFAULT NULL
    , revoked_at   TIMESTAMP(0) WITHOUT TIME ZONE DEFAULT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX UNIQ_access_tokens_token_hash ON access_tokens (token_hash);
CREATE INDEX IDX_access_tokens_user_id ON access_tokens (user_id);
```

Note: existing pgsql tables use `UUID` for id columns, but the sqlc type
overrides map both to Go `string`; `TEXT` here avoids a `uuid.UUID` mapping
surprise and matches the sqlite sibling. If `sqlc generate` (Step 4) produces
non-string ids anyway, check `sqlc.yaml` overrides and mirror what the
`operation_requests_ids` (TEXT-keyed) table does.

- [ ] **Step 3: Write the query files**

`internal/infra/storage/sqlc/query/sqlite/access_tokens.sql`:

```sql
-- Access-token queries (access_tokens): login sessions + personal access
-- tokens. Liveness (revoked/expired) is evaluated in the app layer (Go
-- time.Time), not in SQL, to avoid engine date-format differences — the
-- list/get queries return raw rows.

-- name: InsertAccessToken :exec
INSERT INTO access_tokens (id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetAccessTokenByHash :one
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE token_hash = ?;

-- name: GetAccessTokenByID :one
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE id = ?;

-- name: UpdateAccessToken :exec
UPDATE access_tokens SET last_used_at = ?, expires_at = ?, revoked_at = ? WHERE id = ?;

-- name: ListAccessTokensByUser :many
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE user_id = ? AND kind = ?
ORDER BY created_at, id;

-- name: DeleteAccessToken :exec
DELETE FROM access_tokens WHERE id = ?;
```

`internal/infra/storage/sqlc/query/pgsql/access_tokens.sql` — identical
statements with `$1..$n` placeholders (see the sqlite sibling's comment
convention: reference it instead of repeating the doc comment):

```sql
-- Access-token queries (access_tokens). See the sqlite sibling for the flow;
-- liveness is evaluated in the app layer, not SQL.

-- name: InsertAccessToken :exec
INSERT INTO access_tokens (id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetAccessTokenByHash :one
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE token_hash = $1;

-- name: GetAccessTokenByID :one
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE id = $1;

-- name: UpdateAccessToken :exec
UPDATE access_tokens SET last_used_at = $1, expires_at = $2, revoked_at = $3 WHERE id = $4;

-- name: ListAccessTokensByUser :many
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE user_id = $1 AND kind = $2
ORDER BY created_at, id;

-- name: DeleteAccessToken :exec
DELETE FROM access_tokens WHERE id = $1;
```

- [ ] **Step 4: Regenerate sqlc and verify the build**

Run from repo root:
```bash
go run github.com/sqlc-dev/sqlc/cmd/sqlc generate -f internal/infra/storage/sqlc/sqlc.yaml
go build ./...
```
(Choose the invocation the Makefile/`go generate` uses — check `grep -rn "sqlc" Makefile internal/infra/storage/sqlc/*.go` first and mirror it.)

Expected: both `gen/sqlite/access_tokens.sql.go` and `gen/pgsql/access_tokens.sql.go` exist; `AccessToken` struct fields are exactly `ID, UserID, Kind, TokenHash string; Name, UserAgent *string; CreatedAt, LastUsedAt time.Time; ExpiresAt, RevokedAt *time.Time` in BOTH packages (field-identical is what the pgsql whole-struct conversion shim needs later). Build passes.

- [ ] **Step 5: Run the migration smoke check**

Run: `go test ./internal/infra/storage/... ./internal/test/dbtest/... 2>&1 | tail -5`
Expected: PASS (dbtest applies all migrations including the new one).

- [ ] **Step 6: Commit**

```bash
git add internal/infra/storage
git commit -m "feat(auth): add access_tokens schema and sqlc queries"
```

---

### Task 2: AccessToken model entity + unit tests

**Files:**
- Create: `internal/model/token.go`
- Create: `internal/model/token_test.go`

**Interfaces:**
- Consumes: `vo.Id`, stdlib `time`.
- Produces: `model.TokenKindSession = "session"`, `model.TokenKindPersonal = "personal"`; `model.AccessToken` struct (fields exactly as below); methods `IsLive(now) bool`, `NeedsTouch(now, interval) bool`, `Touch(now, sessionTTL)`, `Revoke(now)`, `IsDead(now, retention) bool`.

- [ ] **Step 1: Write failing unit tests**

`internal/model/token_test.go`:

```go
package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func tokenAt(kind string, exp *time.Time) *AccessToken {
	base := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	return &AccessToken{
		ID: vo.NewId(), UserID: vo.NewId(), Kind: kind, TokenHash: "h",
		CreatedAt: base, LastUsedAt: base, ExpiresAt: exp,
	}
}

func TestAccessToken_IsLive(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	if !tokenAt(TokenKindPersonal, nil).IsLive(now) {
		t.Error("nil expiry (never expires) must be live")
	}
	if !tokenAt(TokenKindSession, &future).IsLive(now) {
		t.Error("future expiry must be live")
	}
	if tokenAt(TokenKindSession, &past).IsLive(now) {
		t.Error("past expiry must be dead")
	}
	revoked := tokenAt(TokenKindPersonal, nil)
	revoked.Revoke(now)
	if revoked.IsLive(now.Add(time.Second)) {
		t.Error("revoked must be dead")
	}
}

func TestAccessToken_TouchSlidesSessionOnly(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	ttl := 30 * 24 * time.Hour

	s := tokenAt(TokenKindSession, nil)
	s.Touch(now, ttl)
	if s.LastUsedAt != now {
		t.Errorf("LastUsedAt = %v, want %v", s.LastUsedAt, now)
	}
	if s.ExpiresAt == nil || !s.ExpiresAt.Equal(now.Add(ttl)) {
		t.Errorf("session ExpiresAt = %v, want %v", s.ExpiresAt, now.Add(ttl))
	}

	patExp := now.Add(48 * time.Hour)
	p := tokenAt(TokenKindPersonal, &patExp)
	p.Touch(now, ttl)
	if p.ExpiresAt == nil || !p.ExpiresAt.Equal(patExp) {
		t.Errorf("PAT expiry must not slide: got %v, want %v", p.ExpiresAt, patExp)
	}
	if p.LastUsedAt != now {
		t.Errorf("PAT LastUsedAt = %v, want %v", p.LastUsedAt, now)
	}
}

func TestAccessToken_NeedsTouch(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	tok := tokenAt(TokenKindSession, nil)
	tok.LastUsedAt = now.Add(-4 * time.Minute)
	if tok.NeedsTouch(now, 5*time.Minute) {
		t.Error("4 minutes old: no touch")
	}
	tok.LastUsedAt = now.Add(-5 * time.Minute)
	if !tok.NeedsTouch(now, 5*time.Minute) {
		t.Error("5 minutes old: touch")
	}
}

func TestAccessToken_RevokeIsIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	tok := tokenAt(TokenKindSession, nil)
	tok.Revoke(now)
	first := *tok.RevokedAt
	tok.Revoke(now.Add(time.Hour))
	if !tok.RevokedAt.Equal(first) {
		t.Error("second Revoke must not move the timestamp")
	}
}

func TestAccessToken_IsDead(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	retention := 30 * 24 * time.Hour

	live := tokenAt(TokenKindPersonal, nil)
	if live.IsDead(now, retention) {
		t.Error("live token is not dead")
	}
	oldExp := now.Add(-retention - time.Hour)
	expired := tokenAt(TokenKindSession, &oldExp)
	if !expired.IsDead(now, retention) {
		t.Error("expired past retention must be dead")
	}
	recentExp := now.Add(-time.Hour)
	recent := tokenAt(TokenKindSession, &recentExp)
	if recent.IsDead(now, retention) {
		t.Error("recently expired stays within retention")
	}
	revoked := tokenAt(TokenKindSession, nil)
	revoked.Revoke(now.Add(-retention - time.Hour))
	if !revoked.IsDead(now, retention) {
		t.Error("revoked past retention must be dead")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/model/ -run TestAccessToken -v 2>&1 | head -20`
Expected: FAIL (compile error: undefined AccessToken).

- [ ] **Step 3: Implement the entity**

`internal/model/token.go`:

```go
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Access-token kinds: a login session (sliding expiry) or a personal access
// token (fixed or no expiry, created explicitly by the user for integrations).
const (
	TokenKindSession  = "session"
	TokenKindPersonal = "personal"
)

// AccessToken is one opaque bearer credential. Only the sha256 hash of the
// token string is stored; the raw token exists client-side only.
type AccessToken struct {
	ID         vo.Id
	UserID     vo.Id
	Kind       string
	TokenHash  string
	Name       *string    // PAT only: the user-given label
	UserAgent  *string    // session only: User-Agent captured at login
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  *time.Time // nil = never expires (PAT); sessions always have one
	RevokedAt  *time.Time
}

func (t *AccessToken) IsLive(now time.Time) bool {
	if t.RevokedAt != nil {
		return false
	}
	return t.ExpiresAt == nil || t.ExpiresAt.After(now)
}

// NeedsTouch reports whether the last-used stamp is stale enough to persist —
// the write-throttle that keeps single-writer SQLite off the hot path.
func (t *AccessToken) NeedsTouch(now time.Time, interval time.Duration) bool {
	return now.Sub(t.LastUsedAt) >= interval
}

// Touch advances the last-used stamp; sessions also slide their expiry window
// (a PAT's expiry is a promise made at creation and never moves).
func (t *AccessToken) Touch(now time.Time, sessionTTL time.Duration) {
	t.LastUsedAt = now
	if t.Kind == TokenKindSession {
		exp := now.Add(sessionTTL)
		t.ExpiresAt = &exp
	}
}

func (t *AccessToken) Revoke(now time.Time) {
	if t.RevokedAt == nil {
		t.RevokedAt = &now
	}
}

// IsDead reports whether the row has been expired/revoked for longer than the
// retention window and can be purged.
func (t *AccessToken) IsDead(now time.Time, retention time.Duration) bool {
	if t.RevokedAt != nil && now.Sub(*t.RevokedAt) > retention {
		return true
	}
	if t.ExpiresAt != nil && now.Sub(*t.ExpiresAt) > retention {
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/model/ -run TestAccessToken -v`
Expected: PASS (all five tests).

- [ ] **Step 5: Commit**

```bash
git add internal/model/token.go internal/model/token_test.go
git commit -m "feat(auth): AccessToken model entity with sliding-expiry lifecycle"
```

---

### Task 3: Token generation + hashing in the user feature

**Files:**
- Create: `internal/user/token.go`
- Create: `internal/user/token_test.go`
- Modify: `docs/superpowers/specs/2026-07-10-opaque-tokens-sessions-design.md` (one line: base62 → base64url alphabet)

**Interfaces:**
- Consumes: `model.TokenKindSession/TokenKindPersonal`.
- Produces: `user.HashAccessToken(raw string) string` (exported — the test
  fixture seeds rows with known raw tokens); package-private
  `generateAccessToken(kind string) (raw, hash string, err error)`; constants
  `SessionTTL = 30*24*time.Hour` (exported; the harness computes seed expiries),
  `touchInterval = 5*time.Minute`, `deadTokenRetention = 30*24*time.Hour`.

- [ ] **Step 1: Write failing tests**

`internal/user/token_test.go`:

```go
package user

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"testing"

	"github.com/econumo/econumo/internal/model"
)

var tokenRe = regexp.MustCompile(`^eco_(ses|pat)_[A-Za-z0-9_-]{43}$`)

func TestGenerateAccessToken(t *testing.T) {
	rawSes, hashSes, err := generateAccessToken(model.TokenKindSession)
	if err != nil {
		t.Fatalf("generate session: %v", err)
	}
	if !tokenRe.MatchString(rawSes) || rawSes[:8] != "eco_ses_" {
		t.Errorf("session token %q does not match eco_ses_<43 urlsafe chars>", rawSes)
	}
	sum := sha256.Sum256([]byte(rawSes))
	if hashSes != hex.EncodeToString(sum[:]) {
		t.Errorf("hash mismatch: %q", hashSes)
	}

	rawPat, _, err := generateAccessToken(model.TokenKindPersonal)
	if err != nil {
		t.Fatalf("generate pat: %v", err)
	}
	if rawPat[:8] != "eco_pat_" {
		t.Errorf("pat token %q must start with eco_pat_", rawPat)
	}

	raw2, _, _ := generateAccessToken(model.TokenKindSession)
	if raw2 == rawSes {
		t.Error("two generated tokens must differ")
	}
}

func TestHashAccessToken(t *testing.T) {
	sum := sha256.Sum256([]byte("eco_ses_x"))
	if got := HashAccessToken("eco_ses_x"); got != hex.EncodeToString(sum[:]) {
		t.Errorf("HashAccessToken = %q", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/ -run "TestGenerateAccessToken|TestHashAccessToken" 2>&1 | head -5`
Expected: FAIL (undefined generateAccessToken / HashAccessToken).

- [ ] **Step 3: Implement**

`internal/user/token.go`:

```go
// Opaque access-token generation and hashing. The raw token is
// "<prefix><base64url of 32 random bytes>" (43 chars of payload, 256-bit
// entropy); only its sha256 hex ever reaches the database. A plain sha256 (not
// argon2/bcrypt) is deliberate: with 256 bits of randomness brute-force is
// infeasible, and verification must stay one cheap indexed lookup per request.
package user

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"time"

	"github.com/econumo/econumo/internal/model"
)

const (
	sessionTokenPrefix  = "eco_ses_"
	personalTokenPrefix = "eco_pat_"
	tokenRandomBytes    = 32

	// SessionTTL is the sliding window: a session dies 30 days after its last
	// use. Exported for the test harness to compute seed expiries.
	SessionTTL = 30 * 24 * time.Hour
	// touchInterval throttles last-used persistence so single-writer SQLite
	// stays off the hot path ("last active" is accurate to ±5 minutes).
	touchInterval = 5 * time.Minute
	// deadTokenRetention is how long an expired/revoked row is kept before the
	// opportunistic purge at login deletes it.
	deadTokenRetention = 30 * 24 * time.Hour
)

func generateAccessToken(kind string) (string, string, error) {
	b := make([]byte, tokenRandomBytes)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", "", err
	}
	prefix := sessionTokenPrefix
	if kind == model.TokenKindPersonal {
		prefix = personalTokenPrefix
	}
	raw := prefix + base64.RawURLEncoding.EncodeToString(b)
	return raw, HashAccessToken(raw), nil
}

// HashAccessToken maps a raw bearer token to its storage/lookup key.
func HashAccessToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/user/ -run "TestGenerateAccessToken|TestHashAccessToken" -v`
Expected: PASS.

- [ ] **Step 5: Amend the spec's alphabet wording**

In `docs/superpowers/specs/2026-07-10-opaque-tokens-sessions-design.md`, replace
"The random part encodes 32 random bytes (~43 base62 chars, 256-bit entropy)."
with
"The random part encodes 32 random bytes with `base64.RawURLEncoding` (43 chars, alphabet `[A-Za-z0-9_-]`, 256-bit entropy)."

- [ ] **Step 6: Commit**

```bash
git add internal/user/token.go internal/user/token_test.go docs/superpowers/specs/2026-07-10-opaque-tokens-sessions-design.md
git commit -m "feat(auth): opaque token generation and hashing"
```

---

### Task 4: AccessTokens repository (engine-adapter pattern)

**Files:**
- Modify: `internal/user/repository.go` (append the `AccessTokens` interface)
- Create: `internal/user/repo/accesstoken.go`
- Create: `internal/user/repo/accesstoken_sqlite.go`
- Create: `internal/user/repo/accesstoken_pgsql.go`
- Create: `internal/user/repo/accesstoken_integration_test.go`

**Interfaces:**
- Consumes: Task 1's generated queries; `model.AccessToken` from Task 2.
- Produces (in package `user`):

```go
// AccessTokens persists opaque bearer credentials (sessions + PATs). Liveness
// is evaluated in the domain (AccessToken.IsLive), not in SQL. GetByHash on a
// missing row returns *errs.NotFoundError.
type AccessTokens interface {
	Insert(ctx context.Context, t *model.AccessToken) error
	GetByHash(ctx context.Context, hash string) (*model.AccessToken, error)
	// GetByID loads one row (logout / revoke-by-id paths). Missing ->
	// *errs.NotFoundError.
	GetByID(ctx context.Context, id vo.Id) (*model.AccessToken, error)
	// Update persists the mutable lifecycle fields (last_used_at, expires_at,
	// revoked_at) of an existing row.
	Update(ctx context.Context, t *model.AccessToken) error
	// ListByUser returns ALL rows (live and dead) of one kind, ordered by
	// (created_at, id); callers filter with IsLive/IsDead.
	ListByUser(ctx context.Context, userID vo.Id, kind string) ([]model.AccessToken, error)
	Delete(ctx context.Context, id vo.Id) error
}
```

  and `userrepo.NewAccessTokenRepo(driver string, tx *backend.TxManager) *AccessTokenRepo`.

- [ ] **Step 1: Write the failing integration test**

`internal/user/repo/accesstoken_integration_test.go` — mirror the setup
helpers used by `repo_integration_test.go` in the same package (check how it
builds `dbtest.New(t)`, seeds a user, and gets a `*backend.TxManager`; reuse
its helpers rather than duplicating). The test body:

```go
func TestAccessTokenRepo_RoundTrip(t *testing.T) {
	// setup: db, txm, a seeded user with id userA (copy the file's pattern)
	repo := NewAccessTokenRepo(db.Engine, txm)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	exp := now.Add(30 * 24 * time.Hour)
	ua := "TestAgent/1.0"
	tok := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), Kind: model.TokenKindSession,
		TokenHash: "hash-1", UserAgent: &ua,
		CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp,
	}
	if err := repo.Insert(ctx, tok); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := repo.GetByHash(ctx, "hash-1")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if !got.ID.Equal(tok.ID) || got.Kind != model.TokenKindSession ||
		got.UserAgent == nil || *got.UserAgent != ua ||
		got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) || got.RevokedAt != nil {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	// unknown hash -> NotFound
	if _, err := repo.GetByHash(ctx, "nope"); err == nil {
		t.Fatal("GetByHash(miss) must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("GetByHash(miss) = %T, want NotFound", err)
	}

	// Update persists touch + revoke.
	later := now.Add(10 * time.Minute)
	got.Touch(later, 30*24*time.Hour)
	got.Revoke(later)
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got2, _ := repo.GetByHash(ctx, "hash-1")
	if !got2.LastUsedAt.Equal(later) || got2.RevokedAt == nil || !got2.RevokedAt.Equal(later) {
		t.Errorf("update not persisted: %+v", got2)
	}

	// ListByUser: a PAT (nil expiry, has name) + kind filtering + order.
	name := "ci token"
	pat := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), Kind: model.TokenKindPersonal,
		TokenHash: "hash-2", Name: &name, CreatedAt: now.Add(time.Second), LastUsedAt: now.Add(time.Second),
	}
	if err := repo.Insert(ctx, pat); err != nil {
		t.Fatalf("Insert pat: %v", err)
	}
	sessions, err := repo.ListByUser(ctx, vo.MustParseId(userA), model.TokenKindSession)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("ListByUser(session) = %d, %v; want 1", len(sessions), err)
	}
	pats, err := repo.ListByUser(ctx, vo.MustParseId(userA), model.TokenKindPersonal)
	if err != nil || len(pats) != 1 || pats[0].Name == nil || *pats[0].Name != name || pats[0].ExpiresAt != nil {
		t.Fatalf("ListByUser(personal) mismatch: %+v, %v", pats, err)
	}

	// Delete removes the row.
	if err := repo.Delete(ctx, tok.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByHash(ctx, "hash-1"); err == nil {
		t.Error("deleted row still found")
	}
}
```

Also add a duplicate-hash test: inserting a second row with `TokenHash:
"hash-2"` must return an error (unique index). And a GetByID pair: the PAT's
id round-trips; a random `vo.NewId()` → NotFound.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/repo/ -run TestAccessTokenRepo -v 2>&1 | head -5`
Expected: FAIL (undefined NewAccessTokenRepo).

- [ ] **Step 3: Implement the repo**

`internal/user/repo/accesstoken.go`:

```go
// AccessTokenRepo persists opaque bearer credentials (access_tokens): login
// sessions and personal access tokens. Liveness is evaluated in the domain
// (model.AccessToken.IsLive), not in SQL.
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

type (
	accessTokenRow          = sqlitegen.AccessToken
	insertAccessTokenParams = sqlitegen.InsertAccessTokenParams
	updateAccessTokenParams = sqlitegen.UpdateAccessTokenParams
	listAccessTokensParams  = sqlitegen.ListAccessTokensByUserParams
)

type accessTokenQuerier interface {
	InsertAccessToken(ctx context.Context, db backend.DBTX, p insertAccessTokenParams) error
	GetAccessTokenByHash(ctx context.Context, db backend.DBTX, hash string) (accessTokenRow, error)
	GetAccessTokenByID(ctx context.Context, db backend.DBTX, id string) (accessTokenRow, error)
	UpdateAccessToken(ctx context.Context, db backend.DBTX, p updateAccessTokenParams) error
	ListAccessTokensByUser(ctx context.Context, db backend.DBTX, p listAccessTokensParams) ([]accessTokenRow, error)
	DeleteAccessToken(ctx context.Context, db backend.DBTX, id string) error
}

type AccessTokenRepo struct {
	tx *backend.TxManager
	q  accessTokenQuerier
}

var _ user.AccessTokens = (*AccessTokenRepo)(nil)

func NewAccessTokenRepo(driver string, tx *backend.TxManager) *AccessTokenRepo {
	switch driver {
	case "sqlite":
		return &AccessTokenRepo{tx: tx, q: accessTokenSqliteQuerier{}}
	case "postgresql":
		return &AccessTokenRepo{tx: tx, q: accessTokenPgsqlQuerier{}}
	default:
		panic("accesstokenrepo: unknown database driver " + driver)
	}
}

func (r *AccessTokenRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *AccessTokenRepo) Insert(ctx context.Context, t *model.AccessToken) error {
	return r.q.InsertAccessToken(ctx, r.db(ctx), insertAccessTokenParams{
		ID: t.ID.String(), UserID: t.UserID.String(), Kind: t.Kind, TokenHash: t.TokenHash,
		Name: t.Name, UserAgent: t.UserAgent,
		CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt,
	})
}

func (r *AccessTokenRepo) GetByHash(ctx context.Context, hash string) (*model.AccessToken, error) {
	row, err := r.q.GetAccessTokenByHash(ctx, r.db(ctx), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Access token not found")
		}
		return nil, err
	}
	return accessTokenFromRow(row)
}

func (r *AccessTokenRepo) GetByID(ctx context.Context, id vo.Id) (*model.AccessToken, error) {
	row, err := r.q.GetAccessTokenByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Access token not found")
		}
		return nil, err
	}
	return accessTokenFromRow(row)
}

func (r *AccessTokenRepo) Update(ctx context.Context, t *model.AccessToken) error {
	return r.q.UpdateAccessToken(ctx, r.db(ctx), updateAccessTokenParams{
		LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt, ID: t.ID.String(),
	})
}

func (r *AccessTokenRepo) ListByUser(ctx context.Context, userID vo.Id, kind string) ([]model.AccessToken, error) {
	rows, err := r.q.ListAccessTokensByUser(ctx, r.db(ctx), listAccessTokensParams{UserID: userID.String(), Kind: kind})
	if err != nil {
		return nil, err
	}
	out := make([]model.AccessToken, 0, len(rows))
	for _, row := range rows {
		t, err := accessTokenFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, nil
}

func (r *AccessTokenRepo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteAccessToken(ctx, r.db(ctx), id.String())
}

func accessTokenFromRow(row accessTokenRow) (*model.AccessToken, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	uid, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.AccessToken{
		ID: id, UserID: uid, Kind: row.Kind, TokenHash: row.TokenHash,
		Name: row.Name, UserAgent: row.UserAgent,
		CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt,
		ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt,
	}, nil
}
```

`accesstoken_sqlite.go` and `accesstoken_pgsql.go` follow the passthrough /
whole-struct-conversion shim pattern of `passwordrequest_sqlite.go` /
`passwordrequest_pgsql.go` exactly (pgsql converts each param struct with
`pgsqlgen.InsertAccessTokenParams(p)` and each returned row with
`accessTokenRow(row)` — this compiles because Task 1 verified the structs are
field-identical).

Append the `AccessTokens` interface (from the **Interfaces** block above) to
`internal/user/repository.go`.

- [ ] **Step 4: Run tests (sqlite, then check gofmt)**

Run: `go test ./internal/user/repo/ -run TestAccessTokenRepo -v && gofmt -l internal/`
Expected: PASS; no gofmt output.

- [ ] **Step 5: Run the pgsql pass if available**

Run: `make test-repo-pgsql 2>&1 | tail -3` (skip if no PostgreSQL is available — CI covers it).
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/user
git commit -m "feat(auth): AccessTokens repository over both engines"
```

---

### Task 5: Authenticate use case, middleware swap, login/logout, harness cutover

This is the pivot task: after it, the API issues and verifies opaque tokens
and no request path touches JWT. It must land as one commit because the
`NewService` constructor, `BuildAPI` signature, middleware contract, and test
harness all interlock.

**Files:**
- Create: `internal/user/authenticate.go`, `internal/user/authenticate_test.go`
- Create: `internal/user/session.go` (createSession + purge; session list/revoke arrive in Task 7)
- Modify: `internal/user/usecase.go` (Service: drop `jwt`, add `tokens AccessTokens`; NewService signature; Logout)
- Modify: `internal/user/login.go` (issue a session instead of a JWT; purge dead rows; new `userAgent` param)
- Modify: `internal/user/api/user.go` (LoginUser passes `r.Header.Get("User-Agent")`; LogoutUser passes the current token id)
- Modify: `internal/web/middleware/auth.go` (TokenVerifier → TokenAuthenticator; JWT() → Auth(); ctx carries userID + tokenID; new 401 strings)
- Modify: `internal/web/middleware/middleware_test.go` (rewrite auth tests)
- Modify: every `internal/<feature>/api/routes.go` (9 files: parameter type `middleware.TokenVerifier` → `middleware.TokenAuthenticator`, `middleware.JWT(` → `middleware.Auth(`)
- Modify: `internal/server/server.go` (BuildAPI drops the `jwtSvc` param; wires `userSvc` as the authenticator; builds `userrepo.NewAccessTokenRepo`)
- Modify: `cmd/econumo/main.go` (drop EnsureKeypair/jwt.New; call `server.BuildAPI(cfg, db, clock.New())`)
- Modify: `internal/cli/container.go` (NewService gets the access-token repo instead of nil jwt)
- Modify: `internal/test/apiparity/harness.go`, `internal/test/apiparity/fixture.go` (seed sessions; Token() returns seeded constants; drop jwt/testkeys)
- Modify: `internal/test/apiparity/normalize.go` (jwtRe → opaque-token regex)
- Modify: `internal/test/apiparity/catalogue_user.go` (reorder user_writes: logout last + pin post-logout 401)
- Regenerate: all goldens in `internal/test/apiparity/testdata/golden/`
- Regenerate: OpenAPI docs (`make swagger`) — login/logout descriptions change

**Interfaces:**
- Consumes: `user.AccessTokens` (Task 4), `generateAccessToken`/`HashAccessToken`/`SessionTTL`/`touchInterval`/`deadTokenRetention` (Task 3), `model.AccessToken` (Task 2).
- Produces:
  - `func (s *Service) Authenticate(ctx context.Context, raw string) (userID vo.Id, tokenID vo.Id, err error)` — invalid/expired/revoked/missing → `*errs.UnauthorizedError("Invalid access token")`.
  - `middleware.TokenAuthenticator` interface with exactly that method; `middleware.Auth(authn TokenAuthenticator, dev bool) Middleware`; `middleware.TokenIDFromCtx(ctx) (vo.Id, bool)`.
  - `server.BuildAPI(cfg config.Config, db *sql.DB, clk port.Clock) http.Handler` (3 params — `jwtSvc` gone).
  - `user.NewService(repo, tx, encode, hasher, tokens AccessTokens, currency, budgets, passwordRequests, mailer, clock, limiter, allowRegistration)` — `tokens` replaces the old `jwt` position.
  - `func (s *Service) Login(ctx, req model.LoginRequest, userAgent string, now time.Time) (*model.LoginResult, error)`.
  - `func (s *Service) Logout(ctx context.Context, tokenID vo.Id) (*model.LogoutResult, error)` — revokes the session, returns the frozen `{"result":"test"}`.
  - apiparity fixture: `OwnerToken`, `GuestToken` string constants (raw seeded session tokens); `Harness.Token(t, userID, email)` keeps its signature and returns them by userID.

- [ ] **Step 1: Write the failing Authenticate use-case test**

`internal/user/authenticate_test.go` — look at how existing `*_test.go` in
`internal/user` build a Service against sqlite (e.g. `login` or `password`
tests; reuse their fixture/service helper). Cover, with a fixed clock you can
advance:

```go
// Pseudocode-free real test skeleton — adapt the service constructor to the
// helper the package already uses. Cases:

func TestAuthenticate_HappyPathAndCurrentIds(t *testing.T) {
	// login (or createSession via Login) -> Authenticate(raw) returns the
	// user's id and the session row's id.
}

func TestAuthenticate_UnknownToken401(t *testing.T) {
	// Authenticate("eco_ses_bogus...") -> *errs.UnauthorizedError, message
	// "Invalid access token".
}

func TestAuthenticate_ExpiredSession401(t *testing.T) {
	// Insert a session with expires_at in the past -> Unauthorized.
}

func TestAuthenticate_RevokedToken401(t *testing.T) {
	// Insert then Revoke+Update -> Unauthorized.
}

func TestAuthenticate_SlidingTouchThrottled(t *testing.T) {
	// t0: login. t0+2min: Authenticate -> last_used_at UNCHANGED in DB
	// (throttled). t0+6min: Authenticate -> last_used_at == t0+6min and
	// expires_at == t0+6min+SessionTTL (slid).
}

func TestAuthenticate_PATExpiryNeverSlides(t *testing.T) {
	// Insert a PAT with expires_at = t0+48h. Authenticate at t0+6min ->
	// last_used_at updated, expires_at STILL t0+48h.
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/ -run TestAuthenticate 2>&1 | head -5`
Expected: FAIL (undefined Authenticate / constructor mismatch).

- [ ] **Step 3: Implement the use cases**

`internal/user/authenticate.go`:

```go
// Authenticate verifies an opaque bearer token against the access_tokens
// store; it is the hot path behind every authenticated request.
package user

import (
	"context"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) Authenticate(ctx context.Context, raw string) (vo.Id, vo.Id, error) {
	t, err := s.tokens.GetByHash(ctx, HashAccessToken(raw))
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return vo.Id{}, vo.Id{}, errs.NewUnauthorized("Invalid access token")
		}
		return vo.Id{}, vo.Id{}, err
	}
	now := s.clock.Now()
	if !t.IsLive(now) {
		return vo.Id{}, vo.Id{}, errs.NewUnauthorized("Invalid access token")
	}
	if t.NeedsTouch(now, touchInterval) {
		t.Touch(now, SessionTTL)
		if err := s.tokens.Update(ctx, t); err != nil {
			return vo.Id{}, vo.Id{}, err
		}
	}
	return t.UserID, t.ID, nil
}
```

`internal/user/session.go` (this task's slice):

```go
// Session lifecycle: creation at login and the opportunistic purge of dead
// rows. List/revoke use cases join in the session-endpoints change.
package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// createSession mints a session row for a fresh login and returns the raw
// bearer token (the only moment it exists server-side).
func (s *Service) createSession(ctx context.Context, userID vo.Id, userAgent string, now time.Time) (string, error) {
	raw, hash, err := generateAccessToken(model.TokenKindSession)
	if err != nil {
		return "", err
	}
	exp := now.Add(SessionTTL)
	t := &model.AccessToken{
		ID: vo.NewId(), UserID: userID, Kind: model.TokenKindSession, TokenHash: hash,
		CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp,
	}
	if userAgent != "" {
		t.UserAgent = &userAgent
	}
	if err := s.tokens.Insert(ctx, t); err != nil {
		return "", err
	}
	return raw, nil
}

// purgeDeadTokens deletes this user's rows that expired/were revoked longer
// than the retention window ago. Best-effort bookkeeping on the login path;
// row counts are tiny, so per-row deletes keep the SQL engine-agnostic.
func (s *Service) purgeDeadTokens(ctx context.Context, userID vo.Id, now time.Time) error {
	for _, kind := range []string{model.TokenKindSession, model.TokenKindPersonal} {
		rows, err := s.tokens.ListByUser(ctx, userID, kind)
		if err != nil {
			return err
		}
		for i := range rows {
			if rows[i].IsDead(now, deadTokenRetention) {
				if err := s.tokens.Delete(ctx, rows[i].ID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
```

In `internal/user/login.go`: signature becomes
`Login(ctx context.Context, req model.LoginRequest, userAgent string, now time.Time)`;
replace the `s.jwt.Issue(...)` block with:

```go
	if err := s.purgeDeadTokens(ctx, u.ID, now); err != nil {
		return nil, err
	}
	token, terr := s.createSession(ctx, u.ID, userAgent, now)
	if terr != nil {
		return nil, terr
	}
```

In `internal/user/usecase.go`: Service field `jwt *jwt.JWT` →
`tokens AccessTokens` (same constructor position); drop the `jwt` import;
`Logout` becomes (GetByID exists since Task 4):

```go
// Logout revokes the presenting session. The "test" literal is a frozen wire
// constant clients depend on (see LogoutResult).
func (s *Service) Logout(ctx context.Context, tokenID vo.Id) (*model.LogoutResult, error) {
	t, err := s.tokens.GetByID(ctx, tokenID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			// Already gone: logout is idempotent.
			return &model.LogoutResult{Result: "test"}, nil
		}
		return nil, err
	}
	t.Revoke(s.clock.Now())
	if err := s.tokens.Update(ctx, t); err != nil {
		return nil, err
	}
	return &model.LogoutResult{Result: "test"}, nil
}
```

- [ ] **Step 4: Swap the middleware**

`internal/web/middleware/auth.go`: replace the TokenVerifier interface, JWT
func, and messages:

```go
// TokenAuthenticator is the narrow contract the auth middleware needs; the
// user feature's Service satisfies it (opaque-token lookup in the DB).
type TokenAuthenticator interface {
	Authenticate(ctx context.Context, token string) (userID vo.Id, tokenID vo.Id, err error)
}

type ctxKeyTokenIDType struct{}

var ctxKeyTokenID ctxKeyTokenIDType

// Auth builds the authentication middleware: extract the Bearer token,
// authenticate it against the access-token store, stash user id + token id in
// the request context. 401s use the frozen error envelope.
func Auth(authn TokenAuthenticator, dev bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				httpx.WriteError(w, errs.NewUnauthorized("Access token not found"), dev)
				return
			}
			userID, tokenID, err := authn.Authenticate(r.Context(), token)
			if err != nil {
				var ue *errs.UnauthorizedError
				if !errors.As(err, &ue) {
					err = errs.NewUnauthorized("Invalid access token")
				}
				httpx.WriteError(w, err, dev)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyUserID, userID)
			ctx = context.WithValue(ctx, ctxKeyTokenID, tokenID)
			reqctx.AddLogAttr(ctx, "user_id", userID.String())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TokenIDFromCtx returns the authenticated request's access-token row id
// (the "current session" for logout / revoke-current / isCurrent marking).
func TokenIDFromCtx(ctx context.Context) (vo.Id, bool) {
	id, ok := ctx.Value(ctxKeyTokenID).(vo.Id)
	return id, ok
}
```

Also: `RequireUser`'s 401 message → `"Access token not found"`; delete the
`jwt` import and the `vo.ParseId(claims.ID)` branch (ids are already `vo.Id`).
Update the auth middleware tests to stub `TokenAuthenticator` (return fixed
ids / an `errs.NewUnauthorized`) and assert the two new messages; keep the
malformed-header table tests as-is otherwise. Note: a non-Unauthorized error
from Authenticate (DB down) must NOT leak internals — the middleware maps it
to the generic 401 above (test this).

- [ ] **Step 5: Mechanical ripple**

- All 9 `internal/<feature>/api/routes.go`: `middleware.TokenVerifier` →
  `middleware.TokenAuthenticator`; `middleware.JWT(verifier, dev)` →
  `middleware.Auth(authn, dev)`; rename local vars `jwt`/`verifier` →
  `auth`/`authn` where they shadow. Find them:
  `grep -rln "middleware.TokenVerifier\|middleware.JWT(" internal/`
- `internal/user/api/user.go`: LoginUser passes
  `r.Header.Get("User-Agent")` into `h.svc.Login(r.Context(), req, r.Header.Get("User-Agent"), h.now.Now())`;
  LogoutUser's closure pulls the token id:

```go
func (h *Handlers) LogoutUser(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, func(ctx context.Context, _ vo.Id) (*model.LogoutResult, error) {
		tokenID, _ := middleware.TokenIDFromCtx(ctx)
		return h.svc.Logout(ctx, tokenID)
	})
}
```
  (Also update the swag `@Description` on LoginUser/LogoutUser to say opaque
  access token, not JWT.)
- `internal/server/server.go`: signature `BuildAPI(cfg config.Config, db *sql.DB, clk port.Clock) http.Handler`; build
  `accessTokens := userrepo.NewAccessTokenRepo(cfg.DatabaseDriver, txm)`; pass
  it to `appuser.NewService(...)` in the old jwt position; the authenticator
  passed to every `RegisterAPI` is `userSvc`.
- `cmd/econumo/main.go`: delete the EnsureKeypair/jwt.New block; call
  `server.BuildAPI(cfg, db, clock.New())`; drop the jwt import.
- `internal/cli/container.go`: `tokensRepo := userrepo.NewAccessTokenRepo(cfg.DatabaseDriver, txm)`
  replaces the `nil` jwt argument.

- [ ] **Step 6: Cut the apiparity harness over to seeded sessions**

`internal/test/apiparity/fixture.go` — add raw-token constants and seed
session rows (find the fixture-builder insert style in
`internal/test/fixture`; add an `AccessToken` insert helper there following
its `User`/`Account` pattern):

```go
const (
	// Raw seeded bearer tokens (43-char payloads — "owner-seed-token-" is 17
	// chars + 26 zeros — deliberately NOT random so both engines seed identical
	// rows). Their sha256 goes into access_tokens.
	OwnerToken = "eco_ses_owner-seed-token-00000000000000000000000000"
	GuestToken = "eco_ses_guest-seed-token-00000000000000000000000000"

	// Seeded session row ids (fixed, non-v7 so they survive normalization —
	// scenarios reference them, e.g. err:revoke-session-foreign).
	OwnerSessionID = "33333333-3333-3333-3333-333333333333"
	GuestSessionID = "44444444-4444-4444-4444-444444444444"
)
```

Add a tiny guard test asserting `len(OwnerToken) == len("eco_ses_")+43` (and
guest) so the constants can't drift from the normalizer regex. Seed for each:
`id` = the constant above, `kind` = session, `token_hash` =
`user.HashAccessToken(OwnerToken)`, `user_agent` = "apiparity",
`created_at`/`last_used_at` = ClockTime, `expires_at` =
ClockTime + `user.SessionTTL`.

`internal/test/apiparity/harness.go`: delete the jwt/testkeys wiring;
`BuildAPI(cfg, db.Raw, clk)`; `Token()` becomes:

```go
// Token returns the seeded session token for one of the fixture users.
func (h *Harness) Token(t *testing.T, userID, email string) string {
	t.Helper()
	switch userID {
	case OwnerID:
		return OwnerToken
	case GuestID:
		return GuestToken
	default:
		t.Fatalf("no seeded token for user %s", userID)
		return ""
	}
}
```

`internal/test/apiparity/normalize.go`: replace `jwtRe` with

```go
	tokenRe = regexp.MustCompile(`eco_(ses|pat)_[A-Za-z0-9_-]{43}`)
```

and `s = jwtRe.ReplaceAllString(s, "<jwt>")` → `s = tokenRe.ReplaceAllString(s, "<token>")`.

`internal/test/apiparity/catalogue_user.go` — `user_writes`: logout now
really revokes, so move `logout-user` to the END and pin the aftermath:

```go
	{Label: "update-password", ...unchanged...},
	{Label: "get-user-data-after", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "owner"},
	// Logout revokes the presenting session (DB-backed tokens) — a subsequent
	// call with the same token must 401 with the frozen envelope.
	{Label: "logout-user", Method: "POST", Path: "/api/v1/user/logout-user", Auth: "owner"},
	{Label: "err:get-user-data-after-logout", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "owner"},
```

Update the scenario's stale "JWT is stateless" comment. Check
`catalogue_negative.go` for 401 scenarios pinning the old messages — they
stay valid (the golden regen updates their bodies).

- [ ] **Step 7: Build, regenerate docs and goldens, inspect**

```bash
make swagger
go build ./... && go vet ./...
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ 2>&1 | tail -3
git diff --stat internal/test/apiparity/testdata/golden/ | tail -5
```

Inspect the golden diff — expected changes ONLY: `<jwt>` → `<token>` in the
login golden, 401 message strings, the reordered user_writes calls, and the
new post-logout 401 body. Anything else = a bug; stop and fix.

- [ ] **Step 8: Full smoke**

Run: `make go-test 2>&1 | tail -10`
Expected: PASS including coverage gate. Then `go test ./internal/user/ -run TestAuthenticate -v` — PASS.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat(auth)!: replace JWT verification with DB-backed opaque session tokens"
```

---

### Task 6: Revocation cascades on password change / reset / CLI

**Files:**
- Modify: `internal/user/session.go` (add `revokeSessions` helper)
- Modify: `internal/user/password.go` (UpdatePassword + ResetPassword cascades)
- Modify: `internal/user/admin.go` (AdminChangePassword, AdminDeactivate cascades)
- Modify: `internal/user/api/password.go` (UpdatePassword handler passes the current token id)
- Test: `internal/user/session_cascade_test.go` (new)
- Regenerate: goldens IF the update-password scenario output shifts (it should not — the response body is unchanged and the owner token survives).

**Interfaces:**
- Consumes: Task 5's Service/tokens plumbing, `middleware.TokenIDFromCtx`.
- Produces:
  - `func (s *Service) revokeSessions(ctx context.Context, userID vo.Id, exceptTokenID vo.Id, now time.Time) error` (package-private; zero `exceptTokenID` = revoke all).
  - `UpdatePassword(ctx, userID vo.Id, currentTokenID vo.Id, req model.UpdatePasswordRequest)` — new second param.
  - AdminDeactivate additionally revokes ALL tokens (both kinds).

- [ ] **Step 1: Write failing cascade tests**

`internal/user/session_cascade_test.go`, using the same service/db helper as
Task 5's tests. Cases (each: create user + 2-3 sessions and 1 PAT via the
repo, run the use case, assert via `tokens.ListByUser` + `IsLive`):

1. `TestUpdatePassword_RevokesOtherSessionsKeepsCurrentAndPATs` — current
   session live, other sessions revoked, PAT live.
2. `TestResetPassword_RevokesAllSessions` — drive the real reset flow (mirror
   the existing reset test's remind→code plumbing, or call the service with a
   seeded password-request row); all sessions revoked, PAT live.
3. `TestAdminChangePassword_RevokesAllSessions` — all sessions revoked, PAT live.
4. `TestAdminDeactivate_RevokesEverything` — sessions AND PATs revoked.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/ -run "Revokes" 2>&1 | head -5`
Expected: FAIL (signature/behavior missing).

- [ ] **Step 3: Implement**

Append to `internal/user/session.go`:

```go
// revokeSessions revokes every live session of the user except exceptTokenID
// (zero id = revoke all). PATs are never touched here: integrations must
// survive a password change; only user:deactivate kills them.
func (s *Service) revokeSessions(ctx context.Context, userID vo.Id, exceptTokenID vo.Id, now time.Time) error {
	rows, err := s.tokens.ListByUser(ctx, userID, model.TokenKindSession)
	if err != nil {
		return err
	}
	for i := range rows {
		if rows[i].ID.Equal(exceptTokenID) || !rows[i].IsLive(now) {
			continue
		}
		rows[i].Revoke(now)
		if err := s.tokens.Update(ctx, &rows[i]); err != nil {
			return err
		}
	}
	return nil
}
```

Wire the four call sites (each after its password write succeeds, using
`s.clock.Now()`):
- `UpdatePassword(ctx, userID, currentTokenID, req)`: `s.revokeSessions(ctx, userID, currentTokenID, now)`.
- `ResetPassword`: `s.revokeSessions(ctx, u.ID, vo.Id{}, now)`.
- `AdminChangePassword`: `s.revokeSessions(ctx, u.ID, vo.Id{}, now)`.
- `AdminDeactivate`: `s.revokeSessions(ctx, u.ID, vo.Id{}, now)` plus the same
  loop over `model.TokenKindPersonal` (inline; or generalize the helper with a
  kinds slice — pick whichever reads cleaner in place).

`internal/user/api/password.go` UpdatePassword handler closure:

```go
	tokenID, _ := middleware.TokenIDFromCtx(ctx)
	return h.svc.UpdatePassword(ctx, userID, tokenID, req)
```

- [ ] **Step 4: Run tests + smoke**

Run: `go test ./internal/user/... && make go-test 2>&1 | tail -5`
Expected: PASS. The user_writes scenario still passes because update-password
spares the presenting token.

- [ ] **Step 5: Commit**

```bash
git add internal/user
git commit -m "feat(auth): revoke sessions on password change/reset/deactivate"
```

---

### Task 7: Session endpoints (list / revoke / revoke-others)

**Files:**
- Create: `internal/model/token_dto.go` (session DTO half)
- Modify: `internal/user/session.go` (ListSessions / RevokeSession / RevokeOtherSessions use cases)
- Create: `internal/user/api/session.go` (3 handlers with swag blocks)
- Modify: `internal/user/api/routes.go` (3 new routes)
- Modify: `internal/test/apiparity/catalogue_user.go` (new `user_sessions` scenario)
- Test: `internal/user/session_endpoint_test.go` or extend `internal/user/api/user_endpoints_test.go` (mirror whichever pattern the api package uses)
- Regenerate: `make swagger`, goldens.

**Interfaces:**
- Consumes: Tasks 5-6 plumbing.
- Produces (in `internal/model/token_dto.go`):

```go
// SessionItem is one row of get-session-list. isCurrent marks the session
// whose token authenticated THIS request.
type SessionItem struct {
	Id         string `json:"id"`
	UserAgent  string `json:"userAgent"` // "" when the login sent no User-Agent
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt"`
	IsCurrent  bool   `json:"isCurrent"`
}

type RevokeSessionRequest struct {
	Id string `json:"id"`
}
// Validate: blank id -> the project's standard blank-field validation error.
// COPY the exact pattern (message + code + field name) from an existing
// single-id request, e.g. the delete-category request DTO.

type RevokeSessionResult struct{}
type RevokeOtherSessionsResult struct{}
```

  Use cases: `ListSessions(ctx, userID, currentTokenID vo.Id) ([]model.SessionItem, error)` (live only, repo order, datetimes via `datetime.Layout`, UTC);
  `RevokeSession(ctx, userID vo.Id, req model.RevokeSessionRequest)` — not-yours/unknown/PAT-id → `errs.NewNotFound("Session not found")`; revoking the current session IS allowed;
  `RevokeOtherSessions(ctx, userID, currentTokenID vo.Id) (*model.RevokeOtherSessionsResult, error)` = `revokeSessions(..., except=current)`.
- Routes (all auth): `GET /api/v1/user/get-session-list`, `POST /api/v1/user/revoke-session`, `POST /api/v1/user/revoke-other-sessions`.

- [ ] **Step 1: Write failing endpoint tests**

Follow the api package's existing endpoint-test harness
(`internal/user/api/user_endpoints_test.go`). Cases:
1. get-session-list after two logins → 2 items, exactly one `isCurrent:true`
   (the presenting token's row), datetime format matches `datetime.Layout`,
   revoked/expired rows absent.
2. revoke-session with the OTHER session's id → 200 `{}` in data; the revoked
   token now 401s; the current one still works.
3. revoke-session with a foreign user's session id → 404 envelope.
4. revoke-session with a PAT id → 404 (it is not a session).
5. revoke-other-sessions → all but current revoked.
6. revoke-session with blank id → the standard blank-validation 400.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/api/ -run Session 2>&1 | head -5`
Expected: FAIL.

- [ ] **Step 3: Implement use cases**

Append to `internal/user/session.go`:

```go
func (s *Service) ListSessions(ctx context.Context, userID, currentTokenID vo.Id) ([]model.SessionItem, error) {
	rows, err := s.tokens.ListByUser(ctx, userID, model.TokenKindSession)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	out := make([]model.SessionItem, 0, len(rows))
	for i := range rows {
		if !rows[i].IsLive(now) {
			continue
		}
		ua := ""
		if rows[i].UserAgent != nil {
			ua = *rows[i].UserAgent
		}
		out = append(out, model.SessionItem{
			Id:         rows[i].ID.String(),
			UserAgent:  ua,
			CreatedAt:  rows[i].CreatedAt.UTC().Format(datetime.Layout),
			LastUsedAt: rows[i].LastUsedAt.UTC().Format(datetime.Layout),
			IsCurrent:  rows[i].ID.Equal(currentTokenID),
		})
	}
	return out, nil
}

func (s *Service) RevokeSession(ctx context.Context, userID vo.Id, req model.RevokeSessionRequest) (*model.RevokeSessionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, errs.NewNotFound("Session not found")
	}
	t, err := s.tokens.GetByID(ctx, id)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, errs.NewNotFound("Session not found")
		}
		return nil, err
	}
	if !t.UserID.Equal(userID) || t.Kind != model.TokenKindSession {
		return nil, errs.NewNotFound("Session not found")
	}
	t.Revoke(s.clock.Now())
	if err := s.tokens.Update(ctx, t); err != nil {
		return nil, err
	}
	return &model.RevokeSessionResult{}, nil
}

func (s *Service) RevokeOtherSessions(ctx context.Context, userID, currentTokenID vo.Id) (*model.RevokeOtherSessionsResult, error) {
	if err := s.revokeSessions(ctx, userID, currentTokenID, s.clock.Now()); err != nil {
		return nil, err
	}
	return &model.RevokeOtherSessionsResult{}, nil
}
```

- [ ] **Step 4: Implement handlers + routes**

`internal/user/api/session.go` — three named handler methods with full swag
blocks (copy the annotation shape from an existing GET-list and POST handler
in this package), bodies delegating to the combinators:

```go
func (h *Handlers) GetSessionList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, func(ctx context.Context, userID vo.Id) ([]model.SessionItem, error) {
		tokenID, _ := middleware.TokenIDFromCtx(ctx)
		return h.svc.ListSessions(ctx, userID, tokenID)
	})
}

func (h *Handlers) RevokeSession(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.RevokeSessionRequest) (*model.RevokeSessionResult, error) {
		reqctx.AddLogAttr(ctx, "session_id", req.Id)
		return h.svc.RevokeSession(ctx, userID, req)
	})
}

func (h *Handlers) RevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, func(ctx context.Context, userID vo.Id) (*model.RevokeOtherSessionsResult, error) {
		tokenID, _ := middleware.TokenIDFromCtx(ctx)
		return h.svc.RevokeOtherSessions(ctx, userID, tokenID)
	})
}
```

Routes in the authenticated group of `internal/user/api/routes.go`:

```go
		mux.Handle("GET /api/v1/user/get-session-list", auth(h.GetSessionList))
		mux.Handle("POST /api/v1/user/revoke-session", auth(h.RevokeSession))
		mux.Handle("POST /api/v1/user/revoke-other-sessions", auth(h.RevokeOtherSessions))
```

(Update the "mounts the 13 user endpoints" count comment.)

- [ ] **Step 5: Add the apiparity scenario**

The route guard fails without one. In `catalogue_user.go`:

```go
	register(Scenario{Name: "user_sessions", Calls: func() []Call {
		return []Call{
			// The seeded owner session is the only one -> a single isCurrent row.
			{Label: "get-session-list", Method: "GET", Path: "/api/v1/user/get-session-list", Auth: "owner"},
			// A second login mints a second session (its token is response-internal).
			{Label: "login-second-session", Method: "POST", Path: "/api/v1/user/login-user", Auth: "",
				Body: map[string]any{"username": OwnerEmail, "password": SeedPassword}},
			{Label: "get-session-list-two", Method: "GET", Path: "/api/v1/user/get-session-list", Auth: "owner"},
			{Label: "revoke-other-sessions", Method: "POST", Path: "/api/v1/user/revoke-other-sessions", Auth: "owner"},
			{Label: "get-session-list-after-revoke", Method: "GET", Path: "/api/v1/user/get-session-list", Auth: "owner"},
			// Foreign session id -> 404 envelope.
			{Label: "err:revoke-session-foreign", Method: "POST", Path: "/api/v1/user/revoke-session", Auth: "guest",
				Body: map[string]any{"id": /* the seeded OWNER session's fixed id constant */ OwnerSessionID},
			},
			{Label: "err:revoke-session-blank", Method: "POST", Path: "/api/v1/user/revoke-session", Auth: "owner",
				Body: map[string]any{"id": ""}},
		}
	}})
```

(Expose the seeded session row ids as fixture constants `OwnerSessionID` /
`GuestSessionID` in Task 5's fixture change if not already done.) The
second-login session row gets a fresh UUIDv7 + datetimes — both redacted by
the normalizer, so goldens stay stable.

- [ ] **Step 6: Regenerate + verify**

```bash
make swagger
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ 2>&1 | tail -3
git diff --stat internal/test/apiparity/testdata/golden/
go test ./internal/user/... && make go-test 2>&1 | tail -5
```
Expected: new goldens only for user_sessions; all green.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(auth): session list/revoke endpoints"
```

---

### Task 8: Personal-token endpoints (create / list / revoke)

**Files:**
- Modify: `internal/model/token_dto.go` (PAT DTO half)
- Create: `internal/user/pat.go` (use cases)
- Create: `internal/user/api/pat.go` (3 handlers with swag blocks)
- Modify: `internal/user/api/routes.go` (3 new routes)
- Modify: `internal/test/apiparity/catalogue_user.go` (`user_personal_tokens` scenario)
- Create: `internal/test/apiparity/token_auth_test.go` (PAT actually authenticates)
- Regenerate: `make swagger`, goldens.

**Interfaces:**
- Consumes: everything above.
- Produces (append to `internal/model/token_dto.go`):

```go
type PersonalTokenItem struct {
	Id         string  `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt string  `json:"lastUsedAt"`
	ExpiresAt  *string `json:"expiresAt"` // null = never expires
}

// CreatePersonalTokenRequest: expiresAt is optional ("" = never) and, when
// set, must parse with datetime.Layout; the future check lives in the use
// case (needs the clock).
type CreatePersonalTokenRequest struct {
	Name      string `json:"name"`
	ExpiresAt string `json:"expiresAt"`
}

// Validate enforces: name 1-64 chars -> "Token name must be 1-64 characters"
// (field "name"); non-empty expiresAt must parse with datetime.Layout ->
// "Invalid expiration date" (field "expiresAt"). Mirror the ValidationError
// construction style of the neighboring DTOs in user_dto.go.

// CreatePersonalTokenResult carries the raw token — the ONLY response that
// ever contains it.
type CreatePersonalTokenResult struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	Token     string  `json:"token"`
	CreatedAt string  `json:"createdAt"`
	ExpiresAt *string `json:"expiresAt"`
}

type RevokePersonalTokenRequest struct {
	Id string `json:"id"`
}
// Validate: same blank-id pattern as RevokeSessionRequest.
type RevokePersonalTokenResult struct{}
```

  Use cases: `CreatePersonalToken(ctx, userID, req)` — expiresAt in the past/now → validation error `"Expiration date must be in the future"` (field "expiresAt"); `ListPersonalTokens(ctx, userID)`; `RevokePersonalToken(ctx, userID, req)` — miss/foreign/session-id → `errs.NewNotFound("Token not found")`.
- Routes: `GET /api/v1/user/get-personal-token-list`, `POST /api/v1/user/create-personal-token`, `POST /api/v1/user/revoke-personal-token`.

- [ ] **Step 1: Write failing tests**

Endpoint tests (same harness as Task 7): create (name only → `expiresAt:null`;
name + future date → echoed), token string matches
`^eco_pat_[A-Za-z0-9_-]{43}$`, list does NOT contain the token string,
past-date create → 400 with the exact message, name of 0 / 65 chars → 400,
revoke → PAT 401s afterwards, foreign/unknown id → 404.

`internal/test/apiparity/token_auth_test.go` (plain test in the apiparity
package, NOT a scenario — scenarios can't thread a created token into a later
call's Auth): boot the sqlite harness, create a PAT over HTTP with the owner
session, then `h.Call(t, "GET", "/api/v1/user/get-user-data", patToken, nil)`
→ 200; revoke it → same call → 401.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/api/ -run PersonalToken 2>&1 | head -5`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/user/pat.go`:

```go
// Personal access tokens: user-created bearer credentials for integrations.
// Full-access (no scopes); optional fixed expiry; never touched by password
// changes — only explicit revocation or user:deactivate kills them.
package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) CreatePersonalToken(ctx context.Context, userID vo.Id, req model.CreatePersonalTokenRequest) (*model.CreatePersonalTokenResult, error) {
	now := s.clock.Now()
	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		exp, err := time.Parse(datetime.Layout, req.ExpiresAt)
		if err != nil {
			// Validate() already rejects this; defense in depth.
			return nil, errs.NewValidation("Invalid expiration date")
		}
		if !exp.After(now) {
			return nil, errs.NewValidation("Expiration date must be in the future")
		}
		expiresAt = &exp
	}
	raw, hash, err := generateAccessToken(model.TokenKindPersonal)
	if err != nil {
		return nil, err
	}
	name := req.Name
	t := &model.AccessToken{
		ID: vo.NewId(), UserID: userID, Kind: model.TokenKindPersonal, TokenHash: hash,
		Name: &name, CreatedAt: now, LastUsedAt: now, ExpiresAt: expiresAt,
	}
	if err := s.tokens.Insert(ctx, t); err != nil {
		return nil, err
	}
	return &model.CreatePersonalTokenResult{
		Id: t.ID.String(), Name: name, Token: raw,
		CreatedAt: now.UTC().Format(datetime.Layout),
		ExpiresAt: formatOptionalDatetime(expiresAt),
	}, nil
}

func (s *Service) ListPersonalTokens(ctx context.Context, userID vo.Id) ([]model.PersonalTokenItem, error) {
	rows, err := s.tokens.ListByUser(ctx, userID, model.TokenKindPersonal)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	out := make([]model.PersonalTokenItem, 0, len(rows))
	for i := range rows {
		if !rows[i].IsLive(now) {
			continue
		}
		name := ""
		if rows[i].Name != nil {
			name = *rows[i].Name
		}
		out = append(out, model.PersonalTokenItem{
			Id: rows[i].ID.String(), Name: name,
			CreatedAt:  rows[i].CreatedAt.UTC().Format(datetime.Layout),
			LastUsedAt: rows[i].LastUsedAt.UTC().Format(datetime.Layout),
			ExpiresAt:  formatOptionalDatetime(rows[i].ExpiresAt),
		})
	}
	return out, nil
}

func (s *Service) RevokePersonalToken(ctx context.Context, userID vo.Id, req model.RevokePersonalTokenRequest) (*model.RevokePersonalTokenResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, errs.NewNotFound("Token not found")
	}
	t, err := s.tokens.GetByID(ctx, id)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, errs.NewNotFound("Token not found")
		}
		return nil, err
	}
	if !t.UserID.Equal(userID) || t.Kind != model.TokenKindPersonal {
		return nil, errs.NewNotFound("Token not found")
	}
	t.Revoke(s.clock.Now())
	if err := s.tokens.Update(ctx, t); err != nil {
		return nil, err
	}
	return &model.RevokePersonalTokenResult{}, nil
}

func formatOptionalDatetime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(datetime.Layout)
	return &s
}
```

(Adjust `errs.NewValidation` calls to the field-aware constructor the project
actually uses — check `user_dto.go`'s validators and match.)

Handlers in `internal/user/api/pat.go` + 3 routes, exactly the Task 7 shapes
(`Handle` for create/revoke with `reqctx.AddLogAttr(ctx, "token_id", ...)` on
revoke, `HandleNoBody` for the list).

- [ ] **Step 4: apiparity scenario**

```go
	register(Scenario{Name: "user_personal_tokens", Calls: func() []Call {
		return []Call{
			{Label: "create-personal-token", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "CI export", "expiresAt": ""}},
			{Label: "create-personal-token-expiring", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "Short lived", "expiresAt": "2030-01-01 00:00:00"}},
			{Label: "get-personal-token-list", Method: "GET", Path: "/api/v1/user/get-personal-token-list", Auth: "owner"},
			{Label: "err:create-personal-token-past", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "Expired", "expiresAt": "2020-01-01 00:00:00"}},
			{Label: "err:create-personal-token-blank-name", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "", "expiresAt": ""}},
			{Label: "err:revoke-personal-token-unknown", Method: "POST", Path: "/api/v1/user/revoke-personal-token", Auth: "owner",
				Body: map[string]any{"id": "00000000-0000-0000-0000-000000000009"}},
		}
	}})
```

The created token strings and UUIDv7 ids are redacted by the normalizer
(`<token>` / `<generated-uuid>`), so the goldens are stable. Note the fixed
`expiresAt` datetimes are also normalized to `<datetime>` — fine.

- [ ] **Step 5: Regenerate + verify**

```bash
make swagger
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ 2>&1 | tail -3
git diff --stat internal/test/apiparity/testdata/golden/
go test ./internal/user/... ./internal/test/apiparity/ && make go-test 2>&1 | tail -5
```
Expected: new goldens for user_personal_tokens only; PAT-auth test green.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(auth): personal access token endpoints"
```

---

### Task 9: Remove the JWT machinery + docs

**Files:**
- Delete: `internal/shared/jwt/` (whole package), `internal/test/testkeys/`
- Modify: `internal/cli/setup_commands.go` (drop `jwt:generate`), `internal/cli/cli.go` (comment)
- Modify: `cmd/econumo/main.go` (any leftover imports/help text)
- Modify: `internal/config/config.go` (drop `JWTPrivateKeyPath`, `JWTPublicKeyPath`, `JWTPassphrase`; keep `ResolveProjectDir` only if something else uses it)
- Modify: `.env.example`, `README.md`, `docs/*` mentioning JWT env vars, `deployment/docker/Dockerfile` (jwt dir handling, if any), `docker-compose.yml` comments
- Modify: `CLAUDE.md` (auth + frozen-contract sections, env list, CLI list)
- Modify: `go.mod` / `go.sum` (`go mod tidy` drops golang-jwt + youmark/pkcs8)

**Interfaces:**
- Consumes: Tasks 5-8 having removed all runtime JWT call sites.
- Produces: a repo with zero `jwt` references outside docs history.

- [ ] **Step 1: Find every remaining reference**

Run: `grep -rln "shared/jwt\|testkeys\|ECONUMO_JWT\|jwt:generate\|EnsureKeypair" --include="*.go" --include="*.yml" --include="*.yaml" --include="*.md" --include="Dockerfile" --include=".env.example" . | grep -v docs/superpowers | grep -v "gen/"`
Work through the list. Delete the two packages, the CLI command, the config
fields. `internal/web/apidoc` or swagger docs may embed "JWT" wording — update
the swag general-info annotations and regenerate.

- [ ] **Step 2: go mod tidy + build + full test**

```bash
go mod tidy
go build ./... && make go-test 2>&1 | tail -5
```
Expected: `github.com/golang-jwt/jwt/v5` and `github.com/youmark/pkcs8` gone
from go.mod; all green. If `enginecompare`/`reset_password` build-tagged tests
reference testkeys, fix them here:
`go vet -tags enginecompare ./... && go test -tags enginecompare -run xxx ./... 2>&1 | tail -3` (compile check).

- [ ] **Step 3: Update the docs**

- `.env.example`: delete the three `ECONUMO_JWT_*` lines; add a comment block:
  "Auth tokens are stored in the database (no signing keys). After upgrading
  from a JWT build, every client logs in again."
- `README.md`: remove keypair/volume-keys wording; `/app/var` now persists the
  database only; note the one-time relogin in the upgrade notes.
- `CLAUDE.md`:
  - Replace the "JWT (`internal/shared/jwt/jwt.go`)" frozen block with an
    "Access tokens" block: format `eco_ses_`/`eco_pat_` + 43 url-safe chars,
    sha256-hex storage, sliding 30-day sessions (5-min touch throttle), PAT
    optional expiry, 401 messages `"Access token not found"` / `"Invalid
    access token"`.
  - Authentication section: DB-backed opaque tokens; login/logout semantics;
    new routes listed in the API examples.
  - Config section: drop `ECONUMO_JWT_*`; CLI section: drop `jwt:generate`.
  - Architecture tree: remove `jwt` from the `internal/shared` description.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat(auth)!: remove JWT machinery, keys and config"
```

---

### Task 10: Frontend — API client, DTOs, query hooks

**Files:**
- Modify: `web/src/api/dto/user.ts` (Session/PAT DTOs)
- Modify: `web/src/api/user.ts` (6 functions)
- Create: `web/src/features/settings/security.ts` (query hooks)
- Test: `web/src/api/user.test.ts` (extend), `web/src/features/settings/security.test.tsx` (new)

**Interfaces:**
- Consumes: Task 7/8 endpoints.
- Produces:

```ts
// dto/user.ts
export interface SessionDto {
  id: string
  userAgent: string
  createdAt: string
  lastUsedAt: string
  isCurrent: boolean
}
export interface PersonalTokenDto {
  id: string
  name: string
  createdAt: string
  lastUsedAt: string
  expiresAt: string | null
}
export interface CreatedPersonalTokenDto {
  id: string
  name: string
  token: string
  createdAt: string
  expiresAt: string | null
}

// api/user.ts (envelope handling mirrors the file's existing list calls)
export async function getSessionList(): Promise<SessionDto[]>
export async function revokeSession(id: string): Promise<void>
export async function revokeOtherSessions(): Promise<void>
export async function getPersonalTokenList(): Promise<PersonalTokenDto[]>
export async function createPersonalToken(name: string, expiresAt: string | null): Promise<CreatedPersonalTokenDto>
export async function revokePersonalToken(id: string): Promise<void>

// features/settings/security.ts
export function useSessions()            // useQuery, key queryKeys.sessions
export function useRevokeSession()       // useMutation -> invalidate sessions
export function useRevokeOtherSessions() // useMutation -> invalidate sessions
export function usePersonalTokens()      // useQuery, key queryKeys.personalTokens
export function useCreatePersonalToken() // useMutation -> invalidate tokens
export function useRevokePersonalToken() // useMutation -> invalidate tokens
```

  Add `sessions` and `personalTokens` keys to `web/src/app/queryKeys.ts`
  following its existing shape. `createPersonalToken` sends
  `{name, expiresAt: expiresAt ?? ''}` (the API treats `""` as "never").

- [ ] **Step 1: Write failing tests** — msw handlers (see `user.test.ts` for
  the pattern) covering: list unwrap from the `{success,data}` envelope, POST
  bodies (`{id}`, `{name,expiresAt}`), create returning the token. Hook tests
  (see `queries.test.tsx`): mutation invalidates its list.
- [ ] **Step 2: Run to verify failure** — `cd web && pnpm test -- --run src/api/user.test.ts src/features/settings/security.test.tsx 2>&1 | tail -5` → FAIL.
- [ ] **Step 3: Implement** the DTOs, functions, keys and hooks per the interface block, mirroring the file conventions (axios `api` + `apiUrl`, `response.data.data`).
- [ ] **Step 4: Run tests + lint** — `cd web && pnpm test -- --run && pnpm lint` → PASS (the pre-existing `ImportCsvDialog.test.tsx` failure on main is known; do not chase it).
- [ ] **Step 5: Commit** — `git add web && git commit -m "feat(web): sessions and personal-token API client + hooks"`.

---

### Task 11: Frontend — Security section + Sessions page

**Files:**
- Modify: `web/src/features/settings/ProfilePage.tsx` (Security nav group)
- Create: `web/src/features/settings/SessionsPage.tsx`
- Create: `web/src/features/settings/SessionsPage.test.tsx`
- Modify: `web/src/app/routes.tsx` (`/settings/profile/sessions`)
- Modify: `web/src/locales/en-US.ts` (keys)

**Interfaces:**
- Consumes: Task 10 hooks; `SettingsShell`; `ConfirmDialog`.
- Produces: route `/settings/profile/sessions`; ProfilePage "Security" group
  with three rows (Change password → existing route; Sessions; API tokens →
  route arrives Task 12 — add the row here pointing at
  `/settings/profile/tokens`, the route lands next task; if the router 404s on
  a missing route in tests, register both routes in THIS task and let the page
  component land in Task 12 — decide by what the existing tests tolerate).

- [ ] **Step 1: Write failing SessionsPage tests** — msw-driven: renders both
  sessions with relative "last active", marks exactly one row "Current
  session" and hides its revoke button (or renders it as sign-out), revoke
  button on the other row calls `revoke-session` with that id and the list
  refreshes, "Sign out other devices" calls `revoke-other-sessions`, revoking
  the CURRENT session (via the sign-out affordance) navigates to `/logout`.
  Follow the router/msw setup of `ChangePasswordPage.test.tsx`.
- [ ] **Step 2: Run to verify failure.**
- [ ] **Step 3: Implement SessionsPage** — `SettingsShell` with crumbs
  `Settings → Profile → Sessions` (mirror ChangePasswordPage's shell props);
  list rows: parsed user-agent via a small local helper (regex for
  `Firefox|Chrome|Safari|Edg` + `Windows|Mac|Linux|Android|iPhone|iPad`,
  fallback = raw string truncated, empty = t('…unknown_device')); createdAt /
  lastUsedAt rendered with whatever date-formatting helper the codebase
  already uses (`grep -rn "formatD\|dayjs\|date-fns\|Intl.DateTimeFormat" web/src/lib | head` — reuse, don't add a dependency; remember API datetimes
  are UTC "Y-m-d H:i:s"); per-row revoke behind `ConfirmDialog`; a "Sign out
  other devices" button; current-session sign-out routes to `/logout` (the
  existing LogoutPage already calls the API and purges the token).
- [ ] **Step 4: Add the ProfilePage Security group** — a section header +
  three navigation rows in the page's existing row idiom (see how the page
  renders its change-password link today; extend that block). i18n keys under
  `pages.settings.security.*` in `en-US.ts`.
- [ ] **Step 5: Register the route** in `routes.tsx` next to change-password.
- [ ] **Step 6: Run tests + lint** — `cd web && pnpm test -- --run && pnpm lint` → PASS.
- [ ] **Step 7: Commit** — `git add web && git commit -m "feat(web): security section and sessions page"`.

---

### Task 12: Frontend — Personal tokens page

**Files:**
- Create: `web/src/features/settings/PersonalTokensPage.tsx`
- Create: `web/src/features/settings/PersonalTokensPage.test.tsx`
- Modify: `web/src/app/routes.tsx` (`/settings/profile/tokens`)
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**
- Consumes: Task 10 hooks; shadcn `Dialog`/`Select`/`Input`; ConfirmDialog.
- Produces: route `/settings/profile/tokens`.

- [ ] **Step 1: Write failing tests** — renders the token list (name, created,
  expires or "Never"); create flow: open dialog → name + expiry select
  (30/90/365 days / custom date / never; custom shows a date input) → submit
  posts `{name, expiresAt}` where expiresAt is computed client-side as an
  end-of-day UTC "Y-m-d H:i:s" string (assert the exact body for the 30-days
  choice given a mocked `Date.now`); success swaps the dialog content to the
  show-once view containing the token text, a copy button
  (`navigator.clipboard.writeText` spy), and the "won't be shown again"
  warning; closing it returns to the refreshed list; revoke behind confirm
  posts `{id}`.
- [ ] **Step 2: Run to verify failure.**
- [ ] **Step 3: Implement the page** — same shell anatomy as SessionsPage;
  expiry computation helper `expiresAtFrom(choice: '30d'|'90d'|'365d'|'never'|Date): string | null`
  (exported for the test); the show-once state must live in the dialog (do
  NOT cache the token in the query cache).
- [ ] **Step 4: Route + locales.**
- [ ] **Step 5: Run the full frontend suite + lint** — `cd web && pnpm test -- --run && pnpm lint` → PASS.
- [ ] **Step 6: Commit** — `git add web && git commit -m "feat(web): personal access tokens page"`.

---

### Task 13: Final verification sweep

**Files:** none new — this is the gate.

- [ ] **Step 1: Full backend suite including engine comparison**

Run: `make test 2>&1 | tail -15`
Expected: go-test + pgsql repo rerun + enginecompare + web suite all green
(needs Docker for the PostgreSQL stack; if unavailable run
`make go-test && make web-test` and flag the gap to the user).

- [ ] **Step 2: Grep for stragglers**

```bash
grep -rn "JWT\|jwt" --include="*.go" internal/ cmd/ | grep -v "_test.go" | grep -vi "docs generated"
grep -rn "ECONUMO_JWT" . --include="*.md" --include="*.example" --include="*.yml" | grep -v docs/superpowers
```
Expected: no runtime references; docs clean (swagger-generated files may
legitimately contain the word in historic descriptions — regenerate if so).

- [ ] **Step 3: Live smoke (run skill)**

Boot the server against a scratch sqlite DB, then with curl: login → capture
opaque token → `get-session-list` shows 1 current session → create PAT →
call `get-user-data` with the PAT → revoke PAT → same call 401s → logout →
session token 401s. Then `pnpm dev` the SPA and click through Profile →
Security → Sessions and API tokens.

- [ ] **Step 4: Commit any fixes; do NOT merge**

Leave the branch for review; suggest `superpowers:finishing-a-development-branch`.
