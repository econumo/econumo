# OAuth apps: `app:*` CLI, official apps and single-user user apps — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** User apps (self-registered MCP OAuth clients) belong to the first user who approves them, operator-created official apps serve every user and are marked "Official" in the UI, and eight `app:*` CLI commands manage both.

**Architecture:** Two columns on `oauth_clients` (`owner_user_id`, `is_global`) with a backfill from the earliest grant. `authserver.Service.validate` refuses a foreign-owned client like an unknown one; `ApproveAuthorization` claims an unowned client with a conditional UPDATE inside its existing transaction. Operator use cases live in a new `internal/authserver/apps.go` (Service methods typed in `internal/model`); `internal/cli/app_commands.go` is a thin shell over them, wired through the CLI container. The consent and Connected apps DTOs gain an additive `isOfficial`, which the SPA renders as a badge.

**Tech Stack:** Go (stdlib `net/http`, `text/tabwriter`), sqlc per-engine queries (SQLite + PostgreSQL), golden-file apiparity suite, i18n catalogues in `locales/`, React 19 + vitest in `web/`.

**Spec:** `docs/superpowers/specs/2026-11-08-oauth-apps-cli-design.md`

## Global Constraints

- Terms: user-facing text (CLI output, UI, docs) says **official** app (CLI-registered) and **user** app (self-registered). The DB column stays `is_global`; repository methods touching it keep `Global` in their names (`UpdateGlobalClient`, `MakeClientGlobal`); service, CLI and DTO names say `Official`.
- Go is at `/usr/local/go/bin` (not on PATH): prefix commands with `PATH=/usr/local/go/bin:$HOME/go/bin:$PATH` (sqlc lives in `~/go/bin`).
- Migration version `20261108000000`, both engines (sorts after main's newest SQL `20261104000000` and command `20261103000001`; renumber if main moves before merge — `TestEngineVersionSetsMatch` needs the same version on both engines).
- `.sql` files ASCII-only, comments included; never put a `;` on its own line in a query file. Regenerate with `cd internal/infra/storage/sqlc && sqlc generate` after every query or migration change.
- A user app: `is_global = false`, `owner_user_id` NULL until first approval. An official app: `is_global = true`, `owner_user_id` always NULL.
- Purge (30-day age and the 1,000 cap) touches only `is_global = false AND last_used_at IS NULL` rows.
- Error code `authserver.app_not_yours`, English text exactly: `This app is connected to a different Econumo account. Remove Econumo from the app and add it again.` It must not name, hint at or confirm the owner. In all 11 locales and in `errs.AllCodes`.
- Exchange and refresh never re-check the owner.
- Official-app validation equals `/oauth/register`: name <= 100 runes after stripping invisible runes; 1-10 redirect URIs, each <= 2048 bytes, `ValidateRedirectURI`.
- CLI: ids only in log lines; a client secret is printed once to stdout and never logged; failures print `error: ...` and exit 1 (2 stays reserved for unknown commands). Commands work with the OAuth server disabled; `app:register` warns on stderr when it is.
- Lock order is user row first, then `oauth_grants` / `access_tokens` (the existing rule in `grants.go`). `access_tokens.grant_id` has no FK: tokens are revoked before a client delete.
- Comments: only the non-obvious why (CLAUDE.md "Comments — write sparingly"); no PHP references.
- Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never `git add -A`; add named paths.

## Review Focus

1. **Tokens and codes after `app:delete`.** A refresh token or a not-yet-exchanged code of a deleted app must fail (`invalid_client` / `invalid_grant`), not resurrect anything. Test: Task 5 `TestDeleteApp_RefreshAndCodeFailAfterwards`.
2. **`app:make-official` on a claimed app.** The former owner's grant keeps refreshing and a second user can now approve. Test: Task 5 `TestMakeAppOfficial_OwnerKeepsGrantOthersCanApprove`.
3. **Flag placement.** Operators type `--public` before, between or after positionals, `--name=X` or `--name X`, repeated `--redirect-uri`. An unknown flag is an error and writes nothing. Test: Task 6 `TestParseAppArgs` and `TestAppRegisterUnknownFlagWritesNothing`.
4. **`app:update` with nothing to change, or on a user app.** Error, row untouched. Test: Task 6 `TestAppUpdateRefusals`.
5. **`app:disconnect --user` for a user with no connection.** Exit 0, "0 connection(s) revoked", other users' grants untouched; an unknown app id exits 1. Test: Task 6 `TestAppDisconnect`.

---

### Task 1: Migration — owner and global columns, backfill

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20261108000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20261108000000.sql`
- Create: `internal/infra/storage/migrations/migration_20261108_test.go`
- Create: `internal/infra/storage/migrations/migration_20261108_pgsql_test.go` (`//go:build enginecompare`)

**Interfaces:**
- Produces: columns `oauth_clients.owner_user_id` (sqlite `TEXT`, pgsql `UUID`; NULL; FK `users(id) ON DELETE CASCADE`), `oauth_clients.is_global` (`BOOLEAN NOT NULL`, default false); index `IDX_oauth_clients_owner_user_id`; `IDX_oauth_clients_unused` recreated with predicate `last_used_at IS NULL AND is_global = <false>`.

- [ ] **Step 1: Write the failing tests.** In `migration_20261108_test.go`, `TestMigration20261108_BackfillsClientOwners` calls `runUpTo(t, "oauth_owner", "20261108000000")` then a shared `checkOAuthOwnerBackfill(t, db, rebind func(string) string, finish func())` (same shape as `checkCategoryTypeMigration` in `migration_20261017_test.go`). Seed: users A, B, C; clients `none` (no grants), `one` (one grant of B), `many` (grants: A created `2026-01-01 00:00:00` and revoked, id `...a1`; C created `2026-01-01 00:00:00`, id `...c1` (> A's id); B created `2026-01-02 00:00:00`). Then `finish()` and assert:

```go
// owner(id) reads owner_user_id (sql.NullString), isGlobal(id) reads is_global as bool
owner("none") is NULL; owner("one") == B; owner("many") == A // earliest, revoked counts, tie by grant id
!isGlobal(x) for all three
// FK cascade: DELETE FROM users WHERE id = B  -> client "one" row gone
```

The pgsql twin `TestMigration20261108_Pgsql` copies the setup of `TestMigration20261017_Pgsql` (own schema `test_migration_20261108`, skip without `DATABASE_TEST_PGSQL_URL`) and calls the same `checkOAuthOwnerBackfill` with the `$N` rebind. The cascade check runs with foreign keys on: on sqlite issue `PRAGMA foreign_keys = ON` first (the in-memory `runUpTo` DB uses a raw `sql.Open`).

- [ ] **Step 2: Run to verify it fails.** `go test ./internal/infra/storage/migrations/ -run TestMigration20261108` → FAIL (`no such column: owner_user_id`).

- [ ] **Step 3: Write both migrations.** Order: `ALTER TABLE ... ADD COLUMN owner_user_id ... DEFAULT NULL REFERENCES users (id) ON DELETE CASCADE`; `ADD COLUMN is_global BOOLEAN DEFAULT '0' NOT NULL` (sqlite) / `BOOLEAN NOT NULL DEFAULT false` (pgsql); `CREATE INDEX IDX_oauth_clients_owner_user_id`; `DROP INDEX IDX_oauth_clients_unused` + recreate with the new predicate (`is_global = 0` sqlite, `is_global = false` pgsql); backfill, identical on both engines:

```sql
UPDATE oauth_clients SET owner_user_id = (
    SELECT g.user_id FROM oauth_grants g WHERE g.client_id = oauth_clients.id
    ORDER BY g.created_at, g.id LIMIT 1);
```

A two-line header comment: why the backfill picks the earliest grant (spec section 4: nobody already connected is cut off; the earliest user is the one whose client registered it).

- [ ] **Step 4: Run to verify it passes.** `go test ./internal/infra/storage/migrations/` → PASS (includes `TestEngineVersionSetsMatch`). Do NOT run `sqlc generate` yet: with the six-column `GetOAuthClient` it would emit a `GetOAuthClientRow` instead of `OauthClient` and break the repo build; Task 2 regenerates together with the query changes. Do check the sqlite parser accepts the DDL: `cd internal/infra/storage/sqlc && sqlc vet` (or a throwaway `sqlc generate` followed by `git checkout -- gen`) must report no parse error on `ADD COLUMN ... REFERENCES`.

- [ ] **Step 5: Commit.** `git add internal/infra/storage/migrations/sqlite/20261108000000.sql internal/infra/storage/migrations/pgsql/20261108000000.sql internal/infra/storage/migrations/migration_20261108_test.go internal/infra/storage/migrations/migration_20261108_pgsql_test.go` — `feat: Add owner and global flag to OAuth clients`.

---

### Task 2: Queries, model and repository

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/oauth_server.sql`, `internal/infra/storage/sqlc/query/pgsql/oauth_server.sql`, regenerated `internal/infra/storage/sqlc/gen/{sqlite,pgsql}`
- Modify: `internal/model/authserver.go`
- Modify: `internal/authserver/repository.go`, `internal/authserver/repo/{repo.go,sqlite.go,pgsql.go}`
- Modify: `internal/test/fixture/entities.go` (`OAuthClient` builder)
- Test: `internal/authserver/repo/repo_integration_test.go`

**Interfaces:**
- Consumes: Task 1 columns.
- Produces (model):

```go
type OAuthClient struct { /* existing fields */ OwnerUserID *vo.Id; IsGlobal bool }
// OAuthApp is a client with its unrevoked grants, for the operator commands.
type OAuthApp struct { Client OAuthClient; Grants []OAuthGrant }
type ConnectedGrant struct { /* existing fields */ ClientIsGlobal bool }
```

- Produces (`authserver.Repository` additions; `InsertClient`/`GetClient` now carry the two fields):

```go
ClaimClient(ctx context.Context, id, userID vo.Id) (int64, error)            // only is_global=false AND owner_user_id IS NULL
ListClients(ctx context.Context) ([]model.OAuthClient, error)                 // created_at, id
UpdateGlobalClient(ctx context.Context, id vo.Id, name string, redirectURIs []string) (int64, error) // WHERE is_global
MakeClientGlobal(ctx context.Context, id vo.Id) (int64, error)               // is_global=true, owner NULL, WHERE NOT is_global
SetClientSecret(ctx context.Context, id vo.Id, hash string) (int64, error)   // WHERE secret_hash IS NOT NULL
LockClient(ctx context.Context, id vo.Id) error                              // errs.NotFound when absent; FOR UPDATE on PostgreSQL
DeleteClient(ctx context.Context, id vo.Id) (int64, error)
ListClientGrants(ctx context.Context, clientID vo.Id) ([]model.OAuthGrant, error) // unrevoked, created_at, id
ListAllUnrevokedGrants(ctx context.Context) ([]model.OAuthGrant, error)           // client_id, created_at, id
RevokeClientGrants(ctx context.Context, clientID vo.Id, now time.Time) ([]vo.Id, error) // UPDATE ... RETURNING id
```

- Produces (fixture): `fixture.OAuthClient{ID, Name, RedirectURIs, OwnerUserID string, IsGlobal bool}` — empty `OwnerUserID` inserts NULL.

- [ ] **Step 1: Write the failing repo test** `TestRepo_ClientOwnershipAndAdmin` in `repo_integration_test.go` (runs on both engines via `dbtest.New`):

```go
// insert self client (owner nil) and global client
n, _ := r.ClaimClient(ctx, self, userA)   // n == 1; GetClient(self).OwnerUserID == userA
n, _ = r.ClaimClient(ctx, self, userB)    // n == 0; owner still userA
n, _ = r.ClaimClient(ctx, global, userA)  // n == 0; owner stays nil, IsGlobal true
n, _ = r.UpdateGlobalClient(ctx, self, "x", uris)   // n == 0
n, _ = r.UpdateGlobalClient(ctx, global, "New", []string{"https://a.example/cb"}) // n == 1, read back
n, _ = r.MakeClientGlobal(ctx, self)      // n == 1; IsGlobal, OwnerUserID == nil
n, _ = r.SetClientSecret(ctx, publicID, "h") // n == 0 (public stays public)
r.LockClient(ctx, vo.NewId())             // errs.AsNotFound ok
// grants: g1 (A, live), g2 (B, revoked) on clientX
ListClientGrants(clientX) == [g1]; ListAllUnrevokedGrants contains g1, not g2
ids, _ := r.RevokeClientGrants(ctx, clientX, now) // == [g1]
r.DeleteClient(ctx, clientX)              // 1; GetClient NotFound; GetGrant(g2) NotFound (cascade)
```

Extend the existing connected-grants assertion: `ListUnrevokedGrants` reports `ClientIsGlobal` true for a grant on the global client and false otherwise. Plus `TestRepo_PurgeSkipsGlobalClients`: an old unused global client and an old unused self client; `PurgeUnusedClients(cutoff)` returns 1 and the global row remains; `PurgeExcessUnusedClients(0)` leaves the global row.

- [ ] **Step 2: Run to verify it fails.** `go test ./internal/authserver/repo/` → FAIL (undefined methods).

- [ ] **Step 3: Queries.** Edit both `oauth_server.sql` files (sqlite `?`, `0/1`; pgsql `$N`, `false/true`):
  - `InsertOAuthClient` and `GetOAuthClient` list all eight columns in table order (`id, name, redirect_uris, secret_hash, created_at, last_used_at, owner_user_id, is_global`) so sqlc keeps returning `OauthClient`.
  - `ListUnrevokedOAuthGrants` also selects `c.is_global` (Connected apps needs it); `ListUnrevokedGrants` fills `ConnectedGrant.ClientIsGlobal`.
  - Purge queries add `AND is_global = 0` (`= false`) to the outer WHERE and the `PurgeExcessUnusedOAuthClients` subquery, matching the new partial index predicate.
  - New: `ClaimOAuthClient :execrows`, `ListOAuthClients :many`, `UpdateGlobalOAuthClient :execrows`, `MakeOAuthClientGlobal :execrows`, `SetOAuthClientSecret :execrows`, `LockOAuthClient :one` (sqlite `SELECT id FROM oauth_clients WHERE id = ?`; pgsql the same plus `FOR UPDATE` — a comment on the pgsql one: plain FOR UPDATE is required because it is the only mode that conflicts with the KEY SHARE lock a concurrent grant INSERT takes through its FK), `DeleteOAuthClient :execrows`, `ListOAuthClientGrants :many`, `ListAllUnrevokedOAuthGrants :many` (both select the full grant column list so they return `OauthGrant`), `RevokeClientOAuthGrants :many` (`UPDATE oauth_grants SET revoked_at = ? WHERE client_id = ? AND revoked_at IS NULL RETURNING id`).
  - Run `sqlc generate`; generated `OauthClient` gains `OwnerUserID *string`, `IsGlobal bool` on both engines, so the pgsql whole-struct conversions keep compiling.

- [ ] **Step 4: Repo + model.** Add the fields to `model.OAuthClient` and the `OAuthApp` type; add a `hydrateClient(row clientRow) (*model.OAuthClient, error)` shared by `GetClient` and `ListClients`; one querier method per query in `repo.go`, `sqlite.go` (passthrough) and `pgsql.go` (struct conversion, like the existing ones). `LockClient` maps `sql.ErrNoRows` to `errs.NewNotFound("OAuth client not found")`. Extend `fixture.OAuthClient` and its INSERT with `owner_user_id` (via `nullable`) and `is_global`.

- [ ] **Step 5: Run to verify it passes.** `go test ./internal/authserver/... ./internal/test/fixture/...` → PASS; `go vet ./...` clean.

- [ ] **Step 6: Commit.** `git add` the query files, `internal/infra/storage/sqlc/gen`, `internal/model/authserver.go`, `internal/authserver/repository.go`, `internal/authserver/repo/`, `internal/test/fixture/entities.go` — `feat: Store and query OAuth client ownership`.

---

### Task 3: The owner rule on consent and the `isOfficial` field

**Files:**
- Modify: `internal/authserver/authorize.go`, `internal/authserver/grants.go`
- Modify: `internal/model/authserver_dto.go` (`AuthorizationRequestResult.IsOfficial bool \`json:"isOfficial"\``, `ConnectedAppResult.IsOfficial bool \`json:"isOfficial"\``)
- Modify: `internal/web/apidoc/docs/*` (regenerated by `make swagger`)
- Modify: `internal/shared/errs/codes.go` (`CodeAuthServerAppNotYours = "authserver.app_not_yours"`, added to `AllCodes`)
- Modify: all 11 `locales/<lang>.json` (`errors.authserver.app_not_yours`)
- Modify: `docs/regression-test-plan.md` (section 12a), `docs/mcp-setup.md`, `CLAUDE.md` (MCP OAuth bullet list)
- Test: `internal/authserver/authorize_test.go`, `internal/authserver/register_test.go`, `internal/authserver/token_test.go`

**Interfaces:**
- Consumes: `Repository.ClaimClient`, `model.OAuthClient.{OwnerUserID,IsGlobal}` (Task 2).
- Produces: `func appNotYours() error` (a `*errs.ValidationError` with `MsgCode: errs.CodeAuthServerAppNotYours`), used again in Task 5 tests. Wire field `isOfficial` on `get-authorization-request` and `get-connected-app-list` (consumed by Task 7).

- [ ] **Step 1: Write the failing tests** (in `authorize_test.go` unless noted; a second user comes from `fixture.New(t, db).User(fixture.User{})` — extend `newTestService` or add `newTestServiceUsers(t, n)` returning the db so tests can seed):
  - `TestApprove_ClaimsUnownedClient`: register, approve as A → `GetClient().OwnerUserID == A`.
  - `TestOtherUserIsRefused`: after A's approval, B's `DescribeAuthorization`, `ApproveAuthorization` and `DeclineAuthorization` each return `hasCode(err, errs.CodeAuthServerAppNotYours)`, the result's `RedirectURL`/`ErrorRedirectURL` is empty, no code row was inserted for B (`ConsumeCode` of nothing — assert via a count of `oauth_authorization_codes` rows for B == 0), and the error text contains neither A's id nor A's email. Also with a mismatched redirect URI B still gets `app_not_yours` (the owner check runs before the redirect check, so a stranger cannot probe registered URIs).
  - `TestOwnerReconnects`: A approves twice → both succeed, owner unchanged.
  - `TestDecline_DoesNotClaim`: A declines an unowned client → `OwnerUserID == nil`; B can then approve and becomes owner.
  - `TestOfficialApp_UsableByEveryone`: insert an official client via `repo.InsertClient(&model.OAuthClient{..., IsGlobal: true})`; A and B both describe (result `IsOfficial == true`) and approve → success, `OwnerUserID` stays nil. A user app's describe result has `IsOfficial == false`.
  - `grants_test.go` `TestConnectedApps`: a grant on an official client lists with `IsOfficial == true`, one on a user app with `false`.
  - `TestApprove_LosesClaimRace`: wrap the repo in `racingRepo{Repository; before func()}` whose `ClaimClient` first claims the client for B through the inner repo, then delegates; A's approval returns `app_not_yours` and no code exists for A.
  - `TestApprove_ConcurrentApprovalsOneWins`: two goroutines approve as A and B on a fresh client; exactly one error, with code `app_not_yours`, and the owner equals the winner (real race on the pgsql tier).
  - `token_test.go` `TestRefresh_NonOwnerGrantFromBeforeTheMigration`: client owned by A; insert a live grant for B with `repo.InsertGrant` (as the backfill leaves it); B's refresh succeeds and rotates; B's `DescribeAuthorization` gets `app_not_yours`.
  - `register_test.go`: in `TestRegisterPurgesUnusedClients` and `TestRegisterBoundsTheClientsNobodyApproved`, insert an old unused global client too and assert it survives the purge.

- [ ] **Step 2: Run to verify they fail.** `go test ./internal/authserver/ -run 'Claim|OtherUser|Owner|Decline_|OfficialApp|ClaimRace|OneWins|NonOwner|Register|ConnectedApps'` → FAIL.

- [ ] **Step 3: Implement.** In `validate`, right after the `GetClient` lookup and before `MatchRedirectURI`: `if !c.IsGlobal && c.OwnerUserID != nil && !c.OwnerUserID.Equal(userID) { return nil, nil, appNotYours() }` — `validate` gains a `userID vo.Id` parameter (all three callers have it). In `ApproveAuthorization`, inside the transaction after `IsTokenLive` and before `InsertCode`: when `!c.IsGlobal && c.OwnerUserID == nil`, call `ClaimClient`; on 0 rows re-read with `GetClient` and return `appNotYours()` unless the owner is now `userID`. Decline is unchanged apart from the extra argument. `DescribeAuthorization` sets `IsOfficial: c.IsGlobal`; `ListConnectedApps` sets `IsOfficial: r.ClientIsGlobal`. Run `make swagger` so the committed OpenAPI docs carry the field. Error copy (English) per Global Constraints; other locales:

| lang | text |
|---|---|
| de | Diese App ist mit einem anderen Econumo-Konto verbunden. Entfernen Sie Econumo aus der App und fügen Sie es erneut hinzu. |
| es | Esta aplicación está conectada a otra cuenta de Econumo. Quita Econumo de la aplicación y vuelve a añadirlo. |
| fr | Cette application est connectée à un autre compte Econumo. Retirez Econumo de l'application, puis ajoutez-le à nouveau. |
| it | Questa app è collegata a un altro account Econumo. Rimuovi Econumo dall'app e aggiungilo di nuovo. |
| nl | Deze app is gekoppeld aan een ander Econumo-account. Verwijder Econumo uit de app en voeg het opnieuw toe. |
| pl | Ta aplikacja jest połączona z innym kontem Econumo. Usuń Econumo z aplikacji i dodaj je ponownie. |
| pt | Esta aplicação está ligada a outra conta Econumo. Remova o Econumo da aplicação e adicione-o novamente. |
| ru | Это приложение подключено к другому аккаунту Econumo. Удалите Econumo из приложения и добавьте его снова. |
| uk | Цей застосунок підключено до іншого акаунта Econumo. Видаліть Econumo із застосунку та додайте його знову. |
| zh | 此应用已连接到另一个 Econumo 账号。请在应用中移除 Econumo，然后重新添加。 |

The SPA needs no change for the error: `ConsentPage.tsx` renders any non-401 describe failure through `apiErrorMessage` (the server-translated `message`) with no buttons, and approve/decline failures the same way. The `isOfficial` rendering is Task 7.

- [ ] **Step 4: Docs.** Regression plan 12a: add "Signed in as user B, opening a consent link for an app user A already connected shows 'This app is connected to a different Econumo account…', no buttons and no redirect, and the page names nobody", and "User A disconnects the app in Connected apps and connects again from the same client: it works". `docs/mcp-setup.md`: under "What approving gives the app", say an app registered by Claude/Codex belongs to the account that first approves it; another person adds Econumo in their own client. `CLAUDE.md` MCP OAuth bullets: one bullet stating the owner rule (claim on first approval under the user lock, `app_not_yours` on describe/approve/decline before the redirect check, decline never claims, exchange/refresh never re-check, official apps (`is_global`) exempt, purge skips them) and that both consent and Connected apps DTOs carry `isOfficial`.

- [ ] **Step 5: Run to verify it passes.** `go test ./internal/authserver/... ./internal/test/i18ntest/... ./internal/shared/...` → PASS.

- [ ] **Step 6: Commit.** `git add internal/authserver/authorize.go internal/authserver/grants.go internal/authserver/authorize_test.go internal/authserver/grants_test.go internal/authserver/register_test.go internal/authserver/token_test.go internal/model/authserver_dto.go internal/web/apidoc/docs internal/shared/errs/codes.go locales/*.json docs/regression-test-plan.md docs/mcp-setup.md CLAUDE.md` — `feat: Make a user OAuth app belong to its first approver; expose isOfficial`.

---

### Task 4: apiparity scenarios for the consent error

**Files:**
- Modify: `internal/test/apiparity/fixture.go`, `internal/test/apiparity/catalogue_authserver.go`
- Modify: `internal/test/apiparity/testdata/golden/authserver_consent*` (regenerated)

**Interfaces:**
- Consumes: `fixture.OAuthClient{OwnerUserID, IsGlobal}` (Task 2), `app_not_yours` (Task 3).
- Produces: constant `OAuthGlobalClientID = "e1000000-0000-0000-0000-000000000002"`.

- [ ] **Step 1: Fixture.** Seed `OAuthClientID` with `OwnerUserID: OwnerID` (what the backfill gives it: the owner holds the seeded grant) and a second client `OAuthGlobalClientID` ("Econumo Share", `https://share.example.test/auth/callback`, `IsGlobal: true`).

- [ ] **Step 2: Scenarios** in `authserver_consent`: `err:get-authorization-request-app-not-yours`, `err:approve-authorization-app-not-yours`, `err:decline-authorization-app-not-yours` (Auth `guest`, `OAuthClientID`); `get-authorization-request-global-app` and `approve-authorization-global-app` (Auth `guest`, `OAuthGlobalClientID`, its callback). Repoint the existing `decline-authorization-readonly` call to `OAuthGlobalClientID` with its callback so it still proves a readonly user reaches decline (the readonly user does not own `OAuthClientID`).

- [ ] **Step 3: Regenerate and inspect.** `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` then `git diff internal/test/apiparity/testdata`. Expected: every `get-authorization-request` and `get-connected-app-list` response gains `"isOfficial"` (false for `OAuthClientID`, true for the global one) and nothing else changes in existing labels; the five new labels appear; the three `err:` ones are 400 envelopes with `message` equal to the English text and `errors: {}`; `decline-authorization-readonly` changes only in its redirect host (share.example.test). Anything else changing is a regression — stop and investigate.

- [ ] **Step 4: Run.** `go test ./internal/test/apiparity/ ./internal/test/mcpparity/ ./internal/test/revisionguard/` → PASS.

- [ ] **Step 5: Commit.** `git add internal/test/apiparity/fixture.go internal/test/apiparity/catalogue_authserver.go internal/test/apiparity/testdata/golden` — `test: Cover the app-not-yours consent error in apiparity`.

---

### Task 5: Operator use cases in authserver

**Files:**
- Create: `internal/authserver/apps.go`
- Modify: `internal/authserver/register.go` (extract shared metadata checks)
- Test: `internal/authserver/apps_test.go`

**Interfaces:**
- Consumes: Task 2 repository methods, `revokeGrant`, `newSecret`, `ValidateRedirectURI`.
- Produces:

```go
var (
	ErrAppNotOfficial     = errors.New("only an official app can be changed; a user app's redirect URIs belong to its user's client")
	ErrAppAlreadyOfficial = errors.New("the app is already official")
	ErrAppPublic        = errors.New("the app is public: it has no secret to rotate")
	ErrAppNameRequired  = errors.New("name is required")
)
func (s *Service) RegisterOfficialApp(ctx context.Context, name string, redirectURIs []string, public bool) (model.OAuthClient, string, error) // secret "" when public
func (s *Service) ListApps(ctx context.Context) ([]model.OAuthApp, error)                 // Grants = live only (IsLive(now))
func (s *Service) GetApp(ctx context.Context, id vo.Id) (model.OAuthApp, error)           // errs.NotFound "OAuth app not found"
func (s *Service) UpdateOfficialApp(ctx context.Context, id vo.Id, name *string, redirectURIs []string) error // nil/empty = unchanged
func (s *Service) MakeAppOfficial(ctx context.Context, id vo.Id) error
func (s *Service) RotateAppSecret(ctx context.Context, id vo.Id) (string, error)
func (s *Service) DisconnectApp(ctx context.Context, id vo.Id, userID *vo.Id) (int, error) // live grants revoked
func (s *Service) DeleteApp(ctx context.Context, id vo.Id) (int, error)                    // grants revoked before the delete
// register.go, shared with Register:
func cleanClientName(raw string) (string, *OAuthError)       // strips invisible runes, trims, > 100 runes -> invalid_client_metadata
func checkRedirectURIs(uris []string) *OAuthError             // 1-10, <= 2048 bytes, ValidateRedirectURI -> invalid_redirect_uri
```

- [ ] **Step 1: Write the failing tests** in `apps_test.go`:
  - `TestRegisterOfficialApp`: confidential → `IsGlobal`, `OwnerUserID == nil`, secret non-empty and `hashSecret(secret) == *SecretHash`; then `Token` exchange works with the secret sent either way (`client_secret_post` field / basic — whatever `model.TokenRequest` carries, as in `TestToken_ConfidentialClientSucceeds`). `public=true` → `SecretHash == nil`, secret "". Validation: name of 101 runes, name `"​\n"` (→ `ErrAppNameRequired`), 0 and 11 URIs, a 2049-byte URI, `http://example.com/cb` → error, nothing inserted (`ListApps` empty). Works on a service built with `appURL ""` (disabled).
  - `TestListAndGetApps`: an official app, an owned user app with live grants of the owner and of user B and one revoked grant → `GetApp` returns 2 live grants; `ListApps` returns both apps in creation order; unknown id → `errs.AsNotFound`.
  - `TestUpdateOfficialApp`: rename only, URIs only (same validation errors as register), both; a live grant survives; a user app → `ErrAppNotOfficial`, row unchanged.
  - `TestMakeAppOfficial_OwnerKeepsGrantOthersCanApprove` (Review Focus 2): A approves + exchanges a user app; `MakeAppOfficial` → `IsGlobal`, owner nil; A's refresh succeeds; B's approve succeeds; second call → `ErrAppAlreadyOfficial`.
  - `TestRotateAppSecret`: a grant exists; rotate → new secret differs; exchange/refresh with the old secret → `invalid_client`; with the new one → success; the pre-rotation refresh token still refreshes with the new secret; public app → `ErrAppPublic`.
  - `TestDisconnectApp`: grants of A and B; `DisconnectApp(id, &A)` → 1, B's grant live, `creds.locked` contains A, `creds.revoked` contains A's grant id; `DisconnectApp(id, nil)` → 1 (B); the client remains.
  - `TestDeleteApp`: grants of A and B → returns 2; `creds.revoked` holds both grant ids; `creds.locked` holds A and B in ascending id order; `GetClient` NotFound; grant rows gone.
  - `TestDeleteApp_RefreshAndCodeFailAfterwards` (Review Focus 1): approve (code issued, not exchanged) + a separate exchanged grant; `DeleteApp`; refresh → `OAuthError` `invalid_client`; exchanging the outstanding code → `invalid_client`.

- [ ] **Step 2: Run to verify they fail.** `go test ./internal/authserver/ -run 'OfficialApp|Apps|AppOfficial|AppSecret|DisconnectApp|DeleteApp'` → FAIL.

- [ ] **Step 3: Implement.** Extract `cleanClientName`/`checkRedirectURIs` from `Register` keeping its error codes and descriptions byte-identical (`TestRegister*` and the oauthhttp tests must stay green). Admin methods return plain errors: an `*OAuthError` from the helpers becomes `errors.New(e.Description)`. `RegisterOfficialApp` does not purge and is not rate-limited. `DisconnectApp` lists `ListClientGrants`, keeps live ones (optionally of `userID`) and calls `revokeGrant` per grant (its own transaction, user lock first). `DeleteApp` runs one transaction:

```text
users  := distinct UserID of ListClientGrants(id), sorted by String()
for u in users: creds.LockForOAuth(u)          // lock order: users first
repo.LockClient(id)                             // NotFound -> "OAuth app not found"
ids := repo.RevokeClientGrants(id, now)         // after the lock: includes a grant a concurrent exchange just committed
for g in ids: creds.RevokeOAuthGrantTokens(g)   // access_tokens.grant_id has no FK
repo.DeleteClient(id)                           // codes, grants, spent hashes cascade
```

A short comment on `DeleteApp` explaining the client lock (a concurrent exchange's grant INSERT either commits before the lock and is revoked here, or fails its FK after the delete).

- [ ] **Step 4: Run to verify it passes.** `go test ./internal/authserver/...` → PASS.

- [ ] **Step 5: Commit.** `git add internal/authserver/apps.go internal/authserver/apps_test.go internal/authserver/register.go` — `feat: Add operator use cases for OAuth apps`.

---

### Task 6: `app:*` CLI commands

**Files:**
- Create: `internal/cli/app_commands.go`, `internal/cli/app_commands_test.go`
- Modify: `internal/cli/cli.go` (`commandList` appends `appCommands()...`), `internal/cli/container.go` (field `apps *appauthserver.Service`), `internal/cli/cli_test.go` (`TestCommandRegistry` want list)
- Modify: `CLAUDE.md` (CLI command list + an `app:*` paragraph), `docs/mcp-setup.md` (operator section)

**Interfaces:**
- Consumes: Task 5 Service methods and errors; `authserver.RedirectHost`; `datetime.Layout`.
- Produces:

```go
func appCommands() []command
// Flags may appear anywhere; "--k v" and "--k=v"; bool flags take no value; repeated value flags accumulate.
func parseAppArgs(args []string, boolFlags, valueFlags []string) (positional []string, flags map[string][]string, err error)
```

Container wiring, mirroring `server.BuildAPI`: `appauthserver.NewService(authserverrepo.NewRepo(cfg.DatabaseDriver, txm), userSvc, txm, clk, nil, cfg.AppURL)`.

Command behaviour (usage strings are the summaries):

| Command | Output (stdout) / audit line (slog.Info, ids only) |
|---|---|
| `app:register <name> <redirect-uri>... [--public]` | `App "<name>" registered (official).`, `client_id: <id>`, `client_secret: <secret>` (or `client_secret: none (public client, PKCE only)`), `The secret is shown only once.`; stderr warning `warning: the OAuth server is disabled (ECONUMO_URL is not an https origin); the app will work once it is enabled` when `!c.apps.Enabled()`. Log `app-register` `client_id`, `public`. |
| `app:list [--official] [--user]` | `text/tabwriter` table `ID KIND NAME REDIRECT OWNER CREATED LAST USED CONNECTIONS`; kind `official`/`user`; redirect = `RedirectHost` of the first URI; owner id or `-`; last used = latest of the client's `last_used_at` and its live grants' `last_used_at`, `never` when none; connections `N`, and for a user app with live grants of users other than the owner `N (M from other users)`. Both flags or neither = all. |
| `app:show <id>` | `ID`, `Name`, `Kind`, `Owner`, `Auth` (`confidential`/`public`), `Created`, `Last used`, `Redirect URIs` (one per line), `Connections: N[ (M from other users)]`, then a table `USER CONNECTED LAST USED`. |
| `app:update <id> [--name <name>] [--redirect-uri <uri>]...` | `App <id> updated.` Log `app-update` `client_id`. No option → usage error. |
| `app:make-official <id>` | `App <id> is now official.` Log `app-make-official`. |
| `app:rotate-secret <id>` | `client_secret: <secret>` + `The old secret no longer works.` Log `app-rotate-secret`. |
| `app:disconnect <id> [--user <user-id>]` | `<n> connection(s) revoked.` Log `app-disconnect` `client_id`, `user_id` (when given), `grants`. |
| `app:delete <id>` | `App <id> deleted (<n> connection(s) revoked).` Log `app-delete` `client_id`, `grants`. |

Ids are trimmed and parsed with `vo.ParseId`; a bad id is `invalid app id "<x>"` / `invalid user id "<x>"`.

- [ ] **Step 1: Write the failing tests** in `app_commands_test.go`, using `cliEnvDB` and a `captureOutput(t, fn func()) (stdout, stderr string)` helper (swap `os.Stdout`/`os.Stderr` with pipes); a `clientID(stdout)` helper parses the `client_id:` line:
  - `TestParseAppArgs` (Review Focus 3): `["Claude","--public","https://a/cb"]` → pos `[Claude https://a/cb]`, public set; `["--redirect-uri=https://a","--redirect-uri","https://b","id"]` → two values; `["--name"]` → error (missing value); `["--bogus"]` → error.
  - `TestAppRegister`: confidential → exit 0, stdout has a 43-char secret, DB row `is_global = 1`, `owner_user_id IS NULL`, `secret_hash = hex(sha256(secret))`; stderr carries the disabled warning (no `ECONUMO_URL` in the test env); a `slog` handler installed on a buffer for the run shows `app-register` with the client id and never the secret. `--public` → `secret_hash IS NULL`. Missing redirect URI, `http://example.com/cb`, 101-rune name → exit 1, `SELECT COUNT(*) FROM oauth_clients` unchanged.
  - `TestAppRegisterUnknownFlagWritesNothing` (Review Focus 3): `app:register X https://a/cb --bogus` → exit 1, no row.
  - `TestAppListAndShow`: a never-used official app shows `0` and `never`; a user app seeded by SQL with owner A and live grants of A and B shows `2 (1 from other users)`; kind column prints `official` / `user`; `--official` / `--user` filter; `app:show` lists every URI, `public`/`confidential`, and both connection rows; unknown id → exit 1.
  - `TestAppUpdateRefusals` (Review Focus 4): no options → exit 1; a user app → exit 1; row (name, redirect_uris) byte-identical afterwards; a valid rename + URI replace on an official app → exit 0 and stored.
  - `TestAppMakeOfficialAndRotate`: user app with owner → `app:make-official` exit 0, `is_global = 1`, owner NULL, second run exit 1; `app:rotate-secret` on a confidential app prints a new secret whose hash is stored and differs from the old hash; on a public app exit 1.
  - `TestAppDisconnect` (Review Focus 5): grants of A and B → `--user A` revokes only A's (`revoked_at` set) and prints `1 connection(s) revoked.`; `--user C` (no grant) exits 0 with `0 connection(s) revoked.`; no flag revokes B's; unknown app id exits 1.
  - `TestAppDeleteKillsLiveTokensOnARunningServer`: open the CLI's file DB with `sqlite.New().Open` and build `server.BuildAPI(config.Config{DatabaseDriver: "sqlite", CurrencyBase: "USD", AllowRegistration: true, AppURL: "https://econumo.example.test", ...}, db, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)})`; `app:register Share https://share.example.test/cb --public`; over the handler: register + log in a user, approve with PKCE (challenge `E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM`, verifier `dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk`), exchange at `/oauth/token` → access token; `POST /mcp` initialize → 200; `Run(["app:delete", id])` → 0; the same `POST /mcp` → 401 (the server reads tokens from the database per request, confirmed in `internal/user/authenticate.go`, so a separate CLI process needs no signal); `oauth_clients`, `oauth_grants` rows for the id are gone.
  - `TestCommandRegistry`: add the eight `app:*` names to `want`.

- [ ] **Step 2: Run to verify they fail.** `go test ./internal/cli/ -run 'App|ParseAppArgs|CommandRegistry'` → FAIL.

- [ ] **Step 3: Implement** `app_commands.go` and the container field per the table. Errors from the service are returned unchanged so `Run` prints `error: <msg>` and exits 1.

- [ ] **Step 4: Docs.** `CLAUDE.md`: add the eight commands to the CLI command block and one paragraph (official vs user apps, secret printed once, `app:delete` revokes tokens first, works with OAuth disabled). In the MCP OAuth section, the registration bullet notes that `/oauth/register` creates user apps only and that official apps come from `app:register`. `docs/mcp-setup.md`: new section "For operators: official apps" — `econumo app:register "Claude" https://claude.ai/api/mcp/auth_callback` for a Claude directory listing with Anthropic-held credentials or an organization's own OAuth client; `econumo app:register "Econumo Share" https://share.econumo.com/auth/callback`; a `curl`-registered app is single-user; `app:list` shows `N from other users` for a client several people share, convertible with `app:make-official`; official apps carry an "Official" badge on the consent page and in Connected apps.

- [ ] **Step 5: Run to verify it passes.** `go test ./internal/cli/... ./internal/test/archtest/...` → PASS.

- [ ] **Step 6: Commit.** `git add internal/cli/app_commands.go internal/cli/app_commands_test.go internal/cli/cli.go internal/cli/container.go internal/cli/cli_test.go CLAUDE.md docs/mcp-setup.md` — `feat: Manage OAuth apps from the command line`.

---

### Task 7: SPA — the "Official" badge and the unverified-name line

**Files:**
- Modify: `web/src/api/dto/authserver.ts` (`isOfficial: boolean` on `AuthorizationRequestResultDto` and `ConnectedAppDto`)
- Modify: `web/src/features/authserver/ConsentPage.tsx`, `web/src/features/authserver/ConnectedAppsPage.tsx`
- Modify: all 11 `locales/<lang>.json`: `authserver.consent.official`, `authserver.consent.unverified`, `authserver.apps.official`
- Modify: `docs/regression-test-plan.md` (section 12a)
- Test: `web/src/features/authserver/ConsentPage.test.tsx`, `web/src/features/authserver/ConnectedAppsPage.test.tsx`

**Interfaces:**
- Consumes: wire field `isOfficial` (Task 3).
- Produces: nothing other tasks use.

- [ ] **Step 1: Write the failing tests.** Extend the mocked DTOs in both specs with `isOfficial` (existing fixtures get `false`).
  - `ConsentPage.test.tsx` `it('marks an official app and adds no unverified-name line')`: `isOfficial: true` → `getByText('Official')` is present, `queryByText(/Econumo doesn't verify its name/)` is null.
  - `it('warns that a user app named itself, above the address line')`: `isOfficial: false` → no `Official`; the text `Registered by the app itself. Econumo doesn't verify its name; check the address below.` is present and comes before the `After you allow, you'll be sent to {host}.` line in document order (`compareDocumentPosition` → `DOCUMENT_POSITION_FOLLOWING`).
  - `ConnectedAppsPage.test.tsx` `test('marks official apps only')`: two apps, one official → exactly one `Official` badge, inside the official app's row; the user app's row shows nothing extra.

- [ ] **Step 2: Run to verify they fail.** `cd web && pnpm exec vitest run src/features/authserver/ConsentPage.test.tsx src/features/authserver/ConnectedAppsPage.test.tsx --maxWorkers=4` → FAIL.

- [ ] **Step 3: Implement.** Use the shadcn `Badge` from `@/components/ui/badge` with `variant="secondary"` (as in `ViewTransactionDialog.tsx`), placed next to the client name inside a `flex flex-wrap items-center gap-2` wrapper on the consent page and inline after the truncated name on the Connected apps row (keep the name truncating; the badge `shrink-0`). The unverified line is `<p className="text-xs text-muted-foreground">` immediately before the existing redirect/loopback `<p>` in the muted block, rendered only when `!result.isOfficial`. No bold for the new elements (badge, size and muted color carry the hierarchy). No new analytics event: nothing here is a user action. Catalogue values (`consent.official` and `apps.official` share the word; `consent.unverified` is the line):

| lang | Official | unverified |
|---|---|---|
| en | Official | Registered by the app itself. Econumo doesn't verify its name; check the address below. |
| de | Offiziell | Von der App selbst registriert. Econumo prüft ihren Namen nicht; prüfen Sie die Adresse unten. |
| es | Oficial | Registrada por la propia aplicación. Econumo no verifica su nombre; comprueba la dirección de abajo. |
| fr | Officielle | Enregistrée par l'application elle-même. Econumo ne vérifie pas son nom ; vérifiez l'adresse ci-dessous. |
| it | Ufficiale | Registrata dall'app stessa. Econumo non verifica il suo nome; controlla l'indirizzo qui sotto. |
| nl | Officieel | Door de app zelf geregistreerd. Econumo controleert de naam niet; controleer het adres hieronder. |
| pl | Oficjalna | Zarejestrowana przez samą aplikację. Econumo nie weryfikuje jej nazwy; sprawdź adres poniżej. |
| pt | Oficial | Registada pela própria aplicação. O Econumo não verifica o nome; confirme o endereço abaixo. |
| ru | Официальное | Зарегистрировано самим приложением. Econumo не проверяет его название; проверьте адрес ниже. |
| uk | Офіційний | Зареєстровано самим застосунком. Econumo не перевіряє його назву; перевірте адресу нижче. |
| zh | 官方 | 由应用自行注册。Econumo 不验证其名称，请核对下方的地址。 |

- [ ] **Step 4: Regression plan.** Section 12a: "📱 An app registered with `app:register` shows an 'Official' badge next to its name on the consent page and in Connected apps"; "📱 An app that registered itself (Claude, Codex) shows no badge and, on the consent page, the muted line 'Registered by the app itself. Econumo doesn't verify its name; check the address below.' above the 'After you allow…' line; Connected apps shows nothing extra for it".

- [ ] **Step 5: Run to verify it passes.** `cd web && pnpm exec tsc -b && pnpm lint && pnpm exec vitest run src/features/authserver/ConsentPage.test.tsx src/features/authserver/ConnectedAppsPage.test.tsx --maxWorkers=4` → PASS; `go test ./internal/test/i18ntest/` → PASS (key parity and frontend `t()` coverage).

- [ ] **Step 6: Commit.** `git add web/src/api/dto/authserver.ts web/src/features/authserver/ConsentPage.tsx web/src/features/authserver/ConsentPage.test.tsx web/src/features/authserver/ConnectedAppsPage.tsx web/src/features/authserver/ConnectedAppsPage.test.tsx locales/*.json docs/regression-test-plan.md` — `feat: Mark official OAuth apps in consent and Connected apps`.

---

## Final verification

- [ ] `make go-test` → PASS (build, vet, gofmt, OpenAPI-docs-fresh, sqlite suites incl. apiparity/mcpparity/i18ntest/archtest, coverage gate >= 80).
- [ ] Throwaway PostgreSQL on loopback:

```bash
docker run -d --name econumo-apps-pgtest -e POSTGRES_USER=econumo -e POSTGRES_PASSWORD=econumo \
  -e POSTGRES_DB=econumo_test -p 127.0.0.1:5452:5432 postgres:17-alpine
export DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@127.0.0.1:5452/econumo_test?sslmode=disable'
make test-repo-pgsql   # repo/unit suite on PostgreSQL (claim race, LockClient FOR UPDATE, migration twin)
make test-engines      # sqlite vs PostgreSQL byte-identical apiparity/mcpparity
make test-upgrade      # published releases upgraded through 20261108000000 on both engines
docker rm -f econumo-apps-pgtest
```

Each must PASS with the pgsql tests actually running (not skipped): check `-v` output of `TestMigration20261108_Pgsql` once.
- [ ] Web: `cd web && pnpm exec tsc -b && pnpm lint && pnpm exec vitest run src/features/authserver/ConsentPage.test.tsx src/features/authserver/ConnectedAppsPage.test.tsx --maxWorkers=4` → PASS.
