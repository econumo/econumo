# Transaction Import — Stage 1 (Core) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the provider-independent core of transaction import — the nine `import_*` tables, the `access_tokens.scope` column with middleware enforcement, the `internal/imports` package (model, repo, ledger, runs, event inbox, matcher), env-configurable matcher thresholds, and `isImported` on the transaction list — with no provider wired yet.

**Architecture:** A new vertical feature package `internal/imports` (behavior only; entities/DTOs in `internal/model/imports.go`) owning a sqlc engine-adapter repo over the new tables. Cross-feature coupling stays one-directional and tiny: the `transaction` repo reads `import_transaction_links` with one hand-built batch query for `isImported` (no import → transaction import), and auth scope lives entirely in `user` + `middleware`. The matcher is a pure function over an `IngestEvent` and a candidate slice so it is unit-testable without a DB; the ledger's tombstone semantics come from `ON DELETE SET NULL` on `import_transaction_links.transaction_id` and are pinned by repo tests on both engines.

**Tech Stack:** Go 1.2x stdlib, sqlc (sqlite + pgsql generated packages), `modernc.org/sqlite` / `pgx`, `vo.DecimalNumber`, the repo's `dbtest`/`fixture`/`apiparity` harnesses; React/TypeScript DTO touch-ups only on the SPA side.

**Spec:** `docs/superpowers/specs/2026-08-15-transaction-import-design.md` (Parts 1–5 and the Stage 1 line of "Staging", lines 1400–1404). Read Part 2 (schema), Part 3 (deletion), Part 4 (isImported), Part 5 (pipeline + matcher + config) before starting.

## Global Constraints

- Features never import features. `internal/imports` may import only `internal/shared`, `internal/model`, `internal/infra/...`; `internal/transaction` must NOT import `internal/imports`; `internal/config` (a leaf) must NOT import `internal/imports`. `go test ./internal/test/archtest/` enforces it.
- Engine parity: every SQL change lands in BOTH `internal/infra/storage/{migrations,sqlc/query}/{sqlite,pgsql}`; sqlc is regenerated with `go generate ./internal/infra/storage/sqlc/` (or `cd internal/infra/storage/sqlc && ~/go/bin/sqlc generate`). A `;` on its own line inside a query silently truncates the generated SQL — keep it at the end of the last line.
- New migration file name: `20260901000000.sql` (latest existing is `20260818000000.sql`).
- Wire contract is frozen: datetimes `"2006-01-02 15:04:05"`, booleans-as-int `0`/`1` on list DTOs (`isImported` follows `isArchived`), 401 text exactly `"Invalid access token"`, validation messages exactly `"This value should not be blank."` (`errs.CodeIsBlank`) and `"The value you selected is not a valid choice."` (`errs.CodeInvalidChoice`).
- Token scopes: `full` | `ingest`. Sessions are always `full`. `create-personal-token` REQUIRES `scope` (blank → validation error; unknown → validation error). An `ingest` token is accepted ONLY on routes prefixed `/api/v1/import/ingest-`; everywhere else it is rejected with the 401 envelope. Unknown/empty stored scope fails closed (rejected everywhere).
- Matcher thresholds (spec Part 5, "Configuration"): `ECONUMO_IMPORT_MATCH_DAYS` default 3 (0–31), `ECONUMO_IMPORT_TIP_DAYS` default 5 (0–31), `ECONUMO_IMPORT_TIP_TOLERANCE` default 20 (percent, 0–100), `ECONUMO_IMPORT_TOKEN_MIN_LENGTH` default 3 (1–16). Strict parse: malformed/negative/out-of-range fails at boot.
- Ids are UUID strings (`vo.Id`); new ids are `vo.NewId()` (v7). Amounts are `vo.DecimalNumber` in Go, `NUMERIC(19,8)` in SQL, strings on the wire.
- Golden files (`internal/test/apiparity/testdata/golden/`, `internal/test/mcpparity/testdata/golden/`) are regenerated with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/` and the diff INSPECTED — never hand-edited.
- Comments: only the *why* of non-obvious logic; no restating names; swag `// @…` blocks stay intact.
- Every task ends with `gofmt -l ./internal ./cmd` printing nothing and `go vet ./...` clean. Commit after every task on the current branch (`txn-import-planning`, pushed as `feature/transaction-import`).

---

### Task 1: Migration — the nine import tables + `access_tokens.scope`

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260901000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260901000000.sql`
- Modify: `internal/infra/storage/sqlc/query/sqlite/access_tokens.sql`, `internal/infra/storage/sqlc/query/pgsql/access_tokens.sql`
- Regenerate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}/*` (sqlc)
- Modify: `internal/model/token.go`, `internal/user/repo/accesstoken.go`, `internal/user/repo/accesstoken_integration_test.go`, `internal/user/session.go`, `internal/user/pat.go`, `internal/user/authenticate_test.go` (`seedToken`), `internal/user/api/harness_test.go:268-277`, `internal/test/fixture/entities.go:653-677`
- Test: `internal/infra/storage/migrations/migrations_import_test.go` (new), `internal/user/repo/accesstoken_integration_test.go`

**Interfaces:**
- Produces: `model.TokenScope` (`string`), `model.TokenScopeFull = "full"`, `model.TokenScopeIngest = "ingest"`, `model.ParseTokenScope(s string) (TokenScope, error)`, field `model.AccessToken.Scope TokenScope`; column `access_tokens.scope TEXT NOT NULL`; tables `import_sources`, `import_credential_keys`, `import_runs`, `import_events`, `import_account_links`, `import_transaction_links`, `import_rules`, `import_rule_labels`, `import_link_applied_labels`; `fixture.AccessToken.Scope string` (default `"full"`).

- [ ] **Step 1: Write the failing migration test**

Create `internal/infra/storage/migrations/migrations_import_test.go`:

```go
package migrations_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

// The nine import tables plus access_tokens.scope come from 20260901000000.
// Probing each with a no-row SELECT is engine-neutral and fails loudly on a
// missing table or column.
func TestMigration20260901_ImportTablesAndTokenScope(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	for _, q := range []string{
		"SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at FROM import_sources WHERE 1 = 0",
		"SELECT user_id, key_ciphertext, created_at FROM import_credential_keys WHERE 1 = 0",
		"SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, started_at, finished_at FROM import_runs WHERE 1 = 0",
		"SELECT id, source_id, run_id, payload, payload_hash, status, parse_error, received_at FROM import_events WHERE 1 = 0",
		"SELECT id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at FROM import_account_links WHERE 1 = 0",
		"SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at FROM import_transaction_links WHERE 1 = 0",
		"SELECT id, user_id, source_id, position, match_payee, match_description, match_amount_min, match_amount_max, action, category_id, payee_id, tag_id, created_at, updated_at FROM import_rules WHERE 1 = 0",
		"SELECT rule_id, label_id FROM import_rule_labels WHERE 1 = 0",
		"SELECT link_id, label_id FROM import_link_applied_labels WHERE 1 = 0",
		"SELECT scope FROM access_tokens WHERE 1 = 0",
	} {
		if _, err := db.Raw.ExecContext(ctx, db.Rebind(q)); err != nil {
			t.Errorf("%s: %v", q, err)
		}
	}
	// scope has no default: the column must be supplied on every insert.
	_, err := db.Raw.ExecContext(ctx, db.Rebind(
		"INSERT INTO access_tokens (id, user_id, kind, token_hash, created_at, last_used_at) VALUES (?, ?, ?, ?, ?, ?)"),
		"0f000000-0000-0000-0000-000000000001", "0f000000-0000-0000-0000-0000000000aa", "session", "h", "2026-01-01 00:00:00", "2026-01-01 00:00:00")
	if err == nil {
		t.Error("inserting an access token without scope must fail (NOT NULL, no default)")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/infra/storage/migrations/ -run TestMigration20260901 -v`
Expected: FAIL — `no such table: import_sources` (and the others), plus `no such column: scope`.

- [ ] **Step 3: Write the SQLite migration**

Create `internal/infra/storage/migrations/sqlite/20260901000000.sql`:

```sql
-- Transaction import (docs/superpowers/specs/2026-08-15-transaction-import-design.md, Part 2).
-- Nine tables plus a scope on access tokens. The link ledger is the source of
-- truth for "already seen" — its rows survive the deletion of the transaction
-- they point at (transaction_id goes NULL: a tombstone), so a deleted import
-- is never re-created on the next sync.

CREATE TABLE import_sources
(
    id                    TEXT NOT NULL
    , user_id               TEXT NOT NULL
    , provider              TEXT NOT NULL
    , name                  TEXT NOT NULL
    , credential_ciphertext TEXT DEFAULT NULL
    , status                TEXT NOT NULL
    , last_synced_at        DATETIME DEFAULT NULL
    , created_at            DATETIME NOT NULL
    , updated_at            DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_sources_user_id ON import_sources (user_id);

CREATE TABLE import_credential_keys
(
    user_id        TEXT NOT NULL
    , key_ciphertext TEXT NOT NULL
    , created_at     DATETIME NOT NULL
    , PRIMARY KEY (user_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE import_runs
(
    id               TEXT NOT NULL
    , user_id          TEXT NOT NULL
    , source_id        TEXT NOT NULL
    , provider         TEXT NOT NULL
    , params           TEXT NOT NULL
    , status           TEXT NOT NULL
    , imported_count   INTEGER DEFAULT 0 NOT NULL
    , matched_count    INTEGER DEFAULT 0 NOT NULL
    , skipped_count    INTEGER DEFAULT 0 NOT NULL
    , failed_count     INTEGER DEFAULT 0 NOT NULL
    , started_at       DATETIME NOT NULL
    , finished_at      DATETIME DEFAULT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_runs_source_id ON import_runs (source_id);

-- Raw push payloads (Apple Wallet). (source_id, payload_hash) is unique so a
-- Shortcut that re-fires the same tap is a no-op at the inbox.
CREATE TABLE import_events
(
    id           TEXT NOT NULL
    , source_id    TEXT NOT NULL
    , run_id       TEXT DEFAULT NULL
    , payload      TEXT NOT NULL
    , payload_hash TEXT NOT NULL
    , status       TEXT NOT NULL
    , parse_error  TEXT DEFAULT NULL
    , received_at  DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE
    , FOREIGN KEY (run_id) REFERENCES import_runs (id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX UNIQ_import_events_source_payload ON import_events (source_id, payload_hash);
CREATE INDEX IDX_import_events_run_id ON import_events (run_id);

CREATE TABLE import_account_links
(
    id                  TEXT NOT NULL
    , source_id           TEXT NOT NULL
    , external_account_id TEXT NOT NULL
    , external_name       TEXT NOT NULL
    , external_currency   TEXT DEFAULT NULL
    , account_id          TEXT DEFAULT NULL
    , mode                TEXT NOT NULL
    , created_at          DATETIME NOT NULL
    , updated_at          DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
    , UNIQUE (source_id, external_account_id)
    , CHECK (mode = 'ignore' OR account_id IS NOT NULL)
);
CREATE INDEX IDX_import_account_links_account_id ON import_account_links (account_id);

-- The ledger. transaction_id is SET NULL (not CASCADE) on transaction delete:
-- the row stays as a tombstone so the external id counts as seen forever.
CREATE TABLE import_transaction_links
(
    id                      TEXT NOT NULL
    , source_id               TEXT NOT NULL
    , run_id                  TEXT DEFAULT NULL
    , event_id                TEXT DEFAULT NULL
    , external_account_id     TEXT NOT NULL
    , external_transaction_id TEXT NOT NULL
    , transaction_id          TEXT DEFAULT NULL
    , status                  TEXT NOT NULL
    , external_payee          TEXT NOT NULL
    , external_description    TEXT NOT NULL
    , external_amount         NUMERIC(19, 8) NOT NULL
    , external_currency       TEXT DEFAULT NULL
    , external_posted_at      DATETIME NOT NULL
    , applied_category_id     TEXT DEFAULT NULL
    , applied_payee_id        TEXT DEFAULT NULL
    , applied_tag_id          TEXT DEFAULT NULL
    , applied_rule_id         TEXT DEFAULT NULL
    , imported_at             DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE
    , FOREIGN KEY (run_id) REFERENCES import_runs (id) ON DELETE SET NULL
    , FOREIGN KEY (event_id) REFERENCES import_events (id) ON DELETE SET NULL
    , FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE SET NULL
    , UNIQUE (source_id, external_account_id, external_transaction_id)
);
CREATE INDEX IDX_import_transaction_links_transaction_id ON import_transaction_links (transaction_id);
CREATE INDEX IDX_import_transaction_links_run_id ON import_transaction_links (run_id);

CREATE TABLE import_rules
(
    id                TEXT NOT NULL
    , user_id           TEXT NOT NULL
    , source_id         TEXT DEFAULT NULL
    , position          INTEGER NOT NULL
    , match_payee       TEXT DEFAULT NULL
    , match_description TEXT DEFAULT NULL
    , match_amount_min  NUMERIC(19, 8) DEFAULT NULL
    , match_amount_max  NUMERIC(19, 8) DEFAULT NULL
    , action            TEXT NOT NULL
    , category_id       TEXT DEFAULT NULL
    , payee_id          TEXT DEFAULT NULL
    , tag_id            TEXT DEFAULT NULL
    , created_at        DATETIME NOT NULL
    , updated_at        DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE
    , FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE SET NULL
    , FOREIGN KEY (payee_id) REFERENCES payees (id) ON DELETE SET NULL
    , FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE SET NULL
    , CHECK (action <> 'skip' OR (category_id IS NULL AND payee_id IS NULL AND tag_id IS NULL))
);
CREATE INDEX IDX_import_rules_user_id ON import_rules (user_id);

CREATE TABLE import_rule_labels
(
    rule_id    TEXT NOT NULL
    , label_id   TEXT NOT NULL
    , PRIMARY KEY (rule_id, label_id)
    , FOREIGN KEY (rule_id) REFERENCES import_rules (id) ON DELETE CASCADE
    , FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_rule_labels_label_id ON import_rule_labels (label_id);

CREATE TABLE import_link_applied_labels
(
    link_id    TEXT NOT NULL
    , label_id   TEXT NOT NULL
    , PRIMARY KEY (link_id, label_id)
    , FOREIGN KEY (link_id) REFERENCES import_transaction_links (id) ON DELETE CASCADE
    , FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_link_applied_labels_label_id ON import_link_applied_labels (label_id);

-- access_tokens.scope: 'full' (sessions, ordinary PATs) or 'ingest' (a PAT
-- that may only call /api/v1/import/ingest-*). NOT NULL with no default so
-- every writer states the scope explicitly. SQLite cannot add a NOT NULL
-- column without a default, so the table is rebuilt (same shape as the
-- earlier rebuilds; the runner hoists the PRAGMA lines).
PRAGMA foreign_keys = OFF;
CREATE TABLE access_tokens_new
(
    id           TEXT NOT NULL
    , user_id      TEXT NOT NULL
    , kind         TEXT NOT NULL
    , token_hash   TEXT NOT NULL
    , scope        TEXT NOT NULL
    , name         TEXT DEFAULT NULL
    , user_agent   TEXT DEFAULT NULL
    , created_at   DATETIME NOT NULL
    , last_used_at DATETIME NOT NULL
    , expires_at   DATETIME DEFAULT NULL
    , revoked_at   DATETIME DEFAULT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
INSERT INTO access_tokens_new (id, user_id, kind, token_hash, scope, name, user_agent, created_at, last_used_at, expires_at, revoked_at)
SELECT id, user_id, kind, token_hash, 'full', name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens;
DROP TABLE access_tokens;
ALTER TABLE access_tokens_new RENAME TO access_tokens;
CREATE UNIQUE INDEX UNIQ_access_tokens_token_hash ON access_tokens (token_hash);
CREATE INDEX IDX_access_tokens_user_id ON access_tokens (user_id);
CREATE INDEX IDX_access_tokens_revoked_at ON access_tokens (revoked_at);
CREATE INDEX IDX_access_tokens_expires_at ON access_tokens (expires_at);
PRAGMA foreign_keys = ON;
```

Before finalising, check how the existing sqlite rebuild migrations in the same directory handle `PRAGMA foreign_keys` (`grep -l "foreign_keys" internal/infra/storage/migrations/sqlite/*.sql`) and mirror that file's exact placement of the two PRAGMA lines — the runner hoists them and runs `foreign_key_check` afterwards.

- [ ] **Step 4: Write the PostgreSQL migration**

Create `internal/infra/storage/migrations/pgsql/20260901000000.sql` with the same tables, types translated: `TEXT NOT NULL` ids → `UUID NOT NULL`, `DATETIME` → `TIMESTAMP(0) WITHOUT TIME ZONE`, counts `INTEGER` → `BIGINT` (so sqlc emits `int64` on both engines and the row structs stay identical), `NUMERIC(19, 8)` unchanged, `BOOLEAN` none needed. `provider`/`status`/`mode`/`action`/`params`/`payload`/`*_hash`/`external_*` stay `TEXT`. Same index and constraint names. The scope change is two statements instead of a rebuild:

```sql
ALTER TABLE access_tokens ADD COLUMN scope TEXT NOT NULL DEFAULT 'full';
ALTER TABLE access_tokens ALTER COLUMN scope DROP DEFAULT;
```

Everything else is written out in full (every `CREATE TABLE` / `CREATE INDEX` from Step 3 with the type substitutions above).

- [ ] **Step 5: Run the migration test on sqlite**

Run: `go test ./internal/infra/storage/migrations/ -run TestMigration20260901 -v`
Expected: PASS.

- [ ] **Step 6: Add `scope` to the access-token queries and regenerate sqlc**

In `internal/infra/storage/sqlc/query/sqlite/access_tokens.sql`:
- `InsertAccessToken`: column list becomes `(id, user_id, kind, token_hash, scope, name, user_agent, created_at, last_used_at, expires_at, revoked_at)` with 11 `?`.
- `GetAccessTokenByHash`: add `t.scope` after `t.token_hash` in the select list.
- `GetAccessTokenByID` and `ListAccessTokensByUser`: add `scope` after `token_hash`.

Mirror in `query/pgsql/access_tokens.sql` (`$1..$11` for the insert). Then:

Run: `go generate ./internal/infra/storage/sqlc/ && go build ./...`
Expected: build FAILS in `internal/user/repo/accesstoken.go` (`Scope` missing from `insertAccessTokenParams`) — that is the signal the generation took.

- [ ] **Step 7: Add `TokenScope` to the model**

In `internal/model/token.go` add:

```go
// TokenScope narrows what a bearer token may call. Sessions and ordinary PATs
// are full; an ingest PAT exists so a phone Shortcut can hold a credential that
// can push transactions and do nothing else.
type TokenScope string

const (
	TokenScopeFull   TokenScope = "full"
	TokenScopeIngest TokenScope = "ingest"
)

// ParseTokenScope fails closed: anything but the two known values is an error.
func ParseTokenScope(s string) (TokenScope, error) {
	switch TokenScope(s) {
	case TokenScopeFull, TokenScopeIngest:
		return TokenScope(s), nil
	}
	return "", errs.NewValidation("Validation failed", errs.FieldError{
		Key: "scope", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice,
	})
}
```

and the field `Scope TokenScope` on `AccessToken` (after `TokenHash`). Check `internal/model/token.go`'s imports — add `"github.com/econumo/econumo/internal/shared/errs"` if not present.

- [ ] **Step 8: Plumb `Scope` through the user repo**

In `internal/user/repo/accesstoken.go`:
- `Insert`: before the query call add
  ```go
  if _, err := model.ParseTokenScope(string(t.Scope)); err != nil {
      return fmt.Errorf("access token %s: %w", t.ID, err)
  }
  ```
  and `Scope: string(t.Scope),` in the params. (Add `"fmt"` to imports.)
- `tokenRowFromHashRow`: copy `Scope: row.Scope`.
- `accessTokenFromRow`: `Scope: model.TokenScope(row.Scope)`.

Also update every constructor of `model.AccessToken` to set `Scope: model.TokenScopeFull`:
- `internal/user/session.go` `createSession`
- `internal/user/pat.go` `CreatePersonalToken` (temporarily `TokenScopeFull`; Task 2 switches it to the request's scope)
- `internal/user/authenticate_test.go` `seedToken`
- `internal/user/api/harness_test.go:268-277`
- `internal/user/repo/accesstoken_integration_test.go` (all four literals: `tok`, `pat`, `dup`, and the `insert` closure in `TestAccessTokenRepo_DeleteDead`)

In `internal/test/fixture/entities.go` extend the `AccessToken` builder struct with `Scope string` and in its insert use `scope` column with `orDefault(a.Scope, "full")` — if no such helper exists, inline:
```go
scope := a.Scope
if scope == "" {
    scope = "full"
}
```
and add `scope` to the column list/values (11 columns).

- [ ] **Step 9: Extend the repo round-trip test**

In `TestAccessTokenRepo_RoundTrip` set `Scope: model.TokenScopeIngest` on `pat` and assert after `ListByUser(personal)`:

```go
if pats[0].Scope != model.TokenScopeIngest {
    t.Errorf("pat scope = %q, want ingest", pats[0].Scope)
}
if got.Scope != model.TokenScopeFull {
    t.Errorf("session scope = %q, want full", got.Scope)
}
```

and add a new test:

```go
func TestAccessTokenRepo_InsertRejectsEmptyScope(t *testing.T) {
	db := dbtest.New(t)
	seedTokenUser(t, db, userA)
	repo := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	tok := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), Kind: model.TokenKindSession,
		TokenHash: "hash-noscope", CreatedAt: now, LastUsedAt: now,
	}
	if err := repo.Insert(context.Background(), tok); err == nil {
		t.Fatal("Insert without scope must fail")
	}
}
```

- [ ] **Step 10: Run the affected suites**

Run: `go build ./... && go test ./internal/user/... ./internal/infra/... ./internal/test/fixture/... ./internal/test/apiparity/ ./internal/test/mcpparity/`
Expected: PASS (apiparity goldens are unchanged — `scope` is not yet on any wire DTO).

- [ ] **Step 11: Commit**

```bash
gofmt -l ./internal ./cmd; go vet ./...
git add internal/infra/storage internal/model/token.go internal/user internal/test/fixture
git commit -m "feat(import): import tables + access_tokens.scope migration"
```

### Task 2: `Principal`, PAT scope on the wire, and middleware enforcement

**Files:**
- Modify: `internal/model/token.go` (add `Principal`), `internal/model/token_dto.go:50-105`
- Modify: `internal/web/middleware/auth.go`, `internal/web/middleware/middleware_test.go:272` (`stubAuthn`)
- Modify: `internal/user/authenticate.go`, `internal/user/authenticate_test.go` (call sites at lines ~91–241), `internal/user/pat.go`
- Modify: `internal/server/glue_timezone.go:40`, `internal/server/glue_timezone_test.go:54,62,69`, `internal/test/authstub/authstub.go:21`
- Test: `internal/model/token_dto_test.go` (new or existing), `internal/web/middleware/auth_scope_test.go` (new), `internal/user/pat_test.go` (existing PAT tests — grep `CreatePersonalToken` under `internal/user/`)

**Interfaces:**
- Consumes: `model.TokenScope`, `model.AccessToken.Scope` (Task 1).
- Produces: `model.Principal{UserID, TokenID vo.Id; Level AccessLevel; Scope TokenScope}`; `middleware.TokenAuthenticator.Authenticate(ctx, token string) (model.Principal, error)`; `CreatePersonalTokenRequest.Scope string`; `PersonalTokenItem.Scope string`; `CreatePersonalTokenResult.Scope string`; `middleware.IngestPathPrefix = "/api/v1/import/ingest-"`.

- [ ] **Step 1: Failing DTO validation tests**

Add to `internal/model/token_dto_test.go` (create the file with `package model_test` if it does not exist; import `errs`, `model`, `testing`):

```go
func TestCreatePersonalTokenRequest_Scope(t *testing.T) {
	cases := []struct {
		name  string
		scope string
		code  string
	}{
		{"full ok", "full", ""},
		{"ingest ok", "ingest", ""},
		{"blank", "", errs.CodeIsBlank},
		{"unknown", "admin", errs.CodeInvalidChoice},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := model.CreatePersonalTokenRequest{Name: "ci", Scope: tc.scope}.Validate()
			if tc.code == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			ve, ok := err.(*errs.ValidationError)
			if !ok {
				t.Fatalf("got %T, want *errs.ValidationError", err)
			}
			var found bool
			for _, f := range ve.Fields {
				if f.Key == "scope" && f.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("no scope field error with code %q in %+v", tc.code, ve.Fields)
			}
		})
	}
}
```

Check the real names of `errs.ValidationError`'s field slice (`grep -n "type ValidationError" -A 8 internal/shared/errs/*.go`) and adjust `ve.Fields` accordingly.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/model/ -run TestCreatePersonalTokenRequest_Scope`
Expected: FAIL — `unknown field Scope`.

- [ ] **Step 3: Add `scope` to the DTOs**

In `internal/model/token_dto.go`:

```go
type PersonalTokenItem struct {
	Id         string  `json:"id"`
	Name       string  `json:"name"`
	Scope      string  `json:"scope"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt string  `json:"lastUsedAt"`
	ExpiresAt  *string `json:"expiresAt"` // null = never expires
}

// scope is REQUIRED: a client that predates scopes must fail loudly rather
// than silently mint a full-access token it did not ask for.
type CreatePersonalTokenRequest struct {
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	ExpiresAt string `json:"expiresAt"`
}
```

In `Validate()`, after the name check:

```go
	if strings.TrimSpace(r.Scope) == "" {
		fields = append(fields, errs.FieldError{Key: "scope", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	} else if _, err := ParseTokenScope(r.Scope); err != nil {
		fields = append(fields, errs.FieldError{Key: "scope", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
```

Add `Scope string \`json:"scope"\`` to `CreatePersonalTokenResult` after `Name`.

- [ ] **Step 4: Run the DTO test**

Run: `go test ./internal/model/ -run TestCreatePersonalTokenRequest_Scope`
Expected: PASS.

- [ ] **Step 5: Introduce `Principal` and change the authenticator seam**

In `internal/model/token.go`:

```go
// Principal is what a bearer token resolves to: who is calling, through which
// token row, with what access level and scope.
type Principal struct {
	UserID  vo.Id
	TokenID vo.Id
	Level   AccessLevel
	Scope   TokenScope
}
```

In `internal/web/middleware/auth.go` change the interface:

```go
type TokenAuthenticator interface {
	Authenticate(ctx context.Context, token string) (model.Principal, error)
}

// IngestPathPrefix is the only route family an ingest-scoped token may reach.
const IngestPathPrefix = "/api/v1/import/ingest-"
```

and in `Auth`, replace the `userID, tokenID, level, err := authn.Authenticate(...)` block with:

```go
			p, err := authn.Authenticate(r.Context(), token)
			if err != nil {
				var ue *errs.UnauthorizedError
				if !errors.As(err, &ue) {
					err = errs.NewUnauthorized("Invalid access token")
				}
				httpx.WriteError(r.Context(), w, err)
				return
			}
			// Scope is checked as "not full" so an empty or unknown stored
			// scope fails closed. The 401 text is identical to a bad token on
			// purpose: an ingest credential must not reveal that it is valid
			// elsewhere.
			if p.Scope != model.TokenScopeFull && !strings.HasPrefix(r.URL.Path, IngestPathPrefix) {
				httpx.WriteError(r.Context(), w, errs.NewUnauthorized("Invalid access token"))
				return
			}
			userID, tokenID, level := p.UserID, p.TokenID, p.Level
```

Everything below (`StoredLanguage`, the 402 gate, context population) keeps using `userID`, `tokenID`, `level` unchanged.

- [ ] **Step 6: Update every implementation and call site**

`internal/user/authenticate.go`:

```go
func (s *Service) Authenticate(ctx context.Context, raw string) (model.Principal, error) {
	t, level, until, err := s.tokens.GetByHash(ctx, HashAccessToken(raw))
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return model.Principal{}, errs.NewUnauthorized("Invalid access token")
		}
		return model.Principal{}, err
	}
	now := s.clock.Now()
	if !t.IsLive(now) {
		return model.Principal{}, errs.NewUnauthorized("Invalid access token")
	}
	if t.NeedsTouch(now, touchInterval) {
		t.Touch(now, SessionTTL)
		if err := s.tokens.Update(ctx, t); err != nil {
			return model.Principal{}, err
		}
	}
	u := model.User{AccessLevel: level, AccessUntil: until}
	return model.Principal{UserID: t.UserID, TokenID: t.ID, Level: u.EffectiveAccessLevel(now), Scope: t.Scope}, nil
}
```

`internal/server/glue_timezone.go`:

```go
func (a *timezoneTrackingAuthenticator) Authenticate(ctx context.Context, token string) (model.Principal, error) {
	p, err := a.inner.Authenticate(ctx, token)
	if err != nil || !reqctx.IsLocationExplicit(ctx) {
		return p, err
	}
	tz := reqctx.Location(ctx).String()
	if prev, ok := a.seen.Load(p.UserID); ok && prev.(string) == tz {
		return p, nil
	}
	if perr := a.users.PersistTimezone(ctx, p.UserID, tz); perr != nil {
		slog.WarnContext(ctx, "timezone persist failed", slog.Any("err", perr))
	} else {
		a.seen.Store(p.UserID, tz)
	}
	return p, nil
}
```

`internal/test/authstub/authstub.go`:

```go
func (a Authenticator) Authenticate(_ context.Context, token string) (model.Principal, error) {
	id, err := vo.ParseId(token)
	if err != nil {
		return model.Principal{}, errs.NewUnauthorized("Invalid access token")
	}
	level := a.Level
	if level == "" {
		level = model.AccessLevelFull
	}
	return model.Principal{UserID: id, TokenID: id, Level: level, Scope: model.TokenScopeFull}, nil
}
```

`internal/web/middleware/middleware_test.go` `stubAuthn`: return `model.Principal{UserID: s.userID, TokenID: s.tokenID, Level: s.level, Scope: model.TokenScopeFull}, s.err` (add a `scope model.TokenScope` field defaulting to full if you prefer; the scope test below uses its own stub).

`internal/server/glue_timezone_test.go` (lines 54, 62, 69): `_, _, _, err :=` → `_, err :=`.
`internal/user/authenticate_test.go`: `gotUser, gotTok, _, err :=` → `p, err :=` then use `p.UserID` / `p.TokenID`; `_, _, level, err :=` → `p, err :=` then `p.Level`. Add one assertion in the happy-path test: `if p.Scope != model.TokenScopeFull { t.Errorf("scope = %q", p.Scope) }`.

`internal/user/pat.go`: replace the `// Full-access (no scopes)` header sentence with `// Scoped (full | ingest); optional fixed expiry; never touched by password`; in `CreatePersonalToken` set `Scope: model.TokenScope(req.Scope)` on the token (Validate already guarantees it parses) and `Scope: req.Scope` on the result; in `ListPersonalTokens` set `Scope: string(rows[i].Scope)` on each `PersonalTokenItem`.

Then fix every existing `CreatePersonalTokenRequest{...}` literal in `internal/user/*_test.go` and `internal/user/api/*_test.go` to include `Scope: "full"` (`grep -rn "CreatePersonalTokenRequest{" internal/`).

- [ ] **Step 7: Build and run the touched packages**

Run: `go build ./... && go test ./internal/model/ ./internal/user/... ./internal/web/... ./internal/server/`
Expected: PASS.

- [ ] **Step 8: Failing middleware scope test**

Create `internal/web/middleware/auth_scope_test.go`:

```go
package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/middleware"
)

type scopedAuthn struct{ scope model.TokenScope }

func (a scopedAuthn) Authenticate(context.Context, string) (model.Principal, error) {
	id := vo.NewId()
	return model.Principal{UserID: id, TokenID: id, Level: model.AccessLevelFull, Scope: a.scope}, nil
}

func TestAuth_ScopeGate(t *testing.T) {
	cases := []struct {
		name   string
		scope  model.TokenScope
		method string
		path   string
		want   int
	}{
		{"full anywhere", model.TokenScopeFull, http.MethodGet, "/api/v1/user/get-user-data", http.StatusOK},
		{"ingest on ingest route", model.TokenScopeIngest, http.MethodPost, "/api/v1/import/ingest-apple-wallet", http.StatusOK},
		{"ingest on read route", model.TokenScopeIngest, http.MethodGet, "/api/v1/user/get-user-data", http.StatusUnauthorized},
		{"ingest on other import route", model.TokenScopeIngest, http.MethodPost, "/api/v1/import/create-source", http.StatusUnauthorized},
		{"empty scope fails closed", model.TokenScope(""), http.MethodGet, "/api/v1/user/get-user-data", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ran bool
			h := middleware.Auth(scopedAuthn{scope: tc.scope})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ran = true }))
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer x")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tc.want, rec.Body.String())
			}
			if (tc.want == http.StatusOK) != ran {
				t.Fatalf("handler ran = %v", ran)
			}
			if tc.want == http.StatusUnauthorized && !strings.Contains(rec.Body.String(), `"message":"Invalid access token"`) {
				t.Fatalf("401 body must carry the frozen message, got %s", rec.Body.String())
			}
		})
	}
}
```

- [ ] **Step 9: Run it**

Run: `go test ./internal/web/middleware/ -run TestAuth_ScopeGate -v`
Expected: PASS (the gate was added in Step 5; if any case fails, fix the gate — it must sit BEFORE the stored-language and 402 logic).

- [ ] **Step 10: Commit**

```bash
gofmt -l ./internal ./cmd; go vet ./...
git add internal/model internal/web/middleware internal/user internal/server internal/test/authstub
git commit -m "feat(auth): token scopes — Principal, scoped PATs, ingest-only gate"
```

### Task 3: Scope on the API surface — parity scenarios, goldens, SPA DTOs, docs

**Files:**
- Modify: `internal/test/apiparity/fixture.go` (seed an ingest PAT), `internal/test/apiparity/harness.go:222-245` (new `"ingest"` auth kind), `internal/test/apiparity/apiparity.go:15-17` (doc the new kind), `internal/test/apiparity/catalogue_user.go:120-135`
- Regenerate: `internal/test/apiparity/testdata/golden/*`, `internal/test/mcpparity/testdata/golden/*`
- Modify: `web/src/api/user.ts:137`, `web/src/api/dto/user.ts:58`
- Modify: `CLAUDE.md` (Authentication + Access tokens sections), `docs/regression-test-plan.md` (section 12, PAT item at ~338–340)

**Interfaces:**
- Consumes: `CreatePersonalTokenRequest.Scope`, `PersonalTokenItem.Scope`, middleware gate (Task 2); `fixture.AccessToken.Scope` (Task 1).
- Produces: apiparity constants `IngestToken`, `IngestTokenID`; auth kind `"ingest"`.

- [ ] **Step 1: Seed an ingest-scoped PAT for the owner**

In `internal/test/apiparity/fixture.go` add to the const block:

```go
	// An ingest-scoped PAT of the owner: valid credential, but only on
	// /api/v1/import/ingest-* — every other route must 401 it.
	IngestToken   = "eco_pat_owner-ingest-token-000000000000000000000000"
	IngestTokenID = "99999999-9999-9999-9999-999999999999"
```

(`len("owner-ingest-token-000000000000000000000000")` must be 43 — count it: 19 + 24 = 43.) In `Seed`, after the Readonly session:

```go
	f.AccessToken(fixture.AccessToken{ID: IngestTokenID, UserID: OwnerID, Kind: model.TokenKindPersonal,
		TokenHash: appuser.HashAccessToken(IngestToken), Name: "Phone shortcut", Scope: string(model.TokenScopeIngest)})
```

In `harness.go`'s per-call `switch c.Auth` add `case "ingest": tok = IngestToken` and extend the `Auth` field comment in `apiparity.go` with `"ingest" (the owner's ingest-scoped PAT)`.

- [ ] **Step 2: Extend the PAT scenario and add the scope-gate scenario**

In `catalogue_user.go`, `user_personal_tokens`: add `"scope": "full"` to the four existing create bodies, and append:

```go
			{Label: "create-personal-token-ingest", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "Phone", "scope": "ingest", "expiresAt": ""}},
			{Label: "err:create-personal-token-blank-scope", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "No scope", "expiresAt": ""}},
			{Label: "err:create-personal-token-bad-scope", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "Bad scope", "scope": "admin", "expiresAt": ""}},
			{Label: "get-personal-token-list-after", Method: "GET", Path: "/api/v1/user/get-personal-token-list", Auth: "owner"},
```

and a new scenario in the same file:

```go
func init() {
	register(Scenario{Name: "user_ingest_token_scope", Calls: func() []Call {
		return []Call{
			// The seeded ingest PAT is live, yet every non-ingest route rejects
			// it with the same envelope as an unknown token.
			{Label: "err:ingest-token-on-read", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "ingest"},
			{Label: "err:ingest-token-on-write", Method: "POST", Path: "/api/v1/category/create-category", Auth: "ingest",
				Body: map[string]any{"id": "00000000-0000-0000-0000-00000000c0de", "name": "Nope", "type": "expense"}},
			// The seeded ingest PAT shows up in the owner's list with its scope.
			{Label: "get-personal-token-list", Method: "GET", Path: "/api/v1/user/get-personal-token-list", Auth: "owner"},
		}
	}})
}
```

Check the exact create-category body shape used elsewhere in the catalogue (`grep -n "create-category" internal/test/apiparity/catalogue_category.go`) and copy it.

- [ ] **Step 3: Regenerate goldens and inspect**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/ && git status --short internal/test && git diff internal/test/apiparity/testdata/golden | head -150`
Expected: new golden files for the two scenarios; the only changes to EXISTING goldens are (a) `"scope":"full"` appearing in personal-token create/list payloads and (b) nothing else. Any other diff means a regression — stop and fix before continuing. Then `go test ./internal/test/apiparity/ ./internal/test/mcpparity/` must PASS without the env var.

- [ ] **Step 4: SPA DTOs**

`web/src/api/dto/user.ts` (`PersonalTokenDto`): add `scope: 'full' | 'ingest'`. `web/src/api/user.ts` `createPersonalToken`: post `{ name, scope: 'full', expiresAt: expiresAt ?? '' }` — the Settings UI creates full tokens only; ingest tokens are minted from Settings → Data in stage 2.

Run: `cd web && pnpm lint && pnpm test`
Expected: PASS.

- [ ] **Step 5: Docs**

`CLAUDE.md`, "Authentication" bullet list — extend the second bullet: `Two kinds: session … and personal (…). Every token carries a scope: full (sessions, ordinary PATs) or ingest (a PAT that may ONLY call /api/v1/import/ingest-*; anywhere else it gets the frozen 401 "Invalid access token"). create-personal-token requires scope.` In "Access tokens (internal/user/token.go)" under the frozen contract add: `- scope: "full" | "ingest" on create-personal-token (required; blank → common.is_blank, unknown → common.invalid_choice) and echoed on get-personal-token-list.`

`docs/regression-test-plan.md`, section 12, next to the PAT item: add `- [ ] Create a personal token with scope "full" — it works everywhere; an "ingest" token (create via API) is rejected with 401 on every non-import route.`

- [ ] **Step 6: Commit**

```bash
gofmt -l ./internal ./cmd; go vet ./...
git add internal/test web/src CLAUDE.md docs/regression-test-plan.md
git commit -m "feat(auth): scope on the PAT wire contract + parity coverage"
```

### Task 4: `internal/imports` model, fixture builders, sources + event inbox repo

**Files:**
- Create: `internal/model/imports.go`
- Create: `internal/imports/doc.go`, `internal/imports/repository.go`, `internal/imports/hash.go`, `internal/imports/hash_test.go`
- Create: `internal/infra/storage/sqlc/query/sqlite/imports.sql`, `internal/infra/storage/sqlc/query/pgsql/imports.sql`; regenerate sqlc
- Create: `internal/imports/repo/repo.go`, `internal/imports/repo/sqlite.go`, `internal/imports/repo/pgsql.go`, `internal/imports/repo/repo_integration_test.go`
- Create: `internal/test/fixture/imports.go`

**Interfaces:**
- Consumes: tables from Task 1.
- Produces (all in `package model`, file `imports.go`):
  - constants `ImportProviderSimpleFIN = "simplefin"`, `ImportProviderCSV = "csv"`, `ImportProviderAppleWallet = "apple-wallet"`; `ImportProviderIsPush(p string) bool` (true only for apple-wallet); `ImportSourceStatusActive = "active"`; `ImportEventStatusProcessed/Failed/Discarded = "processed"/"failed"/"discarded"`; `ImportLinkStatusLinked/Queued/Skipped = "linked"/"queued"/"skipped"`; `ImportRunStatusRunning/Completed/Failed/Partial`.
  - structs `ImportSource`, `ImportEvent`, `ImportRun`, `ImportTransactionLink` (with methods `IsSeen() bool`, `IsTombstone() bool`), `IngestEvent`, `ImportCandidate`, `ImportCandidateLink` (exact field lists in Step 3).
  - `imports.HashPayload(b []byte) string` (sha256 hex).
  - `imports.Repository` interface (Step 4) and `importsrepo.NewRepo(driver string, tx *backend.TxManager) *Repo`.
  - `fixture.Builder.ImportSource(fixture.ImportSource) string`, `fixture.Builder.ImportTransactionLink(fixture.ImportTransactionLink) string`.

Naming note: `internal/model/transaction_view.go` already defines `ImportAccount`, `ImportNamed`, `ImportMapping`, `ImportRequest`, `ImportResult` for the CSV upload endpoint. Do not reuse those names.

- [ ] **Step 1: Failing hash test**

`internal/imports/hash_test.go`:

```go
package imports_test

import (
	"testing"

	"github.com/econumo/econumo/internal/imports"
)

func TestHashPayload(t *testing.T) {
	// sha256("abc"), a well-known vector.
	if got := imports.HashPayload([]byte("abc")); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("HashPayload = %s", got)
	}
	if imports.HashPayload([]byte("abc")) == imports.HashPayload([]byte("abd")) {
		t.Fatal("distinct payloads must hash differently")
	}
}
```

Run: `go test ./internal/imports/ -run TestHashPayload` — Expected: FAIL (package does not exist).

- [ ] **Step 2: Package skeleton + hash**

`internal/imports/doc.go`:

```go
// Package imports is the transaction-import feature: external sources
// (push events from a phone, pull providers later), the link ledger that
// remembers every external transaction ever seen, import runs, and the
// matcher that decides whether an incoming record is a new transaction or one
// the user already entered by hand.
package imports
```

`internal/imports/hash.go`:

```go
package imports

import (
	"crypto/sha256"
	"encoding/hex"
)

func HashPayload(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
```

Run: `go test ./internal/imports/ -run TestHashPayload` — Expected: PASS.

- [ ] **Step 3: Model types**

`internal/model/imports.go`:

```go
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	ImportProviderSimpleFIN   = "simplefin"
	ImportProviderCSV         = "csv"
	ImportProviderAppleWallet = "apple-wallet"

	ImportSourceStatusActive = "active"

	ImportEventStatusProcessed = "processed"
	ImportEventStatusFailed    = "failed"
	ImportEventStatusDiscarded = "discarded"

	ImportLinkStatusLinked  = "linked"
	ImportLinkStatusQueued  = "queued"
	ImportLinkStatusSkipped = "skipped"

	ImportRunStatusRunning   = "running"
	ImportRunStatusCompleted = "completed"
	ImportRunStatusFailed    = "failed"
	ImportRunStatusPartial   = "partial"
)

// Push providers deliver one event per tap and see the transaction before
// the bank posts it; pull providers see the posted record. The matcher
// treats the two differently (a push link is the "tip" a later bank record
// adopts).
func ImportProviderIsPush(provider string) bool {
	return provider == ImportProviderAppleWallet
}

type ImportSource struct {
	ID                   vo.Id
	UserID               vo.Id
	Provider             string
	Name                 string
	CredentialCiphertext *string
	Status               string
	LastSyncedAt         *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type ImportEvent struct {
	ID          vo.Id
	SourceID    vo.Id
	RunID       *vo.Id
	Payload     string
	PayloadHash string
	Status      string
	ParseError  *string
	ReceivedAt  time.Time
}

type ImportRun struct {
	ID            vo.Id
	UserID        vo.Id
	SourceID      vo.Id
	Provider      string
	Params        string
	Status        string
	ImportedCount int
	MatchedCount  int
	SkippedCount  int
	FailedCount   int
	StartedAt     time.Time
	FinishedAt    *time.Time
}

// ImportTransactionLink is one ledger row: what an external transaction id
// became. A linked row whose TransactionID is nil is a tombstone — the user
// deleted the transaction, and the external id must never be re-imported.
type ImportTransactionLink struct {
	ID                    vo.Id
	SourceID              vo.Id
	RunID                 *vo.Id
	EventID               *vo.Id
	ExternalAccountID     string
	ExternalTransactionID string
	TransactionID         *vo.Id
	Status                string
	ExternalPayee         string
	ExternalDescription   string
	ExternalAmount        string
	ExternalCurrency      *string
	ExternalPostedAt      time.Time
	AppliedCategoryID     *vo.Id
	AppliedPayeeID        *vo.Id
	AppliedTagID          *vo.Id
	AppliedRuleID         *vo.Id
	ImportedAt            time.Time
}

// A queued row is not "seen": the user has not decided yet, so the same
// external id may be re-offered by a later sync.
func (l ImportTransactionLink) IsSeen() bool { return l.Status != ImportLinkStatusQueued }

func (l ImportTransactionLink) IsTombstone() bool {
	return l.Status == ImportLinkStatusLinked && l.TransactionID == nil
}

// IngestEvent is one normalized external transaction, the matcher's input.
// Amount is the absolute value as a decimal string; Type carries the sign.
type IngestEvent struct {
	ExternalAccountID     string
	ExternalTransactionID string
	Type                  TransactionType
	Amount                string
	Currency              string
	PostedAt              time.Time
	Payee                 string
	Description           string
}

// ImportCandidate is an existing transaction the matcher may adopt, with the
// ledger rows already pointing at it.
type ImportCandidate struct {
	TransactionID vo.Id
	Type          TransactionType
	Amount        string
	SpentAt       time.Time
	Links         []ImportCandidateLink
}

type ImportCandidateLink struct {
	SourceID       vo.Id
	Provider       string
	ExternalAmount string
	ExternalPayee  string
}
```

Run: `go build ./... && go test ./internal/test/archtest/` — Expected: PASS (`model` still imports only `shared`).

- [ ] **Step 4: Repository interface**

`internal/imports/repository.go`:

```go
package imports

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Repository interface {
	InsertSource(ctx context.Context, s *model.ImportSource) error
	GetSource(ctx context.Context, id vo.Id) (*model.ImportSource, error)

	// InsertEvent is the inbox dedupe point: a payload already stored for the
	// source (same hash) is dropped and inserted=false is returned.
	InsertEvent(ctx context.Context, e *model.ImportEvent) (inserted bool, err error)
	GetEvent(ctx context.Context, id vo.Id) (*model.ImportEvent, error)
	UpdateEventStatus(ctx context.Context, id vo.Id, status string, parseError *string) error

	InsertRun(ctx context.Context, r *model.ImportRun) error
	GetRun(ctx context.Context, id vo.Id) (*model.ImportRun, error)
	UpdateRun(ctx context.Context, r *model.ImportRun) error

	InsertLink(ctx context.Context, l *model.ImportTransactionLink) error
	GetLinkByExternalKey(ctx context.Context, sourceID vo.Id, externalAccountID, externalTransactionID string) (*model.ImportTransactionLink, error)
	ListLinksByTransaction(ctx context.Context, transactionID vo.Id) ([]model.ImportTransactionLink, error)
}
```

Runs and links are implemented in Task 5; this task implements sources + events and leaves the rest returning `errors.New("not implemented")` ONLY until Task 5 replaces them (Task 5 must delete every such stub).

- [ ] **Step 5: sqlc queries (sources + events)**

`internal/infra/storage/sqlc/query/sqlite/imports.sql`:

```sql
-- Transaction import: sources, the push-event inbox, runs, and the link
-- ledger. Liveness/tombstone logic lives in Go (model.ImportTransactionLink).

-- name: InsertImportSource :exec
INSERT INTO import_sources (id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetImportSourceByID :one
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE id = ?;

-- name: InsertImportEvent :execrows
-- The (source_id, payload_hash) unique index makes a re-fired push a no-op;
-- the caller reads the row count to learn whether this payload was new.
INSERT INTO import_events (id, source_id, run_id, payload, payload_hash, status, parse_error, received_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source_id, payload_hash) DO NOTHING;

-- name: GetImportEventByID :one
SELECT id, source_id, run_id, payload, payload_hash, status, parse_error, received_at
FROM import_events
WHERE id = ?;

-- name: UpdateImportEventStatus :exec
UPDATE import_events SET status = ?, parse_error = ?, run_id = ? WHERE id = ?;

-- name: InsertImportRun :exec
INSERT INTO import_runs (id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, started_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetImportRunByID :one
SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, started_at, finished_at
FROM import_runs
WHERE id = ?;

-- name: UpdateImportRun :exec
UPDATE import_runs
SET status = ?, imported_count = ?, matched_count = ?, skipped_count = ?, failed_count = ?, finished_at = ?
WHERE id = ?;

-- name: InsertImportTransactionLink :exec
INSERT INTO import_transaction_links (id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetImportTransactionLinkByExternalKey :one
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE source_id = ? AND external_account_id = ? AND external_transaction_id = ?;

-- name: ListImportTransactionLinksByTransaction :many
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE transaction_id = ?
ORDER BY imported_at, id;
```

`query/pgsql/imports.sql`: identical with `$1..$N` placeholders (the pgsql convention in this repo puts the closing `;` on its own line — that is fine for pgsql files; look at `query/pgsql/tags.sql` and copy its style). Note `UpdateImportEventStatus` sets `run_id` too so a processed event records the run that consumed it.

Run: `go generate ./internal/infra/storage/sqlc/ && go build ./...` — Expected: PASS; `gen/sqlite/imports.sql.go` and `gen/pgsql/imports.sql.go` exist and `sqlitegen.ImportSource` / `pgsqlgen.ImportSource` have identical field types (`diff <(grep -A12 "type ImportSource struct" internal/infra/storage/sqlc/gen/sqlite/models.go) <(grep -A12 "type ImportSource struct" internal/infra/storage/sqlc/gen/pgsql/models.go)` prints nothing). Do the same diff for `ImportEvent`, `ImportRun`, `ImportTransactionLink`. If pgsql emits `int32` for the counts, the migration used `INTEGER` instead of `BIGINT` — fix the migration, not the adapter.

- [ ] **Step 6: Failing repo tests (sources + events)**

`internal/imports/repo/repo_integration_test.go`:

```go
package repo_test

import (
	"context"
	"testing"
	"time"

	importsrepo "github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usdID = "dffc2a06-6f29-4704-8575-31709adee926" // seeded by the baseline migration
	userA = "0a000000-0000-0000-0000-000000000001"
	acct1 = "0a000000-0000-0000-0000-0000000000a1"
)

var fixedTime = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*importsrepo.Repo, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: userA, Email: "a@example.test", Name: "A"})
	f.Account(fixture.Account{ID: acct1, UserID: userA, CurrencyID: usdID, Name: "Card"})
	return importsrepo.NewRepo(db.Engine, db.TX), db
}

func newSource(id string) *model.ImportSource {
	return &model.ImportSource{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Provider: model.ImportProviderAppleWallet,
		Name: "iPhone", Status: model.ImportSourceStatusActive, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
}

func TestRepo_SourceRoundTrip(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatalf("InsertSource: %v", err)
	}
	got, err := repo.GetSource(ctx, src.ID)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got.Provider != model.ImportProviderAppleWallet || got.Name != "iPhone" || got.CredentialCiphertext != nil || got.LastSyncedAt != nil || !got.CreatedAt.Equal(fixedTime) {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if _, err := repo.GetSource(ctx, vo.NewId()); err == nil {
		t.Fatal("GetSource(miss) must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("GetSource(miss) = %T, want NotFound", err)
	}
}

func TestRepo_EventInboxDedupes(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	ev := &model.ImportEvent{
		ID: vo.NewId(), SourceID: src.ID, Payload: `{"amount":"4.50"}`, PayloadHash: "h1",
		Status: model.ImportEventStatusProcessed, ReceivedAt: fixedTime,
	}
	if inserted, err := repo.InsertEvent(ctx, ev); err != nil || !inserted {
		t.Fatalf("first InsertEvent = %v, %v; want inserted", inserted, err)
	}
	dup := &model.ImportEvent{
		ID: vo.NewId(), SourceID: src.ID, Payload: `{"amount":"4.50"}`, PayloadHash: "h1",
		Status: model.ImportEventStatusProcessed, ReceivedAt: fixedTime.Add(time.Minute),
	}
	if inserted, err := repo.InsertEvent(ctx, dup); err != nil || inserted {
		t.Fatalf("duplicate InsertEvent = %v, %v; want not inserted, no error", inserted, err)
	}
	// Same hash on ANOTHER source is a different inbox.
	src2 := newSource("0c000000-0000-0000-0000-000000000002")
	if err := repo.InsertSource(ctx, src2); err != nil {
		t.Fatal(err)
	}
	other := &model.ImportEvent{ID: vo.NewId(), SourceID: src2.ID, Payload: "x", PayloadHash: "h1",
		Status: model.ImportEventStatusProcessed, ReceivedAt: fixedTime}
	if inserted, err := repo.InsertEvent(ctx, other); err != nil || !inserted {
		t.Fatalf("other-source InsertEvent = %v, %v; want inserted", inserted, err)
	}

	msg := "bad json"
	if err := repo.UpdateEventStatus(ctx, ev.ID, model.ImportEventStatusFailed, &msg); err != nil {
		t.Fatalf("UpdateEventStatus: %v", err)
	}
	got, err := repo.GetEvent(ctx, ev.ID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.Status != model.ImportEventStatusFailed || got.ParseError == nil || *got.ParseError != msg || got.RunID != nil {
		t.Errorf("event after update: %+v", got)
	}
}

func TestRepo_SourceDeleteCascadesEvents(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	ev := &model.ImportEvent{ID: vo.NewId(), SourceID: src.ID, Payload: "p", PayloadHash: "h",
		Status: model.ImportEventStatusProcessed, ReceivedAt: fixedTime}
	if _, err := repo.InsertEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Raw.ExecContext(ctx, db.Rebind("DELETE FROM import_sources WHERE id = ?"), src.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetEvent(ctx, ev.ID); err == nil {
		t.Fatal("event must cascade with its source")
	}
}
```

Run: `go test ./internal/imports/repo/` — Expected: FAIL to compile (`importsrepo` missing).

- [ ] **Step 7: Repo implementation (sources + events; runs + links stubs)**

`internal/imports/repo/repo.go`:

```go
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	sourceRow           = sqlitegen.ImportSource
	eventRow            = sqlitegen.ImportEvent
	runRow              = sqlitegen.ImportRun
	linkRow             = sqlitegen.ImportTransactionLink
	insertSourceParams  = sqlitegen.InsertImportSourceParams
	insertEventParams   = sqlitegen.InsertImportEventParams
	updateEventParams   = sqlitegen.UpdateImportEventStatusParams
	insertRunParams     = sqlitegen.InsertImportRunParams
	updateRunParams     = sqlitegen.UpdateImportRunParams
	insertLinkParams    = sqlitegen.InsertImportTransactionLinkParams
	linkByExternalKeyPs = sqlitegen.GetImportTransactionLinkByExternalKeyParams
)

type querier interface {
	InsertImportSource(ctx context.Context, db backend.DBTX, p insertSourceParams) error
	GetImportSourceByID(ctx context.Context, db backend.DBTX, id string) (sourceRow, error)
	InsertImportEvent(ctx context.Context, db backend.DBTX, p insertEventParams) (int64, error)
	GetImportEventByID(ctx context.Context, db backend.DBTX, id string) (eventRow, error)
	UpdateImportEventStatus(ctx context.Context, db backend.DBTX, p updateEventParams) error
	InsertImportRun(ctx context.Context, db backend.DBTX, p insertRunParams) error
	GetImportRunByID(ctx context.Context, db backend.DBTX, id string) (runRow, error)
	UpdateImportRun(ctx context.Context, db backend.DBTX, p updateRunParams) error
	InsertImportTransactionLink(ctx context.Context, db backend.DBTX, p insertLinkParams) error
	GetImportTransactionLinkByExternalKey(ctx context.Context, db backend.DBTX, p linkByExternalKeyPs) (linkRow, error)
	ListImportTransactionLinksByTransaction(ctx context.Context, db backend.DBTX, transactionID string) ([]linkRow, error)
}

type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ imports.Repository = (*Repo)(nil)

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{tx: tx, q: sqliteQuerier{}}
	case "postgresql":
		return &Repo{tx: tx, q: pgsqlQuerier{}}
	default:
		panic("importsrepo: unknown database driver " + driver)
	}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) InsertSource(ctx context.Context, s *model.ImportSource) error {
	return r.q.InsertImportSource(ctx, r.db(ctx), insertSourceParams{
		ID: s.ID.String(), UserID: s.UserID.String(), Provider: s.Provider, Name: s.Name,
		CredentialCiphertext: s.CredentialCiphertext, Status: s.Status, LastSyncedAt: s.LastSyncedAt,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	})
}

func (r *Repo) GetSource(ctx context.Context, id vo.Id) (*model.ImportSource, error) {
	row, err := r.q.GetImportSourceByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import source not found")
		}
		return nil, err
	}
	return sourceFromRow(row)
}

func sourceFromRow(row sourceRow) (*model.ImportSource, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.ImportSource{
		ID: id, UserID: userID, Provider: row.Provider, Name: row.Name,
		CredentialCiphertext: row.CredentialCiphertext, Status: row.Status, LastSyncedAt: row.LastSyncedAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *Repo) InsertEvent(ctx context.Context, e *model.ImportEvent) (bool, error) {
	n, err := r.q.InsertImportEvent(ctx, r.db(ctx), insertEventParams{
		ID: e.ID.String(), SourceID: e.SourceID.String(), RunID: optionalID(e.RunID),
		Payload: e.Payload, PayloadHash: e.PayloadHash, Status: e.Status, ParseError: e.ParseError,
		ReceivedAt: e.ReceivedAt,
	})
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func (r *Repo) GetEvent(ctx context.Context, id vo.Id) (*model.ImportEvent, error) {
	row, err := r.q.GetImportEventByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import event not found")
		}
		return nil, err
	}
	return eventFromRow(row)
}

func (r *Repo) UpdateEventStatus(ctx context.Context, id vo.Id, status string, parseError *string) error {
	// run_id is left as-is by passing the stored value back: read first so a
	// status update never detaches an event from its run.
	cur, err := r.q.GetImportEventByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewNotFound("Import event not found")
		}
		return err
	}
	return r.q.UpdateImportEventStatus(ctx, r.db(ctx), updateEventParams{
		Status: status, ParseError: parseError, RunID: cur.RunID, ID: id.String(),
	})
}

func eventFromRow(row eventRow) (*model.ImportEvent, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	sourceID, err := vo.ParseId(row.SourceID)
	if err != nil {
		return nil, err
	}
	runID, err := parseOptionalID(row.RunID)
	if err != nil {
		return nil, err
	}
	return &model.ImportEvent{
		ID: id, SourceID: sourceID, RunID: runID, Payload: row.Payload, PayloadHash: row.PayloadHash,
		Status: row.Status, ParseError: row.ParseError, ReceivedAt: row.ReceivedAt,
	}, nil
}

func optionalID(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func parseOptionalID(s *string) (*vo.Id, error) {
	if s == nil {
		return nil, nil
	}
	id, err := vo.ParseId(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
```

Add the temporary stubs for `InsertRun`, `GetRun`, `UpdateRun`, `InsertLink`, `GetLinkByExternalKey`, `ListLinksByTransaction` at the bottom, each `return ..., errors.New("importsrepo: not implemented")` (Task 5 deletes them).

`internal/imports/repo/sqlite.go` — a `sqliteQuerier struct{}` with `var _ querier = sqliteQuerier{}` and one passthrough per interface method: `return sqlitegen.New(db).InsertImportSource(ctx, p)` etc. (exactly the shape of `internal/tag/repo/sqlite.go`).

`internal/imports/repo/pgsql.go` — a `pgsqlQuerier struct{}` converting whole structs: `pgsqlgen.New(db).InsertImportSource(ctx, pgsqlgen.InsertImportSourceParams(p))`, `s, err := ...GetImportSourceByID(ctx, id); return sourceRow(s), err`, list methods loop-convert `linkRow(row)` (exactly the shape of `internal/tag/repo/pgsql.go`). If a struct conversion fails to compile, the two generated structs differ — fix the migration/query types (BIGINT vs INTEGER, etc.), never hand-map fields.

- [ ] **Step 8: Run repo tests on both engines**

Run: `go test ./internal/imports/... && make test-repo-pgsql` (or `DBTEST_ENGINE=pgsql go test ./internal/imports/repo/` with `DATABASE_TEST_PGSQL_URL` set)
Expected: PASS on both. The `ON CONFLICT … DO NOTHING` row count must be 0 for the duplicate on BOTH engines.

- [ ] **Step 9: Fixture builders**

`internal/test/fixture/imports.go`:

```go
package fixture

import "time"

// ImportSource seeds one import_sources row; Provider defaults to
// "apple-wallet", Status to "active".
type ImportSource struct {
	ID       string
	UserID   string
	Provider string
	Name     string
	Status   string
}

func (b *Builder) ImportSource(s ImportSource) string {
	b.t.Helper()
	id := b.orNewID(s.ID)
	provider := s.Provider
	if provider == "" {
		provider = "apple-wallet"
	}
	status := s.Status
	if status == "" {
		status = "active"
	}
	now := b.now()
	b.insert(`INSERT INTO import_sources (id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, NULL, ?, NULL, ?, ?)`,
		id, s.UserID, provider, s.Name, status, now, now)
	return id
}

// ImportTransactionLink seeds one ledger row. TransactionID "" -> NULL (a
// tombstone when Status is "linked"); Status defaults to "linked".
type ImportTransactionLink struct {
	ID                    string
	SourceID              string
	ExternalAccountID     string
	ExternalTransactionID string
	TransactionID         string
	Status                string
	ExternalPayee         string
	ExternalDescription   string
	ExternalAmount        string // decimal text, e.g. "12.50000000"
	ExternalPostedAt      time.Time
}

func (b *Builder) ImportTransactionLink(l ImportTransactionLink) string {
	b.t.Helper()
	id := b.orNewID(l.ID)
	status := l.Status
	if status == "" {
		status = "linked"
	}
	posted := l.ExternalPostedAt
	if posted.IsZero() {
		posted = b.now()
	}
	b.insert(`INSERT INTO import_transaction_links (id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at)
		VALUES (?, ?, NULL, NULL, ?, ?, ?, ?, ?, ?, ?, NULL, ?, NULL, NULL, NULL, NULL, ?)`,
		id, l.SourceID, l.ExternalAccountID, l.ExternalTransactionID, nullable(l.TransactionID), status,
		l.ExternalPayee, l.ExternalDescription, l.ExternalAmount, posted, b.now())
	return id
}
```

Check `b.now()`, `b.orNewID`, `b.insert`, `nullable` exist with these names in `internal/test/fixture/entities.go` (they do at the time of writing) — the file lives in the same package so they are directly reachable.

Run: `go build ./... && go vet ./internal/test/fixture/` — Expected: PASS.

- [ ] **Step 10: Commit**

```bash
gofmt -l ./internal ./cmd; go vet ./...
git add internal/model/imports.go internal/imports internal/infra/storage/sqlc internal/test/fixture/imports.go
git commit -m "feat(import): imports model, sources + event inbox repo, fixtures"
```

### Task 5: Link ledger + runs repo, tombstone and cascade semantics

**Files:**
- Modify: `internal/imports/repo/repo.go` (replace the Task 4 stubs), `internal/imports/repo/repo_integration_test.go`
- Test: `internal/imports/repo/repo_integration_test.go`, `internal/model/imports_test.go` (new)

**Interfaces:**
- Consumes: `imports.Repository` (Task 4), fixture builders (Task 4).
- Produces: working `InsertRun`, `GetRun`, `UpdateRun`, `InsertLink`, `GetLinkByExternalKey`, `ListLinksByTransaction`.

- [ ] **Step 1: Failing model tests for seen/tombstone**

`internal/model/imports_test.go`:

```go
package model_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestImportTransactionLink_SeenAndTombstone(t *testing.T) {
	tx := vo.NewId()
	cases := []struct {
		name      string
		link      model.ImportTransactionLink
		seen      bool
		tombstone bool
	}{
		{"linked live", model.ImportTransactionLink{Status: model.ImportLinkStatusLinked, TransactionID: &tx}, true, false},
		{"linked deleted", model.ImportTransactionLink{Status: model.ImportLinkStatusLinked}, true, true},
		{"skipped", model.ImportTransactionLink{Status: model.ImportLinkStatusSkipped}, true, false},
		{"queued", model.ImportTransactionLink{Status: model.ImportLinkStatusQueued}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.link.IsSeen() != tc.seen {
				t.Errorf("IsSeen = %v, want %v", tc.link.IsSeen(), tc.seen)
			}
			if tc.link.IsTombstone() != tc.tombstone {
				t.Errorf("IsTombstone = %v, want %v", tc.link.IsTombstone(), tc.tombstone)
			}
		})
	}
}
```

Run: `go test ./internal/model/ -run TestImportTransactionLink` — Expected: PASS already (methods landed in Task 4); keep the test — it pins the seen/queued rule the matcher depends on.

- [ ] **Step 2: Failing repo tests for runs and links**

Append to `internal/imports/repo/repo_integration_test.go`:

```go
func newLink(srcID string, extTx string, txID *vo.Id, status string) *model.ImportTransactionLink {
	return &model.ImportTransactionLink{
		ID: vo.NewId(), SourceID: vo.MustParseId(srcID), ExternalAccountID: "card-1", ExternalTransactionID: extTx,
		TransactionID: txID, Status: status, ExternalPayee: "COFFEE SHOP", ExternalDescription: "coffee",
		ExternalAmount: "4.50000000", ExternalPostedAt: fixedTime, ImportedAt: fixedTime,
	}
}

func TestRepo_RunRoundTrip(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	run := &model.ImportRun{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), SourceID: src.ID, Provider: model.ImportProviderAppleWallet,
		Params: "{}", Status: model.ImportRunStatusRunning, StartedAt: fixedTime,
	}
	if err := repo.InsertRun(ctx, run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	fin := fixedTime.Add(2 * time.Second)
	run.Status, run.ImportedCount, run.MatchedCount, run.SkippedCount, run.FailedCount, run.FinishedAt = model.ImportRunStatusCompleted, 3, 1, 2, 0, &fin
	if err := repo.UpdateRun(ctx, run); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	got, err := repo.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != model.ImportRunStatusCompleted || got.ImportedCount != 3 || got.MatchedCount != 1 || got.SkippedCount != 2 || got.FailedCount != 0 ||
		got.FinishedAt == nil || !got.FinishedAt.Equal(fin) || got.Params != "{}" {
		t.Errorf("run after update: %+v", got)
	}
	if _, err := repo.GetRun(ctx, vo.NewId()); err == nil {
		t.Fatal("GetRun(miss) must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("GetRun(miss) = %T, want NotFound", err)
	}
}

func TestRepo_LinkLedger(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "iPhone"})
	txID := vo.MustParseId(f.Transaction(fixture.Transaction{UserID: userA, AccountID: acct1, Type: 0, Amount: "4.50000000", Description: "coffee"}))

	l := newLink(srcID, "ext-1", &txID, model.ImportLinkStatusLinked)
	if err := repo.InsertLink(ctx, l); err != nil {
		t.Fatalf("InsertLink: %v", err)
	}
	// Same external key twice is a ledger bug, not an upsert.
	if err := repo.InsertLink(ctx, newLink(srcID, "ext-1", &txID, model.ImportLinkStatusLinked)); err == nil {
		t.Fatal("duplicate external key must fail")
	}

	got, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-1")
	if err != nil {
		t.Fatalf("GetLinkByExternalKey: %v", err)
	}
	if got.TransactionID == nil || !got.TransactionID.Equal(txID) || got.ExternalAmount != "4.5" || !got.ExternalPostedAt.Equal(fixedTime) || !got.IsSeen() {
		t.Errorf("link mismatch: %+v", got)
	}
	if _, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-nope"); err == nil {
		t.Fatal("miss must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("miss = %T, want NotFound", err)
	}

	byTx, err := repo.ListLinksByTransaction(ctx, txID)
	if err != nil || len(byTx) != 1 || !byTx[0].ID.Equal(l.ID) {
		t.Fatalf("ListLinksByTransaction = %+v, %v; want the one link", byTx, err)
	}
}

// The regression this feature exists to prevent: deleting an imported
// transaction must NOT make the next sync re-create it. The FK is SET NULL,
// so the ledger row survives as a tombstone that still counts as seen.
func TestRepo_DeletedTransactionLeavesTombstone(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "iPhone"})
	txID := vo.MustParseId(f.Transaction(fixture.Transaction{UserID: userA, AccountID: acct1, Type: 0, Amount: "4.50000000", Description: "coffee"}))
	if err := repo.InsertLink(ctx, newLink(srcID, "ext-1", &txID, model.ImportLinkStatusLinked)); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Raw.ExecContext(ctx, db.Rebind("DELETE FROM transactions WHERE id = ?"), txID.String()); err != nil {
		t.Fatalf("delete transaction: %v", err)
	}

	got, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-1")
	if err != nil {
		t.Fatalf("ledger row must survive the transaction delete: %v", err)
	}
	if got.TransactionID != nil || got.Status != model.ImportLinkStatusLinked || !got.IsTombstone() || !got.IsSeen() {
		t.Errorf("tombstone mismatch: %+v", got)
	}
	if rows, err := repo.ListLinksByTransaction(ctx, txID); err != nil || len(rows) != 0 {
		t.Errorf("ListLinksByTransaction(deleted) = %d rows, %v; want none", len(rows), err)
	}
}

func TestRepo_QueuedLinkIsNotSeen(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "iPhone"})
	if err := repo.InsertLink(ctx, newLink(srcID, "ext-q", nil, model.ImportLinkStatusQueued)); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-q")
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSeen() {
		t.Error("a queued link must not count as seen")
	}
}

func TestRepo_SourceDeleteCascadesRunsAndLinks(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "iPhone"})
	run := &model.ImportRun{ID: vo.NewId(), UserID: vo.MustParseId(userA), SourceID: vo.MustParseId(srcID),
		Provider: model.ImportProviderAppleWallet, Params: "{}", Status: model.ImportRunStatusRunning, StartedAt: fixedTime}
	if err := repo.InsertRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	l := newLink(srcID, "ext-1", nil, model.ImportLinkStatusSkipped)
	l.RunID = &run.ID
	if err := repo.InsertLink(ctx, l); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Raw.ExecContext(ctx, db.Rebind("DELETE FROM import_sources WHERE id = ?"), srcID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetRun(ctx, run.ID); err == nil {
		t.Error("run must cascade with its source")
	}
	if _, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-1"); err == nil {
		t.Error("link must cascade with its source")
	}
}

// Deleting a run must not delete its ledger rows (SET NULL), or the history
// of what was imported would vanish with a cleanup.
func TestRepo_RunDeleteDetachesLinks(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "iPhone"})
	run := &model.ImportRun{ID: vo.NewId(), UserID: vo.MustParseId(userA), SourceID: vo.MustParseId(srcID),
		Provider: model.ImportProviderAppleWallet, Params: "{}", Status: model.ImportRunStatusCompleted, StartedAt: fixedTime}
	if err := repo.InsertRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	l := newLink(srcID, "ext-1", nil, model.ImportLinkStatusSkipped)
	l.RunID = &run.ID
	if err := repo.InsertLink(ctx, l); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Raw.ExecContext(ctx, db.Rebind("DELETE FROM import_runs WHERE id = ?"), run.ID.String()); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-1")
	if err != nil {
		t.Fatalf("link must survive run delete: %v", err)
	}
	if got.RunID != nil {
		t.Errorf("run_id must be NULL after run delete, got %v", got.RunID)
	}
}
```

Run: `go test ./internal/imports/repo/` — Expected: FAIL with `not implemented` on the run/link tests.

- [ ] **Step 3: Implement runs and links**

Replace the stubs in `internal/imports/repo/repo.go`:

```go
func (r *Repo) InsertRun(ctx context.Context, run *model.ImportRun) error {
	return r.q.InsertImportRun(ctx, r.db(ctx), insertRunParams{
		ID: run.ID.String(), UserID: run.UserID.String(), SourceID: run.SourceID.String(), Provider: run.Provider,
		Params: run.Params, Status: run.Status,
		ImportedCount: int64(run.ImportedCount), MatchedCount: int64(run.MatchedCount),
		SkippedCount: int64(run.SkippedCount), FailedCount: int64(run.FailedCount),
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	})
}

func (r *Repo) GetRun(ctx context.Context, id vo.Id) (*model.ImportRun, error) {
	row, err := r.q.GetImportRunByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import run not found")
		}
		return nil, err
	}
	return runFromRow(row)
}

func (r *Repo) UpdateRun(ctx context.Context, run *model.ImportRun) error {
	return r.q.UpdateImportRun(ctx, r.db(ctx), updateRunParams{
		Status: run.Status,
		ImportedCount: int64(run.ImportedCount), MatchedCount: int64(run.MatchedCount),
		SkippedCount: int64(run.SkippedCount), FailedCount: int64(run.FailedCount),
		FinishedAt: run.FinishedAt, ID: run.ID.String(),
	})
}

func runFromRow(row runRow) (*model.ImportRun, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	sourceID, err := vo.ParseId(row.SourceID)
	if err != nil {
		return nil, err
	}
	return &model.ImportRun{
		ID: id, UserID: userID, SourceID: sourceID, Provider: row.Provider, Params: row.Params, Status: row.Status,
		ImportedCount: int(row.ImportedCount), MatchedCount: int(row.MatchedCount),
		SkippedCount: int(row.SkippedCount), FailedCount: int(row.FailedCount),
		StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
	}, nil
}

func (r *Repo) InsertLink(ctx context.Context, l *model.ImportTransactionLink) error {
	return r.q.InsertImportTransactionLink(ctx, r.db(ctx), insertLinkParams{
		ID: l.ID.String(), SourceID: l.SourceID.String(), RunID: optionalID(l.RunID), EventID: optionalID(l.EventID),
		ExternalAccountID: l.ExternalAccountID, ExternalTransactionID: l.ExternalTransactionID,
		TransactionID: optionalID(l.TransactionID), Status: l.Status,
		ExternalPayee: l.ExternalPayee, ExternalDescription: l.ExternalDescription,
		ExternalAmount: l.ExternalAmount, ExternalCurrency: l.ExternalCurrency, ExternalPostedAt: l.ExternalPostedAt,
		AppliedCategoryID: optionalID(l.AppliedCategoryID), AppliedPayeeID: optionalID(l.AppliedPayeeID),
		AppliedTagID: optionalID(l.AppliedTagID), AppliedRuleID: optionalID(l.AppliedRuleID),
		ImportedAt: l.ImportedAt,
	})
}

func (r *Repo) GetLinkByExternalKey(ctx context.Context, sourceID vo.Id, externalAccountID, externalTransactionID string) (*model.ImportTransactionLink, error) {
	row, err := r.q.GetImportTransactionLinkByExternalKey(ctx, r.db(ctx), linkByExternalKeyPs{
		SourceID: sourceID.String(), ExternalAccountID: externalAccountID, ExternalTransactionID: externalTransactionID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import link not found")
		}
		return nil, err
	}
	return linkFromRow(row)
}

func (r *Repo) ListLinksByTransaction(ctx context.Context, transactionID vo.Id) ([]model.ImportTransactionLink, error) {
	rows, err := r.q.ListImportTransactionLinksByTransaction(ctx, r.db(ctx), transactionID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportTransactionLink, 0, len(rows))
	for _, row := range rows {
		l, err := linkFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

func linkFromRow(row linkRow) (*model.ImportTransactionLink, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	sourceID, err := vo.ParseId(row.SourceID)
	if err != nil {
		return nil, err
	}
	l := &model.ImportTransactionLink{
		ID: id, SourceID: sourceID,
		ExternalAccountID: row.ExternalAccountID, ExternalTransactionID: row.ExternalTransactionID, Status: row.Status,
		ExternalPayee: row.ExternalPayee, ExternalDescription: row.ExternalDescription,
		ExternalAmount: vo.NewDecimal(row.ExternalAmount).String(), ExternalCurrency: row.ExternalCurrency,
		ExternalPostedAt: row.ExternalPostedAt, ImportedAt: row.ImportedAt,
	}
	for _, f := range []struct {
		src *string
		dst **vo.Id
	}{
		{row.RunID, &l.RunID}, {row.EventID, &l.EventID}, {row.TransactionID, &l.TransactionID},
		{row.AppliedCategoryID, &l.AppliedCategoryID}, {row.AppliedPayeeID, &l.AppliedPayeeID},
		{row.AppliedTagID, &l.AppliedTagID}, {row.AppliedRuleID, &l.AppliedRuleID},
	} {
		v, err := parseOptionalID(f.src)
		if err != nil {
			return nil, err
		}
		*f.dst = v
	}
	return l, nil
}
```

`ExternalAmount` goes through `vo.NewDecimal(...).String()` (which trims trailing zeros: `"4.50000000"` → `"4.5"`) because pgsql `NUMERIC` and sqlite text differ in trailing zeros; normalizing on read is what makes the test above pass on both engines. If sqlc emitted `ExternalAmount` as something other than `string` on either engine, look at how `transactions.amount` is typed in `sqlc.yaml` overrides and add the same override for `import_transaction_links.external_amount` (and `import_rules.match_amount_min/max`).

- [ ] **Step 4: Run on both engines**

Run: `go test ./internal/imports/... ./internal/model/ && make test-repo-pgsql`
Expected: PASS on both. If the tombstone test fails on sqlite with the row deleted instead of nulled, `foreign_keys` is on but the FK clause is wrong — re-check `ON DELETE SET NULL` in the migration; if it fails with an FK error, the fixture transaction insert is missing something the `transactions` FK needs (compare `fixture.Transaction`).

- [ ] **Step 5: Commit**

```bash
gofmt -l ./internal ./cmd; go vet ./...
git add internal/imports internal/model/imports_test.go
git commit -m "feat(import): link ledger + runs repo with tombstone semantics"
```

### Task 6: The matcher + `MatcherConfig` + env-configured thresholds

**Files:**
- Create: `internal/imports/matcher.go`, `internal/imports/matcher_test.go`
- Modify: `internal/config/config.go` (four fields + parsing), `internal/config/config_test.go`
- Modify: `CLAUDE.md` (Configuration list), `.env.example` (after the `ECONUMO_CURRENCY_UPDATE_INTERVAL` block, ~line 78)
- Modify: `docs/superpowers/specs/2026-08-15-transaction-import-design.md` ("Matcher configuration": the last sentence of the paragraph after the table)

**Interfaces:**
- Consumes: `model.IngestEvent`, `model.ImportCandidate`, `model.ImportCandidateLink`, `model.ImportProviderIsPush` (Task 4); `vo.NewDecimal`.
- Produces: `imports.MatcherConfig{MatchDays, TipDays, TipTolerancePct, TokenMinLength int}`, `imports.DefaultMatcherConfig() MatcherConfig`, `imports.MatchKind` (`MatchCreate`, `MatchAdopt`, `MatchTipAdopt`), `imports.MatchResult{Kind MatchKind; TransactionID vo.Id; CorrectAmount bool}`, `imports.Match(ev model.IngestEvent, sourceID vo.Id, candidates []model.ImportCandidate, cfg MatcherConfig) MatchResult`; `config.Config.ImportMatchDays/ImportTipDays/ImportTipTolerancePct/ImportTokenMinLength int`.

Contract of `Match` (spec Part 5 stages 2–4; stage 1 "exact" is the caller's `GetLinkByExternalKey` + `IsSeen()` check, not the matcher's job):
- Candidates are the transactions the caller pre-selected for the same Econumo account and a date window; the matcher ignores any candidate whose `Type` differs from the event's, and any candidate that already carries a link from `sourceID` (one source links a transaction at most once).
- Stage 2 (adopt): `candidate.Amount == ev.Amount` (decimal equality) and `|day(candidate.SpentAt) - day(ev.PostedAt)| <= MatchDays`. Day granularity: compare `time.Date(y, m, d, 0,0,0,0, UTC)` of each after `.UTC()`.
- Stage 3 (tip adopt): the matcher does not know the event's provider; it checks candidates that have a link whose `Provider` is push (`ImportProviderIsPush`). For each such push link: `0 <= day(ev.PostedAt) - day(candidate.SpentAt) <= TipDays`; `|ev.Amount - link.ExternalAmount| <= link.ExternalAmount * TipTolerancePct / 100`; every token of `link.ExternalPayee` with `len >= TokenMinLength` is contained in the token set of `ev.Payee + " " + ev.Description`; a push payee with NO qualifying token never matches. Best = smallest `|ev.Amount - link.ExternalAmount|`, then smallest day distance, then lowest `TransactionID.String()`. `CorrectAmount = candidate.Amount == link.ExternalAmount` (decimal equality) — the user has not edited it.
- Stage 2 beats stage 3; within stage 2 the best is the nearest date, then lowest `TransactionID.String()`.
- Otherwise `MatchCreate`.

- [ ] **Step 1: Failing matcher tests**

`internal/imports/matcher_test.go`:

```go
package imports_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

var (
	srcBank  = vo.MustParseId("0c000000-0000-0000-0000-00000000000b")
	srcPhone = vo.MustParseId("0c000000-0000-0000-0000-00000000000f")
	txA      = vo.MustParseId("d0000000-0000-0000-0000-00000000000a")
	txB      = vo.MustParseId("d0000000-0000-0000-0000-00000000000b")
	day0     = time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)
)

func day(n int) time.Time { return day0.AddDate(0, 0, n) }

func event(amount string, posted time.Time, payee string) model.IngestEvent {
	return model.IngestEvent{ExternalAccountID: "card", ExternalTransactionID: "x", Type: model.TransactionTypeExpense,
		Amount: amount, Currency: "USD", PostedAt: posted, Payee: payee}
}

func pushLink(amount, payee string) model.ImportCandidateLink {
	return model.ImportCandidateLink{SourceID: srcPhone, Provider: model.ImportProviderAppleWallet, ExternalAmount: amount, ExternalPayee: payee}
}

func TestMatch_NoCandidatesCreates(t *testing.T) {
	got := imports.Match(event("4.5", day(0), "COFFEE"), srcBank, nil, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchCreate {
		t.Fatalf("got %+v, want create", got)
	}
}

func TestMatch_AdoptsSameAmountWithinWindow(t *testing.T) {
	cands := []model.ImportCandidate{
		{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "4.5", SpentAt: day(-3)},
		{TransactionID: txB, Type: model.TransactionTypeExpense, Amount: "4.5", SpentAt: day(-1)},
	}
	got := imports.Match(event("4.50", day(0), "COFFEE"), srcBank, cands, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchAdopt || !got.TransactionID.Equal(txB) || got.CorrectAmount {
		t.Fatalf("got %+v, want adopt txB (nearest date)", got)
	}
}

func TestMatch_AdoptRespectsWindowAndType(t *testing.T) {
	cfg := imports.DefaultMatcherConfig()
	outside := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "4.5", SpentAt: day(-4)}}
	if got := imports.Match(event("4.5", day(0), "COFFEE"), srcBank, outside, cfg); got.Kind != imports.MatchCreate {
		t.Fatalf("4 days away with MatchDays=3: got %+v, want create", got)
	}
	income := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeIncome, Amount: "4.5", SpentAt: day(0)}}
	if got := imports.Match(event("4.5", day(0), "COFFEE"), srcBank, income, cfg); got.Kind != imports.MatchCreate {
		t.Fatalf("income candidate for an expense event: got %+v, want create", got)
	}
	cfg.MatchDays = 0
	sameDay := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "4.5", SpentAt: day(0).Add(-14 * time.Hour)}}
	if got := imports.Match(event("4.5", day(0), "COFFEE"), srcBank, sameDay, cfg); got.Kind != imports.MatchAdopt {
		t.Fatalf("MatchDays=0 must still match the same calendar day: got %+v", got)
	}
}

func TestMatch_SkipsCandidateAlreadyLinkedFromSameSource(t *testing.T) {
	cands := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "4.5", SpentAt: day(0),
		Links: []model.ImportCandidateLink{{SourceID: srcBank, Provider: model.ImportProviderSimpleFIN, ExternalAmount: "4.5"}}}}
	if got := imports.Match(event("4.5", day(0), "COFFEE"), srcBank, cands, imports.DefaultMatcherConfig()); got.Kind != imports.MatchCreate {
		t.Fatalf("got %+v, want create", got)
	}
}

func TestMatch_TipAdopt(t *testing.T) {
	cands := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0),
		Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}}}
	got := imports.Match(event("46", day(2), "SQ *STARBUCKS 12345 SEATTLE WA"), srcBank, cands, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchTipAdopt || !got.TransactionID.Equal(txA) || !got.CorrectAmount {
		t.Fatalf("got %+v, want tip-adopt txA with amount correction", got)
	}
}

func TestMatch_TipAdoptKeepsUserEditedAmount(t *testing.T) {
	// The user changed 40 -> 41 after the tap: link, but do not overwrite.
	cands := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "41", SpentAt: day(0),
		Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}}}
	got := imports.Match(event("46", day(2), "STARBUCKS SEATTLE"), srcBank, cands, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchTipAdopt || got.CorrectAmount {
		t.Fatalf("got %+v, want tip-adopt without correction", got)
	}
}

func TestMatch_TipAdoptBounds(t *testing.T) {
	cfg := imports.DefaultMatcherConfig()
	base := func() []model.ImportCandidate {
		return []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0),
			Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}}}
	}
	cases := []struct {
		name string
		ev   model.IngestEvent
		want imports.MatchKind
	}{
		{"posted before tap", event("46", day(-1), "STARBUCKS"), imports.MatchCreate},
		{"posted on tap day", event("46", day(0), "STARBUCKS"), imports.MatchTipAdopt},
		{"posted at +5 days", event("46", day(5), "STARBUCKS"), imports.MatchTipAdopt},
		{"posted at +6 days", event("46", day(6), "STARBUCKS"), imports.MatchCreate},
		{"amount +20% exactly", event("48", day(1), "STARBUCKS"), imports.MatchTipAdopt},
		{"amount +20.01%", event("48.004", day(1), "STARBUCKS"), imports.MatchCreate},
		{"amount -20% exactly", event("32", day(1), "STARBUCKS"), imports.MatchTipAdopt},
		{"merchant token missing", event("46", day(1), "SQ COFFEE SEATTLE"), imports.MatchCreate},
		{"merchant in description", model.IngestEvent{Type: model.TransactionTypeExpense, Amount: "46", PostedAt: day(1), Payee: "SQ", Description: "starbucks seattle"}, imports.MatchTipAdopt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := imports.Match(tc.ev, srcBank, base(), cfg); got.Kind != tc.want {
				t.Fatalf("got %+v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatch_TipAdoptTokenRules(t *testing.T) {
	cfg := imports.DefaultMatcherConfig()
	// "Sq" is shorter than TokenMinLength, so only "starbucks" must be present.
	cands := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0),
		Links: []model.ImportCandidateLink{pushLink("40", "Sq Starbucks")}}}
	if got := imports.Match(event("46", day(1), "STARBUCKS #1"), srcBank, cands, cfg); got.Kind != imports.MatchTipAdopt {
		t.Fatalf("short tokens must be ignored: got %+v", got)
	}
	// A push payee with no qualifying token can never match: nothing to contain.
	empty := []model.ImportCandidate{{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0),
		Links: []model.ImportCandidateLink{pushLink("40", "7-11")}}}
	if got := imports.Match(event("46", day(1), "7 11 STORE"), srcBank, empty, cfg); got.Kind != imports.MatchCreate {
		t.Fatalf("no qualifying push token: got %+v, want create", got)
	}
	cfg.TokenMinLength = 1
	if got := imports.Match(event("46", day(1), "7 11 STORE"), srcBank, empty, cfg); got.Kind != imports.MatchTipAdopt {
		t.Fatalf("TokenMinLength=1 must accept single-char tokens: got %+v", got)
	}
}

func TestMatch_TipAdoptPicksSmallestAmountDeltaThenNearestDate(t *testing.T) {
	cands := []model.ImportCandidate{
		{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0), Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}},
		{TransactionID: txB, Type: model.TransactionTypeExpense, Amount: "45", SpentAt: day(-3), Links: []model.ImportCandidateLink{pushLink("45", "Starbucks")}},
	}
	got := imports.Match(event("46", day(1), "STARBUCKS"), srcBank, cands, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchTipAdopt || !got.TransactionID.Equal(txB) {
		t.Fatalf("got %+v, want txB (delta 1 beats delta 6)", got)
	}
	tie := []model.ImportCandidate{
		{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(-2), Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}},
		{TransactionID: txB, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0), Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}},
	}
	got = imports.Match(event("46", day(1), "STARBUCKS"), srcBank, tie, imports.DefaultMatcherConfig())
	if !got.TransactionID.Equal(txB) {
		t.Fatalf("got %+v, want txB (nearest date breaks the amount tie)", got)
	}
}

func TestMatch_ExactAmountBeatsTip(t *testing.T) {
	cands := []model.ImportCandidate{
		{TransactionID: txA, Type: model.TransactionTypeExpense, Amount: "40", SpentAt: day(0), Links: []model.ImportCandidateLink{pushLink("40", "Starbucks")}},
		{TransactionID: txB, Type: model.TransactionTypeExpense, Amount: "46", SpentAt: day(1)},
	}
	got := imports.Match(event("46", day(1), "STARBUCKS"), srcBank, cands, imports.DefaultMatcherConfig())
	if got.Kind != imports.MatchAdopt || !got.TransactionID.Equal(txB) {
		t.Fatalf("got %+v, want plain adopt of txB", got)
	}
}
```

Run: `go test ./internal/imports/ -run TestMatch` — Expected: FAIL (undefined `imports.Match`, etc.).

- [ ] **Step 2: Implement the matcher**

`internal/imports/matcher.go`:

```go
package imports

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MatcherConfig holds the four tunable thresholds (ECONUMO_IMPORT_*). They
// are guesses until real bank data has been through them, so they are
// configuration, not constants.
type MatcherConfig struct {
	MatchDays       int // ± window for the same-amount adopt
	TipDays         int // forward window (bank posts after the tap)
	TipTolerancePct int // percent of the tap amount
	TokenMinLength  int // shortest merchant token that must be contained
}

func DefaultMatcherConfig() MatcherConfig {
	return MatcherConfig{MatchDays: 3, TipDays: 5, TipTolerancePct: 20, TokenMinLength: 3}
}

type MatchKind int

const (
	MatchCreate MatchKind = iota
	MatchAdopt
	MatchTipAdopt
)

type MatchResult struct {
	Kind          MatchKind
	TransactionID vo.Id
	// CorrectAmount is set on a tip adopt when the transaction still carries
	// the tap-time amount, so the posted amount may overwrite it.
	CorrectAmount bool
}

// Match decides what an incoming event becomes, given the transactions the
// caller pre-selected on the same account. The exact-key check against the
// ledger happens before this (it needs the DB); Match is pure.
func Match(ev model.IngestEvent, sourceID vo.Id, candidates []model.ImportCandidate, cfg MatcherConfig) MatchResult {
	amount := vo.NewDecimal(ev.Amount)
	evDay := dayOf(ev.PostedAt)

	type scored struct {
		c    model.ImportCandidate
		days int
	}
	var adopt []scored
	for _, c := range candidates {
		if c.Type != ev.Type || linkedFrom(c, sourceID) {
			continue
		}
		if !vo.NewDecimal(c.Amount).Equals(amount) {
			continue
		}
		if d := absInt(daysBetween(dayOf(c.SpentAt), evDay)); d <= cfg.MatchDays {
			adopt = append(adopt, scored{c, d})
		}
	}
	if len(adopt) > 0 {
		sort.Slice(adopt, func(i, j int) bool {
			if adopt[i].days != adopt[j].days {
				return adopt[i].days < adopt[j].days
			}
			return adopt[i].c.TransactionID.String() < adopt[j].c.TransactionID.String()
		})
		return MatchResult{Kind: MatchAdopt, TransactionID: adopt[0].c.TransactionID}
	}

	type tipScored struct {
		c       model.ImportCandidate
		delta   vo.DecimalNumber
		days    int
		correct bool
	}
	var tips []tipScored
	evTokens := tokens(ev.Payee+" "+ev.Description, 1)
	pct := vo.NewDecimal(strconv.Itoa(cfg.TipTolerancePct))
	for _, c := range candidates {
		if c.Type != ev.Type || linkedFrom(c, sourceID) {
			continue
		}
		days := daysBetween(dayOf(c.SpentAt), evDay) // positive = bank posted after the tap
		if days < 0 || days > cfg.TipDays {
			continue
		}
		for _, l := range c.Links {
			if !model.ImportProviderIsPush(l.Provider) {
				continue
			}
			tap := vo.NewDecimal(l.ExternalAmount)
			delta := amount.Sub(tap).Abs()
			if delta.IsGreaterThan(tap.Mul(pct).Div(vo.NewDecimal("100"))) {
				continue
			}
			need := tokens(l.ExternalPayee, cfg.TokenMinLength)
			if len(need) == 0 || !containsAll(evTokens, need) {
				continue
			}
			tips = append(tips, tipScored{c: c, delta: delta, days: days, correct: vo.NewDecimal(c.Amount).Equals(tap)})
		}
	}
	if len(tips) == 0 {
		return MatchResult{Kind: MatchCreate}
	}
	sort.Slice(tips, func(i, j int) bool {
		if !tips[i].delta.Equals(tips[j].delta) {
			return tips[i].delta.IsLessThan(tips[j].delta)
		}
		if tips[i].days != tips[j].days {
			return tips[i].days < tips[j].days
		}
		return tips[i].c.TransactionID.String() < tips[j].c.TransactionID.String()
	})
	best := tips[0]
	return MatchResult{Kind: MatchTipAdopt, TransactionID: best.c.TransactionID, CorrectAmount: best.correct}
}

func linkedFrom(c model.ImportCandidate, sourceID vo.Id) bool {
	for _, l := range c.Links {
		if l.SourceID.Equal(sourceID) {
			return true
		}
	}
	return false
}

func dayOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// daysBetween returns b - a in whole days; both are midnight UTC.
func daysBetween(a, b time.Time) int { return int(b.Sub(a).Hours() / 24) }

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// tokens lowercases, turns every non-ASCII-alphanumeric rune into a space,
// splits, and keeps tokens of at least minLen runes. Deterministic; no regex.
// Non-Latin merchant names therefore yield no tokens and never tip-match —
// they fall through to create, which is the safe side.
func tokens(s string, minLen int) map[string]bool {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	out := map[string]bool{}
	for _, tok := range strings.Fields(b.String()) {
		if len(tok) >= minLen {
			out[tok] = true
		}
	}
	return out
}

func containsAll(have, need map[string]bool) bool {
	for tok := range need {
		if !have[tok] {
			return false
		}
	}
	return true
}
```

Run: `go test ./internal/imports/ -run TestMatch -v` — Expected: PASS. If `"amount +20.01%"` unexpectedly matches, `Div` rounds — check `vo.DecimalNumber.Div`'s scale (8 places): `40 * 20 / 100 = 8` exactly; `48.004 - 40 = 8.004 > 8`, so it must NOT match.

- [ ] **Step 3: Failing config tests**

Append to `internal/config/config_test.go`:

```go
func TestLoad_ImportMatcherDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ImportMatchDays != 3 || cfg.ImportTipDays != 5 || cfg.ImportTipTolerancePct != 20 || cfg.ImportTokenMinLength != 3 {
		t.Errorf("defaults = %d/%d/%d/%d", cfg.ImportMatchDays, cfg.ImportTipDays, cfg.ImportTipTolerancePct, cfg.ImportTokenMinLength)
	}
}

func TestLoad_ImportMatcherBounds(t *testing.T) {
	cases := []struct {
		key, val string
		ok       bool
	}{
		{"ECONUMO_IMPORT_MATCH_DAYS", "0", true},
		{"ECONUMO_IMPORT_MATCH_DAYS", "31", true},
		{"ECONUMO_IMPORT_MATCH_DAYS", "32", false},
		{"ECONUMO_IMPORT_MATCH_DAYS", "-1", false},
		{"ECONUMO_IMPORT_MATCH_DAYS", "three", false},
		{"ECONUMO_IMPORT_TIP_DAYS", "31", true},
		{"ECONUMO_IMPORT_TIP_DAYS", "32", false},
		{"ECONUMO_IMPORT_TIP_TOLERANCE", "100", true},
		{"ECONUMO_IMPORT_TIP_TOLERANCE", "101", false},
		{"ECONUMO_IMPORT_TOKEN_MIN_LENGTH", "1", true},
		{"ECONUMO_IMPORT_TOKEN_MIN_LENGTH", "0", false},
		{"ECONUMO_IMPORT_TOKEN_MIN_LENGTH", "16", true},
		{"ECONUMO_IMPORT_TOKEN_MIN_LENGTH", "17", false},
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.val, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
			t.Setenv(tc.key, tc.val)
			_, err := Load()
			if (err == nil) != tc.ok {
				t.Fatalf("Load() err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}
```

Match the existing test file's package name and how it calls `Load` (look at `TestLoad_CurrencyUpdateInterval` around line 518 and mirror it exactly).

Run: `go test ./internal/config/ -run TestLoad_ImportMatcher` — Expected: FAIL (fields undefined).

- [ ] **Step 4: Parse the four variables**

In `internal/config/config.go`, add to `Config` after `CurrencyUpdateIntervalDays`:

```go
	ImportMatchDays       int // ECONUMO_IMPORT_MATCH_DAYS: ± window for the same-amount import adopt (default 3, 0-31)
	ImportTipDays         int // ECONUMO_IMPORT_TIP_DAYS: how many days after a tap a bank record may post (default 5, 0-31)
	ImportTipTolerancePct int // ECONUMO_IMPORT_TIP_TOLERANCE: percent of the tap amount a posted amount may differ by (default 20, 0-100)
	ImportTokenMinLength  int // ECONUMO_IMPORT_TOKEN_MIN_LENGTH: shortest merchant token that must be contained (default 3, 1-16)
```

and after the `c.CurrencyUpdateIntervalDays = interval` line:

```go
	// Matcher thresholds are guesses until real bank data has been through
	// them, so they are tunable — but a typo must fail at boot, not silently
	// change what counts as a duplicate.
	for _, p := range []struct {
		key      string
		def      int
		min, max int
		dst      *int
	}{
		{"ECONUMO_IMPORT_MATCH_DAYS", 3, 0, 31, &c.ImportMatchDays},
		{"ECONUMO_IMPORT_TIP_DAYS", 5, 0, 31, &c.ImportTipDays},
		{"ECONUMO_IMPORT_TIP_TOLERANCE", 20, 0, 100, &c.ImportTipTolerancePct},
		{"ECONUMO_IMPORT_TOKEN_MIN_LENGTH", 3, 1, 16, &c.ImportTokenMinLength},
	} {
		n, err := getIntStrict(p.key, p.def)
		if err != nil {
			return Config{}, err
		}
		if n < p.min || n > p.max {
			return Config{}, fmt.Errorf("%s %d is out of range (%d-%d)", p.key, n, p.min, p.max)
		}
		*p.dst = n
	}
```

Run: `go test ./internal/config/` — Expected: PASS.

- [ ] **Step 5: Document**

`.env.example`, after the currency-update block:

```
# Transaction-import matcher thresholds (docs/superpowers/specs/2026-08-15-transaction-import-design.md, Part 5).
# Malformed or out-of-range values fail at boot.
#ECONUMO_IMPORT_MATCH_DAYS=3          # ± days for the same-amount adopt (0-31)
#ECONUMO_IMPORT_TIP_DAYS=5            # days a bank record may post after a phone tap (0-31)
#ECONUMO_IMPORT_TIP_TOLERANCE=20      # percent the posted amount may differ from the tap amount (0-100)
#ECONUMO_IMPORT_TOKEN_MIN_LENGTH=3    # shortest merchant token that must be contained (1-16)
```

`CLAUDE.md` Configuration list, after the `ECONUMO_CURRENCY_UPDATE_INTERVAL` bullet:

```
- `ECONUMO_IMPORT_MATCH_DAYS` / `ECONUMO_IMPORT_TIP_DAYS` / `ECONUMO_IMPORT_TIP_TOLERANCE` /
  `ECONUMO_IMPORT_TOKEN_MIN_LENGTH` — transaction-import matcher thresholds (defaults 3 / 5 / 20 / 3;
  ranges 0–31 days, 0–31 days, 0–100 percent, 1–16 chars). Strict parse: malformed or out-of-range
  fails at boot. Read into `imports.MatcherConfig`; the matcher itself is a pure function of
  `(event, candidates, config)`.
```

Spec amendment (`docs/superpowers/specs/2026-08-15-transaction-import-design.md`, "Matcher configuration"): replace `Each one is documented in CLAUDE.md's Configuration list and .env.example in the PR that introduces it (stage 1 for the first two, stage 2 for the tip-matching pair).` with `All four are documented in CLAUDE.md's Configuration list and .env.example in stage 1, which implements and tests every matcher stage; nothing consumes MatcherConfig at composition time until stage 2 wires the first provider.`

- [ ] **Step 6: Commit**

```bash
gofmt -l ./internal ./cmd; go vet ./...
git add internal/imports internal/config CLAUDE.md .env.example docs/superpowers/specs/2026-08-15-transaction-import-design.md
git commit -m "feat(import): matcher with env-configured thresholds"
```

### Task 7: `isImported` on the transaction list

**Files:**
- Modify: `internal/transaction/repository.go` (interface, after `LabelsByTransactionIDs`)
- Modify: `internal/transaction/repo/repo.go` (after `labelsByTransactionIDsChunk`)
- Test: `internal/transaction/repo/repo_integration_test.go` (append)
- Modify: `internal/transaction/import_ownership_test.go` (`stubImportRepo` gains the method)
- Modify: `internal/model/transaction_dto.go:18-37` (`TransactionResult`)
- Modify: `internal/transaction/usecase.go` (`buildResult`, `toResult`), `internal/transaction/read.go` (list loop)
- Modify: `internal/test/apiparity/fixture.go` (seed one import link on `Txn2`), goldens under `internal/test/apiparity/testdata/golden/`
- Modify: `web/src/api/dto/transaction.ts` + every object literal the compiler flags
- Modify: `docs/superpowers/specs/2026-08-15-transaction-import-design.md` (Part 4 + "One deliberate exception" paragraph + the Future-work bullet)
- Modify: `docs/regression-test-plan.md` (section 5)

**Interfaces:**
- Consumes: `fixture.Builder.ImportSource`, `fixture.Builder.ImportTransactionLink` (Task 4); `placeholders(driver, start, n)` and `r.db(ctx)` already in `internal/transaction/repo/repo.go`.
- Produces: `Repository.ImportedTransactionIDs(ctx, ids []vo.Id) (map[string]bool, error)`; `model.TransactionResult.IsImported int` (`json:"isImported"`, 0/1); `buildResult(t, author, labelIDs, imported bool)`.

Design note (deviation from the spec, recorded in Step 6): the spec's Part 4 says "an `EXISTS` subquery in the list query". The list is served by two sqlc query files AND the hand-built `ListByAccountIDs` — three places to thread a subquery through, versus one batch method that mirrors `LabelsByTransactionIDs` exactly (chunked `IN`, engine placeholders). The batch method also keeps the cross-feature table reference in ONE greppable spot. The observable wire result is identical.

- [ ] **Step 1: Failing repo test**

Append to `internal/transaction/repo/repo_integration_test.go`:

```go
func TestTransactionRepo_ImportedTransactionIDs(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	const tx1, tx2, tx3 = "d0000000-0000-0000-0000-0000000000d1", "d0000000-0000-0000-0000-0000000000d2", "d0000000-0000-0000-0000-0000000000d3"
	for _, id := range []string{tx1, tx2, tx3} {
		if err := repo.Save(ctx, expense(id, acct1, "1.00000000", fixedTime)); err != nil {
			t.Fatal(err)
		}
	}
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "Bank"})
	// tx1 has two links (two sources would each link it once in real life; here
	// two external keys from one source is enough to prove no duplication).
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: srcID, ExternalAccountID: "acc", ExternalTransactionID: "e1", TransactionID: tx1, ExternalAmount: "1.00000000"})
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: srcID, ExternalAccountID: "acc", ExternalTransactionID: "e2", TransactionID: tx1, ExternalAmount: "1.00000000"})
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: srcID, ExternalAccountID: "acc", ExternalTransactionID: "e3", TransactionID: tx2, ExternalAmount: "1.00000000"})
	// A tombstone (transaction_id NULL) must not mark anything.
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: srcID, ExternalAccountID: "acc", ExternalTransactionID: "e4", ExternalAmount: "1.00000000"})

	got, err := repo.ImportedTransactionIDs(ctx, []vo.Id{vo.MustParseId(tx1), vo.MustParseId(tx2), vo.MustParseId(tx3)})
	if err != nil {
		t.Fatalf("ImportedTransactionIDs: %v", err)
	}
	if !got[tx1] || !got[tx2] || got[tx3] || len(got) != 2 {
		t.Fatalf("got %v, want {tx1, tx2}", got)
	}
	empty, err := repo.ImportedTransactionIDs(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty ids: %v, %v", empty, err)
	}
}
```

Run: `go test ./internal/transaction/repo/ -run TestTransactionRepo_ImportedTransactionIDs` — Expected: FAIL (method undefined).

- [ ] **Step 2: Implement the batch lookup**

`internal/transaction/repository.go`, after `LabelsByTransactionIDs`:

```go
	// ImportedTransactionIDs reports which of the given transactions carry at
	// least one live import link (the list endpoint's isImported flag).
	ImportedTransactionIDs(ctx context.Context, ids []vo.Id) (map[string]bool, error)
```

`internal/transaction/repo/repo.go`, after `labelsByTransactionIDsChunk`:

```go
// ImportedTransactionIDs reads import_transaction_links, a table the imports
// feature owns. This is the one place the transaction feature touches it —
// SQL, not a Go import, so archtest cannot see the coupling; keep it here and
// nowhere else. Same chunking + empty-IN guard as LabelsByTransactionIDs.
func (r *Repo) ImportedTransactionIDs(ctx context.Context, ids []vo.Id) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	for len(ids) > 0 {
		n := len(ids)
		if n > labelsByTransactionIDsChunkSize {
			n = labelsByTransactionIDsChunkSize
		}
		chunk := ids[:n]
		ids = ids[n:]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id.String()
		}
		query := "SELECT DISTINCT transaction_id FROM import_transaction_links WHERE transaction_id IN (" +
			placeholders(r.driver, 1, len(args)) + ")"
		rows, err := r.db(ctx).QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var txID string
			if err := rows.Scan(&txID); err != nil {
				rows.Close()
				return nil, err
			}
			out[txID] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}
```

`internal/transaction/import_ownership_test.go`, next to the other `stubImportRepo` methods:

```go
func (r *stubImportRepo) ImportedTransactionIDs(ctx context.Context, ids []vo.Id) (map[string]bool, error) {
	return nil, nil
}
```

Run: `go build ./... && go test ./internal/transaction/repo/ -run TestTransactionRepo_ImportedTransactionIDs` — Expected: PASS. If `go build` reports another type failing to satisfy `transaction.Repository`, add the same stub method there.

- [ ] **Step 3: Thread the flag to the wire**

`internal/model/transaction_dto.go`, in `TransactionResult` after `LabelIds`:

```go
	// IsImported is 1 when at least one import link (any provider) points at
	// this transaction — int 0/1 like isArchived. Full provenance is a
	// separate per-transaction read (stage 2), so the list carries only the flag.
	IsImported int `json:"isImported"`
```

`internal/transaction/usecase.go`: change the signature to `func (s *Service) buildResult(t *model.Transaction, author model.UserResult, labelIDs []string, imported bool) model.TransactionResult`, and add to the returned literal:

```go
		IsImported:         boolToInt(imported),
```

plus, after `strPtrDecimal`:

```go
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

`toResult` (single-row path for create/update/delete results) resolves the flag with one lookup:

```go
func (s *Service) toResult(ctx context.Context, t *model.Transaction, labelIDs []string) (model.TransactionResult, error) {
	author, err := s.users.GetOwner(ctx, t.UserID.String())
	if err != nil {
		return model.TransactionResult{}, err
	}
	imported, err := s.repo.ImportedTransactionIDs(ctx, []vo.Id{t.ID})
	if err != nil {
		return model.TransactionResult{}, err
	}
	return s.buildResult(t, model.UserResult{Id: author.ID, Avatar: author.Avatar, Name: author.Name}, labelIDs, imported[t.ID.String()]), nil
}
```

`internal/transaction/read.go`, right after the `labelsByTx` batch call:

```go
	importedByTx, err := s.repo.ImportedTransactionIDs(ctx, txIDs)
	if err != nil {
		return nil, err
	}
```

and the loop line becomes `items = append(items, s.buildResult(t, author, labelsByTx[t.ID.String()], importedByTx[t.ID.String()]))`.

Run: `go build ./... && go vet ./internal/transaction/...` — Expected: PASS (grep `buildResult(` to confirm no other caller: `grep -rn "buildResult(" internal/`).

- [ ] **Step 4: Seed one link in apiparity and regenerate goldens**

`internal/test/apiparity/fixture.go`: add consts

```go
	// One import link on Txn2 so get-transaction-list pins isImported=1 on a
	// real row (Txn1 stays 0). Nothing else reads import_* tables in stage 1.
	ImportSourcePhone = "0c000000-0000-0000-0000-000000000001"
	ImportLinkTxn2    = "0d000000-0000-0000-0000-000000000001"
```

and at the end of `Seed`, after the budget-access line:

```go
	f.ImportSource(fixture.ImportSource{ID: ImportSourcePhone, UserID: OwnerID, Provider: model.ImportProviderAppleWallet, Name: "iPhone"})
	f.ImportTransactionLink(fixture.ImportTransactionLink{ID: ImportLinkTxn2, SourceID: ImportSourcePhone,
		ExternalAccountID: "wallet", ExternalTransactionID: "tap-1", TransactionID: Txn2,
		ExternalPayee: "Employer", ExternalAmount: "1000.00000000", ExternalPostedAt: ClockTime})
```

Then:

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/
git diff --stat internal/test/apiparity/testdata internal/test/mcpparity/testdata
git diff internal/test/apiparity/testdata | grep '^[+-] ' | grep -v isImported
```

Expected: the last grep prints NOTHING — every changed line is an added `"isImported": 0` (or `1` for `Txn2` rows). Any other diff line is a bug; stop and investigate. If the MCP transaction tools render `TransactionResult` too, their goldens change the same way — inspect with the same grep.

Run: `go test ./internal/test/apiparity/ ./internal/test/mcpparity/ ./internal/transaction/...` — Expected: PASS.

- [ ] **Step 5: SPA DTO**

`web/src/api/dto/transaction.ts`, in `TransactionDto` after `recurringId`:

```ts
  /** 1 when at least one import link points at this transaction (server-owned; 0/1 like isArchived). */
  isImported: 0 | 1
```

Run `cd web && pnpm exec tsc -b` and add `isImported: 0` to every object literal it flags (expected: `useAccountTransactions.ts`, `features/recurring/{queries,asTransaction}.ts`, `budgets/BudgetTransactionsDialog.tsx`, and the test files `AccountPage.test.tsx`, `queries.test.tsx`, `TransactionRow.test.tsx`, `useTransactionForm.test.ts`, `ViewTransactionDialog.test.tsx`). For a row the SPA fabricates locally (optimistic insert, a recurring template rendered as a transaction) the correct value is `0`. Then `pnpm test && pnpm lint` — Expected: PASS.

- [ ] **Step 6: Docs**

Spec `docs/superpowers/specs/2026-08-15-transaction-import-design.md`:
- Replace the "One deliberate exception" paragraph (starts `**One deliberate exception**, see Part 4:`) with: `**One deliberate exception**, see Part 4: the transaction repo's \`ImportedTransactionIDs\` batch lookup reads \`import_transaction_links\` directly in SQL. This is a knowing shortcut, to be refactored later.`
- In Part 4, replace the first paragraph's `**Implementation: an \`EXISTS\` subquery** from the transaction list query into \`import_transaction_links\` — no join, so multiple links cannot duplicate rows, and no uniqueness constraint is needed for correctness.` with `**Implementation: a batch lookup** — \`Repository.ImportedTransactionIDs(ctx, ids)\` runs \`SELECT DISTINCT transaction_id FROM import_transaction_links WHERE transaction_id IN (...)\` in chunks, exactly like \`LabelsByTransactionIDs\`, and the list builder sets the flag per row. Multiple links collapse to one flag, so no uniqueness constraint is needed for correctness.`
- In the "Consequences to accept" list, replace `The subquery reads a table another feature owns … so the later refactor is greppable.` with `The lookup reads a table another feature owns. \`archtest\` will **not** catch this coupling — it is SQL, not a Go import. The repo method carries a comment marking the cross-feature reference, and it is the only place the transaction feature names the table.` and delete the bullet `Two query files (\`query/{sqlite,pgsql}\`) plus \`sqlc generate\`.`
- Future work: `Refactor the Part 4 \`EXISTS\` subquery to a proper port + glue adapter.` → `Refactor the Part 4 \`ImportedTransactionIDs\` SQL shortcut to a proper port + glue adapter.`

`docs/regression-test-plan.md`, section 5, add after the "Future-dated transaction" item:

```
- [ ] Transaction list rows carry `isImported` (0/1) in the API response; a
      transaction with an import link (any provider) reads 1, hand-entered
      reads 0 (no UI badge yet — stage 2 of the import feature).
```

- [ ] **Step 7: Commit**

```bash
gofmt -l ./internal ./cmd; go vet ./...
git add internal/transaction internal/model/transaction_dto.go internal/test/apiparity internal/test/mcpparity web/src docs/superpowers/specs/2026-08-15-transaction-import-design.md docs/regression-test-plan.md
git commit -m "feat(transaction): isImported flag on the list from import links"
```

---

### Task 8: Register the feature in the docs and run the full gate

**Files:**
- Modify: `CLAUDE.md` (Architecture → "Feature packages" paragraph and tree; API conventions)
- Verify: `internal/test/archtest`, whole suite

**Interfaces:** none new.

- [ ] **Step 1: CLAUDE.md**

In the "Feature packages (vertical slices)" paragraph, change `Each of the twelve features (\`account\`, \`admin\`, \`budget\`, \`category\`, \`connection\`, \`currency\`, \`payee\`, \`recurring\`, \`system\`, \`tag\`, \`transaction\`, \`user\`)` to `Each of the thirteen features (\`account\`, \`admin\`, \`budget\`, \`category\`, \`connection\`, \`currency\`, \`imports\`, \`payee\`, \`recurring\`, \`system\`, \`tag\`, \`transaction\`, \`user\`)`.

In the tree, the `<feature>/` line lists `(account, budget, category, connection, currency, payee, system, tag, transaction, user)` — insert `imports,` after `currency,`.

Under "Not every feature has a `repository.go`/`ports.go`" append a sentence: `\`imports\` (the bank/phone transaction-import subsystem, spec in \`docs/superpowers/specs/2026-08-15-transaction-import-design.md\`) ships in stages: stage 1 is the persistence + matcher core with no HTTP edge and no provider, so it has \`repository.go\` and \`repo/\` but no \`api/\` or \`ports.go\` yet; the package name is \`imports\` (not \`import\`, a Go keyword) while its future routes live under \`/api/v1/import/\`.`

In "Authentication" (the bullet starting `Two kinds:`), confirm Task 3 already added the scope sentence; if not, add: `Every token has a \`scope\`: \`full\` (sessions, and PATs created with \`scope: "full"\`) or \`ingest\` (PATs only; accepted solely on \`/api/v1/import/ingest-*\`, 401 \`"Invalid access token"\` anywhere else — the credential a phone Shortcut holds).`

- [ ] **Step 2: Architecture guard + smoke tier**

```bash
go test ./internal/test/archtest/
make go-test
```

Expected: PASS, coverage ≥ `GO_COVER_MIN`. `archtest` auto-detects `internal/imports` as a feature; a failure there means a feature import leaked (`internal/imports` must import nothing under `internal/` except `shared`, `model`, `infra/...`; `internal/config` and `internal/transaction` must not import `internal/imports`).

- [ ] **Step 3: PostgreSQL parity**

```bash
make test-repo-pgsql
go test -tags enginecompare ./internal/test/enginecompare/ ./internal/test/apiparity/ ./internal/test/mcpparity/
```

(`make test` runs all of this plus the frontend suite; use it if a Postgres is already provisioned — it auto-starts one via compose otherwise.) Expected: PASS. A byte-mismatch on `isImported` or `scope` between engines points at the pgsql adapter's struct conversion (`internal/imports/repo/pgsql.go`) or the pgsql migration's column types.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: register the imports feature package"
```
