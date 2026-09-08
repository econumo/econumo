# OAuth / OIDC Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Users sign in to Econumo with Google, Apple, or one custom OpenID Connect provider on the web and in the app, with identities linked in Settings and passwordless provisioning.

**Architecture:** A feature-free protocol package `internal/infra/oidc` (discovery, PKCE, JWKS verification, Apple secret) under a new feature package `internal/oauth` (states, handoffs, identities, callback resolution) mounted at `/api/v1/oauth/`. The user feature exposes three small methods behind ports wired in `internal/server`. The SPA adds provider buttons, a `/oauth/callback` route, a Linked accounts page, and an app deep-link handler.

**Tech Stack:** Go 1.25 stdlib (crypto/rsa, crypto/ecdsa, net/http), sqlc, SQLite + PostgreSQL migrations, React 19 + TanStack Query + react-i18next, vitest + msw, Capacitor App/Browser plugins.

**Spec:** `docs/superpowers/specs/2026-09-07-oauth-login-design.md` (read it first; every rule below argues from it).

## Global Constraints

- No new direct Go dependency. `golang.org/x/oauth2` and `coreos/go-oidc` are NOT used; stdlib only.
- Features never import features: `internal/oauth` must not import `internal/user` (archtest fails the build). All cross-feature calls go through ports in `internal/oauth/ports.go` and `internal/user/ports.go`, adapters in `internal/server/glue_*.go`.
- Driver strings are exactly `"sqlite"` and `"postgresql"`.
- Datetimes on the wire use `datetime.Layout` (`"2006-01-02 15:04:05"`); nullable datetimes are `*time.Time`, nullable text is `*string` (never `sql.Null*`).
- Every new `errs` code goes in `codes.go` const block AND `AllCodes` AND `errors.*` in all 11 catalogues (`de en es fr it nl pl pt ru uk zh`).
- Every new `t('...')` key exists in all 11 catalogues with identical `{placeholders}`.
- Every new `METRICS` key is referenced as `METRICS.KEY` in non-test source.
- Migration version for this feature: `20260907000000` (both engines, same filename).
- Provider ids: `google`, `apple`, `oidc`. Client values: `web`, `app`. Intents: `login`, `link`.
- Redirect URI per provider: `<ECONUMO_URL>/api/v1/oauth/callback-<provider>`.
- State TTL 10 minutes; handoff TTL 60 seconds; outbound HTTP timeout 10 seconds; clock skew 60 seconds.
- Password-less users: `users.algorithm = 'none'`, `password = ''`, `salt = ''`.
- Run gofmt on every Go file you touch; run `make swagger` after adding/changing any annotated handler or DTO used by one.
- Commit after every task with a `feat(oauth): …` / `test(oauth): …` message ending in `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.

## File map

| Path | Responsibility |
|---|---|
| `internal/config/config.go` | the three provider slots + trust flag, validated at boot |
| `internal/infra/storage/migrations/{sqlite,pgsql}/20260907000000.sql` | `users_identities`, `oauth_states`, `oauth_handoffs`, `access_tokens.provider/id_token` |
| `internal/infra/storage/sqlc/query/{sqlite,pgsql}/oauth.sql` | queries for the three new tables |
| `internal/infra/storage/sqlc/query/{sqlite,pgsql}/access_tokens.sql` | two new columns in every column list |
| `internal/model/oauth.go`, `internal/model/oauth_dto.go` | entities + wire DTOs for the oauth feature |
| `internal/model/user.go`, `token.go`, `user_dto.go`, `token_dto.go` | `AlgorithmNone`, `NewPasswordlessUser`, `HasPassword`, `AccessToken.Provider/IDToken`, `SessionItem.Provider`, `CurrentUserResult.HasPassword`, `LogoutResult.LogoutUrl/Provider` |
| `internal/shared/errs/codes.go` | `oauth.*` codes |
| `internal/infra/oidc/` | `random.go`, `pkce.go`, `client.go` (discovery, auth URL, exchange, userinfo, end-session), `jwt.go` (parse + verify), `jwks.go`, `apple.go`; `oidctest/fake.go` (fake issuer) |
| `internal/oauth/` | `ports.go`, `repository.go`, `providers.go`, `service.go`, `start.go`, `callback.go`, `handoff.go`, `identities.go`, `name.go` |
| `internal/oauth/repo/` | `identity.go`, `state.go`, `handoff.go` + `_sqlite.go`/`_pgsql.go` adapters |
| `internal/oauth/api/` | `handler.go`, `routes.go`, `oauth.go` (handlers with swag annotations) |
| `internal/user/` | `external.go` (ProvisionExternalUser, CreateExternalSession, ReplaceVerifiedEmail), `session.go` (provider on createSession), `usecase.go` (Logout URL), `ports.go` (LogoutURLBuilder) |
| `internal/user/repo/accesstoken*.go` | persist provider/id_token |
| `internal/server/server.go`, `glue_oauth_users.go`, `glue_user_logout.go` | wiring |
| `internal/web/middleware/auth.go` | two allowlist entries |
| `internal/test/apiparity/{harness.go,normalize.go,smoke_test.go,catalogue_oauth.go,guard_test.go}` | 302 capture, oauth scenarios, minRoutes |
| `internal/test/fixture/entities.go` | `User.Algorithm`, `Identity` seed |
| `internal/cli/user_commands.go` | `Password:` line in user:show |
| `Makefile` | `../../oauth` in `SWAG_INIT` |
| `web/src/api/oauth.ts`, `web/src/api/dto/oauth.ts` | typed client |
| `web/src/features/auth/oauthQueries.ts`, `ProviderButtons.tsx`, `OAuthCallbackPage.tsx`, `LoginPage.tsx`, `RegistrationPage.tsx`, `LogoutPage.tsx` | login-side UI |
| `web/src/features/settings/LinkedAccountsPage.tsx`, `ProfilePage.tsx`, `SessionsPage.tsx`, `security.ts` | settings UI |
| `web/src/lib/appBoot.ts`, `externalLinks.ts`, `deepLinks.ts`, `metrics.ts`, `api/client.ts`, `api/user.ts`, `api/dto/user.ts`, `app/routes.tsx`, `app/router-pages.ts` | plumbing |
| `mobile/ios/App/App/Info.plist`, `mobile/android/app/src/main/AndroidManifest.xml`, `mobile/README.md` | `econumo://` scheme |
| `locales/*.json` (11) | all new keys |
| `CLAUDE.md`, `.env.example`, `docs/oidc-setup.md`, `docs/regression-test-plan.md` | docs |

---

### Task 1: Configuration slots

**Files:**
- Modify: `internal/config/config.go` (struct at lines 15-85, `Load()` after the `ECONUMO_URL` block at lines 211-219)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces on `config.Config`:
  ```go
  OAuthGoogleClientID, OAuthGoogleClientSecret string
  OAuthAppleClientID, OAuthAppleTeamID, OAuthAppleKeyID, OAuthApplePrivateKey string // PEM, "\n" escapes unescaped
  OIDCIssuerURL, OIDCClientID, OIDCClientSecret, OIDCName string
  OIDCScopes []string
  OIDCTrustEmail bool
  func (c Config) OAuthGoogleEnabled() bool
  func (c Config) OAuthAppleEnabled() bool
  func (c Config) OIDCEnabled() bool
  func (c Config) OAuthEnabled() bool // any of the three
  ```

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/config_test.go`:

```go
func TestLoad_OAuthGoogleRequiresBoth(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	t.Setenv("ECONUMO_URL", "https://money.example.test")
	t.Setenv("ECONUMO_OAUTH_GOOGLE_CLIENT_ID", "abc.apps.googleusercontent.com")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ECONUMO_OAUTH_GOOGLE_CLIENT_SECRET") {
		t.Fatalf("half-configured google slot must name the missing variable, got %v", err)
	}
}

func TestLoad_OAuthRequiresAppURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	t.Setenv("ECONUMO_OAUTH_GOOGLE_CLIENT_ID", "abc")
	t.Setenv("ECONUMO_OAUTH_GOOGLE_CLIENT_SECRET", "def")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ECONUMO_URL") {
		t.Fatalf("a provider without ECONUMO_URL must fail naming ECONUMO_URL, got %v", err)
	}
}

func TestLoad_OAuthGoogleEnabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	t.Setenv("ECONUMO_URL", "https://money.example.test")
	t.Setenv("ECONUMO_OAUTH_GOOGLE_CLIENT_ID", "abc")
	t.Setenv("ECONUMO_OAUTH_GOOGLE_CLIENT_SECRET", "def")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.OAuthGoogleEnabled() || c.OAuthAppleEnabled() || c.OIDCEnabled() || !c.OAuthEnabled() {
		t.Fatalf("google=%v apple=%v oidc=%v any=%v", c.OAuthGoogleEnabled(), c.OAuthAppleEnabled(), c.OIDCEnabled(), c.OAuthEnabled())
	}
}

func TestLoad_OAuthApplePrivateKeyUnescapesNewlines(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	t.Setenv("ECONUMO_URL", "https://money.example.test")
	t.Setenv("ECONUMO_OAUTH_APPLE_CLIENT_ID", "com.example.web")
	t.Setenv("ECONUMO_OAUTH_APPLE_TEAM_ID", "TEAM123456")
	t.Setenv("ECONUMO_OAUTH_APPLE_KEY_ID", "KEY1234567")
	t.Setenv("ECONUMO_OAUTH_APPLE_PRIVATE_KEY", `-----BEGIN PRIVATE KEY-----\nMIGH\n-----END PRIVATE KEY-----`)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.OAuthApplePrivateKey, "\nMIGH\n") {
		t.Fatalf("literal \\n must be unescaped, got %q", c.OAuthApplePrivateKey)
	}
}

func TestLoad_OIDCDefaultsAndValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	t.Setenv("ECONUMO_URL", "https://money.example.test")
	t.Setenv("ECONUMO_OIDC_ISSUER_URL", "https://auth.example.test/application/o/econumo/")
	t.Setenv("ECONUMO_OIDC_CLIENT_ID", "cid")
	t.Setenv("ECONUMO_OIDC_CLIENT_SECRET", "sec")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.OIDCName != "SSO" || strings.Join(c.OIDCScopes, " ") != "openid profile email" || c.OIDCTrustEmail {
		t.Fatalf("defaults: name=%q scopes=%v trust=%v", c.OIDCName, c.OIDCScopes, c.OIDCTrustEmail)
	}

	t.Setenv("ECONUMO_OIDC_SCOPES", "profile,email")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "openid") {
		t.Fatalf("scopes without openid must fail, got %v", err)
	}
	t.Setenv("ECONUMO_OIDC_SCOPES", "openid,email")
	t.Setenv("ECONUMO_OIDC_ISSUER_URL", "http://auth.example.test")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("plain http issuer on a non-loopback host must fail, got %v", err)
	}
	t.Setenv("ECONUMO_OIDC_ISSUER_URL", "http://127.0.0.1:9000")
	if _, err := Load(); err != nil {
		t.Fatalf("loopback http issuer must be accepted: %v", err)
	}
	t.Setenv("ECONUMO_OIDC_TRUST_EMAIL", "maybe")
	if _, err := Load(); err == nil {
		t.Fatal("malformed ECONUMO_OIDC_TRUST_EMAIL must fail")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestLoad_OAuth|TestLoad_OIDC' -v`
Expected: compile errors (unknown fields/methods).

- [ ] **Step 3: Implement**

Add to the `Config` struct after `AppURL`:

```go
	// OAuth / OIDC provider slots (see docs/superpowers/specs/2026-09-07-oauth-login-design.md §3).
	// A slot is enabled when every required variable is set; a partial slot fails at boot.
	OAuthGoogleClientID     string // ECONUMO_OAUTH_GOOGLE_CLIENT_ID
	OAuthGoogleClientSecret string // ECONUMO_OAUTH_GOOGLE_CLIENT_SECRET
	OAuthAppleClientID      string // ECONUMO_OAUTH_APPLE_CLIENT_ID (the Services ID)
	OAuthAppleTeamID        string // ECONUMO_OAUTH_APPLE_TEAM_ID
	OAuthAppleKeyID         string // ECONUMO_OAUTH_APPLE_KEY_ID
	OAuthApplePrivateKey    string // ECONUMO_OAUTH_APPLE_PRIVATE_KEY: the .p8 PEM; literal "\n" escapes unescaped
	OIDCIssuerURL           string // ECONUMO_OIDC_ISSUER_URL
	OIDCClientID            string // ECONUMO_OIDC_CLIENT_ID
	OIDCClientSecret        string // ECONUMO_OIDC_CLIENT_SECRET
	OIDCName                string // ECONUMO_OIDC_NAME: button label, default "SSO"
	OIDCScopes              []string // ECONUMO_OIDC_SCOPES: default openid profile email; must contain openid
	OIDCTrustEmail          bool   // ECONUMO_OIDC_TRUST_EMAIL: treat the issuer's email claim as verified (default false)
```

Add the methods at the end of the file:

```go
func (c Config) OAuthGoogleEnabled() bool { return c.OAuthGoogleClientID != "" }
func (c Config) OAuthAppleEnabled() bool  { return c.OAuthAppleClientID != "" }
func (c Config) OIDCEnabled() bool        { return c.OIDCIssuerURL != "" }
func (c Config) OAuthEnabled() bool {
	return c.OAuthGoogleEnabled() || c.OAuthAppleEnabled() || c.OIDCEnabled()
}
```

In `Load()`, right after the `ECONUMO_URL` block, add:

```go
	if err := loadOAuth(&c); err != nil {
		return Config{}, err
	}
```

and add the helper (a new section in config.go):

```go
// loadOAuth reads the three provider slots. A slot is all-or-nothing: the
// first missing required variable of a partially set slot is named in the
// error, and any enabled slot requires ECONUMO_URL (redirect URIs derive from it).
func loadOAuth(c *Config) error {
	requireAll := func(slot string, vars ...string) (bool, error) {
		set := 0
		for _, v := range vars {
			if os.Getenv(v) != "" {
				set++
			}
		}
		if set == 0 {
			return false, nil
		}
		for _, v := range vars {
			if os.Getenv(v) == "" {
				return false, fmt.Errorf("%s: %s is required when the other %s variables are set", slot, v, slot)
			}
		}
		return true, nil
	}

	google, err := requireAll("ECONUMO_OAUTH_GOOGLE", "ECONUMO_OAUTH_GOOGLE_CLIENT_ID", "ECONUMO_OAUTH_GOOGLE_CLIENT_SECRET")
	if err != nil {
		return err
	}
	if google {
		c.OAuthGoogleClientID = os.Getenv("ECONUMO_OAUTH_GOOGLE_CLIENT_ID")
		c.OAuthGoogleClientSecret = os.Getenv("ECONUMO_OAUTH_GOOGLE_CLIENT_SECRET")
	}

	apple, err := requireAll("ECONUMO_OAUTH_APPLE", "ECONUMO_OAUTH_APPLE_CLIENT_ID", "ECONUMO_OAUTH_APPLE_TEAM_ID", "ECONUMO_OAUTH_APPLE_KEY_ID", "ECONUMO_OAUTH_APPLE_PRIVATE_KEY")
	if err != nil {
		return err
	}
	if apple {
		c.OAuthAppleClientID = os.Getenv("ECONUMO_OAUTH_APPLE_CLIENT_ID")
		c.OAuthAppleTeamID = os.Getenv("ECONUMO_OAUTH_APPLE_TEAM_ID")
		c.OAuthAppleKeyID = os.Getenv("ECONUMO_OAUTH_APPLE_KEY_ID")
		// Env files are single-line; the PEM's newlines arrive as literal "\n".
		c.OAuthApplePrivateKey = strings.ReplaceAll(os.Getenv("ECONUMO_OAUTH_APPLE_PRIVATE_KEY"), `\n`, "\n")
	}

	oidc, err := requireAll("ECONUMO_OIDC", "ECONUMO_OIDC_ISSUER_URL", "ECONUMO_OIDC_CLIENT_ID", "ECONUMO_OIDC_CLIENT_SECRET")
	if err != nil {
		return err
	}
	if oidc {
		raw := os.Getenv("ECONUMO_OIDC_ISSUER_URL")
		u, perr := url.Parse(raw)
		if perr != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname()))) {
			return fmt.Errorf("ECONUMO_OIDC_ISSUER_URL: must be an absolute https URL (plain http only for loopback hosts): %q", raw)
		}
		c.OIDCIssuerURL = strings.TrimSuffix(raw, "/")
		c.OIDCClientID = os.Getenv("ECONUMO_OIDC_CLIENT_ID")
		c.OIDCClientSecret = os.Getenv("ECONUMO_OIDC_CLIENT_SECRET")
		c.OIDCName = getEnv("ECONUMO_OIDC_NAME", "SSO")
		c.OIDCScopes = getStringList("ECONUMO_OIDC_SCOPES", []string{"openid", "profile", "email"})
		if !slices.Contains(c.OIDCScopes, "openid") {
			return fmt.Errorf("ECONUMO_OIDC_SCOPES: must contain \"openid\", got %q", strings.Join(c.OIDCScopes, ","))
		}
		trust, terr := getBoolStrict("ECONUMO_OIDC_TRUST_EMAIL", false)
		if terr != nil {
			return terr
		}
		c.OIDCTrustEmail = trust
	}

	if (google || apple || oidc) && c.AppURL == "" {
		return fmt.Errorf("ECONUMO_URL is required when an OAuth/OIDC provider is configured (redirect URIs derive from it)")
	}
	return nil
}
```

If `isLoopbackHost` does not already exist next to the `ECONUMO_BILLING_URL` validation, add:

```go
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
```

Check `getStringList` splits on commas and trims (it does, config.go:377); imports needed: `net`, `net/url`, `slices`, `strings`.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/config/ -v`
Expected: PASS (all, including the existing ones).

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat(oauth): provider configuration slots

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: Schema, sqlc queries, generated code

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260907000000.sql`, `internal/infra/storage/migrations/pgsql/20260907000000.sql`
- Create: `internal/infra/storage/sqlc/query/sqlite/oauth.sql`, `internal/infra/storage/sqlc/query/pgsql/oauth.sql`
- Modify: `internal/infra/storage/sqlc/query/sqlite/access_tokens.sql`, `internal/infra/storage/sqlc/query/pgsql/access_tokens.sql` (every column list)
- Regenerate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}`
- Test: `internal/infra/storage/migrations/migration_20260907_test.go`

**Interfaces:**
- Produces generated types: `sqlitegen.UsersIdentity`, `sqlitegen.OauthState`, `sqlitegen.OauthHandoff`, `sqlitegen.AccessToken` (now with `Provider *string`, `IDToken *string`), and queries `GetIdentityByProviderSubject`, `GetIdentityByUserProvider`, `ListIdentitiesByUser`, `CountIdentitiesByUser`, `UpsertIdentity`, `DeleteIdentityByUserProvider`, `InsertOAuthState`, `GetOAuthState`, `DeleteOAuthState`, `DeleteExpiredOAuthStates`, `InsertOAuthHandoff`, `GetOAuthHandoff`, `DeleteOAuthHandoff`, `DeleteExpiredOAuthHandoffs`.

- [ ] **Step 1: Write the failing migration test**

Look at `internal/infra/storage/migrations/migration_20260817_test.go` for the helper the package uses to open a migrated sqlite DB, and write `migration_20260907_test.go` in the same style:

```go
package migrations_test

import "testing"

func TestMigration20260907_OAuthTables(t *testing.T) {
	// runUpTo/runAll live in migration_20260803_test.go (same package).
	db := runUpTo(t, "oauth_tables", "20260907000000")
	runAll(t, db)
	const ts = "'2026-01-01 00:00:00'"
	if _, err := db.Exec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES ('u1', 'u1', 'u1@e.test', 'U', '', 'x', '', ` + ts + `, ` + ts + `)`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users_identities (id, user_id, provider, subject, email, created_at, updated_at)
		 VALUES ('i1', (SELECT id FROM users LIMIT 1), 'google', 'sub-1', 'a@example.test', '2026-09-07 00:00:00', '2026-09-07 00:00:00')`,
		`INSERT INTO oauth_states (state_hash, provider, nonce, code_verifier, client, intent, link_user_id, created_at, expires_at)
		 VALUES ('h1', 'google', 'n', 'v', 'web', 'login', NULL, '2026-09-07 00:00:00', '2026-09-07 00:10:00')`,
		`SELECT provider, id_token FROM access_tokens LIMIT 1`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// (provider, subject) is unique
	if _, err := db.Exec(`INSERT INTO users_identities (id, user_id, provider, subject, email, created_at, updated_at)
		VALUES ('i2', (SELECT id FROM users LIMIT 1), 'google', 'sub-1', 'b@example.test', '2026-09-07 00:00:00', '2026-09-07 00:00:00')`); err == nil {
		t.Fatal("duplicate (provider, subject) must be rejected")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/infra/storage/migrations/ -run TestMigration20260907 -v`
Expected: FAIL, "no such table: users_identities".

- [ ] **Step 3: Write the migrations**

`internal/infra/storage/migrations/sqlite/20260907000000.sql`:

```sql
-- OAuth / OIDC login (docs/superpowers/specs/2026-09-07-oauth-login-design.md §4).
-- users_identities maps a provider subject to a user; email is display only.
-- oauth_states holds the in-flight authorization requests (server-side because
-- Apple's cross-site form POST carries no SameSite cookie and the app's browser
-- sheet shares no storage with the SPA); oauth_handoffs the one-shot codes the
-- client exchanges for a session so the token never travels in a URL.
CREATE TABLE users_identities
(
    id           TEXT     NOT NULL
    , user_id    TEXT     NOT NULL
    , provider   TEXT     NOT NULL
    , subject    TEXT     NOT NULL
    , email      TEXT     NOT NULL DEFAULT ''
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX users_identities_provider_subject_uniq ON users_identities (provider, subject);
CREATE UNIQUE INDEX users_identities_user_provider_uniq ON users_identities (user_id, provider);

CREATE TABLE oauth_states
(
    state_hash      TEXT     NOT NULL
    , provider      TEXT     NOT NULL
    , nonce         TEXT     NOT NULL
    , code_verifier TEXT     NOT NULL DEFAULT ''
    , client        TEXT     NOT NULL
    , intent        TEXT     NOT NULL
    , link_user_id  TEXT
    , created_at    DATETIME NOT NULL
    , expires_at    DATETIME NOT NULL
    , PRIMARY KEY (state_hash)
);
CREATE INDEX oauth_states_expires_at_idx ON oauth_states (expires_at);

CREATE TABLE oauth_handoffs
(
    code_hash    TEXT     NOT NULL
    , user_id    TEXT     NOT NULL
    , provider   TEXT     NOT NULL
    , id_token   TEXT
    , created_at DATETIME NOT NULL
    , expires_at DATETIME NOT NULL
    , PRIMARY KEY (code_hash)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX oauth_handoffs_expires_at_idx ON oauth_handoffs (expires_at);

-- Sessions minted from a provider remember it (sessions list, logout notice);
-- id_token is kept only for the custom slot's RP-initiated logout.
ALTER TABLE access_tokens ADD COLUMN provider TEXT;
ALTER TABLE access_tokens ADD COLUMN id_token TEXT;
```

`internal/infra/storage/migrations/pgsql/20260907000000.sql`:

```sql
-- See the sqlite sibling for the semantics.
CREATE TABLE users_identities
(
    id           UUID     NOT NULL
    , user_id    UUID     NOT NULL
    , provider   TEXT     NOT NULL
    , subject    TEXT     NOT NULL
    , email      TEXT     NOT NULL DEFAULT ''
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX users_identities_provider_subject_uniq ON users_identities (provider, subject);
CREATE UNIQUE INDEX users_identities_user_provider_uniq ON users_identities (user_id, provider);

CREATE TABLE oauth_states
(
    state_hash      TEXT     NOT NULL
    , provider      TEXT     NOT NULL
    , nonce         TEXT     NOT NULL
    , code_verifier TEXT     NOT NULL DEFAULT ''
    , client        TEXT     NOT NULL
    , intent        TEXT     NOT NULL
    , link_user_id  UUID
    , created_at    TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expires_at    TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (state_hash)
);
CREATE INDEX oauth_states_expires_at_idx ON oauth_states (expires_at);

CREATE TABLE oauth_handoffs
(
    code_hash    TEXT     NOT NULL
    , user_id    UUID     NOT NULL
    , provider   TEXT     NOT NULL
    , id_token   TEXT
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expires_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (code_hash)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX oauth_handoffs_expires_at_idx ON oauth_handoffs (expires_at);

ALTER TABLE access_tokens ADD COLUMN provider TEXT;
ALTER TABLE access_tokens ADD COLUMN id_token TEXT;
```

- [ ] **Step 4: Write the queries**

`internal/infra/storage/sqlc/query/sqlite/oauth.sql` (keep each `;` at the END of the last SQL line of a statement, never alone on its own line, and no comment between a statement and its `;`):

```sql
-- OAuth feature queries: identities, in-flight states, one-shot handoffs.

-- name: GetIdentityByProviderSubject :one
SELECT id, user_id, provider, subject, email, created_at, updated_at
FROM users_identities
WHERE provider = ? AND subject = ?;

-- name: GetIdentityByUserProvider :one
SELECT id, user_id, provider, subject, email, created_at, updated_at
FROM users_identities
WHERE user_id = ? AND provider = ?;

-- name: ListIdentitiesByUser :many
SELECT id, user_id, provider, subject, email, created_at, updated_at
FROM users_identities
WHERE user_id = ?
ORDER BY created_at, id;

-- name: CountIdentitiesByUser :one
SELECT COUNT(*) FROM users_identities WHERE user_id = ?;

-- name: UpsertIdentity :exec
INSERT INTO users_identities (id, user_id, provider, subject, email, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    email      = excluded.email,
    updated_at = excluded.updated_at;

-- name: DeleteIdentityByUserProvider :execrows
DELETE FROM users_identities WHERE user_id = ? AND provider = ?;

-- name: InsertOAuthState :exec
INSERT INTO oauth_states (state_hash, provider, nonce, code_verifier, client, intent, link_user_id, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetOAuthState :one
SELECT state_hash, provider, nonce, code_verifier, client, intent, link_user_id, created_at, expires_at
FROM oauth_states
WHERE state_hash = ?;

-- name: DeleteOAuthState :exec
DELETE FROM oauth_states WHERE state_hash = ?;

-- name: DeleteExpiredOAuthStates :execrows
DELETE FROM oauth_states WHERE expires_at < ?;

-- name: InsertOAuthHandoff :exec
INSERT INTO oauth_handoffs (code_hash, user_id, provider, id_token, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetOAuthHandoff :one
SELECT code_hash, user_id, provider, id_token, created_at, expires_at
FROM oauth_handoffs
WHERE code_hash = ?;

-- name: DeleteOAuthHandoff :exec
DELETE FROM oauth_handoffs WHERE code_hash = ?;

-- name: DeleteExpiredOAuthHandoffs :execrows
DELETE FROM oauth_handoffs WHERE expires_at < ?;
```

`internal/infra/storage/sqlc/query/pgsql/oauth.sql`: the same statements with `$1..$N` placeholders (e.g. `WHERE provider = $1 AND subject = $2`, the state insert `VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`).

In BOTH `access_tokens.sql` files add `provider, id_token` to every column list:
- `InsertAccessToken`: `(…, expires_at, revoked_at, provider, id_token) VALUES (…, ?, ?)` (pgsql: `$11, $12`).
- `GetAccessTokenByHash`: `t.created_at, t.last_used_at, t.expires_at, t.revoked_at, t.provider, t.id_token, u.access_level, u.access_until`.
- `GetAccessTokenByID` and `ListAccessTokensByUser`: `…, expires_at, revoked_at, provider, id_token`.

- [ ] **Step 5: Regenerate and build**

Run: `go generate ./internal/infra/storage/sqlc/... && go build ./...`
Expected: build FAILS only in `internal/user/repo` (`accessTokenRow` struct literal in `tokenRowFromHashRow` now misses fields is fine; the `InsertAccessTokenParams` literal compiles because missing fields default to nil). If the build fails, fix only what the generation changed: `tokenRowFromHashRow` should copy `Provider: row.Provider, IDToken: row.IDToken`. (`sqlc` names the column `IDToken`; check `gen/sqlite/models.go` and use the generated name.)

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/infra/storage/... ./internal/user/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/infra/storage internal/user/repo
git commit -m "feat(oauth): identities, states, handoffs schema and queries

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: Model, DTOs, error codes

**Files:**
- Create: `internal/model/oauth.go`, `internal/model/oauth_dto.go`
- Modify: `internal/model/user.go` (constants at 34-39, struct 101-116, after `NewUser`), `internal/model/token.go` (struct), `internal/model/user_dto.go` (`CurrentUserResult`, `LogoutResult`), `internal/model/token_dto.go` (`SessionItem`), `internal/shared/errs/codes.go`
- Test: `internal/model/oauth_test.go`, `internal/model/user_test.go` (append)

**Interfaces (produces):**

```go
// model
const AlgorithmNone = "none"
func NewPasswordlessUser(id vo.Id, encryptedEmail, name, avatar string, now time.Time) *User
func (u *User) HasPassword() bool
type AccessToken struct { …; Provider *string; IDToken *string }
type SessionItem struct { …; Provider string `json:"provider"` }
type CurrentUserResult struct { …; HasPassword bool `json:"hasPassword"` }
type LogoutResult struct { Result string `json:"result"`; LogoutUrl string `json:"logoutUrl"`; Provider string `json:"provider"` }

const (
	OAuthProviderGoogle = "google"; OAuthProviderApple = "apple"; OAuthProviderOIDC = "oidc"
	OAuthClientWeb = "web"; OAuthClientApp = "app"
	OAuthIntentLogin = "login"; OAuthIntentLink = "link"
	OAuthStateTTL = 10 * time.Minute; OAuthHandoffTTL = 60 * time.Second
)
type Identity struct { ID, UserID vo.Id; Provider, Subject, Email string; CreatedAt, UpdatedAt time.Time }
func NewIdentity(id, userID vo.Id, provider, subject, email string, now time.Time) *Identity
func (i *Identity) UpdateEmail(email string, now time.Time)
type OAuthState struct { StateHash, Provider, Nonce, CodeVerifier, Client, Intent string; LinkUserID vo.Id; CreatedAt, ExpiresAt time.Time }
func (s *OAuthState) IsExpired(now time.Time) bool
type OAuthHandoff struct { CodeHash string; UserID vo.Id; Provider string; IDToken *string; CreatedAt, ExpiresAt time.Time }
func (h *OAuthHandoff) IsExpired(now time.Time) bool
func IsOAuthProvider(id string) bool

// DTOs
type ProviderItem struct { Id string `json:"id"`; Name string `json:"name"` }
type StartOAuthRequest struct { Provider string `json:"provider"`; Client string `json:"client"` } // Validate
type StartOAuthResult struct { Url string `json:"url"` }
type ExchangeHandoffRequest struct { Code string `json:"code"` } // Validate
type IdentityItem struct { Provider string `json:"provider"`; Email string `json:"email"`; CreatedAt string `json:"createdAt"` }
type UnlinkIdentityRequest struct { Provider string `json:"provider"` } // Validate
type UnlinkIdentityResult struct{}

// errs
CodeOAuthProviderNotConfigured = "oauth.provider_not_configured"
CodeOAuthHandoffInvalid        = "oauth.handoff_invalid"
CodeOAuthLastIdentity          = "oauth.last_identity"
CodeOAuthIdentityNotFound      = "oauth.identity_not_found"
```

- [ ] **Step 1: Write the failing tests**

`internal/model/oauth_test.go`:

```go
package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestNewPasswordlessUser(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	u := NewPasswordlessUser(vo.NewId(), "a@example.test", "Alice", "face:sky", now)
	if u.Algorithm != AlgorithmNone || u.Password != "" || u.Salt != "" || !u.EmailVerified || !u.IsActive {
		t.Fatalf("unexpected passwordless user: %+v", u)
	}
	if u.HasPassword() {
		t.Fatal("passwordless user must report no password")
	}
	u.UpdatePassword("hash", AlgorithmArgon2id, now)
	if !u.HasPassword() {
		t.Fatal("after UpdatePassword the user has a password")
	}
}

func TestOAuthStateAndHandoffExpiry(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	s := OAuthState{ExpiresAt: now.Add(OAuthStateTTL)}
	if s.IsExpired(now) || !s.IsExpired(now.Add(OAuthStateTTL)) {
		t.Fatal("state expiry is exclusive of ExpiresAt")
	}
	h := OAuthHandoff{ExpiresAt: now.Add(OAuthHandoffTTL)}
	if h.IsExpired(now) || !h.IsExpired(now.Add(OAuthHandoffTTL)) {
		t.Fatal("handoff expiry is exclusive of ExpiresAt")
	}
}

func TestStartOAuthRequestValidate(t *testing.T) {
	cases := []struct {
		req  StartOAuthRequest
		want string // field key of the first error, "" when valid
	}{
		{StartOAuthRequest{Provider: "google", Client: "web"}, ""},
		{StartOAuthRequest{Provider: "oidc", Client: "app"}, ""},
		{StartOAuthRequest{Provider: "github", Client: "web"}, "provider"},
		{StartOAuthRequest{Provider: "", Client: "web"}, "provider"},
		{StartOAuthRequest{Provider: "google", Client: "desktop"}, "client"},
	}
	for _, c := range cases {
		err := c.req.Validate()
		if c.want == "" {
			if err != nil {
				t.Fatalf("%+v: unexpected %v", c.req, err)
			}
			continue
		}
		v, ok := errs.AsValidation(err)
		if !ok || len(v.Fields) == 0 || v.Fields[0].Key != c.want {
			t.Fatalf("%+v: want field %q error, got %v", c.req, c.want, err)
		}
	}
	if err := (ExchangeHandoffRequest{}).Validate(); err == nil {
		t.Fatal("blank code must fail")
	}
	if err := (UnlinkIdentityRequest{Provider: "apple"}).Validate(); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/model/ -run 'Passwordless|OAuthState|StartOAuth' -v`
Expected: compile errors.

- [ ] **Step 3: Implement**

`internal/model/user.go`: extend the constants block and add after `NewUser`:

```go
const (
	AlgorithmSHA512   = "sha512"
	AlgorithmArgon2id = "argon2id"
	// AlgorithmNone marks a user provisioned through an external identity who has
	// never set a password: the hasher fails closed on it, so password login
	// yields the ordinary "Invalid credentials." until the reset flow writes a hash.
	AlgorithmNone = "none"
)

// NewPasswordlessUser constructs a user provisioned from a verified external
// identity (OAuth/OIDC). No hash, no salt, algorithm "none"; the email counts as
// verified because the callback rejected unverified claims before reaching here.
func NewPasswordlessUser(id vo.Id, encryptedEmail, name, avatar string, now time.Time) *User {
	u := NewUser(id, encryptedEmail, name, avatar, "", "", now)
	u.Algorithm = AlgorithmNone
	return u
}

func (u *User) HasPassword() bool { return u.Algorithm != AlgorithmNone }
```

`internal/model/token.go`: add to `AccessToken` after `RevokedAt`:

```go
	// Provider is the OAuth provider id a session was minted through ("" or nil
	// for password logins); IDToken is kept only for the custom OIDC slot so
	// logout can send id_token_hint. Both nil for PATs.
	Provider *string
	IDToken  *string
```

`internal/model/token_dto.go`: add `Provider string \`json:"provider"\`` to `SessionItem` after `LastUsedAt` (before `IsCurrent`).

`internal/model/user_dto.go`: add `HasPassword bool \`json:"hasPassword"\`` to `CurrentUserResult` after `AccessUntil`; replace `LogoutResult` with:

```go
// LogoutResult is the logout response. The wire shape keeps the frozen
// {"result":"test"} quirk; logoutUrl/provider are additive: logoutUrl is the
// IdP's end-session URL to navigate to ("" when none), provider the session's
// OAuth provider id ("" for password sessions) so the client can name it.
type LogoutResult struct {
	Result    string `json:"result"`
	LogoutUrl string `json:"logoutUrl"`
	Provider  string `json:"provider"`
}
```

`internal/model/oauth.go`:

```go
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	OAuthProviderGoogle = "google"
	OAuthProviderApple  = "apple"
	OAuthProviderOIDC   = "oidc"

	OAuthClientWeb = "web"
	OAuthClientApp = "app"

	OAuthIntentLogin = "login"
	OAuthIntentLink  = "link"

	OAuthStateTTL   = 10 * time.Minute
	OAuthHandoffTTL = 60 * time.Second
)

// OAuthProviders is the fixed display order of the provider slots.
var OAuthProviders = []string{OAuthProviderGoogle, OAuthProviderApple, OAuthProviderOIDC}

func IsOAuthProvider(id string) bool {
	for _, p := range OAuthProviders {
		if p == id {
			return true
		}
	}
	return false
}

// Identity binds an external subject to a user. (provider, subject) is the
// key; email is the last value seen, display only.
type Identity struct {
	ID        vo.Id
	UserID    vo.Id
	Provider  string
	Subject   string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewIdentity(id, userID vo.Id, provider, subject, email string, now time.Time) *Identity {
	return &Identity{ID: id, UserID: userID, Provider: provider, Subject: subject, Email: email, CreatedAt: now, UpdatedAt: now}
}

func (i *Identity) UpdateEmail(email string, now time.Time) {
	if i.Email == email {
		return
	}
	i.Email = email
	i.UpdatedAt = now
}

// OAuthState is one in-flight authorization request, keyed by the sha256 of the
// state parameter; consumed (deleted) on the first callback that presents it.
type OAuthState struct {
	StateHash    string
	Provider     string
	Nonce        string
	CodeVerifier string
	Client       string
	Intent       string
	LinkUserID   vo.Id // zero unless Intent == OAuthIntentLink
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

func (s *OAuthState) IsExpired(now time.Time) bool { return !now.Before(s.ExpiresAt) }

// OAuthHandoff is the one-shot code the client exchanges for a session.
type OAuthHandoff struct {
	CodeHash  string
	UserID    vo.Id
	Provider  string
	IDToken   *string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (h *OAuthHandoff) IsExpired(now time.Time) bool { return !now.Before(h.ExpiresAt) }
```

`internal/model/oauth_dto.go`:

```go
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

type ProviderItem struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type StartOAuthRequest struct {
	Provider string `json:"provider"`
	Client   string `json:"client"`
}

func (r StartOAuthRequest) Validate() error {
	var fields []errs.FieldError
	if !IsOAuthProvider(r.Provider) {
		fields = append(fields, errs.FieldError{Key: "provider", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	if r.Client != OAuthClientWeb && r.Client != OAuthClientApp {
		fields = append(fields, errs.FieldError{Key: "client", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type StartOAuthResult struct {
	Url string `json:"url"`
}

type ExchangeHandoffRequest struct {
	Code string `json:"code"`
}

func (r ExchangeHandoffRequest) Validate() error {
	if strings.TrimSpace(r.Code) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "code", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type IdentityItem struct {
	Provider  string `json:"provider"`
	Email     string `json:"email"`
	CreatedAt string `json:"createdAt"`
}

type UnlinkIdentityRequest struct {
	Provider string `json:"provider"`
}

func (r UnlinkIdentityRequest) Validate() error {
	if !IsOAuthProvider(r.Provider) {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "provider", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	return nil
}

type UnlinkIdentityResult struct{}
```

Check the exact "invalid choice" English string used elsewhere with `grep -rn CodeInvalidChoice internal/model | head -3` and copy it verbatim.

`internal/shared/errs/codes.go`: add to the const block after `CodeUserEmailUnchanged`:

```go
	CodeOAuthProviderNotConfigured = "oauth.provider_not_configured"
	CodeOAuthHandoffInvalid        = "oauth.handoff_invalid"
	CodeOAuthLastIdentity          = "oauth.last_identity"
	CodeOAuthIdentityNotFound      = "oauth.identity_not_found"
```

and append the four to `AllCodes` after `CodeUserEmailUnchanged`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/model/ ./internal/shared/...`
Expected: PASS. (`internal/test/i18ntest` will fail until Task 12 adds the catalogue keys; that is expected and tracked there.)

- [ ] **Step 5: Commit**

```bash
git add internal/model internal/shared/errs
git commit -m "feat(oauth): model, DTOs and error codes

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 4: Access-token persistence of provider/id_token + fixture support

**Files:**
- Modify: `internal/user/repo/accesstoken.go` (`Insert`, `tokenRowFromHashRow`, `accessTokenFromRow`)
- Modify: `internal/test/fixture/entities.go` (`User` gains `Algorithm`; new `Identity` seed)
- Test: `internal/user/repo/accesstoken_integration_test.go` (append)

**Interfaces:**
- Produces: `fixture.User.Algorithm string` (default `sha512`), `fixture.Identity{ID, UserID, Provider, Subject, Email string}` + `(*Builder).Identity(Identity) string`.

- [ ] **Step 1: Write the failing test**

Append to `internal/user/repo/accesstoken_integration_test.go` (match the file's existing helper names for opening the db and seeding a user; the pattern is `dbtest.New(t)` + `fixture.New(t, db).User(fixture.User{…})`):

```go
func TestAccessTokenRepo_ProviderAndIDTokenRoundTrip(t *testing.T) {
	db := dbtest.New(t)
	userID := fixture.New(t, db).User(fixture.User{})
	repo := NewAccessTokenRepo(db.Engine, db.TX)
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	provider, idTok := "oidc", "eyJ.header.sig"
	exp := now.Add(time.Hour)
	tok := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userID), Kind: model.TokenKindSession,
		TokenHash: "h-provider", CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp,
		Provider: &provider, IDToken: &idTok,
	}
	if err := repo.Insert(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(context.Background(), tok.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider == nil || *got.Provider != "oidc" || got.IDToken == nil || *got.IDToken != idTok {
		t.Fatalf("provider/id_token not persisted: %+v", got)
	}
	byHash, _, _, err := repo.GetByHash(context.Background(), "h-provider")
	if err != nil || byHash.Provider == nil || *byHash.Provider != "oidc" {
		t.Fatalf("GetByHash must carry provider: %+v %v", byHash, err)
	}
	list, err := repo.ListByUser(context.Background(), tok.UserID, model.TokenKindSession)
	if err != nil || len(list) != 1 || list[0].Provider == nil {
		t.Fatalf("ListByUser must carry provider: %+v %v", list, err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/user/repo/ -run ProviderAndIDToken -v`
Expected: FAIL ("provider/id_token not persisted").

- [ ] **Step 3: Implement**

In `internal/user/repo/accesstoken.go`:

```go
func (r *AccessTokenRepo) Insert(ctx context.Context, t *model.AccessToken) error {
	return r.q.InsertAccessToken(ctx, r.db(ctx), insertAccessTokenParams{
		ID: t.ID.String(), UserID: t.UserID.String(), Kind: t.Kind, TokenHash: t.TokenHash,
		Name: t.Name, UserAgent: t.UserAgent,
		CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt,
		Provider: t.Provider, IDToken: t.IDToken,
	})
}
```

Add `Provider: row.Provider, IDToken: row.IDToken` to both `tokenRowFromHashRow` and `accessTokenFromRow` (use the generated field names from `gen/sqlite/models.go`; sqlc renders `id_token` as `IDToken`).

`internal/test/fixture/entities.go`: add `Algorithm string // default "sha512"; "none" for a passwordless user` to `User`, and in `(*Builder).User` replace the literal `'sha512'` in the INSERT with a bound `?` carrying `orDefault(u.Algorithm, "sha512")` (write a tiny helper if none exists: `func orDefault(v, d string) string { if v == "" { return d }; return v }`). Then add:

```go
// Identity seeds one users_identities row.
type Identity struct {
	ID       string
	UserID   string
	Provider string // "google" | "apple" | "oidc"
	Subject  string
	Email    string
}

func (b *Builder) Identity(i Identity) string {
	b.t.Helper()
	id := b.orNewID(i.ID)
	now := b.now()
	b.insert(`INSERT INTO users_identities (id, user_id, provider, subject, email, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, i.UserID, i.Provider, i.Subject, i.Email, now, now)
	return id
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/user/... ./internal/test/fixture/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/user/repo internal/test/fixture
git commit -m "feat(oauth): persist session provider and id_token; identity fixture

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: User-feature additions (provision, external session, email mirror, logout URL, hasPassword)

**Files:**
- Create: `internal/user/external.go`
- Modify: `internal/user/register.go` (`createUser` refactor), `internal/user/session.go` (`createSession`, `ListSessions`), `internal/user/login.go` (call site), `internal/user/usecase.go` (`Logout`, `toCurrentUserWithEmail`, new field + setter), `internal/user/ports.go` (`LogoutURLBuilder`), `internal/user/read.go` + `internal/user/repo/read.go` + `internal/model/user_view.go` + `query/{sqlite,pgsql}/user_read.sql` (`algorithm` in the user view), `internal/cli/user_commands.go`
- Test: `internal/user/external_test.go`, `internal/user/login_test.go` (append), `internal/user/session_cascade_test.go` or a new `internal/user/logout_test.go`

**Interfaces (produces):**

```go
// internal/user
type LogoutURLBuilder interface {
	// EndSessionURL returns the IdP end-session URL for a session minted through
	// provider with the given ID token, or "" when the provider supports none.
	EndSessionURL(ctx context.Context, provider, idToken string) (string, error)
}
func (s *Service) SetLogoutURLBuilder(b LogoutURLBuilder)
func (s *Service) ProvisionExternalUser(ctx context.Context, name, email string) (*model.User, error)
func (s *Service) CreateExternalSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string) (*model.LoginResult, error)
func (s *Service) ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string) error
func (s *Service) GetByID(ctx context.Context, id vo.Id) (*model.User, error)      // thin wrapper over repo.GetByID (check it exists; add if not)
func (s *Service) GetByEmail(ctx context.Context, email string) (*model.User, error) // thin wrapper over repo.GetByEmail
```

- [ ] **Step 1: Write the failing tests**

Look at `internal/user/register_test.go` for how a `*Service` is built in-package with fakes (there is a helper constructing `NewService` with a memory repo or the sqlite dbtest; reuse it — call it `newTestService(t)` below, substituting the file's real name). Create `internal/user/external_test.go`:

```go
package user

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func TestProvisionExternalUser_PasswordlessVerifiedWithDefaults(t *testing.T) {
	s := newTestService(t)
	u, err := s.ProvisionExternalUser(context.Background(), "Alice", "Alice@Example.test")
	if err != nil {
		t.Fatal(err)
	}
	if u.Algorithm != model.AlgorithmNone || u.HasPassword() || !u.EmailVerified {
		t.Fatalf("want passwordless verified user, got %+v", u)
	}
	if u.Option(model.OptionCurrency) == nil || u.Option(model.OptionAnalytics) == nil {
		t.Fatal("default options must be seeded like registration")
	}
	if _, err := s.ProvisionExternalUser(context.Background(), "Alice", "alice@example.test"); err == nil {
		t.Fatal("duplicate email must fail")
	}
	// Password login on a passwordless user is the frozen 401.
	_, lerr := s.Login(context.Background(), model.LoginRequest{Username: "alice@example.test", Password: "anything"}, "ua", s.clock.Now())
	if _, ok := errs.AsUnauthorized(lerr); !ok {
		t.Fatalf("want 401, got %v", lerr)
	}
}

func TestCreateExternalSession_StampsProviderAndReturnsLogin(t *testing.T) {
	s := newTestService(t)
	u, err := s.ProvisionExternalUser(context.Background(), "Bob", "bob@example.test")
	if err != nil {
		t.Fatal(err)
	}
	idTok := "raw.id.token"
	res, err := s.CreateExternalSession(context.Background(), u.ID, "Mozilla/5.0", model.OAuthProviderOIDC, &idTok)
	if err != nil {
		t.Fatal(err)
	}
	if res.Token == "" || res.User.Id != u.ID.String() || res.User.HasPassword {
		t.Fatalf("unexpected login result %+v", res)
	}
	sessions, err := s.ListSessions(context.Background(), u.ID, vo.Id{})
	if err != nil || len(sessions) != 1 || sessions[0].Provider != "oidc" {
		t.Fatalf("session must carry provider: %+v %v", sessions, err)
	}
	u.Deactivate(s.clock.Now())
	if err := s.repo.Save(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateExternalSession(context.Background(), u.ID, "ua", model.OAuthProviderOIDC, nil); err == nil {
		t.Fatal("inactive user must not get a session")
	}
}

func TestReplaceVerifiedEmail(t *testing.T) {
	s := newTestService(t)
	u, _ := s.ProvisionExternalUser(context.Background(), "Carol", "carol@old.test")
	if err := s.ReplaceVerifiedEmail(context.Background(), u.ID, "carol@new.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.repo.GetByEmail(context.Background(), "carol@new.test"); err != nil {
		t.Fatalf("new email must resolve: %v", err)
	}
}

type fakeLogoutURLs struct{ url string }

func (f fakeLogoutURLs) EndSessionURL(_ context.Context, provider, idToken string) (string, error) {
	if provider == "oidc" && idToken != "" {
		return f.url, nil
	}
	return "", nil
}

func TestLogout_ReturnsEndSessionURLForOIDCSessions(t *testing.T) {
	s := newTestService(t)
	s.SetLogoutURLBuilder(fakeLogoutURLs{url: "https://idp.example.test/end?x=1"})
	u, _ := s.ProvisionExternalUser(context.Background(), "Dan", "dan@example.test")
	idTok := "t"
	res, _ := s.CreateExternalSession(context.Background(), u.ID, "ua", "oidc", &idTok)
	_, tid, _, err := s.Authenticate(context.Background(), res.Token)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Logout(context.Background(), tid)
	if err != nil || out.LogoutUrl != "https://idp.example.test/end?x=1" || out.Provider != "oidc" || out.Result != "test" {
		t.Fatalf("logout result %+v %v", out, err)
	}
	// A Google session (no id token) logs out locally but still names the provider.
	res2, _ := s.CreateExternalSession(context.Background(), u.ID, "ua", "google", nil)
	_, tid2, _, _ := s.Authenticate(context.Background(), res2.Token)
	out2, _ := s.Logout(context.Background(), tid2)
	if out2.LogoutUrl != "" || out2.Provider != "google" {
		t.Fatalf("google logout %+v", out2)
	}
}
```

`Authenticate` returns `(userID, tokenID, level, err)` (`authenticate.go:13`). Add `"github.com/econumo/econumo/internal/shared/vo"` to the test imports.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/user/ -run 'External|ReplaceVerified|Logout_Returns' -v`
Expected: compile errors.

- [ ] **Step 3: Implement**

`internal/user/ports.go` — append:

```go
// LogoutURLBuilder is the oauth feature's end-session capability, consumed by
// Logout for sessions minted through the custom OIDC slot. nil disables it.
type LogoutURLBuilder interface {
	EndSessionURL(ctx context.Context, provider, idToken string) (string, error)
}
```

`internal/user/usecase.go` — add field `logoutURLs LogoutURLBuilder` to `Service` and:

```go
// SetLogoutURLBuilder installs the oauth feature's end-session adapter after
// construction (the composition root wires the two features in either order).
func (s *Service) SetLogoutURLBuilder(b LogoutURLBuilder) { s.logoutURLs = b }
```

Replace `Logout`:

```go
func (s *Service) Logout(ctx context.Context, tokenID vo.Id) (*model.LogoutResult, error) {
	out := &model.LogoutResult{Result: "test"}
	t, err := s.tokens.GetByID(ctx, tokenID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			// Already gone: logout is idempotent.
			return out, nil
		}
		return nil, err
	}
	t.Revoke(s.clock.Now())
	if err := s.tokens.Update(ctx, t); err != nil {
		return nil, err
	}
	if t.Provider != nil {
		out.Provider = *t.Provider
	}
	// The local revocation above always happens first: a client that ignores
	// the URL still ends its Econumo session.
	if s.logoutURLs != nil && t.Provider != nil && t.IDToken != nil {
		url, uerr := s.logoutURLs.EndSessionURL(ctx, *t.Provider, *t.IDToken)
		if uerr != nil {
			slog.WarnContext(ctx, "end-session url unavailable", "err", uerr.Error())
		} else {
			out.LogoutUrl = url
		}
	}
	return out, nil
}
```

In `toCurrentUserWithEmail` add `HasPassword: u.HasPassword(),` to the returned struct.

`internal/user/session.go` — change `createSession` to take the provider:

```go
func (s *Service) createSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string, now time.Time) (string, error) {
	raw, hash, err := generateAccessToken(model.TokenKindSession)
	if err != nil {
		return "", err
	}
	exp := now.Add(SessionTTL)
	t := &model.AccessToken{
		ID: vo.NewId(), UserID: userID, Kind: model.TokenKindSession, TokenHash: hash,
		CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp, IDToken: idToken,
	}
	if userAgent != "" {
		t.UserAgent = &userAgent
	}
	if provider != "" {
		t.Provider = &provider
	}
	if err := s.tokens.Insert(ctx, t); err != nil {
		return "", err
	}
	return raw, nil
}
```

Update the call in `login.go` to `s.createSession(ctx, u.ID, userAgent, "", nil, now)`. In `ListSessions` add:

```go
		provider := ""
		if rows[i].Provider != nil {
			provider = *rows[i].Provider
		}
```
and `Provider: provider,` in the `SessionItem` literal.

`internal/user/register.go` — split `createUser` so both paths share one assembly:

```go
func (s *Service) createUser(ctx context.Context, name, email, password string, selfService bool) (*model.User, error) {
	salt, serr := newSalt()
	if serr != nil {
		return nil, serr
	}
	passwordHash, herr := s.hasher.Hash(password)
	if herr != nil {
		return nil, herr
	}
	return s.persistNewUser(ctx, name, email, func(id vo.Id, encryptedEmail, avatar string, now time.Time) *model.User {
		return model.NewUser(id, encryptedEmail, name, avatar, passwordHash, salt, now)
	}, selfService, selfService && s.emailVerification)
}

// persistNewUser is the assembly shared by password registration and external
// provisioning: uniqueness, email encoding, default options, analytics
// default, resolved default currency, the self-service trial, and the
// verification gate. build constructs the bare aggregate for the caller's scheme.
func (s *Service) persistNewUser(ctx context.Context, name, email string,
	build func(id vo.Id, encryptedEmail, avatar string, now time.Time) *model.User,
	selfService, requireVerification bool,
) (*model.User, error) {
	loweredEmail := strings.ToLower(strings.TrimSpace(email))
	exists, err := s.repo.ExistsByEmail(ctx, loweredEmail)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, &errs.ValidationError{Msg: "User already exists", MsgCode: errs.CodeUserAlreadyExists}
	}
	encryptedEmail, eerr := s.encode.Encode(strings.TrimSpace(email))
	if eerr != nil {
		return nil, eerr
	}
	now := s.clock.Now()
	u := build(s.repo.NextIdentity(), encryptedEmail, s.avatars.Pick(), now)
	u.SeedDefaultOptions(s.repo.NextIdentity, now)
	u.SetAnalytics(true, s.repo.NextIdentity(), now)
	defaultID, cerr := s.currency.GetIDByCode(ctx, u.ID.String(), s.currency.DefaultCode())
	if cerr != nil {
		return nil, cerr
	}
	u.UpdateCurrency(defaultID, now)
	if selfService && s.trialDays > 0 {
		until := model.TrialEnd(now, s.trialDays)
		u.SetAccess(model.AccessLevelFull, &until, now)
	}
	if requireVerification {
		u.RequireEmailVerification()
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error { return s.repo.Save(ctx, u) }); err != nil {
		return nil, err
	}
	return u, nil
}
```

Keep the existing doc comments' content on `createUser`/`persistNewUser` (trim to what still applies).

`internal/user/external.go`:

```go
// External-identity entry points consumed by the oauth feature through its
// ports (wired in internal/server). They deliberately mirror Register/Login:
// same defaults, same trial, same session shape — only the credential differs.
package user

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ProvisionExternalUser creates a passwordless, email-verified user from a
// provider identity. The caller (the oauth callback) has already established
// the email is verified or trusted and that registration is allowed.
func (s *Service) ProvisionExternalUser(ctx context.Context, name, email string) (*model.User, error) {
	return s.persistNewUser(ctx, name, email, func(id vo.Id, encryptedEmail, avatar string, now time.Time) *model.User {
		return model.NewPasswordlessUser(id, encryptedEmail, name, avatar, now)
	}, true, false)
}

// CreateExternalSession mints a session for a user resolved by the oauth
// feature, stamping the provider (and, for the custom slot, the ID token for
// RP-initiated logout). Inactive users are refused like a password login.
func (s *Service) CreateExternalSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string) (*model.LoginResult, error) {
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !u.IsActive {
		return nil, &errs.UnauthorizedError{Msg: "Invalid credentials.", Code: errs.CodeInvalidCredentials}
	}
	now := s.clock.Now()
	if err := s.purgeDeadTokens(ctx, u.ID, now); err != nil {
		return nil, err
	}
	token, terr := s.createSession(ctx, u.ID, userAgent, provider, idToken, now)
	if terr != nil {
		return nil, terr
	}
	cur, cerr := s.toCurrentUser(ctx, u)
	if cerr != nil {
		return nil, cerr
	}
	// Best-effort, exactly like Login.
	_ = s.repo.UpdateLanguage(ctx, u.ID, reqctx.Language(ctx))
	return &model.LoginResult{Token: token, User: cur}, nil
}

// ReplaceVerifiedEmail mirrors an IdP-side email change onto the primary email
// (the oauth feature applies the eligibility rule: passwordless, one identity,
// address unused). The new address counts as verified.
func (s *Service) ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string) error {
	encrypted, err := s.encode.Encode(strings.TrimSpace(email))
	if err != nil {
		return err
	}
	_, err = s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
		u.UpdateEmail(encrypted, now)
		u.MarkEmailVerified(now)
		return nil
	})
	return err
}

func (s *Service) GetByID(ctx context.Context, id vo.Id) (*model.User, error) { return s.repo.GetByID(ctx, id) }

func (s *Service) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	return s.repo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
}
```

(Imports: `strings`, `time` in addition to those shown. `mutate` is `func (s *Service) mutate(ctx context.Context, userID vo.Id, fn func(u *model.User, now time.Time) error) (*model.User, error)` at usecase.go:115; `Repository.GetByID` exists at repository.go:20.)

Read side: add `algorithm` to `GetUserView` in both `user_read.sql` files, `Algorithm string` to `model.UserViewRow`, map it in `internal/user/repo/read.go`, regenerate sqlc, and set `HasPassword: u.Algorithm != model.AlgorithmNone` in `currentUser` (`read.go`).

CLI: in `user:show` add after the `Email verified` line:

```go
				password := "set"
				if !u.HasPassword() {
					password = "none"
				}
				fmt.Printf("Password:        %s\n", password)
```

- [ ] **Step 4: Run tests**

Run: `go generate ./internal/infra/storage/sqlc/... && go test ./internal/user/... ./internal/cli/... ./internal/model/...`
Expected: PASS. (`internal/test/apiparity` goldens will change because `hasPassword`, `logoutUrl`, `provider` are new wire fields; regenerate them in Task 11, not here.)

- [ ] **Step 5: Commit**

```bash
git add internal/user internal/cli internal/model internal/infra/storage
git commit -m "feat(oauth): user provisioning, external sessions, logout URL, hasPassword

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: Protocol package `internal/infra/oidc`

**Files:**
- Create: `internal/infra/oidc/random.go`, `pkce.go`, `jwt.go`, `jwks.go`, `client.go`, `apple.go`, `oidctest/fake.go`
- Test: `internal/infra/oidc/random_test.go`, `jwt_test.go`, `client_test.go`, `apple_test.go`

**Interfaces (produces):**

```go
package oidc

func RandomToken() (string, error)                 // 32 bytes, base64url no padding (43 chars)
func Sha256Hex(s string) string
func PKCEChallenge(verifier string) string          // S256, base64url no padding

type Issuer struct {
	ID           string
	IssuerURL    string // trailing slash trimmed
	ClientID     string
	ClientSecret func(now time.Time) (string, error)
	Scopes       []string
	UsePKCE      bool
	ResponseMode string // "" | "form_post"
	TrustEmail   bool
	ExtraAuthParams map[string]string // e.g. Google prompt=select_account
}

type Discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

type Claims struct { Subject, Email, Name string; EmailVerified bool }
type Tokens struct { IDToken, AccessToken string }

type Client struct { /* issuer, http client, cached discovery + JWKS */ }
func NewClient(issuer Issuer, hc *http.Client) *Client
func (c *Client) Issuer() Issuer
func (c *Client) Discover(ctx context.Context) (Discovery, error)
func (c *Client) AuthURL(ctx context.Context, state, nonce, codeChallenge, redirectURI string) (string, error)
func (c *Client) Exchange(ctx context.Context, code, codeVerifier, redirectURI string, now time.Time) (Tokens, error)
func (c *Client) VerifyIDToken(ctx context.Context, raw, nonce string, now time.Time) (Claims, error)
func (c *Client) UserInfo(ctx context.Context, accessToken string) (Claims, error)
func (c *Client) EndSessionURL(ctx context.Context, idToken, postLogoutRedirectURI string) (string, error) // "" when unsupported

func ParseApplePrivateKey(pemText string) (*ecdsa.PrivateKey, error)
func AppleClientSecret(teamID, clientID, keyID string, key *ecdsa.PrivateKey, now time.Time) (string, error)
var ErrInvalidToken = errors.New("oidc: invalid id token")

package oidctest
type Fake struct { Server *httptest.Server; ClientID string; Subject, Email, Name string; EmailVerified any /* nil=omit, true/false, "true" */; UserInfoEmail string; RequirePKCE bool; EndSession bool; NoUserInfo bool; … }
func New(t testing.TB) *Fake                        // RSA-2048 key, all endpoints
func (f *Fake) IssuerURL() string
func (f *Fake) Issuer(id string, trust bool) oidc.Issuer   // static secret "fake-secret"
func (f *Fake) IssueCode(nonce, codeChallenge string) string // simulate the user consenting at /authorize
func (f *Fake) SignIDToken(claims map[string]any) string   // RS256 with the fake key, kid "k1"
func (f *Fake) RotateKey(t testing.TB)                     // new key, new kid: exercises the JWKS re-fetch
func (f *Fake) Requests() []string                         // paths hit, for assertions
```

- [ ] **Step 1: Write the failing tests**

`internal/infra/oidc/random_test.go`:

```go
package oidc

import (
	"regexp"
	"testing"
)

func TestRandomTokenShape(t *testing.T) {
	re := regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		s, err := RandomToken()
		if err != nil || !re.MatchString(s) {
			t.Fatalf("token %q err %v", s, err)
		}
		if seen[s] {
			t.Fatal("duplicate token")
		}
		seen[s] = true
	}
}

func TestPKCEChallengeRFCVector(t *testing.T) {
	// RFC 7636 appendix B.
	got := PKCEChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("challenge = %s", got)
	}
}
```

`internal/infra/oidc/jwt_test.go` (uses the fake from `oidctest`, which lives in a subpackage to avoid an import cycle in tests — put the tests in `package oidc_test`):

```go
package oidc_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/oidc/oidctest"
)

func TestVerifyIDToken_HappyAndRejections(t *testing.T) {
	f := oidctest.New(t)
	now := time.Now()
	c := oidc.NewClient(f.Issuer("oidc", false), nil)
	base := func() map[string]any {
		return map[string]any{
			"iss": f.IssuerURL(), "aud": f.ClientID, "sub": "user-1", "email": "a@example.test",
			"email_verified": true, "name": "Alice", "nonce": "n1",
			"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
		}
	}
	claims, err := c.VerifyIDToken(context.Background(), f.SignIDToken(base()), "n1", now)
	if err != nil || claims.Subject != "user-1" || claims.Email != "a@example.test" || !claims.EmailVerified || claims.Name != "Alice" {
		t.Fatalf("claims %+v err %v", claims, err)
	}

	cases := map[string]func(m map[string]any){
		"wrong issuer":   func(m map[string]any) { m["iss"] = "https://evil.example" },
		"wrong audience": func(m map[string]any) { m["aud"] = "other-client" },
		"expired":        func(m map[string]any) { m["exp"] = now.Add(-2 * time.Minute).Unix() },
		"future iat":     func(m map[string]any) { m["iat"] = now.Add(5 * time.Minute).Unix() },
		"bad nonce":      func(m map[string]any) { m["nonce"] = "other" },
	}
	for name, mut := range cases {
		m := base()
		mut(m)
		if _, err := c.VerifyIDToken(context.Background(), f.SignIDToken(m), "n1", now); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
	// Within skew: exp 30s ago is accepted.
	m := base()
	m["exp"] = now.Add(-30 * time.Second).Unix()
	if _, err := c.VerifyIDToken(context.Background(), f.SignIDToken(m), "n1", now); err != nil {
		t.Fatalf("30s skew must pass: %v", err)
	}
	// aud as an array containing the client id is accepted.
	m = base()
	m["aud"] = []string{"x", f.ClientID}
	if _, err := c.VerifyIDToken(context.Background(), f.SignIDToken(m), "n1", now); err != nil {
		t.Fatalf("aud array must pass: %v", err)
	}
	// email_verified as the string "true" (Apple) is accepted.
	m = base()
	m["email_verified"] = "true"
	cl, err := c.VerifyIDToken(context.Background(), f.SignIDToken(m), "n1", now)
	if err != nil || !cl.EmailVerified {
		t.Fatalf("string email_verified: %+v %v", cl, err)
	}
}

func TestVerifyIDToken_SignatureAndKeyRotation(t *testing.T) {
	f := oidctest.New(t)
	now := time.Now()
	c := oidc.NewClient(f.Issuer("oidc", false), nil)
	claims := map[string]any{"iss": f.IssuerURL(), "aud": f.ClientID, "sub": "s", "nonce": "n", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix()}
	tok := f.SignIDToken(claims)
	if _, err := c.VerifyIDToken(context.Background(), tok, "n", now); err != nil {
		t.Fatal(err)
	}
	// Tamper with the payload: signature no longer matches.
	if _, err := c.VerifyIDToken(context.Background(), tok[:len(tok)-4]+"AAAA", "n", now); err == nil {
		t.Fatal("tampered signature must fail")
	}
	// Rotate the key: unknown kid triggers one JWKS refresh and succeeds.
	f.RotateKey(t)
	tok2 := f.SignIDToken(claims)
	if _, err := c.VerifyIDToken(context.Background(), tok2, "n", now); err != nil {
		t.Fatalf("after rotation: %v", err)
	}
	// An unsupported alg is rejected even if a key matches.
	if _, err := c.VerifyIDToken(context.Background(), f.SignWithAlg("none", claims), "n", now); err == nil {
		t.Fatal("alg none must fail")
	}
}
```

`internal/infra/oidc/client_test.go`:

```go
package oidc_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/oidc/oidctest"
)

func TestClient_AuthURLExchangeUserInfoEndSession(t *testing.T) {
	f := oidctest.New(t)
	f.RequirePKCE = true
	f.EndSession = true
	iss := f.Issuer("oidc", false)
	iss.ExtraAuthParams = map[string]string{"prompt": "select_account"}
	c := oidc.NewClient(iss, nil)

	verifier, _ := oidc.RandomToken()
	raw, err := c.AuthURL(context.Background(), "st", "no", oidc.PKCEChallenge(verifier), "https://app.example.test/cb")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	if !strings.HasPrefix(raw, f.IssuerURL()+"/authorize?") || q.Get("response_type") != "code" || q.Get("client_id") != f.ClientID ||
		q.Get("redirect_uri") != "https://app.example.test/cb" || q.Get("scope") != "openid profile email" || q.Get("state") != "st" ||
		q.Get("nonce") != "no" || q.Get("code_challenge") != oidc.PKCEChallenge(verifier) || q.Get("code_challenge_method") != "S256" ||
		q.Get("prompt") != "select_account" {
		t.Fatalf("auth url %s", raw)
	}

	code := f.IssueCode("no", q.Get("code_challenge"))
	toks, err := c.Exchange(context.Background(), code, verifier, "https://app.example.test/cb", time.Now())
	if err != nil || toks.IDToken == "" || toks.AccessToken == "" {
		t.Fatalf("exchange %+v %v", toks, err)
	}
	if _, err := c.Exchange(context.Background(), code, "wrong-verifier", "https://app.example.test/cb", time.Now()); err == nil {
		t.Fatal("wrong verifier must fail")
	}

	f.UserInfoEmail = "info@example.test"
	info, err := c.UserInfo(context.Background(), toks.AccessToken)
	if err != nil || info.Email != "info@example.test" || info.Subject != f.Subject {
		t.Fatalf("userinfo %+v %v", info, err)
	}

	end, err := c.EndSessionURL(context.Background(), toks.IDToken, "https://app.example.test/login")
	if err != nil {
		t.Fatal(err)
	}
	eu, _ := url.Parse(end)
	if !strings.HasPrefix(end, f.IssuerURL()+"/end-session?") || eu.Query().Get("id_token_hint") != toks.IDToken ||
		eu.Query().Get("client_id") != f.ClientID || eu.Query().Get("post_logout_redirect_uri") != "https://app.example.test/login" {
		t.Fatalf("end session url %s", end)
	}
}

func TestClient_EndSessionEmptyWhenUnsupported(t *testing.T) {
	f := oidctest.New(t) // EndSession false by default
	c := oidc.NewClient(f.Issuer("google", true), nil)
	if u, err := c.EndSessionURL(context.Background(), "x", "https://a/"); err != nil || u != "" {
		t.Fatalf("want empty, got %q %v", u, err)
	}
}

func TestClient_FormPostAndNoPKCE(t *testing.T) {
	f := oidctest.New(t)
	iss := f.Issuer("apple", true)
	iss.UsePKCE = false
	iss.ResponseMode = "form_post"
	iss.Scopes = []string{"name", "email"}
	c := oidc.NewClient(iss, nil)
	raw, _ := c.AuthURL(context.Background(), "s", "n", "", "https://a/cb")
	u, _ := url.Parse(raw)
	if u.Query().Get("response_mode") != "form_post" || u.Query().Has("code_challenge") || u.Query().Get("scope") != "name email" {
		t.Fatalf("apple auth url %s", raw)
	}
}
```

`internal/infra/oidc/apple_test.go`:

```go
package oidc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestAppleClientSecret(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	parsed, err := ParseApplePrivateKey(pemText)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	jwt, err := AppleClientSecret("TEAM123456", "com.example.web", "KEY1234567", parsed, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("not a compact JWS: %s", jwt)
	}
	var header map[string]any
	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	_ = json.Unmarshal(hb, &header)
	if header["alg"] != "ES256" || header["kid"] != "KEY1234567" {
		t.Fatalf("header %v", header)
	}
	var claims map[string]any
	pb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(pb, &claims)
	if claims["iss"] != "TEAM123456" || claims["sub"] != "com.example.web" || claims["aud"] != "https://appleid.apple.com" ||
		claims["exp"].(float64) != float64(now.Add(5*time.Minute).Unix()) {
		t.Fatalf("claims %v", claims)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if len(sig) != 64 {
		t.Fatalf("ES256 signature must be raw r||s (64 bytes), got %d", len(sig))
	}
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&key.PublicKey, h[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("signature does not verify")
	}
	if _, err := ParseApplePrivateKey("garbage"); err == nil {
		t.Fatal("garbage PEM must fail")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/infra/oidc/... -v`
Expected: compile errors.

- [ ] **Step 3: Implement**

`random.go`:

```go
// Package oidc implements the relying-party side of OpenID Connect with the
// standard library: discovery, the authorization-code request (with PKCE),
// the code exchange, ID-token verification against the issuer's JWKS, the
// userinfo fallback, RP-initiated logout URLs, and Apple's signed client
// secret. It knows nothing about users or sessions.
package oidc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
)

// RandomToken returns 32 random bytes as unpadded base64url (43 chars), the
// alphabet the access tokens already use.
func RandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func Sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
```

`pkce.go`:

```go
package oidc

import (
	"crypto/sha256"
	"encoding/base64"
)

// PKCEChallenge is the S256 transform of RFC 7636.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
```

`jwt.go`:

```go
package oidc

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

var ErrInvalidToken = errors.New("oidc: invalid id token")

const clockSkew = 60 * time.Second

type jwsHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type idTokenClaims struct {
	Iss           string          `json:"iss"`
	Sub           string          `json:"sub"`
	Aud           json.RawMessage `json:"aud"`
	Exp           int64           `json:"exp"`
	Iat           int64           `json:"iat"`
	Nonce         string          `json:"nonce"`
	Email         string          `json:"email"`
	EmailVerified json.RawMessage `json:"email_verified"`
	Name          string          `json:"name"`
}

// parseCompact splits a JWS, decodes header and claims, and returns the
// signing input and signature for verification.
func parseCompact(raw string) (jwsHeader, idTokenClaims, []byte, []byte, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	var h jwsHeader
	if err := json.Unmarshal(hb, &h); err != nil {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	var c idTokenClaims
	if err := json.Unmarshal(pb, &c); err != nil {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	return h, c, []byte(parts[0] + "." + parts[1]), sig, nil
}

func verifySignature(alg string, key crypto.PublicKey, signingInput, sig []byte) error {
	digest := sha256.Sum256(signingInput)
	switch alg {
	case "RS256":
		pub, ok := key.(*rsa.PublicKey)
		if !ok {
			return ErrInvalidToken
		}
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
			return ErrInvalidToken
		}
		return nil
	case "ES256":
		pub, ok := key.(*ecdsa.PublicKey)
		if !ok || len(sig) != 64 {
			return ErrInvalidToken
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(pub, digest[:], r, s) {
			return ErrInvalidToken
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported alg %q", ErrInvalidToken, alg)
	}
}

// audienceContains accepts both the string and the array form of aud.
func audienceContains(raw json.RawMessage, clientID string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == clientID
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for _, a := range many {
			if a == clientID {
				return true
			}
		}
	}
	return false
}

// parseEmailVerified accepts a JSON bool or the strings "true"/"false" (Apple).
func parseEmailVerified(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s == "true"
	}
	return false
}

func (c idTokenClaims) toClaims() Claims {
	return Claims{Subject: c.Sub, Email: c.Email, EmailVerified: parseEmailVerified(c.EmailVerified), Name: c.Name}
}
```

`jwks.go`:

```go
package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"
)

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (k jwk) publicKey() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		e, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
	case "EC":
		if k.Crv != "P-256" {
			return nil, fmt.Errorf("oidc: unsupported curve %q", k.Crv)
		}
		x, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, err
		}
		y, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}, nil
	}
	return nil, fmt.Errorf("oidc: unsupported kty %q", k.Kty)
}

// minJWKSRefresh bounds how often an unknown kid may trigger a re-fetch, so a
// flood of forged tokens cannot turn the verifier into a JWKS hammer.
const minJWKSRefresh = time.Minute

// keyFor resolves the signing key for kid, re-fetching the JWKS once on a miss.
func (c *Client) keyFor(ctx context.Context, kid string) (crypto.PublicKey, error) {
	c.mu.Lock()
	if k, ok := c.keys[kid]; ok {
		c.mu.Unlock()
		return k, nil
	}
	stale := time.Since(c.keysFetched) < minJWKSRefresh && len(c.keys) > 0
	c.mu.Unlock()
	if stale {
		return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalidToken, kid)
	}
	if err := c.fetchJWKS(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalidToken, kid)
}

func (c *Client) fetchJWKS(ctx context.Context) error {
	disc, err := c.Discover(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, disc.JWKSURI, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc: jwks returned status %d", resp.StatusCode)
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return err
	}
	keys := map[string]crypto.PublicKey{}
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		pub, perr := k.publicKey()
		if perr != nil {
			continue // one exotic key must not poison the set
		}
		keys[k.Kid] = pub
	}
	c.mu.Lock()
	c.keys = keys
	c.keysFetched = time.Now()
	c.mu.Unlock()
	return nil
}
```

`client.go`:

```go
package oidc

import (
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Issuer struct {
	ID              string
	IssuerURL       string
	ClientID        string
	ClientSecret    func(now time.Time) (string, error)
	Scopes          []string
	UsePKCE         bool
	ResponseMode    string
	TrustEmail      bool
	ExtraAuthParams map[string]string
}

// StaticSecret adapts a plain client secret to the ClientSecret seam.
func StaticSecret(secret string) func(time.Time) (string, error) {
	return func(time.Time) (string, error) { return secret, nil }
}

type Discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

type Claims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

type Tokens struct {
	IDToken     string
	AccessToken string
}

type Client struct {
	issuer Issuer
	http   *http.Client

	mu          sync.Mutex
	disc        *Discovery
	keys        map[string]crypto.PublicKey
	keysFetched time.Time
}

func NewClient(issuer Issuer, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	issuer.IssuerURL = strings.TrimSuffix(issuer.IssuerURL, "/")
	return &Client{issuer: issuer, http: hc}
}

func (c *Client) Issuer() Issuer { return c.issuer }

// Discover loads the OpenID configuration once per process (lazily), so an
// issuer that is down at boot only fails the requests that need it.
func (c *Client) Discover(ctx context.Context) (Discovery, error) {
	c.mu.Lock()
	if c.disc != nil {
		d := *c.disc
		c.mu.Unlock()
		return d, nil
	}
	c.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.issuer.IssuerURL+"/.well-known/openid-configuration", nil)
	if err != nil {
		return Discovery{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Discovery{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Discovery{}, fmt.Errorf("oidc: discovery returned status %d", resp.StatusCode)
	}
	var d Discovery
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
		return Discovery{}, err
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.JWKSURI == "" {
		return Discovery{}, errors.New("oidc: discovery document lacks required endpoints")
	}
	c.mu.Lock()
	c.disc = &d
	c.mu.Unlock()
	return d, nil
}

func (c *Client) AuthURL(ctx context.Context, state, nonce, codeChallenge, redirectURI string) (string, error) {
	d, err := c.Discover(ctx)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.issuer.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", strings.Join(c.issuer.Scopes, " "))
	q.Set("state", state)
	q.Set("nonce", nonce)
	if c.issuer.UsePKCE {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
	}
	if c.issuer.ResponseMode != "" {
		q.Set("response_mode", c.issuer.ResponseMode)
	}
	for k, v := range c.issuer.ExtraAuthParams {
		q.Set(k, v)
	}
	sep := "?"
	if strings.Contains(d.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	return d.AuthorizationEndpoint + sep + q.Encode(), nil
}

// Exchange redeems the code with client_secret_post authentication.
func (c *Client) Exchange(ctx context.Context, code, codeVerifier, redirectURI string, now time.Time) (Tokens, error) {
	d, err := c.Discover(ctx)
	if err != nil {
		return Tokens{}, err
	}
	secret, err := c.issuer.ClientSecret(now)
	if err != nil {
		return Tokens{}, err
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", c.issuer.ClientID)
	form.Set("client_secret", secret)
	if c.issuer.UsePKCE {
		form.Set("code_verifier", codeVerifier)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		IDToken     string `json:"id_token"`
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Tokens{}, fmt.Errorf("oidc: token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || body.Error != "" {
		return Tokens{}, fmt.Errorf("oidc: token endpoint status %d error %q", resp.StatusCode, body.Error)
	}
	if body.IDToken == "" {
		return Tokens{}, errors.New("oidc: token response carries no id_token")
	}
	return Tokens{IDToken: body.IDToken, AccessToken: body.AccessToken}, nil
}

func (c *Client) VerifyIDToken(ctx context.Context, raw, nonce string, now time.Time) (Claims, error) {
	h, cl, input, sig, err := parseCompact(raw)
	if err != nil {
		return Claims{}, err
	}
	if h.Alg != "RS256" && h.Alg != "ES256" {
		return Claims{}, fmt.Errorf("%w: unsupported alg %q", ErrInvalidToken, h.Alg)
	}
	key, err := c.keyFor(ctx, h.Kid)
	if err != nil {
		return Claims{}, err
	}
	if err := verifySignature(h.Alg, key, input, sig); err != nil {
		return Claims{}, err
	}
	d, err := c.Discover(ctx)
	if err != nil {
		return Claims{}, err
	}
	iss := strings.TrimSuffix(cl.Iss, "/")
	if iss != strings.TrimSuffix(d.Issuer, "/") && iss != c.issuer.IssuerURL {
		return Claims{}, fmt.Errorf("%w: issuer %q", ErrInvalidToken, cl.Iss)
	}
	if !audienceContains(cl.Aud, c.issuer.ClientID) {
		return Claims{}, fmt.Errorf("%w: audience", ErrInvalidToken)
	}
	if cl.Exp == 0 || now.After(time.Unix(cl.Exp, 0).Add(clockSkew)) {
		return Claims{}, fmt.Errorf("%w: expired", ErrInvalidToken)
	}
	if cl.Iat != 0 && time.Unix(cl.Iat, 0).After(now.Add(clockSkew)) {
		return Claims{}, fmt.Errorf("%w: issued in the future", ErrInvalidToken)
	}
	if nonce != "" && cl.Nonce != nonce {
		return Claims{}, fmt.Errorf("%w: nonce", ErrInvalidToken)
	}
	if cl.Sub == "" {
		return Claims{}, fmt.Errorf("%w: missing sub", ErrInvalidToken)
	}
	return cl.toClaims(), nil
}

// UserInfo fetches the userinfo document; the caller decides how to merge it.
func (c *Client) UserInfo(ctx context.Context, accessToken string) (Claims, error) {
	d, err := c.Discover(ctx)
	if err != nil {
		return Claims{}, err
	}
	if d.UserinfoEndpoint == "" {
		return Claims{}, errors.New("oidc: issuer publishes no userinfo endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.UserinfoEndpoint, nil)
	if err != nil {
		return Claims{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Claims{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Claims{}, fmt.Errorf("oidc: userinfo returned status %d", resp.StatusCode)
	}
	var cl idTokenClaims
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&cl); err != nil {
		return Claims{}, err
	}
	return cl.toClaims(), nil
}

// EndSessionURL builds the RP-initiated logout URL, or "" when the issuer
// publishes no end_session_endpoint.
func (c *Client) EndSessionURL(ctx context.Context, idToken, postLogoutRedirectURI string) (string, error) {
	d, err := c.Discover(ctx)
	if err != nil {
		return "", err
	}
	if d.EndSessionEndpoint == "" {
		return "", nil
	}
	q := url.Values{}
	q.Set("id_token_hint", idToken)
	q.Set("client_id", c.issuer.ClientID)
	q.Set("post_logout_redirect_uri", postLogoutRedirectURI)
	sep := "?"
	if strings.Contains(d.EndSessionEndpoint, "?") {
		sep = "&"
	}
	return d.EndSessionEndpoint + sep + q.Encode(), nil
}
```

`apple.go`:

```go
package oidc

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"time"
)

const AppleIssuerURL = "https://appleid.apple.com"

func ParseApplePrivateKey(pemText string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("oidc: apple private key is not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ec, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("oidc: apple private key is not an ECDSA key")
	}
	return ec, nil
}

// AppleClientSecret is the ES256 JWT Apple requires in place of a static
// client secret: iss = team id, sub = services id, aud = Apple, 5-minute life.
func AppleClientSecret(teamID, clientID, keyID string, key *ecdsa.PrivateKey, now time.Time) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": keyID})
	claims, _ := json.Marshal(map[string]any{
		"iss": teamID, "sub": clientID, "aud": AppleIssuerURL,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	})
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
```

`oidctest/fake.go` (package `oidctest`, imports `oidc` for `Issuer`/`StaticSecret`/`PKCEChallenge`):

```go
// Package oidctest is an in-process OpenID provider for tests: discovery,
// authorize (recorded, not rendered), token, JWKS, userinfo, end-session.
package oidctest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
)

type pending struct {
	nonce     string
	challenge string
}

type Fake struct {
	Server   *httptest.Server
	ClientID string
	Secret   string

	// Claims placed in every ID token / userinfo response.
	Subject       string
	Email         string
	EmailVerified any // nil = omit the claim; bool; or the string "true"/"false"
	Name          string
	// UserInfoEmail, when set, is returned by /userinfo (the ID token then
	// omits email when OmitEmailInIDToken is true).
	UserInfoEmail       string
	OmitEmailInIDToken  bool
	NoUserInfo          bool
	EndSession          bool
	RequirePKCE         bool
	AppleUserField      bool // /token also echoes nothing; tests post `user` themselves

	mu       sync.Mutex
	key      *rsa.PrivateKey
	kid      string
	codes    map[string]pending
	tokens   map[string]bool // issued access tokens
	requests []string
}

func New(t testing.TB) *Fake {
	t.Helper()
	f := &Fake{ClientID: "test-client", Secret: "fake-secret", Subject: "sub-1", Email: "user@example.test",
		EmailVerified: true, Name: "Test User", codes: map[string]pending{}, tokens: map[string]bool{}}
	f.RotateKey(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", f.discovery)
	mux.HandleFunc("/authorize", f.record)
	mux.HandleFunc("/token", f.token)
	mux.HandleFunc("/jwks", f.jwks)
	mux.HandleFunc("/userinfo", f.userinfo)
	mux.HandleFunc("/end-session", f.record)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)
	return f
}

func (f *Fake) IssuerURL() string { return f.Server.URL }

func (f *Fake) Issuer(id string, trust bool) oidc.Issuer {
	return oidc.Issuer{ID: id, IssuerURL: f.Server.URL, ClientID: f.ClientID, ClientSecret: oidc.StaticSecret(f.Secret),
		Scopes: []string{"openid", "profile", "email"}, UsePKCE: true, TrustEmail: trust}
}

func (f *Fake) RotateKey(t testing.TB) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.key = key
	f.kid = "k" + big.NewInt(time.Now().UnixNano()).Text(36)
	f.mu.Unlock()
}

// IssueCode simulates the user consenting: the returned code is bound to the
// nonce and PKCE challenge the authorization request carried.
func (f *Fake) IssueCode(nonce, challenge string) string {
	code, _ := oidc.RandomToken()
	f.mu.Lock()
	f.codes[code] = pending{nonce: nonce, challenge: challenge}
	f.mu.Unlock()
	return code
}

func (f *Fake) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *Fake) claims(nonce string) map[string]any {
	now := time.Now()
	m := map[string]any{"iss": f.Server.URL, "aud": f.ClientID, "sub": f.Subject, "nonce": nonce,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix()}
	if f.Name != "" {
		m["name"] = f.Name
	}
	if f.Email != "" && !f.OmitEmailInIDToken {
		m["email"] = f.Email
		if f.EmailVerified != nil {
			m["email_verified"] = f.EmailVerified
		}
	}
	return m
}

func (f *Fake) SignIDToken(claims map[string]any) string { return f.SignWithAlg("RS256", claims) }

func (f *Fake) SignWithAlg(alg string, claims map[string]any) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, _ := json.Marshal(map[string]string{"alg": alg, "kid": f.kid})
	p, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	if alg == "none" {
		return input + "."
	}
	digest := sha256.Sum256([]byte(input))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}
```

(imports also `"crypto"` and `"strings"`.) Then the handlers:

```go
func (f *Fake) note(r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.Path)
	f.mu.Unlock()
}

func (f *Fake) record(w http.ResponseWriter, r *http.Request) {
	f.note(r)
	w.WriteHeader(http.StatusOK)
}

func (f *Fake) discovery(w http.ResponseWriter, r *http.Request) {
	f.note(r)
	d := map[string]string{
		"issuer": f.Server.URL, "authorization_endpoint": f.Server.URL + "/authorize",
		"token_endpoint": f.Server.URL + "/token", "jwks_uri": f.Server.URL + "/jwks",
	}
	if !f.NoUserInfo {
		d["userinfo_endpoint"] = f.Server.URL + "/userinfo"
	}
	if f.EndSession {
		d["end_session_endpoint"] = f.Server.URL + "/end-session"
	}
	_ = json.NewEncoder(w).Encode(d)
}

func (f *Fake) jwks(w http.ResponseWriter, r *http.Request) {
	f.note(r)
	f.mu.Lock()
	pub := f.key.PublicKey
	kid := f.kid
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func (f *Fake) token(w http.ResponseWriter, r *http.Request) {
	f.note(r)
	_ = r.ParseForm()
	fail := func(code string) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.PostForm.Get("client_id") != f.ClientID || r.PostForm.Get("client_secret") != f.Secret {
		fail("invalid_client")
		return
	}
	f.mu.Lock()
	p, ok := f.codes[r.PostForm.Get("code")]
	delete(f.codes, r.PostForm.Get("code"))
	f.mu.Unlock()
	if !ok {
		fail("invalid_grant")
		return
	}
	if f.RequirePKCE || p.challenge != "" {
		if oidc.PKCEChallenge(r.PostForm.Get("code_verifier")) != p.challenge {
			fail("invalid_grant")
			return
		}
	}
	access, _ := oidc.RandomToken()
	f.mu.Lock()
	f.tokens[access] = true
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": access, "token_type": "Bearer", "id_token": f.SignIDToken(f.claims(p.nonce)),
	})
}

func (f *Fake) userinfo(w http.ResponseWriter, r *http.Request) {
	f.note(r)
	f.mu.Lock()
	ok := f.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	f.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	m := map[string]any{"sub": f.Subject, "name": f.Name}
	email := f.UserInfoEmail
	if email == "" {
		email = f.Email
	}
	if email != "" {
		m["email"] = email
		if f.EmailVerified != nil {
			m["email_verified"] = f.EmailVerified
		}
	}
	_ = json.NewEncoder(w).Encode(m)
}
```

(Drop the unused `AppleUserField` field.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/infra/oidc/... -v -cover`
Expected: PASS, coverage ≥ 85% for the package.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/oidc
git commit -m "feat(oauth): stdlib OIDC relying-party package with fake issuer

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 7: `internal/oauth` repositories

**Files:**
- Create: `internal/oauth/repository.go` (interfaces), `internal/oauth/repo/identity.go`, `identity_sqlite.go`, `identity_pgsql.go`, `state.go`, `state_sqlite.go`, `state_pgsql.go`, `handoff.go`, `handoff_sqlite.go`, `handoff_pgsql.go`
- Test: `internal/oauth/repo/repo_integration_test.go`

**Interfaces (produces, in `package oauth`):**

```go
type Identities interface {
	NextIdentity() vo.Id
	GetByProviderSubject(ctx context.Context, provider, subject string) (*model.Identity, error) // NotFound
	GetByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.Identity, error) // NotFound
	ListByUser(ctx context.Context, userID vo.Id) ([]model.Identity, error)
	CountByUser(ctx context.Context, userID vo.Id) (int64, error)
	Save(ctx context.Context, i *model.Identity) error
	DeleteByUserProvider(ctx context.Context, userID vo.Id, provider string) (int64, error)
}
type States interface {
	Insert(ctx context.Context, s *model.OAuthState) error
	Get(ctx context.Context, stateHash string) (*model.OAuthState, error) // NotFound
	Delete(ctx context.Context, stateHash string) error
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}
type Handoffs interface {
	Insert(ctx context.Context, h *model.OAuthHandoff) error
	Get(ctx context.Context, codeHash string) (*model.OAuthHandoff, error) // NotFound
	Delete(ctx context.Context, codeHash string) error
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}
// package repo
func NewIdentityRepo(driver string, tx *backend.TxManager) *IdentityRepo
func NewStateRepo(driver string, tx *backend.TxManager) *StateRepo
func NewHandoffRepo(driver string, tx *backend.TxManager) *HandoffRepo
```

- [ ] **Step 1: Write the failing integration test**

`internal/oauth/repo/repo_integration_test.go`:

```go
package repo

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestIdentityRepo(t *testing.T) {
	db := dbtest.New(t)
	uid := vo.MustParseId(fixture.New(t, db).User(fixture.User{}))
	r := NewIdentityRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	if _, err := r.GetByProviderSubject(ctx, "google", "s1"); err == nil {
		t.Fatal("want not found")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want *errs.NotFoundError, got %T", err)
	}
	id := model.NewIdentity(r.NextIdentity(), uid, "google", "s1", "a@example.test", now)
	if err := r.Save(ctx, id); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByProviderSubject(ctx, "google", "s1")
	if err != nil || !got.UserID.Equal(uid) || got.Email != "a@example.test" {
		t.Fatalf("%+v %v", got, err)
	}
	id.UpdateEmail("b@example.test", now.Add(time.Minute))
	if err := r.Save(ctx, id); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByUserProvider(ctx, uid, "google")
	if got.Email != "b@example.test" || !got.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("upsert must refresh email/updated_at: %+v", got)
	}
	if n, _ := r.CountByUser(ctx, uid); n != 1 {
		t.Fatalf("count %d", n)
	}
	list, _ := r.ListByUser(ctx, uid)
	if len(list) != 1 || list[0].Provider != "google" {
		t.Fatalf("list %+v", list)
	}
	if n, err := r.DeleteByUserProvider(ctx, uid, "google"); err != nil || n != 1 {
		t.Fatalf("delete %d %v", n, err)
	}
	if n, _ := r.DeleteByUserProvider(ctx, uid, "google"); n != 0 {
		t.Fatal("second delete affects nothing")
	}
}

func TestStateAndHandoffRepos(t *testing.T) {
	db := dbtest.New(t)
	uid := vo.MustParseId(fixture.New(t, db).User(fixture.User{}))
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	states := NewStateRepo(db.Engine, db.TX)
	st := &model.OAuthState{StateHash: "h1", Provider: "oidc", Nonce: "n", CodeVerifier: "v", Client: "web", Intent: "link",
		LinkUserID: uid, CreatedAt: now, ExpiresAt: now.Add(model.OAuthStateTTL)}
	if err := states.Insert(ctx, st); err != nil {
		t.Fatal(err)
	}
	got, err := states.Get(ctx, "h1")
	if err != nil || !got.LinkUserID.Equal(uid) || got.Intent != "link" || got.CodeVerifier != "v" {
		t.Fatalf("%+v %v", got, err)
	}
	st2 := &model.OAuthState{StateHash: "h2", Provider: "google", Nonce: "n", Client: "app", Intent: "login",
		CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-50 * time.Minute)}
	if err := states.Insert(ctx, st2); err != nil {
		t.Fatal(err)
	}
	got2, _ := states.Get(ctx, "h2")
	if !got2.LinkUserID.IsZero() {
		t.Fatal("login state has no link user")
	}
	if n, _ := states.DeleteExpired(ctx, now); n != 1 {
		t.Fatalf("expired purge %d", n)
	}
	if err := states.Delete(ctx, "h1"); err != nil {
		t.Fatal(err)
	}
	if _, err := states.Get(ctx, "h1"); err == nil {
		t.Fatal("deleted state must be gone")
	}

	handoffs := NewHandoffRepo(db.Engine, db.TX)
	tok := "id.tok"
	h := &model.OAuthHandoff{CodeHash: "c1", UserID: uid, Provider: "oidc", IDToken: &tok, CreatedAt: now, ExpiresAt: now.Add(model.OAuthHandoffTTL)}
	if err := handoffs.Insert(ctx, h); err != nil {
		t.Fatal(err)
	}
	hg, err := handoffs.Get(ctx, "c1")
	if err != nil || hg.IDToken == nil || *hg.IDToken != tok || !hg.UserID.Equal(uid) {
		t.Fatalf("%+v %v", hg, err)
	}
	if err := handoffs.Delete(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := handoffs.Get(ctx, "c1"); err == nil {
		t.Fatal("deleted handoff must be gone")
	}
	old := &model.OAuthHandoff{CodeHash: "c2", UserID: uid, Provider: "google", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Hour)}
	_ = handoffs.Insert(ctx, old)
	if n, _ := handoffs.DeleteExpired(ctx, now); n != 1 {
		t.Fatalf("expired purge %d", n)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/oauth/repo/ -v`
Expected: compile errors.

- [ ] **Step 3: Implement**

`internal/oauth/repository.go`:

```go
package oauth

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Identities persists users_identities. Lookups on a missing row return
// *errs.NotFoundError.
type Identities interface {
	NextIdentity() vo.Id
	GetByProviderSubject(ctx context.Context, provider, subject string) (*model.Identity, error)
	GetByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.Identity, error)
	ListByUser(ctx context.Context, userID vo.Id) ([]model.Identity, error)
	CountByUser(ctx context.Context, userID vo.Id) (int64, error)
	Save(ctx context.Context, i *model.Identity) error
	DeleteByUserProvider(ctx context.Context, userID vo.Id, provider string) (int64, error)
}

// States persists in-flight authorization requests (oauth_states).
type States interface {
	Insert(ctx context.Context, s *model.OAuthState) error
	Get(ctx context.Context, stateHash string) (*model.OAuthState, error)
	Delete(ctx context.Context, stateHash string) error
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}

// Handoffs persists the one-shot codes exchanged for a session (oauth_handoffs).
type Handoffs interface {
	Insert(ctx context.Context, h *model.OAuthHandoff) error
	Get(ctx context.Context, codeHash string) (*model.OAuthHandoff, error)
	Delete(ctx context.Context, codeHash string) error
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}
```

`internal/oauth/repo/identity.go` — follow `internal/tag/repo/repo.go` exactly:

```go
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	identityRow            = sqlitegen.UsersIdentity
	upsertIdentityParams   = sqlitegen.UpsertIdentityParams
	identityByUserProvider = sqlitegen.GetIdentityByUserProviderParams
	identityBySubject      = sqlitegen.GetIdentityByProviderSubjectParams
	deleteIdentityParams   = sqlitegen.DeleteIdentityByUserProviderParams
)

type identityQuerier interface {
	GetIdentityByProviderSubject(ctx context.Context, db backend.DBTX, p identityBySubject) (identityRow, error)
	GetIdentityByUserProvider(ctx context.Context, db backend.DBTX, p identityByUserProvider) (identityRow, error)
	ListIdentitiesByUser(ctx context.Context, db backend.DBTX, userID string) ([]identityRow, error)
	CountIdentitiesByUser(ctx context.Context, db backend.DBTX, userID string) (int64, error)
	UpsertIdentity(ctx context.Context, db backend.DBTX, p upsertIdentityParams) error
	DeleteIdentityByUserProvider(ctx context.Context, db backend.DBTX, p deleteIdentityParams) (int64, error)
}

type IdentityRepo struct {
	tx *backend.TxManager
	q  identityQuerier
}

var _ appoauth.Identities = (*IdentityRepo)(nil)

func NewIdentityRepo(driver string, tx *backend.TxManager) *IdentityRepo {
	switch driver {
	case "sqlite":
		return &IdentityRepo{tx: tx, q: identitySqliteQuerier{}}
	case "postgresql":
		return &IdentityRepo{tx: tx, q: identityPgsqlQuerier{}}
	default:
		panic("oauthrepo: unknown database driver " + driver)
	}
}

func (r *IdentityRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *IdentityRepo) NextIdentity() vo.Id { return vo.NewId() }

func (r *IdentityRepo) GetByProviderSubject(ctx context.Context, provider, subject string) (*model.Identity, error) {
	row, err := r.q.GetIdentityByProviderSubject(ctx, r.db(ctx), identityBySubject{Provider: provider, Subject: subject})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Identity not found")
		}
		return nil, err
	}
	return identityFromRow(row)
}

func (r *IdentityRepo) GetByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.Identity, error) {
	row, err := r.q.GetIdentityByUserProvider(ctx, r.db(ctx), identityByUserProvider{UserID: userID.String(), Provider: provider})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Identity not found")
		}
		return nil, err
	}
	return identityFromRow(row)
}

func (r *IdentityRepo) ListByUser(ctx context.Context, userID vo.Id) ([]model.Identity, error) {
	rows, err := r.q.ListIdentitiesByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.Identity, 0, len(rows))
	for _, row := range rows {
		i, err := identityFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, nil
}

func (r *IdentityRepo) CountByUser(ctx context.Context, userID vo.Id) (int64, error) {
	return r.q.CountIdentitiesByUser(ctx, r.db(ctx), userID.String())
}

func (r *IdentityRepo) Save(ctx context.Context, i *model.Identity) error {
	return r.q.UpsertIdentity(ctx, r.db(ctx), upsertIdentityParams{
		ID: i.ID.String(), UserID: i.UserID.String(), Provider: i.Provider, Subject: i.Subject, Email: i.Email,
		CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	})
}

func (r *IdentityRepo) DeleteByUserProvider(ctx context.Context, userID vo.Id, provider string) (int64, error) {
	return r.q.DeleteIdentityByUserProvider(ctx, r.db(ctx), deleteIdentityParams{UserID: userID.String(), Provider: provider})
}

func identityFromRow(row identityRow) (*model.Identity, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	uid, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.Identity{ID: id, UserID: uid, Provider: row.Provider, Subject: row.Subject, Email: row.Email,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}
```

`identity_sqlite.go` / `identity_pgsql.go`: the passthrough / conversion adapters exactly like `internal/tag/repo/sqlite.go` and `pgsql.go` (pgsql converts with `identityRow(row)` and `pgsqlgen.UpsertIdentityParams(p)` etc.).

`state.go`:

```go
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	stateRow          = sqlitegen.OauthState
	insertStateParams = sqlitegen.InsertOAuthStateParams
)

type stateQuerier interface {
	InsertOAuthState(ctx context.Context, db backend.DBTX, p insertStateParams) error
	GetOAuthState(ctx context.Context, db backend.DBTX, hash string) (stateRow, error)
	DeleteOAuthState(ctx context.Context, db backend.DBTX, hash string) error
	DeleteExpiredOAuthStates(ctx context.Context, db backend.DBTX, cutoff time.Time) (int64, error)
}

type StateRepo struct {
	tx *backend.TxManager
	q  stateQuerier
}

var _ appoauth.States = (*StateRepo)(nil)

func NewStateRepo(driver string, tx *backend.TxManager) *StateRepo {
	switch driver {
	case "sqlite":
		return &StateRepo{tx: tx, q: stateSqliteQuerier{}}
	case "postgresql":
		return &StateRepo{tx: tx, q: statePgsqlQuerier{}}
	default:
		panic("oauthrepo: unknown database driver " + driver)
	}
}

func (r *StateRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *StateRepo) Insert(ctx context.Context, s *model.OAuthState) error {
	var link *string
	if !s.LinkUserID.IsZero() {
		v := s.LinkUserID.String()
		link = &v
	}
	return r.q.InsertOAuthState(ctx, r.db(ctx), insertStateParams{
		StateHash: s.StateHash, Provider: s.Provider, Nonce: s.Nonce, CodeVerifier: s.CodeVerifier,
		Client: s.Client, Intent: s.Intent, LinkUserID: link, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt,
	})
}

func (r *StateRepo) Get(ctx context.Context, hash string) (*model.OAuthState, error) {
	row, err := r.q.GetOAuthState(ctx, r.db(ctx), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("State not found")
		}
		return nil, err
	}
	s := &model.OAuthState{StateHash: row.StateHash, Provider: row.Provider, Nonce: row.Nonce, CodeVerifier: row.CodeVerifier,
		Client: row.Client, Intent: row.Intent, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}
	if row.LinkUserID != nil {
		id, perr := vo.ParseId(*row.LinkUserID)
		if perr != nil {
			return nil, perr
		}
		s.LinkUserID = id
	}
	return s, nil
}

func (r *StateRepo) Delete(ctx context.Context, hash string) error {
	return r.q.DeleteOAuthState(ctx, r.db(ctx), hash)
}

func (r *StateRepo) DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.q.DeleteExpiredOAuthStates(ctx, r.db(ctx), cutoff)
}
```

`handoff.go` is the same shape over `sqlitegen.OauthHandoff` / `InsertOAuthHandoffParams` with `UserID: h.UserID.String()`, `IDToken: h.IDToken`, and `Get` parsing `UserID` with `vo.ParseId`. Check the generated names for the `DeleteExpired*` parameter (sqlc names a lone `expires_at < ?` argument `expiresAt time.Time`) and match them in the adapters.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/oauth/... && DBTEST_ENGINE=pgsql go vet -tags enginecompare ./internal/oauth/...`
Expected: PASS / vet clean.

- [ ] **Step 5: Commit**

```bash
git add internal/oauth
git commit -m "feat(oauth): identity, state and handoff repositories

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: `internal/oauth` service

**Files:**
- Create: `internal/oauth/ports.go`, `providers.go`, `service.go`, `start.go`, `callback.go`, `handoff.go`, `identities.go`, `name.go`
- Test: `internal/oauth/service_test.go`, `name_test.go`, `providers_test.go`

**Interfaces (produces):**

```go
package oauth

type Users interface {
	FindByEmail(ctx context.Context, email string) (*model.User, error) // NotFound when absent
	FindByID(ctx context.Context, id vo.Id) (*model.User, error)
	ProvisionExternal(ctx context.Context, name, email string) (*model.User, error)
	ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string) error
	MintSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string) (*model.LoginResult, error)
}

type Provider struct { Client *oidc.Client; Name string } // Name = button label
func ProvidersFromConfig(cfg config.Config, hc *http.Client) ([]Provider, error) // fixed order google, apple, oidc

type CallbackInput struct { Code, State, Error, AppleUser string }
type Service struct { … }
func NewService(providers []Provider, users Users, identities Identities, states States, handoffs Handoffs,
	tx port.TxRunner, clock port.Clock, appURL string, allowRegistration bool) *Service
func (s *Service) ListProviders() []model.ProviderItem
func (s *Service) StartLogin(ctx, req model.StartOAuthRequest) (*model.StartOAuthResult, error)
func (s *Service) StartLink(ctx, userID vo.Id, req model.StartOAuthRequest) (*model.StartOAuthResult, error)
func (s *Service) Callback(ctx, provider string, in CallbackInput) string   // always a redirect URL
func (s *Service) ExchangeHandoff(ctx, req model.ExchangeHandoffRequest, userAgent string) (*model.LoginResult, error)
func (s *Service) ListIdentities(ctx, userID vo.Id) ([]model.IdentityItem, error)
func (s *Service) UnlinkIdentity(ctx, userID vo.Id, req model.UnlinkIdentityRequest) (*model.UnlinkIdentityResult, error)
func (s *Service) EndSessionURL(ctx, provider, idToken string) (string, error)
func (s *Service) RedirectURI(provider string) string  // <appURL>/api/v1/oauth/callback-<provider>
```

Error-redirect codes (exact strings): `denied`, `invalid_state`, `provider_error`, `email_required`, `email_unverified`, `registration_disabled`, `identity_taken`, `provider_already_linked`, `account_inactive`.

- [ ] **Step 1: Write the failing tests**

`internal/oauth/name_test.go`:

```go
package oauth

import "testing"

func TestDeriveName(t *testing.T) {
	cases := []struct{ name, email, want string }{
		{"Alice Example", "a@x.test", "Alice Example"},
		{"  Al  ", "alice.smith@x.test", "alice.smith"},
		{"", "ab@x.test", "User"},
		{"Bartholomew Montgomery-Fitzgerald III", "b@x.test", "Bartholomew Montgome"},
		{"Zoë", "z@x.test", "Zoë"},
	}
	for _, c := range cases {
		if got := deriveName(c.name, c.email); got != c.want {
			t.Errorf("deriveName(%q,%q) = %q, want %q", c.name, c.email, got, c.want)
		}
	}
}
```

`internal/oauth/service_test.go` — an in-memory `Users` fake plus the real sqlite repos and the fake issuer:

```go
package oauth_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/oidc/oidctest"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	oauthrepo "github.com/econumo/econumo/internal/oauth/repo"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// fixedClock is a pointer so a test can advance it after the service is built.
type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time { return c.t }

// fakeUsers is the Users port over the seeded users table: rows are real (so
// the FK constraints on identities/handoffs hold) but the aggregate is kept in
// memory, which is all the oauth service reads.
type fakeUsers struct {
	t         *testing.T
	db        *dbtest.DB
	byID      map[string]*model.User
	byEmail   map[string]*model.User
	minted    []string // providers of minted sessions
	replaced  []string // emails mirrored via ReplaceVerifiedEmail
	provision int
}

func newFakeUsers(t *testing.T, db *dbtest.DB) *fakeUsers {
	return &fakeUsers{t: t, db: db, byID: map[string]*model.User{}, byEmail: map[string]*model.User{}}
}

func (f *fakeUsers) seed(t *testing.T, email string, algorithm string) *model.User {
	id := fixture.New(t, f.db).User(fixture.User{Email: email, Algorithm: algorithm})
	u := &model.User{ID: vo.MustParseId(id), Email: email, Name: "Seed", IsActive: true, Algorithm: algorithm, EmailVerified: true}
	f.byID[id] = u
	f.byEmail[strings.ToLower(email)] = u
	return u
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*model.User, error) {
	if u, ok := f.byEmail[strings.ToLower(email)]; ok {
		return u, nil
	}
	return nil, errs.NewNotFound("User not found")
}
func (f *fakeUsers) FindByID(_ context.Context, id vo.Id) (*model.User, error) {
	if u, ok := f.byID[id.String()]; ok {
		return u, nil
	}
	return nil, errs.NewNotFound("User not found")
}
func (f *fakeUsers) ProvisionExternal(_ context.Context, name, email string) (*model.User, error) {
	f.provision++
	u := f.seed(f.t, email, model.AlgorithmNone)
	u.Name = name
	return u, nil
}
func (f *fakeUsers) ReplaceVerifiedEmail(_ context.Context, userID vo.Id, email string) error {
	f.replaced = append(f.replaced, email)
	return nil
}
func (f *fakeUsers) MintSession(_ context.Context, userID vo.Id, _ string, provider string, idToken *string) (*model.LoginResult, error) {
	f.minted = append(f.minted, provider)
	return &model.LoginResult{Token: "eco_ses_test", User: model.CurrentUserResult{Id: userID.String()}}, nil
}
```

Then the harness and the cases:

```go
type harness struct {
	t      *testing.T
	fake   *oidctest.Fake
	users  *fakeUsers
	svc    *appoauth.Service
	ids    appoauth.Identities
	states appoauth.States
	hands  appoauth.Handoffs
	clock  *fixedClock
}

func newHarness(t *testing.T, trust, allowRegistration bool) *harness {
	db := dbtest.New(t)
	f := oidctest.New(t)
	users := newFakeUsers(t, db)
	ids := oauthrepo.NewIdentityRepo(db.Engine, db.TX)
	states := oauthrepo.NewStateRepo(db.Engine, db.TX)
	hands := oauthrepo.NewHandoffRepo(db.Engine, db.TX)
	clk := &fixedClock{t: time.Now().UTC().Truncate(time.Second)}
	providers := []appoauth.Provider{
		{Client: oidc.NewClient(f.Issuer(model.OAuthProviderGoogle, true), nil), Name: "Google"},
		{Client: oidc.NewClient(f.Issuer(model.OAuthProviderOIDC, trust), nil), Name: "Authentik"},
	}
	svc := appoauth.NewService(providers, users, ids, states, hands, db.TX, clk, "https://app.example.test", allowRegistration)
	return &harness{t: t, fake: f, users: users, svc: svc, ids: ids, states: states, hands: hands, clock: clk}
}

// login drives start-login + the provider's consent + the callback and returns
// the redirect the callback produced.
func (h *harness) login(provider, client string) string {
	h.t.Helper()
	res, err := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: provider, Client: client})
	if err != nil {
		h.t.Fatal(err)
	}
	u, _ := url.Parse(res.Url)
	q := u.Query()
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	return h.svc.Callback(context.Background(), provider, appoauth.CallbackInput{Code: code, State: q.Get("state")})
}

func handoffOf(t *testing.T, redirect string) string {
	t.Helper()
	u, _ := url.Parse(redirect)
	if strings.HasPrefix(redirect, "econumo://") {
		return u.Query().Get("handoff")
	}
	frag, _ := url.ParseQuery(u.Fragment)
	return frag.Get("handoff")
}

func TestListProviders_FixedOrderAndNames(t *testing.T) {
	h := newHarness(t, false, true)
	got := h.svc.ListProviders()
	if len(got) != 2 || got[0].Id != "google" || got[0].Name != "Google" || got[1].Id != "oidc" || got[1].Name != "Authentik" {
		t.Fatalf("%+v", got)
	}
}

func TestStartLogin_UnconfiguredProvider(t *testing.T) {
	h := newHarness(t, false, true)
	_, err := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "apple", Client: "web"})
	v, ok := errs.AsValidation(err)
	if !ok || v.MsgCode != errs.CodeOAuthProviderNotConfigured {
		t.Fatalf("want provider_not_configured, got %v", err)
	}
}

func TestCallback_ProvisionsNewUserAndHandoffExchanges(t *testing.T) {
	h := newHarness(t, false, true)
	h.fake.Email, h.fake.EmailVerified, h.fake.Name = "new@example.test", true, "New Person"
	redirect := h.login("oidc", "web")
	if !strings.HasPrefix(redirect, "https://app.example.test/oauth/callback#handoff=") {
		t.Fatalf("redirect %s", redirect)
	}
	if h.users.provision != 1 {
		t.Fatalf("provision calls %d", h.users.provision)
	}
	res, err := h.svc.ExchangeHandoff(context.Background(), model.ExchangeHandoffRequest{Code: handoffOf(t, redirect)}, "UA/1")
	if err != nil || res.Token == "" || h.users.minted[0] != "oidc" {
		t.Fatalf("%+v %v minted=%v", res, err, h.users.minted)
	}
	// single use
	if _, err := h.svc.ExchangeHandoff(context.Background(), model.ExchangeHandoffRequest{Code: handoffOf(t, redirect)}, "UA/1"); err == nil {
		t.Fatal("handoff must be single use")
	}
	// identity recorded
	id, err := h.ids.GetByProviderSubject(context.Background(), "oidc", h.fake.Subject)
	if err != nil || id.Email != "new@example.test" {
		t.Fatalf("%+v %v", id, err)
	}
}

func TestCallback_AppClientRedirectsToScheme(t *testing.T) {
	h := newHarness(t, false, true)
	redirect := h.login("google", "app")
	if !strings.HasPrefix(redirect, "econumo://oauth?handoff=") {
		t.Fatalf("redirect %s", redirect)
	}
}

func TestCallback_ExistingIdentityLogsIn(t *testing.T) {
	h := newHarness(t, false, false) // registration off: must not matter
	u := h.users.seed(t, "old@example.test", model.AlgorithmArgon2id)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "old@example.test", h.clock.Now()))
	h.fake.Email = "changed@example.test"
	redirect := h.login("google", "web")
	if handoffOf(t, redirect) == "" {
		t.Fatalf("redirect %s", redirect)
	}
	id, _ := h.ids.GetByProviderSubject(context.Background(), "google", h.fake.Subject)
	if id.Email != "changed@example.test" {
		t.Fatal("identity email must follow the claim")
	}
	if len(h.users.replaced) != 0 {
		t.Fatal("a password user's primary email must not move")
	}
}

func TestCallback_EmailDriftMirroredForPasswordlessSingleIdentity(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "old@example.test", model.AlgorithmNone)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "old@example.test", h.clock.Now()))
	h.fake.Email = "renamed@example.test"
	h.login("google", "web")
	if len(h.users.replaced) != 1 || h.users.replaced[0] != "renamed@example.test" {
		t.Fatalf("replaced %v", h.users.replaced)
	}
}

func TestCallback_EmailDriftNotMirroredWhenAddressTaken(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "old@example.test", model.AlgorithmNone)
	h.users.seed(t, "taken@example.test", model.AlgorithmArgon2id)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "old@example.test", h.clock.Now()))
	h.fake.Email = "taken@example.test"
	redirect := h.login("google", "web")
	if handoffOf(t, redirect) == "" || len(h.users.replaced) != 0 {
		t.Fatalf("sign-in proceeds for the identity's owner, primary untouched: %s %v", redirect, h.users.replaced)
	}
}

func TestCallback_AutoLinksVerifiedEmail(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "match@example.test", model.AlgorithmArgon2id)
	h.fake.Email, h.fake.EmailVerified = "match@example.test", true
	redirect := h.login("oidc", "web")
	if handoffOf(t, redirect) == "" {
		t.Fatalf("redirect %s", redirect)
	}
	id, err := h.ids.GetByUserProvider(context.Background(), u.ID, "oidc")
	if err != nil || id.Subject != h.fake.Subject {
		t.Fatalf("auto-link missing: %+v %v", id, err)
	}
}

func TestCallback_UnverifiedEmailRejectedUnlessTrusted(t *testing.T) {
	h := newHarness(t, false, true)
	h.fake.EmailVerified = false
	if r := h.login("oidc", "web"); r != "https://app.example.test/login?oauthError=email_unverified" {
		t.Fatalf("redirect %s", r)
	}
	h.fake.EmailVerified = nil // claim absent
	if r := h.login("oidc", "app"); r != "econumo://oauth?error=email_unverified" {
		t.Fatalf("redirect %s", r)
	}
	trusted := newHarness(t, true, true)
	trusted.fake.EmailVerified = nil
	if r := trusted.login("oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("trusted issuer must sign in: %s", r)
	}
}

func TestCallback_RegistrationDisabled(t *testing.T) {
	h := newHarness(t, false, false)
	if r := h.login("google", "web"); r != "https://app.example.test/login?oauthError=registration_disabled" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_MissingEmailUsesUserinfoThenFails(t *testing.T) {
	h := newHarness(t, false, true)
	h.fake.OmitEmailInIDToken = true
	h.fake.UserInfoEmail = "from-userinfo@example.test"
	if r := h.login("oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("userinfo fallback must sign in: %s", r)
	}
	h2 := newHarness(t, false, true)
	h2.fake.OmitEmailInIDToken = true
	h2.fake.Email = ""
	h2.fake.NoUserInfo = true
	if r := h2.login("oidc", "web"); r != "https://app.example.test/login?oauthError=email_required" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_StateErrors(t *testing.T) {
	h := newHarness(t, false, true)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: "x", State: "unknown"}); r != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("redirect %s", r)
	}
	res, _ := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	q := mustQuery(t, res.Url)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Error: "access_denied", State: q.Get("state")}); r != "https://app.example.test/login?oauthError=denied" {
		t.Fatalf("redirect %s", r)
	}
	// consumed: the same state again is invalid
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("redirect %s", r)
	}
	// provider mismatch
	res2, _ := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "oidc", Client: "web"})
	q2 := mustQuery(t, res2.Url)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: "c", State: q2.Get("state")}); r != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("redirect %s", r)
	}
	// expired state
	res3, _ := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	q3 := mustQuery(t, res3.Url)
	h.clock.t = h.clock.t.Add(model.OAuthStateTTL + time.Second)
	code3 := h.fake.IssueCode(q3.Get("nonce"), q3.Get("code_challenge"))
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code3, State: q3.Get("state")}); r != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("expired state: %s", r)
	}
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}
```

Add the link and unlink cases:

```go
func TestStartLinkAndCallback_Link(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	res, err := h.svc.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")})
	if r != "https://app.example.test/settings/profile/linked-accounts?linked=google" {
		t.Fatalf("redirect %s", r)
	}
	if len(h.users.minted) != 0 {
		t.Fatal("link must mint no session")
	}
	// Linking the same subject again is idempotent; a different user gets identity_taken.
	res2, _ := h.svc.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "app"})
	q2 := mustQuery(t, res2.Url)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: h.fake.IssueCode(q2.Get("nonce"), q2.Get("code_challenge")), State: q2.Get("state")}); r != "econumo://oauth?linked=google" {
		t.Fatalf("redirect %s", r)
	}
	other := h.users.seed(t, "other@example.test", model.AlgorithmArgon2id)
	res3, _ := h.svc.StartLink(context.Background(), other.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	q3 := mustQuery(t, res3.Url)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: h.fake.IssueCode(q3.Get("nonce"), q3.Get("code_challenge")), State: q3.Get("state")}); r != "https://app.example.test/login?oauthError=identity_taken" {
		t.Fatalf("redirect %s", r)
	}
	// The same user linking google with a DIFFERENT subject: provider_already_linked.
	h.fake.Subject = "another-google-account"
	res4, _ := h.svc.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	q4 := mustQuery(t, res4.Url)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: h.fake.IssueCode(q4.Get("nonce"), q4.Get("code_challenge")), State: q4.Get("state")}); r != "https://app.example.test/login?oauthError=provider_already_linked" {
		t.Fatalf("redirect %s", r)
	}
}

func TestListAndUnlinkIdentities(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmNone)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", "g1", "me@example.test", h.clock.Now()))
	list, err := h.svc.ListIdentities(context.Background(), u.ID)
	if err != nil || len(list) != 1 || list[0].Provider != "google" || list[0].CreatedAt == "" {
		t.Fatalf("%+v %v", list, err)
	}
	_, err = h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeOAuthLastIdentity {
		t.Fatalf("passwordless single identity must refuse: %v", err)
	}
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "oidc", "o1", "me@example.test", h.clock.Now()))
	if _, err := h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); err == nil {
		t.Fatal("unlinking a missing identity is an error")
	}
	pw := h.users.seed(t, "pw@example.test", model.AlgorithmArgon2id)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), pw.ID, "google", "g2", "pw@example.test", h.clock.Now()))
	if _, err := h.svc.UnlinkIdentity(context.Background(), pw.ID, model.UnlinkIdentityRequest{Provider: "google"}); err != nil {
		t.Fatalf("a password user may unlink their only identity: %v", err)
	}
}

func TestInactiveUserRejected(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "gone@example.test", model.AlgorithmArgon2id)
	u.IsActive = false
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "gone@example.test", h.clock.Now()))
	if r := h.login("google", "web"); r != "https://app.example.test/login?oauthError=account_inactive" {
		t.Fatalf("redirect %s", r)
	}
}

func TestExchangeHandoff_ExpiredAndUnknown(t *testing.T) {
	h := newHarness(t, false, true)
	if _, err := h.svc.ExchangeHandoff(context.Background(), model.ExchangeHandoffRequest{Code: "nope"}, "ua"); err == nil {
		t.Fatal("unknown code must fail")
	} else if u, ok := errs.AsUnauthorized(err); !ok || u.Code != errs.CodeOAuthHandoffInvalid {
		t.Fatalf("want 401 handoff_invalid, got %v", err)
	}
	redirect := h.login("google", "web")
	h.clock.t = h.clock.t.Add(model.OAuthHandoffTTL + time.Second)
	if _, err := h.svc.ExchangeHandoff(context.Background(), model.ExchangeHandoffRequest{Code: handoffOf(t, redirect)}, "ua"); err == nil {
		t.Fatal("expired handoff must fail")
	}
}

func TestEndSessionURL(t *testing.T) {
	h := newHarness(t, false, true)
	h.fake.EndSession = true
	u, err := h.svc.EndSessionURL(context.Background(), "oidc", "tok")
	if err != nil || !strings.Contains(u, "id_token_hint=tok") || !strings.Contains(u, url.QueryEscape("https://app.example.test/login")) {
		t.Fatalf("%q %v", u, err)
	}
	if u, _ := h.svc.EndSessionURL(context.Background(), "apple", "tok"); u != "" {
		t.Fatal("unconfigured provider yields no url")
	}
}
```

`internal/oauth/providers_test.go`:

```go
package oauth

import (
	"testing"

	"github.com/econumo/econumo/internal/config"
)

func TestProvidersFromConfig(t *testing.T) {
	cfg := config.Config{OAuthGoogleClientID: "g", OAuthGoogleClientSecret: "gs",
		OIDCIssuerURL: "https://auth.example.test", OIDCClientID: "c", OIDCClientSecret: "s", OIDCName: "Authentik",
		OIDCScopes: []string{"openid", "email"}, OIDCTrustEmail: true}
	ps, err := ProvidersFromConfig(cfg, nil)
	if err != nil || len(ps) != 2 || ps[0].Client.Issuer().ID != "google" || ps[1].Client.Issuer().ID != "oidc" || ps[1].Name != "Authentik" {
		t.Fatalf("%+v %v", ps, err)
	}
	g := ps[0].Client.Issuer()
	if g.IssuerURL != "https://accounts.google.com" || !g.TrustEmail || !g.UsePKCE || g.ExtraAuthParams["prompt"] != "select_account" {
		t.Fatalf("google issuer %+v", g)
	}
	o := ps[1].Client.Issuer()
	if !o.TrustEmail || len(o.Scopes) != 2 {
		t.Fatalf("oidc issuer %+v", o)
	}
	cfg.OAuthAppleClientID, cfg.OAuthAppleTeamID, cfg.OAuthAppleKeyID, cfg.OAuthApplePrivateKey = "com.example.web", "TEAM", "KEY", "not a pem"
	if _, err := ProvidersFromConfig(cfg, nil); err == nil {
		t.Fatal("a bad apple key must fail")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/oauth/ -v`
Expected: compile errors.

- [ ] **Step 3: Implement**

`internal/oauth/ports.go`:

```go
// Ports: the user-feature capabilities this feature consumes. Implemented in
// internal/server over the user service — features never import each other.
package oauth

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Users interface {
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByID(ctx context.Context, id vo.Id) (*model.User, error)
	// ProvisionExternal creates a passwordless, email-verified user with the
	// registration defaults (trial, options, currency).
	ProvisionExternal(ctx context.Context, name, email string) (*model.User, error)
	// ReplaceVerifiedEmail mirrors an IdP-side email change onto the primary email.
	ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string) error
	// MintSession opens a session stamped with the provider (and the ID token
	// for the custom slot) and returns the login-shaped result.
	MintSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string) (*model.LoginResult, error)
}
```

`internal/oauth/providers.go`:

```go
package oauth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
)

const googleIssuerURL = "https://accounts.google.com"

// Provider is one enabled slot: the protocol client plus the button label.
type Provider struct {
	Client *oidc.Client
	Name   string
}

// ProvidersFromConfig builds the enabled slots in display order. Google and
// Apple are fixed issuers with their quirks preconfigured; the custom slot
// follows the operator's variables.
func ProvidersFromConfig(cfg config.Config, hc *http.Client) ([]Provider, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	var out []Provider
	if cfg.OAuthGoogleEnabled() {
		out = append(out, Provider{Name: "Google", Client: oidc.NewClient(oidc.Issuer{
			ID: model.OAuthProviderGoogle, IssuerURL: googleIssuerURL, ClientID: cfg.OAuthGoogleClientID,
			ClientSecret: oidc.StaticSecret(cfg.OAuthGoogleClientSecret),
			Scopes: []string{"openid", "email", "profile"}, UsePKCE: true, TrustEmail: true,
			ExtraAuthParams: map[string]string{"prompt": "select_account"},
		}, hc)})
	}
	if cfg.OAuthAppleEnabled() {
		key, err := oidc.ParseApplePrivateKey(cfg.OAuthApplePrivateKey)
		if err != nil {
			return nil, fmt.Errorf("ECONUMO_OAUTH_APPLE_PRIVATE_KEY: %w", err)
		}
		teamID, clientID, keyID := cfg.OAuthAppleTeamID, cfg.OAuthAppleClientID, cfg.OAuthAppleKeyID
		out = append(out, Provider{Name: "Apple", Client: oidc.NewClient(oidc.Issuer{
			ID: model.OAuthProviderApple, IssuerURL: oidc.AppleIssuerURL, ClientID: clientID,
			ClientSecret: func(now time.Time) (string, error) { return oidc.AppleClientSecret(teamID, clientID, keyID, key, now) },
			// Apple returns name/email only with form_post; PKCE is undocumented there,
			// the confidential-client secret plus nonce bind the exchange.
			Scopes: []string{"name", "email"}, UsePKCE: false, ResponseMode: "form_post", TrustEmail: true,
		}, hc)})
	}
	if cfg.OIDCEnabled() {
		out = append(out, Provider{Name: cfg.OIDCName, Client: oidc.NewClient(oidc.Issuer{
			ID: model.OAuthProviderOIDC, IssuerURL: cfg.OIDCIssuerURL, ClientID: cfg.OIDCClientID,
			ClientSecret: oidc.StaticSecret(cfg.OIDCClientSecret),
			Scopes: cfg.OIDCScopes, UsePKCE: true, TrustEmail: cfg.OIDCTrustEmail,
		}, hc)})
	}
	return out, nil
}
```

`internal/oauth/name.go`:

```go
package oauth

import (
	"strings"
	"unicode/utf8"
)

const (
	nameMinRunes = 3
	nameMaxRunes = 20 // the register/update-name rule (model.RegisterRequest.Validate)
	fallbackName = "User"
)

// deriveName turns a provider's display name into one the name rule accepts:
// the claim, else the email local part, else a constant; clamped to 20 runes.
func deriveName(claim, email string) string {
	pick := strings.TrimSpace(claim)
	if utf8.RuneCountInString(pick) < nameMinRunes {
		local := email
		if at := strings.Index(email, "@"); at > 0 {
			local = email[:at]
		}
		pick = strings.TrimSpace(local)
	}
	if utf8.RuneCountInString(pick) < nameMinRunes {
		pick = fallbackName
	}
	if r := []rune(pick); len(r) > nameMaxRunes {
		pick = strings.TrimSpace(string(r[:nameMaxRunes]))
	}
	return pick
}
```

`internal/oauth/service.go`:

```go
// Package oauth is the relying-party feature: sign in and link accounts through
// Google, Apple, or one custom OpenID Connect issuer. See
// docs/superpowers/specs/2026-09-07-oauth-login-design.md.
package oauth

import (
	"context"
	"log/slog"
	"net/url"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
)

type Service struct {
	providers         []Provider
	byID              map[string]Provider
	users             Users
	identities        Identities
	states            States
	handoffs          Handoffs
	tx                port.TxRunner
	clock             port.Clock
	appURL            string
	allowRegistration bool
}

func NewService(providers []Provider, users Users, identities Identities, states States, handoffs Handoffs,
	tx port.TxRunner, clock port.Clock, appURL string, allowRegistration bool) *Service {
	byID := map[string]Provider{}
	for _, p := range providers {
		byID[p.Client.Issuer().ID] = p
	}
	return &Service{providers: providers, byID: byID, users: users, identities: identities, states: states,
		handoffs: handoffs, tx: tx, clock: clock, appURL: strings.TrimSuffix(appURL, "/"), allowRegistration: allowRegistration}
}

func (s *Service) ListProviders() []model.ProviderItem {
	out := make([]model.ProviderItem, 0, len(s.providers))
	for _, p := range s.providers {
		out = append(out, model.ProviderItem{Id: p.Client.Issuer().ID, Name: p.Name})
	}
	return out
}

func (s *Service) provider(id string) (Provider, error) {
	p, ok := s.byID[id]
	if !ok {
		return Provider{}, &errs.ValidationError{Msg: "Sign-in provider is not configured", MsgCode: errs.CodeOAuthProviderNotConfigured}
	}
	return p, nil
}

// RedirectURI is the one URL an operator registers per provider.
func (s *Service) RedirectURI(provider string) string {
	return s.appURL + "/api/v1/oauth/callback-" + provider
}

// EndSessionURL implements the user feature's LogoutURLBuilder port.
func (s *Service) EndSessionURL(ctx context.Context, provider, idToken string) (string, error) {
	p, ok := s.byID[provider]
	if !ok {
		return "", nil
	}
	return p.Client.EndSessionURL(ctx, idToken, s.appURL+"/login")
}

// redirect targets (spec §6.3)

func (s *Service) successURL(client, handoff string) string {
	if client == model.OAuthClientApp {
		return "econumo://oauth?handoff=" + url.QueryEscape(handoff)
	}
	return s.appURL + "/oauth/callback#handoff=" + url.QueryEscape(handoff)
}

func (s *Service) linkedURL(client, provider string) string {
	if client == model.OAuthClientApp {
		return "econumo://oauth?linked=" + url.QueryEscape(provider)
	}
	return s.appURL + "/settings/profile/linked-accounts?linked=" + url.QueryEscape(provider)
}

func (s *Service) errorURL(client, code string) string {
	if client == model.OAuthClientApp {
		return "econumo://oauth?error=" + url.QueryEscape(code)
	}
	return s.appURL + "/login?oauthError=" + url.QueryEscape(code)
}

func logWarn(ctx context.Context, msg string, err error, attrs ...any) {
	slog.WarnContext(ctx, msg, append([]any{"err", err.Error()}, attrs...)...)
}
```

(add `strings` import.)

`internal/oauth/start.go`:

```go
package oauth

import (
	"context"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) StartLogin(ctx context.Context, req model.StartOAuthRequest) (*model.StartOAuthResult, error) {
	return s.start(ctx, req, model.OAuthIntentLogin, vo.Id{})
}

func (s *Service) StartLink(ctx context.Context, userID vo.Id, req model.StartOAuthRequest) (*model.StartOAuthResult, error) {
	return s.start(ctx, req, model.OAuthIntentLink, userID)
}

// start creates the state row and returns the provider's authorization URL.
// Expired states are purged opportunistically here, the cheapest moment.
func (s *Service) start(ctx context.Context, req model.StartOAuthRequest, intent string, linkUser vo.Id) (*model.StartOAuthResult, error) {
	p, err := s.provider(req.Provider)
	if err != nil {
		return nil, err
	}
	state, err := oidc.RandomToken()
	if err != nil {
		return nil, err
	}
	nonce, err := oidc.RandomToken()
	if err != nil {
		return nil, err
	}
	verifier, challenge := "", ""
	if p.Client.Issuer().UsePKCE {
		if verifier, err = oidc.RandomToken(); err != nil {
			return nil, err
		}
		challenge = oidc.PKCEChallenge(verifier)
	}
	now := s.clock.Now()
	if _, err := s.states.DeleteExpired(ctx, now); err != nil {
		return nil, err
	}
	if _, err := s.handoffs.DeleteExpired(ctx, now); err != nil {
		return nil, err
	}
	if err := s.states.Insert(ctx, &model.OAuthState{
		StateHash: oidc.Sha256Hex(state), Provider: req.Provider, Nonce: nonce, CodeVerifier: verifier,
		Client: req.Client, Intent: intent, LinkUserID: linkUser, CreatedAt: now, ExpiresAt: now.Add(model.OAuthStateTTL),
	}); err != nil {
		return nil, err
	}
	u, err := p.Client.AuthURL(ctx, state, nonce, challenge, s.RedirectURI(req.Provider))
	if err != nil {
		return nil, err
	}
	return &model.StartOAuthResult{Url: u}, nil
}
```

`internal/oauth/callback.go`:

```go
package oauth

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// CallbackInput is what the provider sent back: query parameters for Google
// and the custom slot, form fields for Apple (AppleUser is Apple's one-time
// JSON `user` field carrying the name).
type CallbackInput struct {
	Code      string
	State     string
	Error     string
	AppleUser string
}

// Callback resolves the provider's answer to a redirect URL. It never returns
// an error: every failure becomes an error redirect (spec §6.2/§6.3), and the
// internal cause goes to the operation log only.
func (s *Service) Callback(ctx context.Context, provider string, in CallbackInput) string {
	client := model.OAuthClientWeb
	st, err := s.consumeState(ctx, provider, in.State)
	if err != nil {
		logWarn(ctx, "oauth callback: state", err, "provider", provider)
		return s.errorURL(client, "invalid_state")
	}
	client = st.Client
	reqctx.AddLogAttr(ctx, "oauth_intent", st.Intent)
	if in.Error != "" {
		return s.errorURL(client, "denied")
	}
	p, perr := s.provider(provider)
	if perr != nil {
		return s.errorURL(client, "provider_error")
	}
	now := s.clock.Now()
	toks, err := p.Client.Exchange(ctx, in.Code, st.CodeVerifier, s.RedirectURI(provider), now)
	if err != nil {
		logWarn(ctx, "oauth callback: exchange", err, "provider", provider)
		return s.errorURL(client, "provider_error")
	}
	claims, err := p.Client.VerifyIDToken(ctx, toks.IDToken, st.Nonce, now)
	if err != nil {
		logWarn(ctx, "oauth callback: id token", err, "provider", provider)
		return s.errorURL(client, "provider_error")
	}
	if claims.Email == "" && toks.AccessToken != "" {
		claims = s.userinfoFallback(ctx, p, toks.AccessToken, claims)
	}
	if claims.Name == "" && in.AppleUser != "" {
		claims.Name = appleName(in.AppleUser)
	}
	// Step 4: email present and verified/trusted, for every intent.
	if claims.Email == "" {
		return s.errorURL(client, "email_required")
	}
	if !claims.EmailVerified && !p.Client.Issuer().TrustEmail {
		return s.errorURL(client, "email_unverified")
	}
	email := strings.ToLower(strings.TrimSpace(claims.Email))

	if st.Intent == model.OAuthIntentLink {
		return s.link(ctx, st, provider, claims.Subject, email)
	}
	return s.login(ctx, st, provider, claims, email, toks.IDToken)
}

// consumeState loads and DELETES the state row; a consumed state is invalid.
func (s *Service) consumeState(ctx context.Context, provider, state string) (*model.OAuthState, error) {
	if state == "" {
		return nil, errs.NewNotFound("state missing")
	}
	hash := oidc.Sha256Hex(state)
	st, err := s.states.Get(ctx, hash)
	if err != nil {
		return nil, err
	}
	if derr := s.states.Delete(ctx, hash); derr != nil {
		return nil, derr
	}
	if st.Provider != provider {
		return nil, errs.NewNotFound("state provider mismatch")
	}
	if st.IsExpired(s.clock.Now()) {
		return nil, errs.NewNotFound("state expired")
	}
	return st, nil
}

// userinfoFallback fills email/verified/name only where the ID token left them
// empty, and only when userinfo's sub matches. Failure is not fatal.
func (s *Service) userinfoFallback(ctx context.Context, p Provider, accessToken string, claims oidc.Claims) oidc.Claims {
	info, err := p.Client.UserInfo(ctx, accessToken)
	if err != nil || info.Subject != claims.Subject {
		if err != nil {
			logWarn(ctx, "oauth callback: userinfo", err, "provider", p.Client.Issuer().ID)
		}
		return claims
	}
	if claims.Email == "" {
		claims.Email = info.Email
		claims.EmailVerified = info.EmailVerified
	}
	if claims.Name == "" {
		claims.Name = info.Name
	}
	return claims
}

// appleName extracts "firstName lastName" from Apple's first-sign-in user field.
func appleName(raw string) string {
	var u struct {
		Name struct {
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
		} `json:"name"`
	}
	if json.Unmarshal([]byte(raw), &u) != nil {
		return ""
	}
	return strings.TrimSpace(u.Name.FirstName + " " + u.Name.LastName)
}

func (s *Service) login(ctx context.Context, st *model.OAuthState, provider string, claims oidc.Claims, email, idToken string) string {
	now := s.clock.Now()
	var tokenForSession *string
	if provider == model.OAuthProviderOIDC {
		t := idToken
		tokenForSession = &t
	}

	// Step 5: existing identity.
	id, err := s.identities.GetByProviderSubject(ctx, provider, claims.Subject)
	if err == nil {
		u, uerr := s.users.FindByID(ctx, id.UserID)
		if uerr != nil {
			logWarn(ctx, "oauth callback: identity owner", uerr, "provider", provider)
			return s.errorURL(st.Client, "provider_error")
		}
		if !u.IsActive {
			return s.errorURL(st.Client, "account_inactive")
		}
		id.UpdateEmail(email, now)
		if serr := s.identities.Save(ctx, id); serr != nil {
			logWarn(ctx, "oauth callback: identity save", serr, "provider", provider)
			return s.errorURL(st.Client, "provider_error")
		}
		s.mirrorEmailDrift(ctx, u, email, provider)
		return s.mintHandoff(ctx, st, u.ID, provider, tokenForSession)
	}
	if _, ok := errs.AsNotFound(err); !ok {
		logWarn(ctx, "oauth callback: identity lookup", err, "provider", provider)
		return s.errorURL(st.Client, "provider_error")
	}

	// Step 6: auto-link by (verified) email.
	u, err := s.users.FindByEmail(ctx, email)
	if err == nil {
		if !u.IsActive {
			return s.errorURL(st.Client, "account_inactive")
		}
		if serr := s.identities.Save(ctx, model.NewIdentity(s.identities.NextIdentity(), u.ID, provider, claims.Subject, email, now)); serr != nil {
			logWarn(ctx, "oauth callback: auto-link", serr, "provider", provider)
			return s.errorURL(st.Client, "provider_error")
		}
		reqctx.AddLogAttr(ctx, "oauth_linked", true)
		return s.mintHandoff(ctx, st, u.ID, provider, tokenForSession)
	}
	if _, ok := errs.AsNotFound(err); !ok {
		logWarn(ctx, "oauth callback: user lookup", err, "provider", provider)
		return s.errorURL(st.Client, "provider_error")
	}

	// Step 7: provision.
	if !s.allowRegistration {
		return s.errorURL(st.Client, "registration_disabled")
	}
	u, err = s.users.ProvisionExternal(ctx, deriveName(claims.Name, email), email)
	if err != nil {
		logWarn(ctx, "oauth callback: provision", err, "provider", provider)
		return s.errorURL(st.Client, "provider_error")
	}
	if serr := s.identities.Save(ctx, model.NewIdentity(s.identities.NextIdentity(), u.ID, provider, claims.Subject, email, now)); serr != nil {
		logWarn(ctx, "oauth callback: identity insert", serr, "provider", provider)
		return s.errorURL(st.Client, "provider_error")
	}
	reqctx.AddLogAttr(ctx, "oauth_provisioned", true)
	return s.mintHandoff(ctx, st, u.ID, provider, tokenForSession)
}

// mirrorEmailDrift applies the spec's email-drift rule for an existing identity.
func (s *Service) mirrorEmailDrift(ctx context.Context, u *model.User, email, provider string) {
	if strings.EqualFold(strings.TrimSpace(u.Email), email) {
		return
	}
	if u.HasPassword() {
		return
	}
	n, err := s.identities.CountByUser(ctx, u.ID)
	if err != nil || n != 1 {
		return
	}
	if other, ferr := s.users.FindByEmail(ctx, email); ferr == nil {
		slog.WarnContext(ctx, "oauth email drift: address belongs to another user",
			"user_id", u.ID.String(), "other_user_id", other.ID.String(), "provider", provider)
		return
	}
	if err := s.users.ReplaceVerifiedEmail(ctx, u.ID, email); err != nil {
		logWarn(ctx, "oauth email drift: replace failed", err, "user_id", u.ID.String())
	}
}

func (s *Service) mintHandoff(ctx context.Context, st *model.OAuthState, userID vo.Id, provider string, idToken *string) string {
	code, err := oidc.RandomToken()
	if err != nil {
		return s.errorURL(st.Client, "provider_error")
	}
	now := s.clock.Now()
	if err := s.handoffs.Insert(ctx, &model.OAuthHandoff{CodeHash: oidc.Sha256Hex(code), UserID: userID, Provider: provider,
		IDToken: idToken, CreatedAt: now, ExpiresAt: now.Add(model.OAuthHandoffTTL)}); err != nil {
		logWarn(ctx, "oauth callback: handoff insert", err, "provider", provider)
		return s.errorURL(st.Client, "provider_error")
	}
	reqctx.AddLogAttr(ctx, "user_id", userID.String())
	return s.successURL(st.Client, code)
}

func (s *Service) link(ctx context.Context, st *model.OAuthState, provider, subject, email string) string {
	now := s.clock.Now()
	existing, err := s.identities.GetByProviderSubject(ctx, provider, subject)
	switch {
	case err == nil && !existing.UserID.Equal(st.LinkUserID):
		return s.errorURL(st.Client, "identity_taken")
	case err == nil:
		existing.UpdateEmail(email, now)
		if serr := s.identities.Save(ctx, existing); serr != nil {
			return s.errorURL(st.Client, "provider_error")
		}
		return s.linkedURL(st.Client, provider)
	}
	if _, ok := errs.AsNotFound(err); !ok {
		logWarn(ctx, "oauth link: identity lookup", err, "provider", provider)
		return s.errorURL(st.Client, "provider_error")
	}
	if _, gerr := s.identities.GetByUserProvider(ctx, st.LinkUserID, provider); gerr == nil {
		return s.errorURL(st.Client, "provider_already_linked")
	}
	if serr := s.identities.Save(ctx, model.NewIdentity(s.identities.NextIdentity(), st.LinkUserID, provider, subject, email, now)); serr != nil {
		logWarn(ctx, "oauth link: identity insert", serr, "provider", provider)
		return s.errorURL(st.Client, "provider_error")
	}
	reqctx.AddLogAttr(ctx, "user_id", st.LinkUserID.String())
	return s.linkedURL(st.Client, provider)
}
```

(imports: `log/slog`, `vo`.) Note `u.Email` on the port's `*model.User` is the encrypted value; the API runs salt-free so it is plaintext, and the fake in tests stores plaintext. Compare case-insensitively as written.

`internal/oauth/handoff.go`:

```go
package oauth

import (
	"context"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// ExchangeHandoff redeems a one-shot code for a session. The row is deleted
// before anything else so a replay races nothing.
func (s *Service) ExchangeHandoff(ctx context.Context, req model.ExchangeHandoffRequest, userAgent string) (*model.LoginResult, error) {
	invalid := &errs.UnauthorizedError{Msg: "Sign-in link is invalid or has expired", Code: errs.CodeOAuthHandoffInvalid}
	hash := oidc.Sha256Hex(req.Code)
	h, err := s.handoffs.Get(ctx, hash)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, invalid
		}
		return nil, err
	}
	if derr := s.handoffs.Delete(ctx, hash); derr != nil {
		return nil, derr
	}
	if h.IsExpired(s.clock.Now()) {
		return nil, invalid
	}
	res, err := s.users.MintSession(ctx, h.UserID, userAgent, h.Provider, h.IDToken)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "user_id", h.UserID.String())
	reqctx.AddLogAttr(ctx, "provider", h.Provider)
	return res, nil
}
```

`internal/oauth/identities.go`:

```go
package oauth

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) ListIdentities(ctx context.Context, userID vo.Id) ([]model.IdentityItem, error) {
	rows, err := s.identities.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.IdentityItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.IdentityItem{Provider: r.Provider, Email: r.Email, CreatedAt: r.CreatedAt.UTC().Format(datetime.Layout)})
	}
	return out, nil
}

// UnlinkIdentity refuses to remove the last identity of a passwordless user:
// it would lock them out.
func (s *Service) UnlinkIdentity(ctx context.Context, userID vo.Id, req model.UnlinkIdentityRequest) (*model.UnlinkIdentityResult, error) {
	if _, err := s.identities.GetByUserProvider(ctx, userID, req.Provider); err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, &errs.ValidationError{Msg: "Linked account not found", MsgCode: errs.CodeOAuthIdentityNotFound}
		}
		return nil, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !u.HasPassword() {
		n, cerr := s.identities.CountByUser(ctx, userID)
		if cerr != nil {
			return nil, cerr
		}
		if n <= 1 {
			return nil, &errs.ValidationError{Msg: "Set a password before unlinking your only sign-in method", MsgCode: errs.CodeOAuthLastIdentity}
		}
	}
	if _, err := s.identities.DeleteByUserProvider(ctx, userID, req.Provider); err != nil {
		return nil, err
	}
	return &model.UnlinkIdentityResult{}, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/oauth/... -cover`
Expected: PASS, ≥ 85%.

- [ ] **Step 5: Commit**

```bash
git add internal/oauth
git commit -m "feat(oauth): sign-in, link, handoff and identity use cases

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: HTTP edge `internal/oauth/api` + allowlist + swagger

**Files:**
- Create: `internal/oauth/api/handler.go`, `internal/oauth/api/routes.go`, `internal/oauth/api/oauth.go`
- Modify: `internal/web/middleware/auth.go` (`ReadonlyAllowedPaths`), `Makefile` (`SWAG_INIT` `-d` list), `internal/web/apidoc/docs/*` (regenerated)
- Test: `internal/oauth/api/harness_test.go`, `internal/oauth/api/endpoints_test.go`

**Interfaces (produces):**

```go
package api
type Handlers struct { svc *appoauth.Service }
func NewHandlers(svc *appoauth.Service) *Handlers
func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI
```

Routes (literal strings, exactly):

```
GET  /api/v1/oauth/get-provider-list
POST /api/v1/oauth/start-login
POST /api/v1/oauth/start-link          (auth)
GET  /api/v1/oauth/callback-google
GET  /api/v1/oauth/callback-oidc
POST /api/v1/oauth/callback-apple
POST /api/v1/oauth/exchange-handoff
GET  /api/v1/oauth/get-identity-list   (auth)
POST /api/v1/oauth/unlink-identity     (auth)
```

- [ ] **Step 1: Write the failing tests**

`internal/oauth/api/harness_test.go` — copy `internal/tag/api/harness_test.go` (same in-memory sqlite + `migrate.Run` + `authstub.Authenticator{}` + `router.New`), building the oauth service with the `oidctest.Fake` and a `Users` fake identical in spirit to Task 8's (put it in this test package too; it may insert users via `fixture`). Then `internal/oauth/api/endpoints_test.go`:

```go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// mustQuery parses a URL's query (same helper as internal/oauth's tests; copy it here).

func TestGetProviderList_Public(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/oauth/get-provider-list", "", nil)
	if status != 200 || !strings.Contains(string(env.Data), `"id":"google"`) {
		t.Fatalf("%d %s", status, env.raw)
	}
}

func TestStartLogin_ReturnsAuthorizationURL(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/oauth/start-login", "", map[string]any{"provider": "google", "client": "web"})
	if status != 200 {
		t.Fatalf("%d %s", status, env.raw)
	}
	var res struct{ Url string }
	_ = json.Unmarshal(env.Data, &res)
	if !strings.HasPrefix(res.Url, h.fake.IssuerURL()+"/authorize?") {
		t.Fatalf("url %s", res.Url)
	}
	status, _ = h.do(t, http.MethodPost, "/api/v1/oauth/start-login", "", map[string]any{"provider": "apple", "client": "web"})
	if status != 400 {
		t.Fatalf("unconfigured provider must be 400, got %d", status)
	}
}

func TestCallbackGoogle_RedirectsWithHandoff_ThenExchange(t *testing.T) {
	h := newHarness(t)
	_, env := h.do(t, http.MethodPost, "/api/v1/oauth/start-login", "", map[string]any{"provider": "google", "client": "web"})
	var res struct{ Url string }
	_ = json.Unmarshal(env.Data, &res)
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))

	resp := h.rawGet(t, "/api/v1/oauth/callback-google?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(q.Get("state")))
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://app.example.test/oauth/callback#handoff=") {
		t.Fatalf("location %s", loc)
	}
	frag, _ := url.ParseQuery(strings.SplitN(loc, "#", 2)[1])
	status, raw := h.doRaw(t, http.MethodPost, "/api/v1/oauth/exchange-handoff", "", map[string]any{"code": frag.Get("handoff")})
	if status != 200 || !strings.Contains(string(raw), `"token":"eco_ses_`) || strings.Contains(string(raw), `"success"`) {
		t.Fatalf("exchange must be the raw login shape: %d %s", status, raw)
	}
	status, _ = h.doRaw(t, http.MethodPost, "/api/v1/oauth/exchange-handoff", "", map[string]any{"code": frag.Get("handoff")})
	if status != 401 {
		t.Fatalf("second exchange must be 401, got %d", status)
	}
}

func TestCallbackApple_FormPost(t *testing.T) {
	h := newHarness(t) // the harness configures the fake as "apple" too (form_post, no PKCE)
	_, env := h.do(t, http.MethodPost, "/api/v1/oauth/start-login", "", map[string]any{"provider": "apple", "client": "app"})
	var res struct{ Url string }
	_ = json.Unmarshal(env.Data, &res)
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), "")
	form := url.Values{"code": {code}, "state": {q.Get("state")}, "user": {`{"name":{"firstName":"Ada","lastName":"Lovelace"}}`}}
	resp := h.rawPostForm(t, "/api/v1/oauth/callback-apple", form)
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "econumo://oauth?handoff=") {
		t.Fatalf("%d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestCallback_ErrorRedirect(t *testing.T) {
	h := newHarness(t)
	resp := h.rawGet(t, "/api/v1/oauth/callback-oidc?code=x&state=bogus")
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("%d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestIdentityEndpoints_RequireAuth(t *testing.T) {
	h := newHarness(t)
	if status, _ := h.do(t, http.MethodGet, "/api/v1/oauth/get-identity-list", "", nil); status != 401 {
		t.Fatalf("want 401, got %d", status)
	}
	token := h.issueToken(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/oauth/get-identity-list", token, nil)
	if status != 200 || string(env.Data) != "[]" {
		t.Fatalf("%d %s", status, env.raw)
	}
	status, _ = h.do(t, http.MethodPost, "/api/v1/oauth/unlink-identity", token, map[string]any{"provider": "google"})
	if status != 400 {
		t.Fatalf("unlinking nothing is 400, got %d", status)
	}
	status, env = h.do(t, http.MethodPost, "/api/v1/oauth/start-link", token, map[string]any{"provider": "google", "client": "web"})
	if status != 200 || !strings.Contains(string(env.Data), `"url"`) {
		t.Fatalf("%d %s", status, env.raw)
	}
}
```

The harness needs `rawGet`/`rawPostForm` helpers that use a client with `CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }`, and `doRaw` returning the body without envelope decoding. Fake users in this harness: `MintSession` must return a token starting with `eco_ses_` (any 43-char suffix).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/oauth/api/ -v`
Expected: compile errors.

- [ ] **Step 3: Implement**

`handler.go`:

```go
// Package api wires the oauth module's HTTP edge.
package api

import appoauth "github.com/econumo/econumo/internal/oauth"

type Handlers struct {
	svc *appoauth.Service
}

func NewHandlers(svc *appoauth.Service) *Handlers { return &Handlers{svc: svc} }
```

`routes.go`:

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

// RegisterAPI mounts the nine oauth endpoints. The callbacks are one literal
// route per provider so the apiparity route scanner keeps them under guard.
func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		// Public group (no auth).
		mux.HandleFunc("GET /api/v1/oauth/get-provider-list", h.GetProviderList)
		mux.HandleFunc("POST /api/v1/oauth/start-login", h.StartLogin)
		mux.HandleFunc("GET /api/v1/oauth/callback-google", h.CallbackGoogle)
		mux.HandleFunc("GET /api/v1/oauth/callback-oidc", h.CallbackOIDC)
		mux.HandleFunc("POST /api/v1/oauth/callback-apple", h.CallbackApple)
		mux.HandleFunc("POST /api/v1/oauth/exchange-handoff", h.ExchangeHandoff)

		// Authenticated group.
		mux.Handle("POST /api/v1/oauth/start-link", auth(h.StartLink))
		mux.Handle("GET /api/v1/oauth/get-identity-list", auth(h.GetIdentityList))
		mux.Handle("POST /api/v1/oauth/unlink-identity", auth(h.UnlinkIdentity))
	}
}
```

`oauth.go` (annotations follow `internal/user/api/user.go`; authed routes document 401+500; the two allowlisted POSTs must NOT document 402):

```go
package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
)

// GetProviderList handles GET /api/v1/oauth/get-provider-list (public).
//
// @Summary     List sign-in providers
// @Description Enabled OAuth/OIDC providers in display order (google, apple, oidc).
// @Tags        OAuth
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=[]model.ProviderItem}
// @Failure     500 {object} apidoc.JsonResponseException
// @Router      /api/v1/oauth/get-provider-list [get]
func (h *Handlers) GetProviderList(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, h.svc.ListProviders())
}

// StartLogin handles POST /api/v1/oauth/start-login (public).
//
// @Summary     Start a provider sign-in
// @Description Creates the authorization request and returns the provider URL to navigate to.
// @Tags        OAuth
// @Accept      json
// @Produce     json
// @Param       request body     model.StartOAuthRequest true "Provider and client"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.StartOAuthResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/oauth/start-login [post]
func (h *Handlers) StartLogin(w http.ResponseWriter, r *http.Request) {
	endpoint.HandlePublic(w, r, h.svc.StartLogin)
}

// StartLink handles POST /api/v1/oauth/start-link (auth).
//
// @Summary     Start linking a provider to the current account
// @Tags        OAuth
// @Accept      json
// @Produce     json
// @Param       request body     model.StartOAuthRequest true "Provider and client"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.StartOAuthResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/oauth/start-link [post]
func (h *Handlers) StartLink(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.StartOAuthRequest) (*model.StartOAuthResult, error) {
		return h.svc.StartLink(ctx, userID, req)
	})
}

// CallbackGoogle handles GET /api/v1/oauth/callback-google (public): the
// provider's redirect. Always answers with a 302 to the SPA or the app scheme.
//
// @Summary     Google callback
// @Tags        OAuth
// @Param       code  query string false "Authorization code"
// @Param       state query string true  "State"
// @Param       error query string false "Provider error"
// @Success     302
// @Router      /api/v1/oauth/callback-google [get]
func (h *Handlers) CallbackGoogle(w http.ResponseWriter, r *http.Request) {
	h.callbackQuery(w, r, model.OAuthProviderGoogle)
}

// CallbackOIDC handles GET /api/v1/oauth/callback-oidc (public).
//
// @Summary     Custom OIDC callback
// @Tags        OAuth
// @Param       code  query string false "Authorization code"
// @Param       state query string true  "State"
// @Param       error query string false "Provider error"
// @Success     302
// @Router      /api/v1/oauth/callback-oidc [get]
func (h *Handlers) CallbackOIDC(w http.ResponseWriter, r *http.Request) {
	h.callbackQuery(w, r, model.OAuthProviderOIDC)
}

// CallbackApple handles POST /api/v1/oauth/callback-apple (public): Apple's
// response_mode=form_post lands here as form fields.
//
// @Summary     Apple callback
// @Tags        OAuth
// @Accept      x-www-form-urlencoded
// @Param       code  formData string false "Authorization code"
// @Param       state formData string true  "State"
// @Param       user  formData string false "Apple's one-time user JSON"
// @Param       error formData string false "Provider error"
// @Success     302
// @Router      /api/v1/oauth/callback-apple [post]
func (h *Handlers) CallbackApple(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, h.svc.Callback(r.Context(), model.OAuthProviderApple, appoauth.CallbackInput{}), http.StatusFound)
		return
	}
	in := appoauth.CallbackInput{Code: r.PostForm.Get("code"), State: r.PostForm.Get("state"),
		Error: r.PostForm.Get("error"), AppleUser: r.PostForm.Get("user")}
	reqctx.AddLogAttr(r.Context(), "provider", model.OAuthProviderApple)
	http.Redirect(w, r, h.svc.Callback(r.Context(), model.OAuthProviderApple, in), http.StatusFound)
}

func (h *Handlers) callbackQuery(w http.ResponseWriter, r *http.Request, provider string) {
	q := r.URL.Query()
	in := appoauth.CallbackInput{Code: q.Get("code"), State: q.Get("state"), Error: q.Get("error")}
	reqctx.AddLogAttr(r.Context(), "provider", provider)
	http.Redirect(w, r, h.svc.Callback(r.Context(), provider, in), http.StatusFound)
}

// ExchangeHandoff handles POST /api/v1/oauth/exchange-handoff (public). Like
// login-user it answers with the raw {token,user} body, not the envelope.
//
// @Summary     Exchange a sign-in handoff for a session
// @Tags        OAuth
// @Accept      json
// @Produce     json
// @Param       request body     model.ExchangeHandoffRequest true "Handoff code"
// @Success     200     {object} model.LoginResult "Raw {token,user} body — NOT wrapped in the standard envelope (same shape as login-user)."
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/oauth/exchange-handoff [post]
func (h *Handlers) ExchangeHandoff(w http.ResponseWriter, r *http.Request) {
	var req model.ExchangeHandoffRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.ExchangeHandoff(r.Context(), req, r.Header.Get("User-Agent"))
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.Raw(w, res)
}

// GetIdentityList handles GET /api/v1/oauth/get-identity-list (auth).
//
// @Summary     List linked accounts
// @Tags        OAuth
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=[]model.IdentityItem}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/oauth/get-identity-list [get]
func (h *Handlers) GetIdentityList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.ListIdentities)
}

// UnlinkIdentity handles POST /api/v1/oauth/unlink-identity (auth).
//
// @Summary     Unlink a provider from the current account
// @Tags        OAuth
// @Accept      json
// @Produce     json
// @Param       request body     model.UnlinkIdentityRequest true "Provider"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UnlinkIdentityResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/oauth/unlink-identity [post]
func (h *Handlers) UnlinkIdentity(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.UnlinkIdentityRequest) (*model.UnlinkIdentityResult, error) {
		return h.svc.UnlinkIdentity(ctx, userID, req)
	})
}
```

`internal/web/middleware/auth.go` — add to `ReadonlyAllowedPaths` with a comment line in the doc block ("linking/unlinking a sign-in method is an account-security operation"):

```go
	"/api/v1/oauth/start-link":              true,
	"/api/v1/oauth/unlink-identity":         true,
```

`Makefile`: add `,../../oauth` to the `-d` list in `SWAG_INIT` (after `../../system`). Then run `make swagger` and commit the regenerated `internal/web/apidoc/docs/`.

- [ ] **Step 4: Run tests**

Run: `make swagger && go test ./internal/oauth/... ./internal/web/... && make go-lint`
Expected: PASS; lint clean including `swagger-check`.

- [ ] **Step 5: Commit**

```bash
git add internal/oauth internal/web Makefile
git commit -m "feat(oauth): HTTP edge, readonly allowlist, OpenAPI docs

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 10: Composition root wiring

**Files:**
- Create: `internal/server/glue_oauth_users.go`, `internal/server/glue_user_logout.go`
- Modify: `internal/server/server.go` (build providers, repos, service, handlers; compose routes; boot warning)
- Test: `internal/server/oauth_wiring_test.go`

- [ ] **Step 1: Write the failing test**

`internal/server/oauth_wiring_test.go` (in-package; look at an existing `server_test.go` for how a test builds `BuildAPI` over `dbtest`):

```go
package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestBuildAPI_MountsOAuthRoutesAndProviderList(t *testing.T) {
	db := dbtest.NewSQLite(t)
	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", AllowRegistration: true,
		AppURL: "https://app.example.test", OAuthGoogleClientID: "g", OAuthGoogleClientSecret: "s",
		RateLimitWindow: 15 * time.Minute}
	srv := httptest.NewServer(BuildAPI(cfg, db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/api/v1/oauth/get-provider-list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), `{"id":"google","name":"Google"}`) {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
}

func TestBuildAPI_NoProvidersIsEmptyList(t *testing.T) {
	db := dbtest.NewSQLite(t)
	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", RateLimitWindow: 15 * time.Minute}
	srv := httptest.NewServer(BuildAPI(cfg, db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	resp, _ := http.Get(srv.URL + "/api/v1/oauth/get-provider-list")
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"data":[]`) {
		t.Fatalf("%s", body)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/server/ -run OAuth -v`
Expected: 404 on the route.

- [ ] **Step 3: Implement**

`glue_oauth_users.go`:

```go
// OAuthUsers adapts the user service to the oauth feature's Users port. It
// lives here because features never import each other.
package server

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

type OAuthUsers struct{ users *appuser.Service }

var _ appoauth.Users = (*OAuthUsers)(nil)

func NewOAuthUsers(users *appuser.Service) *OAuthUsers { return &OAuthUsers{users: users} }

func (a *OAuthUsers) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	return a.users.GetByEmail(ctx, email)
}
func (a *OAuthUsers) FindByID(ctx context.Context, id vo.Id) (*model.User, error) {
	return a.users.GetByID(ctx, id)
}
func (a *OAuthUsers) ProvisionExternal(ctx context.Context, name, email string) (*model.User, error) {
	return a.users.ProvisionExternalUser(ctx, name, email)
}
func (a *OAuthUsers) ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string) error {
	return a.users.ReplaceVerifiedEmail(ctx, userID, email)
}
func (a *OAuthUsers) MintSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string) (*model.LoginResult, error) {
	return a.users.CreateExternalSession(ctx, userID, userAgent, provider, idToken)
}
```

`glue_user_logout.go`:

```go
package server

import (
	"context"

	appoauth "github.com/econumo/econumo/internal/oauth"
	appuser "github.com/econumo/econumo/internal/user"
)

// oauthLogoutURLs adapts the oauth service to the user feature's
// LogoutURLBuilder port (RP-initiated logout for the custom slot).
type oauthLogoutURLs struct{ oauth *appoauth.Service }

var _ appuser.LogoutURLBuilder = oauthLogoutURLs{}

func (a oauthLogoutURLs) EndSessionURL(ctx context.Context, provider, idToken string) (string, error) {
	return a.oauth.EndSessionURL(ctx, provider, idToken)
}
```

`server.go` — after `userHandlers := …` add:

```go
	oauthProviders, err := appoauth.ProvidersFromConfig(cfg, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	oauthSvc := appoauth.NewService(oauthProviders, NewOAuthUsers(userSvc),
		oauthrepo.NewIdentityRepo(cfg.DatabaseDriver, txm), oauthrepo.NewStateRepo(cfg.DatabaseDriver, txm),
		oauthrepo.NewHandoffRepo(cfg.DatabaseDriver, txm), txm, clk, cfg.AppURL, cfg.AllowRegistration)
	userSvc.SetLogoutURLBuilder(oauthLogoutURLs{oauth: oauthSvc})
	oauthHandlers := handleroauth.NewHandlers(oauthSvc)
```

and `handleroauth.RegisterAPI(oauthHandlers, authn),` right after the user line in `router.Compose`. Imports: `appoauth "github.com/econumo/econumo/internal/oauth"`, `oauthrepo "github.com/econumo/econumo/internal/oauth/repo"`, `handleroauth "github.com/econumo/econumo/internal/oauth/api"`.

In `cmd/econumo/main.go` after the email-verification warning, add the boot probe (spec §3: warn, never fail):

```go
	// Provider discovery is lazy; probe once so a misconfigured issuer shows up
	// in the boot log instead of on the first sign-in attempt.
	for _, p := range oauthProbe(cfg) {
		slog.Warn("oauth provider discovery failed at boot; sign-in through it will fail until it is reachable", "provider", p.id, "err", p.err)
	}
```

with, in the same file:

```go
type probeResult struct {
	id  string
	err string
}

func oauthProbe(cfg config.Config) []probeResult {
	providers, err := oauth.ProvidersFromConfig(cfg, nil)
	if err != nil {
		return []probeResult{{id: "config", err: err.Error()}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out []probeResult
	for _, p := range providers {
		if _, derr := p.Client.Discover(ctx); derr != nil {
			out = append(out, probeResult{id: p.Client.Issuer().ID, err: derr.Error()})
		}
	}
	return out
}
```

(import `oauth "github.com/econumo/econumo/internal/oauth"`.) A bad Apple key is a config error and DOES fail boot through `server.Build`; the probe only warns on unreachable issuers.

- [ ] **Step 4: Run tests**

Run: `go build ./... && go test ./internal/server/ ./internal/test/archtest/`
Expected: PASS (archtest confirms `oauth` imports no feature).

- [ ] **Step 5: Commit**

```bash
git add internal/server cmd/econumo
git commit -m "feat(oauth): wire the oauth feature into the composition root

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 11: apiparity coverage and goldens

**Files:**
- Modify: `internal/test/apiparity/harness.go` (`do`, config), `normalize.go`, `smoke_test.go`, `guard_test.go` (`minRoutes`)
- Create: `internal/test/apiparity/catalogue_oauth.go`
- Regenerate: `internal/test/apiparity/testdata/golden/*.golden`
- Test: `internal/test/apiparity` (existing suites), `internal/test/enginecompare` (tagged)

- [ ] **Step 1: Extend the harness to capture redirects**

In `harness.go`:
- Add to `Harness` a `client *http.Client` created in `NewHarness` as
  ```go
  client := srv.Client()
  client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
  ```
  and use `h.client.Do(req)` in `do`.
- Change `do` to return `(int, []byte)` where, for a 3xx response, the body bytes are replaced by `[]byte("Location: " + resp.Header.Get("Location"))` so the golden records the redirect target.
- Add `Call.Form url.Values` (a form-encoded POST body; when non-nil it wins over `Body`, content type `application/x-www-form-urlencoded`) and route it in `Replay`/`do`.
- In `NewHarness` config add `AppURL: "https://app.example.test"` and ONLY the custom slot, pointing at an `oidctest.Fake` started per harness (`f := oidctest.New(t)`; `OIDCIssuerURL: f.IssuerURL()`, `OIDCClientID: f.ClientID`, `OIDCClientSecret: f.Secret`, `OIDCName: "Fake IdP"`, `OIDCScopes: []string{"openid", "profile", "email"}`). Google and Apple stay unconfigured so the smoke suite never touches the network; the provider-list golden shows one entry. `AppURL` also installs `mailer.WithAppLink`, which appends a line to reset emails; the harness's `resetCodeRe` still matches (verify when the goldens regenerate).

In `normalize.go` add:

```go
	// OAuth authorization URLs carry per-request state/nonce/PKCE values.
	oauthParamRe = regexp.MustCompile(`([?&](?:state|nonce|code_challenge)=)[A-Za-z0-9_-]{43}`)
	// The fake issuer's port differs per run.
	fakeIssuerRe = regexp.MustCompile(`http://127\.0\.0\.1:\d+`)
```

and in `NormalizeParity`: `s = oauthParamRe.ReplaceAllString(s, "${1}<random>")`, `s = fakeIssuerRe.ReplaceAllString(s, "<issuer>")`.

- [ ] **Step 2: Write the catalogue**

`internal/test/apiparity/catalogue_oauth.go`:

```go
package apiparity

import "net/url"

// OAuth-module scenarios. The success callback needs a live consent step and is
// covered by internal/oauth's own suite; here the deterministic edges are
// pinned: provider list, start-login/link URLs (random params redacted), the
// error redirects, the raw handoff 401, and the identity list/unlink envelopes.
func init() {
	register(Scenario{Name: "oauth_flows", Calls: func() []Call {
		return []Call{
			{Label: "get-provider-list", Method: "GET", Path: "/api/v1/oauth/get-provider-list"},
			{Label: "start-login", Method: "POST", Path: "/api/v1/oauth/start-login", Body: map[string]any{"provider": "oidc", "client": "web"}},
			{Label: "err:start-login-unconfigured", Method: "POST", Path: "/api/v1/oauth/start-login", Body: map[string]any{"provider": "apple", "client": "web"}},
			{Label: "err:start-login-bad-client", Method: "POST", Path: "/api/v1/oauth/start-login", Body: map[string]any{"provider": "oidc", "client": "desktop"}},
			{Label: "start-link", Method: "POST", Path: "/api/v1/oauth/start-link", Auth: "owner", Body: map[string]any{"provider": "oidc", "client": "app"}},
			{Label: "start-link-readonly-allowed", Method: "POST", Path: "/api/v1/oauth/start-link", Auth: "readonly", Body: map[string]any{"provider": "oidc", "client": "web"}},
			{Label: "err:callback-oidc-bad-state", Method: "GET", Path: "/api/v1/oauth/callback-oidc?code=x&state=bogus"},
			{Label: "err:callback-google-bad-state", Method: "GET", Path: "/api/v1/oauth/callback-google?code=x&state=bogus"},
			{Label: "err:callback-apple-bad-state", Method: "POST", Path: "/api/v1/oauth/callback-apple", Form: url.Values{"code": {"x"}, "state": {"bogus"}}},
			{Label: "err:exchange-handoff-unknown", Method: "POST", Path: "/api/v1/oauth/exchange-handoff", Body: map[string]any{"code": "nope"}},
			{Label: "err:exchange-handoff-blank", Method: "POST", Path: "/api/v1/oauth/exchange-handoff", Body: map[string]any{"code": ""}},
			{Label: "get-identity-list-empty", Method: "GET", Path: "/api/v1/oauth/get-identity-list", Auth: "owner"},
			{Label: "err:unlink-identity-missing", Method: "POST", Path: "/api/v1/oauth/unlink-identity", Auth: "owner", Body: map[string]any{"provider": "google"}},
			{Label: "get-identity-list-seeded", Method: "GET", Path: "/api/v1/oauth/get-identity-list", Auth: "guest"},
			{Label: "unlink-identity", Method: "POST", Path: "/api/v1/oauth/unlink-identity", Auth: "guest", Body: map[string]any{"provider": "google"}},
			{Label: "get-identity-list-after-unlink", Method: "GET", Path: "/api/v1/oauth/get-identity-list", Auth: "guest"},
		}
	}})
}
```

Seed in `fixture.go`'s `Seed`: one identity for the guest — `fixture.New(t, db).Identity(fixture.Identity{ID: "1d000000-0000-0000-0000-000000000001", UserID: GuestID, Provider: "google", Subject: "guest-google-sub", Email: GuestEmail})`. The guest has a password, so unlinking their only identity succeeds.

Also add a `user_sessions`-style check that logout now returns the two new fields: the existing session scenario's golden will change on regeneration; inspect it.

- [ ] **Step 3: Bump the route floor**

`guard_test.go`: `// 112 -> 121 on 2026-09-07: the 9 oauth routes were added.` and `const minRoutes = 121`.

- [ ] **Step 4: Regenerate goldens and inspect**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff --stat internal/test/apiparity/testdata/golden`
Expected diff: the new `oauth_flows.golden`; `hasPassword` added to every current-user payload (login, register, get-user-data); `provider` on session-list items; `logoutUrl`/`provider` on logout. Nothing else. Read the new golden end to end: `start-login` shows `<issuer>/authorize?...state=<random>...`, the three callbacks show `Location: https://app.example.test/login?oauthError=invalid_state`, the handoff 401 shows the frozen unauthorized envelope with the translated message.

- [ ] **Step 5: Run the suites**

Run: `go test ./internal/test/... && make go-test`
Expected: PASS, coverage ≥ 80.

Optionally, if PostgreSQL is available: `make test-repo-pgsql`.

- [ ] **Step 6: Commit**

```bash
git add internal/test
git commit -m "test(oauth): apiparity scenarios, redirect capture, goldens

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 12: Catalogue keys in all 11 languages

**Files:**
- Modify: `locales/de.json`, `en.json`, `es.json`, `fr.json`, `it.json`, `nl.json`, `pl.json`, `pt.json`, `ru.json`, `uk.json`, `zh.json`
- Test: `internal/test/i18ntest` (existing guards)

**Interfaces (produces):** the keys below, present in every catalogue with the same `{placeholders}`.

- [ ] **Step 1: Run the guard to see the current failure**

Run: `go test ./internal/test/i18ntest/`
Expected: FAIL — `errors.oauth.*` missing for the four codes registered in Task 3.

- [ ] **Step 2: Add the English keys**

In `locales/en.json`:

Under `errors` (a new `oauth` object, sibling of `user`):

```json
"oauth": {
  "provider_not_configured": "This sign-in provider is not configured.",
  "handoff_invalid": "The sign-in link is invalid or has expired. Please try again.",
  "last_identity": "Set a password before unlinking your only sign-in method.",
  "identity_not_found": "This account is not linked."
}
```

Under `auth` (a new `oauth` object):

```json
"oauth": {
  "divider": "or continue with",
  "button": {
    "google": "Continue with Google",
    "apple": "Continue with Apple",
    "oidc": "Continue with {name}"
  },
  "callback": {
    "signing_in": "Signing you in…"
  },
  "logout_notice": "You're signed out of Econumo. Your {provider} session may still be active; sign out there to end it.",
  "errors": {
    "denied": "Sign-in was cancelled.",
    "invalid_state": "The sign-in attempt expired or was already used. Please try again.",
    "provider_error": "The sign-in provider returned an error. Please try again.",
    "email_required": "The sign-in provider did not share an email address, which Econumo requires.",
    "email_unverified": "The sign-in provider has not verified this email address.",
    "registration_disabled": "Registration is disabled on this server. Sign in with an existing account first.",
    "identity_taken": "This external account is already linked to another Econumo account.",
    "provider_already_linked": "Your account already has a different account linked for this provider.",
    "account_inactive": "This account has been deactivated."
  }
}
```

Under `user.page.settings.profile` (sibling of `sessions` and `tokens`):

```json
"linked_accounts": {
  "menu_item": "Linked accounts",
  "header": "Linked accounts",
  "description": "External accounts you can sign in with. Link one to sign in without a password.",
  "empty": "No linked accounts yet.",
  "linked_on": "Linked",
  "link": "Link",
  "unlink": "Unlink",
  "confirm_unlink": "Unlink this account? You will no longer be able to sign in with it.",
  "last_identity_hint": "Set a password before unlinking your only sign-in method.",
  "linked_toast": "{provider} account linked.",
  "set_password": {
    "menu_item": "Set a password",
    "description": "You signed up with an external account. We'll email you a code to set a password."
  }
},
```

Under `user.page.settings.profile.sessions` add `"via": "via {provider}"`.

Provider display names used by the SPA when only an id is known (`auth.oauth.provider_name`):

```json
"provider_name": {
  "google": "Google",
  "apple": "Apple",
  "oidc": "SSO"
}
```

(place it inside `auth.oauth`).

- [ ] **Step 3: Translate into the other ten catalogues**

Add the same keys with translated values to `de`, `es`, `fr`, `it`, `nl`, `pl`, `pt`, `ru`, `uk`, `zh`, keeping every `{name}` / `{provider}` placeholder verbatim and the JSON structure identical. Product names (Google, Apple, Econumo, SSO) stay untranslated.

- [ ] **Step 4: Run the guards**

Run: `go test ./internal/test/i18ntest/`
Expected: PASS (key parity, placeholder parity, code coverage).

- [ ] **Step 5: Commit**

```bash
git add locales
git commit -m "feat(oauth): catalogue keys for sign-in providers and linked accounts

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 13: Frontend API client, hooks, metrics, plumbing

**Files:**
- Create: `web/src/api/dto/oauth.ts`, `web/src/api/oauth.ts`, `web/src/features/auth/oauthQueries.ts`
- Modify: `web/src/api/client.ts` (401 exclusion), `web/src/api/user.ts` (`logout` return type), `web/src/api/dto/user.ts` (`hasPassword`, `provider`, `LogoutResultDto`), `web/src/lib/metrics.ts` (4 keys), `web/src/lib/externalLinks.ts` (`BrowserPlugin.close`), `web/src/app/router-pages.ts`
- Test: `web/src/api/oauth.test.ts`, `web/src/features/auth/oauthQueries.test.tsx`

**Interfaces (produces):**

```ts
// api/dto/oauth.ts
export type OAuthProviderId = 'google' | 'apple' | 'oidc'
export interface ProviderDto { id: OAuthProviderId; name: string }
export interface IdentityDto { provider: OAuthProviderId; email: string; createdAt: string }
// api/oauth.ts
export function getProviderList(): Promise<ProviderDto[]>
export function startLogin(provider: OAuthProviderId, client: 'web' | 'app'): Promise<string>  // the url
export function startLink(provider: OAuthProviderId, client: 'web' | 'app'): Promise<string>
export function exchangeHandoff(code: string): Promise<UserLoginItemDto>
export function getIdentityList(): Promise<IdentityDto[]>
export function unlinkIdentity(provider: OAuthProviderId): Promise<void>
// features/auth/oauthQueries.ts
export function useProviders(): UseQueryResult<ProviderDto[]>
export function useStartOAuth(): UseMutationResult<void, unknown, { provider: OAuthProviderId; intent: 'login' | 'link' }>  // navigates
export function useExchangeHandoff(): UseMutationResult<UserLoginItemDto, unknown, string>
export function oauthClient(): 'web' | 'app'
export function openAuthorizationUrl(url: string): void   // location.assign on web, Browser.open in the app
// api/dto/user.ts
export interface LogoutResultDto { result: string; logoutUrl: string; provider: string }
CurrentUserDto.hasPassword: boolean; SessionDto.provider: string
// api/user.ts
export async function logout(): Promise<LogoutResultDto>
// lib/metrics.ts
OAUTH_LOGIN_COMPLETED: 'appOauthLoginCompleted', OAUTH_ACCOUNT_CREATED: 'appOauthAccountCreated', IDENTITY_LINKED: 'appIdentityLinked', IDENTITY_UNLINKED: 'appIdentityUnlinked'
// router-pages.ts
OAUTH_CALLBACK: '/oauth/callback', SETTINGS_LINKED_ACCOUNTS: '/settings/profile/linked-accounts'
```

- [ ] **Step 1: Write the failing tests**

`web/src/api/oauth.test.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { exchangeHandoff, getProviderList, startLogin, unlinkIdentity } from './oauth'

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('reads the provider list from the envelope', async () => {
  server.use(http.get('*/api/v1/oauth/get-provider-list', () =>
    HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }] })))
  expect(await getProviderList()).toEqual([{ id: 'google', name: 'Google' }])
})

it('start-login posts provider and client and returns the url', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/oauth/start-login', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { url: 'https://idp/authorize?x=1' } })
  }))
  expect(await startLogin('google', 'web')).toBe('https://idp/authorize?x=1')
  expect(body).toEqual({ provider: 'google', client: 'web' })
})

it('exchange-handoff returns the raw login body and a 401 does not redirect to /login?reason=expired', async () => {
  server.use(http.post('*/api/v1/oauth/exchange-handoff', () =>
    HttpResponse.json({ token: 'eco_ses_x', user: { id: 'u1', options: [], accessLevel: 'full', accessUntil: '' } })))
  const res = await exchangeHandoff('code')
  expect(res.token).toBe('eco_ses_x')
  server.use(http.post('*/api/v1/oauth/exchange-handoff', () =>
    HttpResponse.json({ success: false, message: 'bad', code: 401, errors: {} }, { status: 401 })))
  localStorage.setItem('token', 'keep-me')
  await expect(exchangeHandoff('bad')).rejects.toBeTruthy()
  expect(localStorage.getItem('token')).toBe('keep-me')
})

it('unlink posts the provider', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/oauth/unlink-identity', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  await unlinkIdentity('apple')
  expect(body).toEqual({ provider: 'apple' })
})
```

`web/src/features/auth/oauthQueries.test.tsx`:

```tsx
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { oauthClient, openAuthorizationUrl, useExchangeHandoff, useStartOAuth } from './oauthQueries'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  delete (window as { Capacitor?: unknown }).Capacitor
})

it('oauthClient reports web outside the app and app inside it', () => {
  expect(oauthClient()).toBe('web')
  window.Capacitor = { isNativePlatform: () => true }
  expect(oauthClient()).toBe('app')
})

it('openAuthorizationUrl assigns location on the web and opens the Browser plugin in the app', () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  openAuthorizationUrl('https://idp/a')
  expect(assign).toHaveBeenCalledWith('https://idp/a')
  const open = vi.fn().mockResolvedValue(undefined)
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { open } } }
  openAuthorizationUrl('https://idp/b')
  expect(open).toHaveBeenCalledWith({ url: 'https://idp/b' })
})

it('useStartOAuth posts to start-login for login and start-link for link, then navigates', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  const hits: string[] = []
  server.use(
    http.post('*/api/v1/oauth/start-login', () => { hits.push('login'); return HttpResponse.json({ success: true, message: '', data: { url: 'https://idp/1' } }) }),
    http.post('*/api/v1/oauth/start-link', () => { hits.push('link'); return HttpResponse.json({ success: true, message: '', data: { url: 'https://idp/2' } }) }),
  )
  const { result } = renderHook(() => useStartOAuth(), { wrapper })
  await result.current.mutateAsync({ provider: 'google', intent: 'login' })
  await result.current.mutateAsync({ provider: 'google', intent: 'link' })
  await waitFor(() => expect(hits).toEqual(['login', 'link']))
  expect(assign).toHaveBeenNthCalledWith(1, 'https://idp/1')
  expect(assign).toHaveBeenNthCalledWith(2, 'https://idp/2')
})

it('useExchangeHandoff stores the token and clears the persisted cache', async () => {
  localStorage.setItem('econumo.query-cache', '{"stale":true}')
  server.use(http.post('*/api/v1/oauth/exchange-handoff', () =>
    HttpResponse.json({ token: 'eco_ses_new', user: { id: 'u1', options: [], accessLevel: 'full', accessUntil: '' } })))
  const { result } = renderHook(() => useExchangeHandoff(), { wrapper })
  await result.current.mutateAsync('code')
  expect(localStorage.getItem('token')).toBe('eco_ses_new')
  expect(localStorage.getItem('econumo.query-cache')).toBeNull()
})
```

(`econumo.query-cache` is `QUERY_CACHE_KEY` in `web/src/lib/queryPersist.ts`.)

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm test -- src/api/oauth.test.ts src/features/auth/oauthQueries.test.tsx`
Expected: FAIL (modules missing).

- [ ] **Step 3: Implement**

`web/src/api/dto/oauth.ts`:

```ts
export type OAuthProviderId = 'google' | 'apple' | 'oidc'

export interface ProviderDto {
  id: OAuthProviderId
  name: string
}

export interface IdentityDto {
  provider: OAuthProviderId
  email: string
  /** frozen wire format "YYYY-MM-DD HH:mm:ss" UTC */
  createdAt: string
}
```

`web/src/api/dto/user.ts`: add `hasPassword: boolean` to `CurrentUserDto`, `provider: string` to `SessionDto`, and:

```ts
export interface LogoutResultDto {
  result: string
  /** IdP end-session URL to navigate to; '' when the session logs out locally */
  logoutUrl: string
  /** OAuth provider id the session was opened through; '' for password sessions */
  provider: string
}
```

`web/src/api/oauth.ts`:

```ts
import { api, apiUrl } from './client'
import type { UserLoginItemDto } from './dto/user'
import type { IdentityDto, OAuthProviderId, ProviderDto } from './dto/oauth'
import { UserOptions } from './dto/user'
import { deriveAccessState } from '@/lib/access'
import { analyticsUserId } from '@/lib/analyticsId'
import { rememberAnalyticsPreference } from '@/lib/analyticsPreference'
import { setAnalyticsUser } from '@/lib/analytics'
import { setAnalyticsAccessState } from '@/lib/metrics'

interface Envelope<T> {
  data: T
}

export type OAuthClient = 'web' | 'app'

export async function getProviderList(): Promise<ProviderDto[]> {
  const response = await api.get<Envelope<ProviderDto[]>>(apiUrl('/api/v1/oauth/get-provider-list'))
  return response.data.data
}

export async function startLogin(provider: OAuthProviderId, client: OAuthClient): Promise<string> {
  const response = await api.post<Envelope<{ url: string }>>(apiUrl('/api/v1/oauth/start-login'), { provider, client })
  return response.data.data.url
}

export async function startLink(provider: OAuthProviderId, client: OAuthClient): Promise<string> {
  const response = await api.post<Envelope<{ url: string }>>(apiUrl('/api/v1/oauth/start-link'), { provider, client })
  return response.data.data.url
}

// exchange-handoff answers with the bare {token, user} body like login-user,
// and primes the same analytics identity.
export async function exchangeHandoff(code: string): Promise<UserLoginItemDto> {
  const response = await api.post<UserLoginItemDto>(apiUrl('/api/v1/oauth/exchange-handoff'), { code })
  const { user } = response.data
  setAnalyticsAccessState(deriveAccessState(user.accessLevel, user.accessUntil))
  setAnalyticsUser(analyticsUserId(user.id))
  rememberAnalyticsPreference(user.options.find((o) => o.name === UserOptions.ANALYTICS)?.value !== '0')
  return response.data
}

export async function getIdentityList(): Promise<IdentityDto[]> {
  const response = await api.get<Envelope<IdentityDto[]>>(apiUrl('/api/v1/oauth/get-identity-list'))
  return response.data.data
}

export async function unlinkIdentity(provider: OAuthProviderId): Promise<void> {
  await api.post(apiUrl('/api/v1/oauth/unlink-identity'), { provider })
}
```

`web/src/api/client.ts` line 31: extend the exclusion so a failed handoff does not masquerade as an expired session:

```ts
    const isCredentialExchange = url.includes('/api/v1/user/login-user') || url.includes('/api/v1/oauth/exchange-handoff')
    if (status === 401 && !isCredentialExchange) {
```

`web/src/api/user.ts`: `export async function logout(): Promise<LogoutResultDto> { const response = await api.post<Envelope<LogoutResultDto>>(apiUrl('/api/v1/user/logout-user')); return response.data.data }` (import the type).

`web/src/lib/metrics.ts`: add after `EMAIL_VERIFICATION_RESENT`:

```ts
  OAUTH_LOGIN_COMPLETED: 'appOauthLoginCompleted',
  OAUTH_ACCOUNT_CREATED: 'appOauthAccountCreated',
  IDENTITY_LINKED: 'appIdentityLinked',
  IDENTITY_UNLINKED: 'appIdentityUnlinked',
```

`web/src/lib/externalLinks.ts`: add `close(): Promise<void>` to `BrowserPlugin` and export the interface.

`web/src/app/router-pages.ts`: add `OAUTH_CALLBACK: '/oauth/callback',` after `LOGOUT` and `SETTINGS_LINKED_ACCOUNTS: '/settings/profile/linked-accounts',` after `SETTINGS_TOKENS`.

`web/src/features/auth/oauthQueries.ts`:

```ts
import { useMutation, useQuery } from '@tanstack/react-query'
import * as oauthApi from '@/api/oauth'
import type { OAuthProviderId } from '@/api/dto/oauth'
import { nativePlugin, isNativeApp } from '@/lib/platform'
import type { BrowserPlugin } from '@/lib/externalLinks'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { setToken } from '@/lib/storage'
import { METRICS, trackEvent } from '@/lib/metrics'

export const providersQueryKey = ['oauth', 'providers'] as const

export function oauthClient(): oauthApi.OAuthClient {
  return isNativeApp() ? 'app' : 'web'
}

// A top-level navigation on the web (Google and Apple require it); the system
// browser sheet in the app (embedded web views are blocked by Google).
export function openAuthorizationUrl(url: string): void {
  const browser = nativePlugin<BrowserPlugin>('Browser')
  if (browser) {
    void browser.open({ url })
    return
  }
  window.location.assign(url)
}

export function useProviders() {
  return useQuery({
    queryKey: providersQueryKey,
    queryFn: oauthApi.getProviderList,
    staleTime: Infinity,
  })
}

export function useStartOAuth() {
  return useMutation({
    mutationFn: async ({ provider, intent }: { provider: OAuthProviderId; intent: 'login' | 'link' }) => {
      const url = intent === 'link'
        ? await oauthApi.startLink(provider, oauthClient())
        : await oauthApi.startLogin(provider, oauthClient())
      openAuthorizationUrl(url)
    },
  })
}

export function useExchangeHandoff() {
  return useMutation({
    mutationFn: (code: string) => oauthApi.exchangeHandoff(code),
    onSuccess: (data) => {
      // the new session may belong to a different user — never restore the
      // previous user's persisted finances
      clearPersistedQueryCache()
      setToken(data.token)
      trackEvent(METRICS.OAUTH_LOGIN_COMPLETED)
      if (isFreshAccount(data.user.createdAt)) {
        trackEvent(METRICS.OAUTH_ACCOUNT_CREATED)
      }
    },
  })
}

// The exchange response does not say whether the callback provisioned the
// account; a createdAt within the last two minutes is the proxy (the handoff
// itself lives sixty seconds). createdAt is the frozen UTC "YYYY-MM-DD HH:mm:ss".
export function isFreshAccount(createdAt: string, now: Date = new Date()): boolean {
  const created = Date.parse(createdAt.replace(' ', 'T') + 'Z')
  return Number.isFinite(created) && now.getTime() - created < 2 * 60 * 1000
}
```

Add a unit test for `isFreshAccount` (fresh, old, malformed) to `oauthQueries.test.tsx`.

- [ ] **Step 4: Run tests and lint**

Run: `cd web && pnpm test -- src/api src/features/auth/oauthQueries.test.tsx src/lib/metrics-coverage.test.ts && pnpm lint`
Expected: the two new files PASS; `metrics-coverage` FAILS on `IDENTITY_LINKED` / `IDENTITY_UNLINKED` until Task 16 (expected; do not add them to `NOT_WIRED`). `tsc -b` (via `pnpm build`) must pass.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(oauth): SPA client, hooks and metrics for provider sign-in

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 14: Provider buttons on the login and registration pages

**Files:**
- Create: `web/src/features/auth/ProviderButtons.tsx`, `web/src/features/auth/providerIcons.tsx`
- Modify: `web/src/features/auth/LoginPage.tsx` (after the Forgot-password button; the `oauthError` alert), `web/src/features/auth/RegistrationPage.tsx` (after the submit button)
- Test: `web/src/features/auth/ProviderButtons.test.tsx`, `web/src/features/auth/LoginPage.test.tsx` (append)

- [ ] **Step 1: Write the failing tests**

`web/src/features/auth/ProviderButtons.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { ProviderButtons } from './ProviderButtons'

function renderButtons(intent: 'login' | 'link' = 'login') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={qc}><ProviderButtons intent={intent} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('renders nothing when no provider is configured', async () => {
  server.use(http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: [] })))
  const { container } = renderButtons()
  await new Promise((r) => setTimeout(r, 20))
  expect(container.querySelector('[data-testid="provider-buttons"]')).toBeNull()
})

it('renders one button per provider, in order, and starts the flow on click', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  server.use(
    http.get('*/api/v1/oauth/get-provider-list', () =>
      HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }, { id: 'apple', name: 'Apple' }, { id: 'oidc', name: 'Authentik' }] })),
    http.post('*/api/v1/oauth/start-login', async ({ request }) => {
      const { provider } = (await request.json()) as { provider: string }
      return HttpResponse.json({ success: true, message: '', data: { url: `https://idp/${provider}` } })
    }),
  )
  renderButtons()
  const buttons = await screen.findAllByRole('button')
  expect(buttons.map((b) => b.textContent)).toEqual(['Continue with Google', 'Continue with Apple', 'Continue with Authentik'])
  expect(screen.getByText('or continue with')).toBeInTheDocument()
  await userEvent.click(buttons[2])
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('https://idp/oidc'))
})
```

Append to `LoginPage.test.tsx` (reuse its `renderPage`/route setup; add a provider-list handler returning `[]` to the shared `beforeEach` so existing tests stay hermetic):

```tsx
it('shows the oauth error from the query string', async () => {
  renderPage('/login?oauthError=email_unverified')
  expect(await screen.findByText('The sign-in provider has not verified this email address.')).toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm test -- src/features/auth`
Expected: FAIL.

- [ ] **Step 3: Implement**

`web/src/features/auth/providerIcons.tsx` (inline SVG marks; lucide dropped brand icons, see `LoginLayout.tsx:54`):

```tsx
import type { OAuthProviderId } from '@/api/dto/oauth'
import { KeyRound } from 'lucide-react'

// Google's mark is multi-colour, so it carries its own fills; Apple's uses
// currentColor. Both follow the vendors' branding rules (App Review checks Apple's).
export function GoogleMark() {
  return (
    <svg viewBox="0 0 24 24" className="size-5" aria-hidden="true">
      <path fill="#4285F4" d="M23.49 12.27c0-.79-.07-1.54-.19-2.27H12v4.51h6.47a5.53 5.53 0 0 1-2.4 3.63v3h3.87c2.27-2.09 3.55-5.17 3.55-8.87z" />
      <path fill="#34A853" d="M12 24c3.24 0 5.95-1.08 7.94-2.91l-3.87-3c-1.08.72-2.45 1.16-4.07 1.16-3.13 0-5.78-2.11-6.73-4.96H1.29v3.09A12 12 0 0 0 12 24z" />
      <path fill="#FBBC05" d="M5.27 14.29A7.2 7.2 0 0 1 4.89 12c0-.8.14-1.57.38-2.29V6.62H1.29A12 12 0 0 0 0 12c0 1.94.46 3.77 1.29 5.38l3.98-3.09z" />
      <path fill="#EA4335" d="M12 4.75c1.77 0 3.35.61 4.6 1.8l3.42-3.42C17.95 1.19 15.24 0 12 0 7.31 0 3.26 2.69 1.29 6.62l3.98 3.09C6.22 6.86 8.87 4.75 12 4.75z" />
    </svg>
  )
}

export function AppleMark() {
  return (
    <svg viewBox="0 0 24 24" className="size-5" fill="currentColor" aria-hidden="true">
      <path d="M16.37 12.64c.03 3.02 2.65 4.02 2.68 4.03-.02.07-.42 1.43-1.38 2.84-.83 1.22-1.7 2.43-3.06 2.46-1.34.02-1.77-.8-3.3-.8-1.53 0-2.01.77-3.28.82-1.31.05-2.31-1.32-3.15-2.53-1.72-2.48-3.03-7.02-1.27-10.08.88-1.52 2.44-2.48 4.14-2.51 1.29-.02 2.51.87 3.3.87.79 0 2.27-1.08 3.83-.92.65.03 2.48.26 3.65 1.98-.09.06-2.18 1.27-2.16 3.84zM13.84 4.65c.7-.85 1.17-2.03 1.04-3.2-1.01.04-2.23.67-2.95 1.52-.65.75-1.22 1.95-1.06 3.1 1.12.09 2.27-.57 2.97-1.42z" />
    </svg>
  )
}

export function ProviderMark({ id }: { id: OAuthProviderId }) {
  if (id === 'google') return <GoogleMark />
  if (id === 'apple') return <AppleMark />
  return <KeyRound className="size-5" aria-hidden="true" />
}
```

`web/src/features/auth/ProviderButtons.tsx`:

```tsx
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { OAuthProviderId } from '@/api/dto/oauth'
import { ProviderMark } from './providerIcons'
import { useProviders, useStartOAuth } from './oauthQueries'

// Apple's button must be black-on-white or white-on-black with the mark;
// Google's must be light with the multi-colour mark. Both get the auth pages'
// h-11 so they line up with the password form's buttons.
const buttonClass: Record<OAuthProviderId, string> = {
  google: 'h-11 w-full border border-border bg-white text-black hover:bg-zinc-50',
  apple: 'h-11 w-full bg-black text-white hover:bg-zinc-800',
  oidc: 'h-11 w-full',
}

export function ProviderButtons({ intent }: { intent: 'login' | 'link' }) {
  const { t } = useTranslation()
  const providers = useProviders()
  const start = useStartOAuth()
  if (!providers.data || providers.data.length === 0) {
    return null
  }
  return (
    <div data-testid="provider-buttons" className="flex flex-col gap-3">
      {intent === 'login' ? (
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <div className="h-px flex-1 bg-border" />
          <span>{t('auth.oauth.divider')}</span>
          <div className="h-px flex-1 bg-border" />
        </div>
      ) : null}
      {providers.data.map((p) => (
        <Button
          key={p.id}
          type="button"
          variant={p.id === 'oidc' ? 'secondary' : 'outline'}
          className={buttonClass[p.id]}
          disabled={start.isPending}
          onClick={() => start.mutate({ provider: p.id, intent })}
        >
          <ProviderMark id={p.id} />
          {p.id === 'oidc' ? t('auth.oauth.button.oidc', { name: p.name }) : t(buttonLabel[p.id])}
        </Button>
      ))}
    </div>
  )
}
```

with, above the component:

```tsx
// Explicit keys (not a template literal) so the i18n key guard sees them.
const buttonLabel: Record<OAuthProviderId, string> = {
  google: 'auth.oauth.button.google',
  apple: 'auth.oauth.button.apple',
  oidc: 'auth.oauth.button.oidc',
}
```

`LoginPage.tsx`:
- `const oauthError = searchParams.get('oauthError')` next to `sessionExpired`.
- After the session-expired alert:

```tsx
          {oauthError ? (
            <Alert variant="destructive">
              <AlertDescription>{t(`auth.oauth.errors.${oauthError}`, { defaultValue: t('auth.oauth.errors.provider_error') })}</AlertDescription>
            </Alert>
          ) : null}
```

- After the "Forgot password?" button (line 174), inside the form: `<ProviderButtons intent="login" />`.

`RegistrationPage.tsx`: after the submit button (line 165), `{config.isRegistrationAllowed() ? <ProviderButtons intent="login" /> : null}` (registration page renders only when allowed anyway; keep the guard for clarity).

- [ ] **Step 4: Run tests**

Run: `cd web && pnpm test -- src/features/auth && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(oauth): provider buttons on the login and registration pages

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 15: Callback route and app deep-link handler

**Files:**
- Create: `web/src/features/auth/OAuthCallbackPage.tsx`, `web/src/lib/deepLinks.ts`
- Modify: `web/src/app/routes.tsx` (sibling of `/logout`), `web/src/lib/appBoot.ts` (`AppPlugin` + install)
- Test: `web/src/features/auth/OAuthCallbackPage.test.tsx`, `web/src/lib/deepLinks.test.ts`

**Interfaces (produces):**

```ts
// lib/deepLinks.ts
export function handleAppUrl(url: string): void   // dispatch econumo://oauth?handoff|linked|error
export function installDeepLinkHandler(): void    // App.addListener('appUrlOpen') -> handleAppUrl
```

- [ ] **Step 1: Write the failing tests**

`web/src/features/auth/OAuthCallbackPage.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { OAuthCallbackPage } from './OAuthCallbackPage'

function renderAt(entry: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/oauth/callback', element: <OAuthCallbackPage /> },
      { path: '/', element: <div data-testid="home" /> },
      { path: '/login', element: <div data-testid="login" /> },
    ],
    { initialEntries: [entry] },
  )
  render(<QueryClientProvider client={qc}><RouterProvider router={router} /></QueryClientProvider>)
  return router
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('exchanges the handoff from the fragment, stores the token and goes home', async () => {
  let posted: unknown
  server.use(http.post('*/api/v1/oauth/exchange-handoff', async ({ request }) => {
    posted = await request.json()
    return HttpResponse.json({ token: 'eco_ses_ok', user: { id: 'u1', options: [], accessLevel: 'full', accessUntil: '', hasPassword: false } })
  }))
  renderAt('/oauth/callback#handoff=abc')
  expect(screen.getByText('Signing you in…')).toBeInTheDocument()
  await waitFor(() => expect(screen.getByTestId('home')).toBeInTheDocument())
  expect(posted).toEqual({ code: 'abc' })
  expect(localStorage.getItem('token')).toBe('eco_ses_ok')
})

it('lands on /login with an error when the exchange fails or the fragment is missing', async () => {
  server.use(http.post('*/api/v1/oauth/exchange-handoff', () =>
    HttpResponse.json({ success: false, message: 'x', code: 401, errors: {} }, { status: 401 })))
  const router = renderAt('/oauth/callback#handoff=bad')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toBe('?oauthError=invalid_state')
  const router2 = renderAt('/oauth/callback')
  await waitFor(() => expect(router2.state.location.search).toBe('?oauthError=invalid_state'))
})
```

`web/src/lib/deepLinks.test.ts`:

```ts
import { handleAppUrl, installDeepLinkHandler } from './deepLinks'
import * as routerRef from '@/app/routerRef'

beforeEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
})

it('routes handoff, linked and error urls and closes the browser sheet', () => {
  const close = vi.fn().mockResolvedValue(undefined)
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { close } } }
  const nav = vi.spyOn(routerRef, 'navigateTo').mockImplementation(() => {})
  handleAppUrl('econumo://oauth?handoff=abc')
  expect(nav).toHaveBeenLastCalledWith('/oauth/callback#handoff=abc')
  handleAppUrl('econumo://oauth?linked=google')
  expect(nav).toHaveBeenLastCalledWith('/settings/profile/linked-accounts?linked=google')
  handleAppUrl('econumo://oauth?error=denied')
  expect(nav).toHaveBeenLastCalledWith('/login?oauthError=denied')
  handleAppUrl('https://example.com/other')
  expect(nav).toHaveBeenCalledTimes(3)
  expect(close).toHaveBeenCalledTimes(3)
})

it('installs the appUrlOpen listener on the App plugin', () => {
  const addListener = vi.fn()
  window.Capacitor = { isNativePlatform: () => true, Plugins: { App: { addListener } } }
  installDeepLinkHandler()
  expect(addListener).toHaveBeenCalledWith('appUrlOpen', expect.any(Function))
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm test -- OAuthCallbackPage deepLinks`
Expected: FAIL.

- [ ] **Step 3: Implement**

`web/src/features/auth/OAuthCallbackPage.tsx`:

```tsx
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router'
import { CoinLoader } from '@/components/CoinLoader'
import { RouterPage } from '@/app/router-pages'
import { useExchangeHandoff } from './oauthQueries'

// The handoff rides in the fragment so it never reaches server logs; it is
// consumed exactly once and the fragment is dropped from history on arrival.
export function OAuthCallbackPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { hash } = useLocation()
  const exchange = useExchangeHandoff()
  const started = useRef(false)

  useEffect(() => {
    if (started.current) {
      return
    }
    started.current = true
    const code = new URLSearchParams(hash.replace(/^#/, '')).get('handoff')
    if (window.location.hash) {
      window.history.replaceState(null, '', window.location.pathname)
    }
    if (!code) {
      void navigate(`${RouterPage.LOGIN}?oauthError=invalid_state`, { replace: true })
      return
    }
    exchange
      .mutateAsync(code)
      .then(() => navigate(RouterPage.HOME, { replace: true }))
      .catch(() => navigate(`${RouterPage.LOGIN}?oauthError=invalid_state`, { replace: true }))
  }, [exchange, hash, navigate])

  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-4">
      <CoinLoader label={t('auth.oauth.callback.signing_in')} />
      <p className="text-sm text-muted-foreground">{t('auth.oauth.callback.signing_in')}</p>
    </div>
  )
}
```

(`CoinLoader` takes an optional `label`; the fragment is read from the router's location so the memory router in tests sees it, and `replaceState` clears it from the browser's history.)

`web/src/app/routes.tsx`: add `{ path: '/oauth/callback', element: <OAuthCallbackPage /> },` right after the `/logout` entry (outside `RequireAuth`).

`web/src/lib/deepLinks.ts`:

```ts
import { navigateTo } from '@/app/routerRef'
import { RouterPage } from '@/app/router-pages'
import type { BrowserPlugin } from './externalLinks'
import { nativePlugin } from './platform'

interface AppUrlPlugin {
  addListener(ev: 'appUrlOpen', cb: (data: { url: string }) => void): unknown
}

// The backend redirects app flows to econumo://oauth?… (spec §6.3). The
// browser sheet is closed first; the SPA route then does the same work as on
// the web. Anything else on the scheme is ignored.
export function handleAppUrl(raw: string): void {
  let url: URL
  try {
    url = new URL(raw)
  } catch {
    return
  }
  if (url.protocol !== 'econumo:' || url.host !== 'oauth') {
    return
  }
  void nativePlugin<BrowserPlugin>('Browser')?.close().catch(() => {})
  const q = url.searchParams
  const handoff = q.get('handoff')
  const linked = q.get('linked')
  const error = q.get('error')
  if (handoff) {
    navigateTo(`${RouterPage.OAUTH_CALLBACK}#handoff=${encodeURIComponent(handoff)}`)
  } else if (linked) {
    navigateTo(`${RouterPage.SETTINGS_LINKED_ACCOUNTS}?linked=${encodeURIComponent(linked)}`)
  } else if (error) {
    navigateTo(`${RouterPage.LOGIN}?oauthError=${encodeURIComponent(error)}`)
  }
}

export function installDeepLinkHandler(): void {
  nativePlugin<AppUrlPlugin>('App')?.addListener('appUrlOpen', ({ url }) => handleAppUrl(url))
}
```

`web/src/lib/appBoot.ts`: import `installDeepLinkHandler` and call it right after `installBackHandler()`. `navigateTo` falls back to `window.location.assign` if the router is not set yet, which is correct at boot.

- [ ] **Step 4: Run tests**

Run: `cd web && pnpm test -- OAuthCallbackPage deepLinks && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(oauth): callback route and app deep-link handler

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 16: Linked accounts page, profile rows, sessions badge

**Files:**
- Create: `web/src/features/settings/LinkedAccountsPage.tsx`
- Modify: `web/src/features/settings/security.ts` (hooks), `web/src/features/settings/ProfilePage.tsx` (Security group rows), `web/src/features/settings/SessionsPage.tsx` (provider badge), `web/src/app/routes.tsx`
- Test: `web/src/features/settings/LinkedAccountsPage.test.tsx`, `web/src/features/settings/ProfilePage.test.tsx` (append), `web/src/features/settings/SessionsPage.test.tsx` (append)

- [ ] **Step 1: Write the failing tests**

`web/src/features/settings/LinkedAccountsPage.test.tsx` (copy the `renderPage`/`mockViewport` scaffolding from `SessionsPage.test.tsx`, routing `/settings/profile/linked-accounts`):

```tsx
const providers = [{ id: 'google', name: 'Google' }, { id: 'apple', name: 'Apple' }]
const identities = [{ provider: 'google', email: 'me@gmail.test', createdAt: '2026-09-01 10:00:00' }]

function mockUser(hasPassword: boolean) {
  server.use(http.get('*/api/v1/user/get-user-data', () =>
    HttpResponse.json({ success: true, message: '', data: { user: { id: 'u1', name: 'Me', email: 'me@example.test', avatar: 'face:sky', options: [], accessLevel: 'full', accessUntil: '', createdAt: '2026-01-01 00:00:00', currency: 'USD', reportPeriod: 'month', hasPassword } } })))
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  server.use(
    http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: providers })),
    http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: identities })),
  )
})

it('lists linked identities and offers Link for the rest', async () => {
  mockUser(true)
  renderPage()
  expect(await screen.findByText('me@gmail.test')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Unlink' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Link' })).toBeInTheDocument() // apple
})

it('unlinks after confirmation and fires the metric', async () => {
  mockUser(true)
  let posted: unknown
  server.use(http.post('*/api/v1/oauth/unlink-identity', async ({ request }) => {
    posted = await request.json()
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Unlink' }))
  await user.click(await screen.findByRole('button', { name: 'Unlink', hidden: false }))
  await waitFor(() => expect(posted).toEqual({ provider: 'google' }))
})

it('disables Unlink with a hint for a passwordless user with one identity', async () => {
  mockUser(false)
  renderPage()
  const btn = await screen.findByRole('button', { name: 'Unlink' })
  expect(btn).toBeDisabled()
  expect(screen.getByText('Set a password before unlinking your only sign-in method.')).toBeInTheDocument()
})

it('shows the linked toast from ?linked=', async () => {
  mockUser(true)
  renderPage('/settings/profile/linked-accounts?linked=google')
  expect(await screen.findByText('Google account linked.')).toBeInTheDocument()
})
```

Append to `ProfilePage.test.tsx`: with `hasPassword: false` the Security group shows "Set a password" instead of "Change password" and a "Linked accounts" row exists. Append to `SessionsPage.test.tsx`: a session with `provider: 'google'` renders "via Google".

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm test -- src/features/settings`
Expected: FAIL.

- [ ] **Step 3: Implement**

`web/src/features/settings/security.ts` — add:

```ts
export function useIdentities() {
  return useQuery({ queryKey: ['oauth', 'identities'], queryFn: oauthApi.getIdentityList })
}

export function useUnlinkIdentity() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (provider: OAuthProviderId) => oauthApi.unlinkIdentity(provider),
    onSuccess: () => {
      trackEvent(METRICS.IDENTITY_UNLINKED)
      void queryClient.invalidateQueries({ queryKey: ['oauth', 'identities'] })
    },
  })
}
```

`LinkedAccountsPage.tsx` (structure mirrors `SessionsPage.tsx`: `SettingsShell` with `backTo={RouterPage.SETTINGS_PROFILE}`, an `InfoBox` description, a list, `ConfirmDialog` for unlink):

```tsx
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { RouterPage } from '@/app/router-pages'
import type { OAuthProviderId } from '@/api/dto/oauth'
import { useUserData } from '@/features/user/queries'
import { useProviders, useStartOAuth } from '@/features/auth/oauthQueries'
import { ProviderMark } from '@/features/auth/providerIcons'
import { METRICS, trackEvent } from '@/lib/metrics'
import { SettingsShell } from './SettingsShell'
import { useIdentities, useUnlinkIdentity } from './security'
import { parseUtcDateTime } from './securityFormat'

export function LinkedAccountsPage() {
  const { t, i18n } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const providers = useProviders()
  const identities = useIdentities()
  const user = useUserData()
  const start = useStartOAuth()
  const unlink = useUnlinkIdentity()
  const [confirm, setConfirm] = useState<OAuthProviderId | null>(null)
  const toasted = useRef(false)

  const linked = searchParams.get('linked')
  useEffect(() => {
    if (!linked || toasted.current) {
      return
    }
    toasted.current = true
    const name = providers.data?.find((p) => p.id === linked)?.name ?? t(`auth.oauth.provider_name.${linked}`)
    toast.success(t('user.page.settings.profile.linked_accounts.linked_toast', { provider: name }))
    trackEvent(METRICS.IDENTITY_LINKED, { provider: linked })
    setSearchParams({}, { replace: true })
  }, [linked, providers.data, setSearchParams, t])

  const hasPassword = user.data?.hasPassword ?? true
  const lastIdentityLocked = !hasPassword && (identities.data?.length ?? 0) <= 1
  const linkedIds = new Set(identities.data?.map((i) => i.provider))
  const unlinked = (providers.data ?? []).filter((p) => !linkedIds.has(p.id))

  return (
    <SettingsShell title={t('user.page.settings.profile.linked_accounts.header')} backTo={RouterPage.SETTINGS_PROFILE}>
      <InfoBox>{t('user.page.settings.profile.linked_accounts.description')}</InfoBox>
      {identities.data?.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('user.page.settings.profile.linked_accounts.empty')}</p>
      ) : null}
      <ul className="flex flex-col gap-2">
        {identities.data?.map((i) => (
          <li key={i.provider} className="flex items-center gap-3 rounded-md bg-econumo-card px-3 py-2.5">
            <ProviderMark id={i.provider} />
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm">{i.email}</div>
              <div className="text-xs text-muted-foreground">
                {t('user.page.settings.profile.linked_accounts.linked_on')} {parseUtcDateTime(i.createdAt).toLocaleDateString(i18n.language)}
              </div>
            </div>
            <Button type="button" variant="secondary" size="sm" disabled={lastIdentityLocked} onClick={() => setConfirm(i.provider)}>
              {t('user.page.settings.profile.linked_accounts.unlink')}
            </Button>
          </li>
        ))}
      </ul>
      {lastIdentityLocked && (identities.data?.length ?? 0) > 0 ? (
        <p className="mt-2 text-xs text-muted-foreground">{t('user.page.settings.profile.linked_accounts.last_identity_hint')}</p>
      ) : null}
      {unlinked.length > 0 ? (
        <ul className="mt-4 flex flex-col gap-2">
          {unlinked.map((p) => (
            <li key={p.id} className="flex items-center gap-3 rounded-md px-3 py-2.5">
              <ProviderMark id={p.id} />
              <div className="flex-1 text-sm">{p.name}</div>
              <Button type="button" size="sm" disabled={start.isPending} onClick={() => start.mutate({ provider: p.id, intent: 'link' })}>
                {t('user.page.settings.profile.linked_accounts.link')}
              </Button>
            </li>
          ))}
        </ul>
      ) : null}
      <ConfirmDialog
        open={confirm !== null}
        onClose={() => setConfirm(null)}
        onConfirm={() => {
          if (confirm) {
            unlink.mutate(confirm)
          }
          setConfirm(null)
        }}
        question={t('user.page.settings.profile.linked_accounts.confirm_unlink')}
        confirmLabel={t('user.page.settings.profile.linked_accounts.unlink')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </SettingsShell>
  )
}
```

`useUserData()` (`web/src/features/user/queries.ts:14`) returns the `CurrentUserDto` as `data`; `parseUtcDateTime` is in `securityFormat.ts:6`. `t(\`auth.oauth.provider_name.${linked}\`)` is a dynamic key; the three keys exist from Task 12. `security.ts` needs imports for `oauthApi` (`@/api/oauth`), `OAuthProviderId`, `useQueryClient`, and `METRICS`/`trackEvent`.

`ProfilePage.tsx` Security group (lines 173-196): add a row to `RouterPage.SETTINGS_LINKED_ACCOUNTS` labelled `user.page.settings.profile.linked_accounts.menu_item`; and when `user.hasPassword === false`, replace the change-password row's label with `user.page.settings.profile.linked_accounts.set_password.menu_item` and make it open the recovery dialog (`RecoveryDialog` from `features/auth`, rendered with the user's email prefilled: add an optional `email?: string` prop to `RecoveryDialog` that seeds and locks the email field).

`SessionsPage.tsx`: next to the device description render `session.provider ? <span className="text-xs text-muted-foreground">{t('user.page.settings.profile.sessions.via', { provider: t(\`auth.oauth.provider_name.${session.provider}\`) })}</span> : null`.

`routes.tsx`: add `{ path: '/settings/profile/linked-accounts', element: <LinkedAccountsPage /> },` after the tokens route.

- [ ] **Step 4: Run tests**

Run: `cd web && pnpm test && pnpm lint && pnpm build`
Expected: PASS, including `metrics-coverage.test.ts` (all four keys now referenced) and `tsc -b`.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(oauth): linked accounts settings, set-a-password row, session provider badge

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 17: Logout page: IdP redirect and local-logout notice

**Files:**
- Modify: `web/src/features/auth/LogoutPage.tsx`
- Test: `web/src/features/auth/LogoutPage.test.tsx` (append)

- [ ] **Step 1: Write the failing tests**

Append (reuse the file's existing render helper and `location.assign` stub):

```tsx
it('navigates to the IdP end-session url when logout returns one', async () => {
  localStorage.setItem('token', 'eco_ses_x')
  server.use(http.post('*/api/v1/user/logout-user', () =>
    HttpResponse.json({ success: true, message: '', data: { result: 'test', logoutUrl: 'https://idp/end?x=1', provider: 'oidc' } })))
  renderPage()
  await waitFor(() => expect(assign).toHaveBeenCalledWith('https://idp/end?x=1'))
  expect(localStorage.getItem('token')).toBeNull()
})

it('shows the local-logout notice for a provider session without an end-session url', async () => {
  localStorage.setItem('token', 'eco_ses_x')
  server.use(http.post('*/api/v1/user/logout-user', () =>
    HttpResponse.json({ success: true, message: '', data: { result: 'test', logoutUrl: '', provider: 'google' } })))
  renderPage()
  expect(await screen.findByText("You're signed out of Econumo. Your Google session may still be active; sign out there to end it.")).toBeInTheDocument()
  expect(assign).not.toHaveBeenCalledWith('/login')
  await userEvent.click(screen.getByRole('button'))
  expect(assign).toHaveBeenCalledWith('/login')
})

it('in the app ignores the end-session url and logs out locally', async () => {
  window.Capacitor = { isNativePlatform: () => true }
  localStorage.setItem('token', 'eco_ses_x')
  server.use(http.post('*/api/v1/user/logout-user', () =>
    HttpResponse.json({ success: true, message: '', data: { result: 'test', logoutUrl: 'https://idp/end', provider: 'oidc' } })))
  renderPage()
  expect(await screen.findByText(/Your SSO session may still be active/)).toBeInTheDocument()
  delete (window as { Capacitor?: unknown }).Capacitor
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm test -- LogoutPage`
Expected: FAIL.

- [ ] **Step 3: Implement**

Rewrite `LogoutPage.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { logout } from '@/api/user'
import { Button } from '@/components/ui/button'
import { resetAnalyticsIdentity } from '@/lib/analytics'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isNativeApp } from '@/lib/platform'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { hasToken, removeToken } from '@/lib/storage'
import { RouterPage } from '@/app/router-pages'

export function LogoutPage() {
  const { t } = useTranslation()
  const [notice, setNotice] = useState<string | null>(null)

  useEffect(() => {
    const run = async () => {
      let logoutUrl = ''
      let provider = ''
      if (hasToken()) {
        try {
          const res = await logout()
          logoutUrl = res.logoutUrl
          provider = res.provider
        } catch {
          // best effort; the token is purged regardless
        }
        trackEvent(METRICS.USER_LOGOUT)
        resetAnalyticsIdentity()
      }
      removeToken()
      clearPersistedQueryCache()
      // The IdP redirect is web-only: an IdP logout page in the app's browser
      // sheet leaves the user with no clean way back (spec §8).
      if (logoutUrl && !isNativeApp()) {
        window.location.assign(logoutUrl)
        return
      }
      if (provider) {
        setNotice(t('auth.oauth.logout_notice', { provider: t(`auth.oauth.provider_name.${provider}`) }))
        return
      }
      window.location.assign(RouterPage.LOGIN)
    }
    void run()
  }, [t])

  if (!notice) {
    return null
  }
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-4 px-6 text-center">
      <p className="max-w-md text-sm text-muted-foreground">{notice}</p>
      <Button type="button" className="h-11" onClick={() => window.location.assign(RouterPage.LOGIN)}>
        {t('common.button.ok.label')}
      </Button>
    </div>
  )
}
```

- [ ] **Step 4: Run tests**

Run: `cd web && pnpm test -- LogoutPage && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(oauth): RP-initiated logout redirect and local-logout notice

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 18: Mobile `econumo://` scheme

**Files:**
- Modify: `mobile/ios/App/App/Info.plist`, `mobile/android/app/src/main/AndroidManifest.xml`, `mobile/README.md`

- [ ] **Step 1: iOS URL type**

In `Info.plist`, inside the top-level `<dict>` (after `CFBundleVersion`):

```xml
	<key>CFBundleURLTypes</key>
	<array>
		<dict>
			<key>CFBundleURLName</key>
			<string>com.econumo.app.oauth</string>
			<key>CFBundleURLSchemes</key>
			<array>
				<string>econumo</string>
			</array>
		</dict>
	</array>
```

- [ ] **Step 2: Android intent filter**

In `AndroidManifest.xml`, inside `<activity … MainActivity>` after the LAUNCHER filter:

```xml
            <intent-filter>
                <action android:name="android.intent.action.VIEW" />
                <category android:name="android.intent.category.DEFAULT" />
                <category android:name="android.intent.category.BROWSABLE" />
                <data android:scheme="econumo" android:host="oauth" />
            </intent-filter>
```

- [ ] **Step 3: README**

Add a section `## Sign-in with Google / Apple / SSO` to `mobile/README.md` explaining: the app opens the backend's authorization URL in the system browser sheet (`@capacitor/browser`); the backend redirects to `econumo://oauth?…`; the `econumo` scheme is registered in `Info.plist` (`CFBundleURLTypes`) and the Android manifest (`VIEW`/`BROWSABLE` filter, host `oauth`); `web/src/lib/deepLinks.ts` dispatches it; nothing to configure per backend (the callback URL registered with the provider is the backend's own `/api/v1/oauth/callback-<provider>`); App Store rule 4.8 means the cloud backend enables Apple whenever Google is enabled.

- [ ] **Step 4: Verify**

Run: `cd mobile && pnpm install --frozen-lockfile && npx cap sync 2>&1 | tail -5` (only if the native toolchains are present; otherwise validate the XML with `xmllint --noout` on both files).

- [ ] **Step 5: Commit**

```bash
git add mobile
git commit -m "feat(oauth): register the econumo:// scheme in the app

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 19: Documentation

**Files:**
- Modify: `CLAUDE.md`, `.env.example`, `docs/regression-test-plan.md`
- Create: `docs/oidc-setup.md`

- [ ] **Step 1: CLAUDE.md**

- Feature list sentence (line 134): "twelve" → "thirteen", add `oauth` to the list.
- Configuration: add bullets after `ECONUMO_URL` for `ECONUMO_OAUTH_GOOGLE_*`, `ECONUMO_OAUTH_APPLE_*`, `ECONUMO_OIDC_*` (issuer, client id/secret, name, scopes, trust-email), stating the all-or-nothing slot rule, that `ECONUMO_URL` becomes required, the derived callback URL `<ECONUMO_URL>/api/v1/oauth/callback-<provider>`, lazy discovery with a boot WARN, and that `ECONUMO_OIDC_TRUST_EMAIL=false` rejects tokens without `email_verified=true`.
- API conventions: add the five public oauth routes to BOTH public-route lists (lines 670-672 and 746); add `start-link` and `unlink-identity` to the read-only allowlist sentence (line 675).
- Authentication section: a bullet "Provider sign-in (`internal/oauth`)…" summarising the flow, identities keyed by `(provider, subject)`, passwordless users (`algorithm = 'none'`), the handoff, and that sessions carry `provider`/`id_token`.
- Notable behaviours: bullets for the auto-link/unverified-email rule, the email-drift rule, and RP-initiated logout (web only).
- Testing: mention `internal/infra/oidc/oidctest` and that apiparity records `Location:` for 3xx responses.

- [ ] **Step 2: .env.example**

After the `ECONUMO_URL` block:

```
# Sign in with Google / Apple / an OpenID Connect provider (all optional; each
# slot is all-or-nothing and requires ECONUMO_URL — the callback URL you register
# with the provider is <ECONUMO_URL>/api/v1/oauth/callback-<google|apple|oidc>).
# See docs/oidc-setup.md.
# ECONUMO_OAUTH_GOOGLE_CLIENT_ID=
# ECONUMO_OAUTH_GOOGLE_CLIENT_SECRET=
# ECONUMO_OAUTH_APPLE_CLIENT_ID=        # the Services ID
# ECONUMO_OAUTH_APPLE_TEAM_ID=
# ECONUMO_OAUTH_APPLE_KEY_ID=
# ECONUMO_OAUTH_APPLE_PRIVATE_KEY=      # the .p8 contents on one line, newlines as \n
# ECONUMO_OIDC_ISSUER_URL=https://auth.example.com/application/o/econumo/
# ECONUMO_OIDC_CLIENT_ID=
# ECONUMO_OIDC_CLIENT_SECRET=
# ECONUMO_OIDC_NAME=SSO
# ECONUMO_OIDC_SCOPES=openid,profile,email
# ECONUMO_OIDC_TRUST_EMAIL=false        # true if your IdP never sends email_verified
```

- [ ] **Step 3: docs/oidc-setup.md**

Write the guide with four sections (Google Cloud console, Apple developer portal incl. the Services ID + key download + "\n" env encoding, Authentik as the generic example with the post-logout redirect URI `<ECONUMO_URL>/login`, Cloudflare Access for SaaS with the issuer URL shape `https://<team>.cloudflareaccess.com/cdn-cgi/access/sso/oidc/<client-id>` and the trust-email note), each ending with the exact callback URL, and a "How accounts are matched" section restating the spec's rules in user terms (verified email auto-links, unverified is rejected, first sign-in creates a passwordless account when registration is on, Set a password in Settings, email drift).

- [ ] **Step 4: Regression test plan**

Under `## 2. Authentication & registration` add items: 📱 sign in with Google, Apple, SSO (desktop + mobile, both `/login` and `/register`); first sign-in creates an account and lands on onboarding; verified-email auto-link to an existing password account; unverified email rejected with the message; registration-disabled message; cancelling at the provider shows "Sign-in was cancelled."; RP-initiated logout returns to `/login`; local-logout notice for Google/Apple; 📱 app: browser sheet opens, returns via `econumo://`, sheet closes. Under `## 12. Profile & security settings`: 📱 Linked accounts list/link/unlink/confirm; last-identity refusal for a passwordless user; "Set a password" row sends the code and the password then works; session list shows "via Google"; email drift on an SSO-only account.

- [ ] **Step 5: Verify and commit**

Run: `make go-test && cd web && pnpm test && pnpm lint`
Expected: PASS.

```bash
git add CLAUDE.md .env.example docs
git commit -m "docs(oauth): configuration reference, setup guide, regression items

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Final verification (before the PR)

- [ ] `make go-test` green (build, vet, gofmt, swagger-check, tests, coverage ≥ 80).
- [ ] `cd web && pnpm test && pnpm lint && pnpm build` green.
- [ ] `go test ./internal/test/archtest/` green (no feature-to-feature import).
- [ ] If PostgreSQL is reachable: `make test-repo-pgsql` and `make test` (enginecompare) green.
- [ ] `git diff main --stat` reviewed: no stray golden changes beyond the fields this feature adds.
