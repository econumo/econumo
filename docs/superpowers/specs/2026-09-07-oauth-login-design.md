# OAuth / OIDC login: Google, Apple, and one custom provider

Users sign in to Econumo with Google, Apple, or any OpenID Connect provider
(Authentik, Keycloak, Cloudflare Access, ...) in addition to the existing
email + password login, on the web and in the iOS/Android app. External
identities are linked to Econumo users in their own table, keyed by the
provider's stable subject, never by email. An account may be created from a
provider identity (passwordless) or linked to an existing password account,
and a user manages their linked identities in Settings.

Resolves the OIDC half of GitHub issue #217; the sections marked out of scope
list what the issue asks for that this design defers.

## 1. Why

Self-hosters centralise authentication behind an IdP and do not want a second
password per app. Cloud users expect "Continue with Google / Apple" and abandon
sign-up forms without it. Both cases are the same mechanism: Econumo acts as an
OAuth 2.0 / OIDC **relying party** (client) of an external issuer.

## 2. Design summary

- Three fixed provider slots configured by environment variables: `google`,
  `apple`, `oidc` (one custom issuer). A slot is enabled by the presence of all
  of its variables; a partial slot fails at boot.
- One backend-mediated authorization-code flow serves web and app. The client
  asks the backend for an authorization URL, the provider redirects to a
  callback on the backend, the backend resolves the identity and redirects the
  browser back to the SPA (web) or to the `econumo://` scheme (app) with a
  short-lived single-use **handoff code**. The client exchanges the handoff for
  the ordinary `{token, user}` login response, presenting the **flow secret**
  the `start-*` call handed it: the flow is bound to the client that began it,
  so a forged callback cannot log a victim into an attacker's account. The
  session token never appears in a URL.
- OAuth state is stored server-side (a table), not in cookies: Apple's
  cross-site `form_post` carries no SameSite cookie and the app's browser sheet
  shares no storage with the SPA.
- Identities live in `users_identities (provider, subject)`. Email claims are
  used only when verified (or trusted by the operator); an unverified email
  rejects the sign-in outright, so a misconfigured IdP can neither take over
  nor create an account.
- Provisioning through a provider follows `ECONUMO_ALLOW_REGISTRATION`; such a
  user has no password (`users.algorithm = 'none'`) until they set one via the
  password-reset flow.
- New feature package `internal/oauth` (use cases, repo, api) over a new
  feature-free protocol package `internal/infra/oidc` (discovery, PKCE, ID-token
  verification over JWKS, Apple client-secret signing). Stdlib only, no new
  direct dependency.
- Logout performs RP-initiated logout on the web for sessions minted from the
  custom slot when the issuer publishes an end-session endpoint.

## 3. Configuration

```
ECONUMO_OAUTH_GOOGLE_CLIENT_ID
ECONUMO_OAUTH_GOOGLE_CLIENT_SECRET

ECONUMO_OAUTH_APPLE_CLIENT_ID        # the Services ID (the web client id)
ECONUMO_OAUTH_APPLE_TEAM_ID
ECONUMO_OAUTH_APPLE_KEY_ID
ECONUMO_OAUTH_APPLE_PRIVATE_KEY      # the .p8 PEM; literal "\n" escapes are unescaped

ECONUMO_OIDC_ISSUER_URL              # discovery at <issuer>/.well-known/openid-configuration
ECONUMO_OIDC_CLIENT_ID
ECONUMO_OIDC_CLIENT_SECRET
ECONUMO_OIDC_NAME                    # button label; default "SSO"
ECONUMO_OIDC_SCOPES                  # default "openid profile email"; must contain "openid"
ECONUMO_OIDC_TRUST_EMAIL             # strict bool, default false; treat the issuer's email claim as verified.
                                     # When false, a token without email_verified=true is rejected
```

Rules, all checked in `config.Load` so a mistake fails at boot:

- A slot is enabled when **all** of its required variables are non-empty
  (`OIDC_NAME`, `OIDC_SCOPES`, `OIDC_TRUST_EMAIL` are optional). Any subset
  set without the rest is an error naming the missing variable.
- Any enabled slot requires `ECONUMO_URL` (absolute http(s) URL, already
  validated). Redirect URIs derive from it and are never configured:
  `<ECONUMO_URL>/api/v1/oauth/callback-<provider>` (`callback-google`,
  `callback-apple`, `callback-oidc`). This is the one URL an operator
  registers per provider.
- `ECONUMO_OIDC_ISSUER_URL` must be an absolute `https://` URL (plain `http`
  allowed only for loopback hosts, like `ECONUMO_BILLING_URL`).
- The Apple private key must parse as a PKCS#8 ECDSA P-256 key.
- Google and Apple have fixed issuers (`https://accounts.google.com`,
  `https://appleid.apple.com`) and fixed scopes (`openid email profile` and
  `name email`). They are internally two more issuer configurations plus
  Apple's quirks (§5.4), not separate code paths.
- No new sign-up flag: a provider identity with no matching account creates
  one only when `ECONUMO_ALLOW_REGISTRATION` is true.

Discovery documents and JWKS are fetched lazily on first use and cached in
memory for the process lifetime; an unknown `kid` triggers one JWKS re-fetch,
rate-limited to once per minute per issuer. `serve` logs a WARN at boot when an
enabled issuer is unreachable but does not fail, so an IdP in the same compose
stack may start after Econumo. Outbound calls use a dedicated `http.Client`
with a 10 s timeout, the pattern of `internal/system/updates.go`.

The SPA discovers enabled providers through the API (§6.1), not through the
served `econumo-config.js`; that document is unchanged.

## 4. Data model

Migrations for both engines, same version stamp.

### 4.1 `users_identities`

| column | type | notes |
|---|---|---|
| `id` | TEXT PK | UUIDv7 |
| `user_id` | TEXT FK → users(id) ON DELETE CASCADE | |
| `provider` | TEXT | `google` / `apple` / `oidc` |
| `subject` | TEXT | the ID token `sub` |
| `email` | TEXT | last email claim seen; display only, never a lookup key |
| `created_at`, `updated_at` | DATETIME | frozen layout |

Unique `(provider, subject)`; index on `user_id`. The custom slot's identities
are keyed by provider id `oidc`, not by issuer URL: changing the issuer of the
slot orphans nothing but also does not migrate subjects (documented).

### 4.2 `oauth_states`

| column | notes |
|---|---|
| `state_hash` TEXT PK | `hex(sha256(state))`; `state` is 32 random bytes base64url |
| `provider` TEXT | |
| `nonce` TEXT | 32 random bytes base64url; compared to the ID token `nonce` |
| `code_verifier` TEXT | PKCE verifier (empty for Apple, §5.4) |
| `flow_hash` TEXT | `hex(sha256(flow))`; the flow secret returned to the initiating client |
| `client` TEXT | `web` / `app`; selects the redirect target |
| `intent` TEXT | `login` / `link` |
| `link_user_id` TEXT NULL | the caller for `intent = link` |
| `created_at`, `expires_at` DATETIME | TTL 10 minutes |

A row is deleted when consumed (success or failure). Expired rows are purged
opportunistically whenever a new state is created, the pattern of dead access
tokens at login.

### 4.3 `oauth_handoffs`

| column | notes |
|---|---|
| `code_hash` TEXT PK | `hex(sha256(code))`; `code` is 32 random bytes base64url |
| `user_id` TEXT FK → users(id) ON DELETE CASCADE | resolved user |
| `provider` TEXT | recorded on the session for analytics/logout |
| `flow_hash` TEXT | copied from the state row; the exchange must present the matching secret |
| `id_token` TEXT NULL | the raw ID token, custom slot only (§8) |
| `created_at`, `expires_at` DATETIME | TTL 60 seconds |

Single use: deleted on exchange. Expired rows are purged with the states.

### 4.4 Existing tables

- `access_tokens` gains `id_token TEXT NULL` and `provider TEXT NULL`. Set on
  sessions minted from a handoff; `id_token` only for the custom slot. Both die
  with the row through the existing purge. The sessions list exposes
  `provider` (`""` for password logins) so a user can tell how a session was
  opened.
- `users` is unchanged. A passwordless user has `algorithm = 'none'`,
  `password = ''`, `salt = ''`. Password verification already dispatches on
  `algorithm` and fails closed on an unknown value; `none` becomes an explicit
  constant with the same effect, so password login for such a user yields the
  normal "Invalid credentials." and leaks nothing.

## 5. Protocol package: `internal/infra/oidc`

Feature-free, imports only the shared kernel and stdlib.

### 5.1 Issuer

```go
type Issuer struct {
    ID           string   // "google" | "apple" | "oidc"
    IssuerURL    string
    ClientID     string
    ClientSecret func(now time.Time) (string, error) // static, or Apple's signed JWT
    Scopes       []string
    UsePKCE      bool
    ResponseMode string   // "" | "form_post"
    TrustEmail   bool
}
```

`Discover(ctx)` loads and caches the OpenID configuration
(`authorization_endpoint`, `token_endpoint`, `jwks_uri`, optional
`userinfo_endpoint` and `end_session_endpoint`). Google and Apple use the same discovery code against
their well-known documents.

### 5.2 Authorization request

`AuthURL(disc, state, nonce, codeChallenge, redirectURI)` builds the request:
`response_type=code`, `client_id`, `redirect_uri`, `scope`, `state`, `nonce`,
`code_challenge` + `code_challenge_method=S256` when `UsePKCE`, and
`response_mode=form_post` when set. Google additionally gets `prompt=select_account`.

### 5.3 Code exchange and ID-token verification

`Exchange(ctx, disc, code, codeVerifier, redirectURI)` posts to the token
endpoint with `client_secret_post` authentication and returns the raw ID token
and the access token. The access token is used at most once, for the
userinfo fallback below, and is never stored.

`VerifyIDToken(ctx, disc, raw, nonce, now) (Claims, error)` checks, in order:
compact JWS structure; `alg` ∈ {RS256, ES256}; signature against the cached
JWKS key with the matching `kid` (one refresh on miss); `iss` equals the
issuer; `aud` contains the client id; `exp` and `iat` within a 60 s skew;
`nonce` equals the expected value. Returns `Subject`, `Email`,
`EmailVerified`, `Name`. `email_verified` is accepted as a JSON bool or the
strings `"true"/"false"` (Apple sends a string).

**Userinfo fallback.** When the verified ID token carries no `email` claim and
the discovery document publishes `userinfo_endpoint`, `UserInfo(ctx, disc,
accessToken)` GETs it with the bearer access token and returns the same
`Claims` shape. The caller accepts the response only when its `sub` equals
the ID token's `sub` (an OIDC Core requirement) and fills in `Email`,
`EmailVerified`, and `Name` **only where the ID token left them empty**; the
ID token's values always win. A userinfo failure is not fatal: resolution
continues with the ID token's claims alone and lands on `email_required`.
This covers issuers such as Microsoft Entra that omit `email` from the ID
token by default. Apple publishes no userinfo endpoint; Google's ID token
always carries the email, so neither reaches the fallback.

### 5.4 Apple

- The client secret is an ES256 JWT (`iss` = team id, `sub` = services id,
  `aud` = `https://appleid.apple.com`, `kid` header = key id, 5-minute `exp`),
  signed with the .p8 key at each exchange.
- Scopes `name email` require `response_mode=form_post`; the callback is a
  POST with `code`, `state`, `id_token`, and, **on the first authorization
  only**, a JSON `user` field carrying `name.firstName/lastName`. The backend
  reads the name from that field when present; later sign-ins carry none, so
  the name is only used at provisioning time.
- PKCE is off for Apple (`UsePKCE = false`): Apple does not document
  `code_challenge`, and the confidential-client secret plus `nonce` already
  bind the exchange. Google and the custom slot always use PKCE.
- Private-relay addresses (`@privaterelay.appleid.com`) are ordinary verified
  emails.

### 5.5 Randomness

`state`, `nonce`, `code_verifier`, and handoff codes are 32 bytes from
`crypto/rand`, base64url without padding (43 chars), the access-token alphabet.

## 6. Feature package: `internal/oauth`

Routes under `/api/v1/oauth/`, registered in `internal/oauth/api/routes.go`
(discovered by the apiparity and archtest guards automatically).

### 6.1 Endpoints

| Route | Auth | Purpose |
|---|---|---|
| `GET get-provider-list` | public | `[{id, name}]`, fixed order google, apple, oidc; only enabled slots; `name` is `Google`, `Apple`, or `ECONUMO_OIDC_NAME`. |
| `POST start-login` | public | Body `{provider, client}`. Creates a state row, returns `{url, flow}` — `flow` is the per-flow secret the client stores and presents at `exchange-handoff`. |
| `POST start-link` | authed, 402 allowlist | Same with `intent = link` and the caller as `link_user_id`. A flow secret is minted too (uniform row shape); the link flow ends on a redirect and never presents it. |
| `GET callback-google`, `GET callback-oidc` | public | `code` + `state` (or `error`) in the query. Respond 302. |
| `POST callback-apple` | public | Apple's `form_post`: `code`, `state`, `id_token`, `user` as form fields. Responds 302. |
| `POST exchange-handoff` | public | Body `{code, flow}`, both required. Returns the raw `{token, user}` of login (no envelope; the second such exception after login). |
| `GET get-identity-list` | authed | `[{provider, email, createdAt}]`. |
| `POST unlink-identity` | authed, 402 allowlist | Body `{provider}`. |

`start-*` return a URL instead of redirecting because `start-link` needs the
bearer header, which a browser navigation cannot carry; the client navigates.
The callbacks are one route per provider (not a `{provider}` path parameter)
so the apiparity route scanner, which only recognises two-segment literal
`/api/v1/<module>/<action>` strings, keeps them under guard.

The public routes join the public list in CLAUDE.md. The two allowlisted
routes join `middleware.ReadonlyAllowedPaths` (linking and unlinking are
account-security operations, like password and email changes).

Validation: `provider` must be an enabled slot (else a coded 400
`oauth.provider_not_configured`); `client` ∈ {`web`, `app`}.

### 6.2 Callback resolution

Common prefix:

1. Provider `error` parameter present (user cancelled, consent denied) →
   redirect with `denied`. The state row is consumed.
2. Load the state row by hash and **delete it**. The delete's affected-row
   count decides the race: a presenter whose delete removed no row is
   rejected, so a concurrent replay cannot both pass on either engine.
   Missing, already consumed, expired, or a provider mismatch → `invalid_state`.
3. Exchange the code with the stored verifier; verify the ID token against
   the stored nonce. Any failure → `provider_error` (details in the operation
   log only, never in the redirect). If the ID token lacks `email`, apply the
   userinfo fallback of §5.3.
4. No email claim → `email_required` (issue #217 Case C is out of scope,
   §14). Then `verified := claims.EmailVerified || issuer.TrustEmail`; not
   verified → `email_unverified`. Google and Apple are configured with
   `TrustEmail = true`; the custom slot follows `ECONUMO_OIDC_TRUST_EMAIL`.
   This check runs for every intent, including an already-linked identity and
   a link from Settings: an issuer whose tokens carry no `email_verified`
   claim cannot be used at all until the operator sets the trust flag, which
   is the explicit decision the flag exists for.

`intent = login`:

5. Identity `(provider, subject)` exists → its user. Inactive user →
   `account_inactive`. Otherwise apply the email-drift rule below and mint a
   handoff.
6. No identity, a user with that email exists (`lower(email)`): inactive →
   `account_inactive`; else insert the identity and mint a handoff. This is the
   auto-link. Step 4 proves that whoever is signing in owns the address — but
   it says nothing about whoever set that account's password, who may never
   have proved it (registration does not always verify). So when the account
   **has** a password the auto-link also revokes every session of that account
   and marks its email verified, exactly as `reset-password` does: the mailbox
   owner is the account owner. A passwordless account was created through a
   provider, so its owner already proved the address and keeps their sessions.
   The insert and the eviction share one transaction — a half-applied link
   would leave the eviction undone while step 5 signs the attacker straight in.
7. No user: registration disabled → `registration_disabled`. Else provision
   (§7) with the email marked verified, insert the identity, mint a handoff.

`intent = link`:

5. Identity exists for another user → `identity_taken`. Exists for
   `link_user_id` → idempotent success. Else insert. No handoff: the user is
   already signed in. Redirect to the Settings page (web) or the scheme with
   `linked=<provider>` (app).

On every successful identity load or insert, `users_identities.email` is
refreshed from the claim.

**Email drift** (step 5, the claim's email differs from the user's primary
email): the primary email is left alone, except when the user is
passwordless **and** has exactly one identity **and** no other user holds the
new address. In that case the IdP is the sole authority over the account, so
the primary email is updated in the same transaction (marked verified; step 4
already guaranteed that), keeping the reset channel reachable for the day the
user sets a password. The rule stops applying the moment the user sets a
password or links a second identity; from then on the primary email moves
only through the change-email flow in Settings. When the new address already
belongs to another Econumo user, the sign-in proceeds unchanged for the
identity's owner (the subject is the key, so an email collision cannot alter
who gets in) and the operation line carries a WARN with both user ids and the
provider, never the address, so an operator can spot an IdP-side subject
reassignment.

### 6.3 Redirect targets

| outcome | `client = web` | `client = app` |
|---|---|---|
| login success | `<ECONUMO_URL>/oauth/callback#handoff=<code>` | `econumo://oauth?handoff=<code>` |
| link success | `<ECONUMO_URL>/settings/profile/linked-accounts?linked=<provider>` | `econumo://oauth?linked=<provider>` |
| link error | `<ECONUMO_URL>/settings/profile/linked-accounts?oauthError=<code>` | `econumo://oauth?linkError=<code>` |
| error | `<ECONUMO_URL>/login?oauthError=<code>` | `econumo://oauth?error=<code>` |

The web handoff travels in the fragment so it never reaches server logs or
`Referer` headers. Error codes are catalogue keys under `auth.oauth.errors.*`
rendered by the SPA in the user's language: `denied`, `invalid_state`,
`provider_error`, `email_required`, `email_unverified`,
`registration_disabled`, `identity_taken`, `account_inactive`. A failure whose
state row named `intent = link` takes the link-error row: a signed-in user
would never see a message rendered on the login page. The intent lives in the
state row, so a failure BEFORE that row loads (unknown or expired state) has no
choice but the web login page — see §15.

### 6.4 Handoff exchange

Hash the code, load the row, then delete it. The DELETE's affected-row count
decides the race: a presenter whose delete removed no row is rejected, so a
concurrent replay cannot both pass on either engine — no explicit transaction
is needed, since a `:execrows` delete is already atomic on both SQLite and
PostgreSQL. Reject missing, raced (zero rows deleted), or expired with a
coded 401 `oauth.handoff_invalid`. Then compare
`sha256(request.flow)` against the row's `flow_hash` in constant time and
reject a mismatch with the same 401 — the row is already gone, so a wrong
secret costs the caller the code too. Then, through the user-feature port: purge dead
tokens, mint a session with the **exchanging request's** user agent (the real
client, not the provider's browser sheet), stamp `provider` and `id_token` on
the session row, best-effort persist the request language exactly as login
does, and return `{token, user}`.

### 6.5 Unlink

Refuse with a coded 400 `oauth.last_identity` when the identity is the user's
only one **and** the user has no password: removing it would lock them out.
Otherwise delete the row. Sessions opened through that identity stay valid
(they are Econumo sessions; the user revokes them from the sessions page).

### 6.6 Rate limiting

No new per-key scope: nothing identifies the caller before the provider
answers, and the handoff is 256 random bits with a 60 s life. `start-login` and
`start-link` do consult the limiter under a `oauth-start` scope registered with
a per-key limit of `0` and called with an empty key, so only the global
per-endpoint per-minute cap (`ECONUMO_RATE_LIMIT_GLOBAL`) applies — that call
is what puts a bound on state-row creation. The same global cap covers every
other new public route. A nil limiter (tests, CLI) disables the check.

### 6.7 Ports and wiring

Features never import features. `internal/oauth/ports.go` declares:

```go
type UserGateway interface {
    FindByEmail(ctx, email) (*model.User, error)             // NotFound when absent; caller checks IsActive
    ProvisionExternal(ctx, name, email string) (*model.User, error)  // email is always verified (§6.2 step 4)
    MintSession(ctx, userID vo.Id, userAgent, provider, idToken string) (*model.LoginResult, error)
    HasPassword(ctx, userID vo.Id) (bool, error)
    ReplaceVerifiedEmail(ctx, userID vo.Id, email string) error   // email-drift mirror (§6.2)
}
```

implemented in `internal/server/glue_oauth_user.go` over two new exported
methods on the user service (`ProvisionExternalUser`, `CreateExternalSession`)
that wrap the existing `createUser` core and `createSession`. In the other
direction, `internal/user/ports.go` gains:

```go
type LogoutURLBuilder interface {
    EndSessionURL(ctx, provider, idToken string) (string, error) // "" when unsupported
}
```

wired in `internal/server/glue_user_logout.go` over the oauth feature; nil
when no slot is configured. `internal/infra/oidc` sits underneath both.

## 7. Passwordless accounts

`ProvisionExternalUser` reuses `createUser` with `selfService = true`, so the
new user gets the same trial, default options, analytics default, currency,
and avatar as a registered one, plus:

- `algorithm = 'none'`, empty hash and salt.
- `name` from the `name` claim (Apple: `firstName` + `lastName` from the
  one-time `user` field), falling back to the email local part; clamped to
  the existing name length rule.
- `email_verified = true` always: step 4 of §6.2 rejected any unverified
  claim before provisioning, so the email-verification gate never applies to
  a provider-provisioned user.

The current-user DTO gains `hasPassword` (bool). `update-password` is
**unchanged** and still requires the current password; a passwordless user
sets one through the existing remind-password → reset-password flow, which
already writes an argon2id hash and revokes all sessions. Once set, the user
is an ordinary password user with linked identities.

CLI `user:show` prints `password: none` for such rows; `user:change-password`
works unchanged (it overwrites the hash and algorithm).

## 8. Logout

`LogoutResult` gains `logoutUrl` and `provider` (both strings, `""` by
default; additive to the frozen `{result}`). `provider` echoes the session
row's provider so the client can name the IdP in the local-logout notice
below. After the local revocation, if the session row carries an
`id_token` and the issuer's discovery publishes `end_session_endpoint`, the
URL is that endpoint with `id_token_hint`, `client_id`, and
`post_logout_redirect_uri=<ECONUMO_URL>/login`. Operators register that
redirect URI at the IdP (documented). Google and Apple publish no end-session
endpoint, so their sessions log out locally, as does every password session.

The web client navigates to `logoutUrl` when present; the app ignores it and
logs out locally (opening an IdP logout page in a browser sheet leaves the
user with no clean way back). Both behaviours are the local-only fallback the
issue allows.

**Local-logout notice.** Whenever a session with a `provider` ends without an
IdP redirect (Google, Apple, a custom issuer without an end-session endpoint,
or any provider in the app), the logout page shows a note: "You're signed
out of Econumo. Your {provider} session may still be active; sign out there
to end it." Password sessions show nothing new.

## 9. Web client (`web/src`)

- **Provider list**: `useProviders()` (TanStack Query, `GET get-provider-list`,
  cached for the session). Used by the login page, the registration page, and
  the linked-accounts page.
- **Login page**: below the password form, an "or continue with" divider and a
  button row; Google and Apple use their brand buttons per the vendors'
  branding rules (Apple checks these at App Store review), the custom slot a
  neutral key icon with its name. No providers → no divider, no row. Buttons
  show even when registration is disabled (existing linked users can sign in).
  `?oauthError=<code>` renders next to the session-expired notice.
- **Registration page**: same row when registration is allowed.
- **Starting a flow**: `POST start-login {provider, client: isNativeApp() ? 'app' : 'web'}`,
  store the returned `flow` under `oauthFlow` — `sessionStorage` on the web
  (scoped to the tab that started the flow), `localStorage` in the app (whose
  browser sheet ends the WebView session) — then `location.assign(url)` on the
  web or the Capacitor Browser plugin (already typed in
  `web/src/lib/externalLinks.ts`) in the app, which shows the system browser
  sheet (embedded web views are blocked by Google).
- **Return route** `/oauth/callback`: reads `handoff` from `location.hash` and
  takes (single use) the stored `oauthFlow`, posts `exchange-handoff {code,
  flow}`, stores the token through the same path as `useLogin` (clears the
  persisted query cache, `setToken`), fires the analytics event, navigates to
  `/`. Spinner while pending. A missing handoff OR a missing flow secret →
  `/login?oauthError=invalid_state` (this browser did not start the sign-in);
  a failed exchange → `invalid_state` on 401 (the handoff itself), else
  `provider_error`. The fragment is cleared from history on arrival.
- **Settings → Linked accounts** (`/settings/profile/linked-accounts`, a row
  beside Sessions and API tokens in the profile page's Security group): list of identities
  (provider, email, linked date); "Link" for every enabled provider not yet
  linked (posts `start-link`, navigates); "Unlink" with confirmation, disabled
  with an explanation when the account has no password and one identity.
  `?linked=<provider>` shows a success toast and invalidates the identity list
  (the link happened on the backend while the browser was away);
  `?oauthError=<code>` renders the `auth.oauth.errors.*` message (falling back
  to `provider_error`) and clears the parameter. The disabled Unlink button is
  `aria-describedby` the hint that explains it.
- **Profile**: the "Change password" row reads "Set a password" when
  `hasPassword` is false and opens the existing recovery dialog (sends the
  reset code) instead of the change-password form.
- **Sessions page**: shows the provider badge on sessions with a provider.
- **Logout page**: after clearing local state, `location.assign(logoutUrl)`
  when present; otherwise, when `provider` is set, the local-logout notice of
  §8 with the provider's display name.
- **Route guard**: unchanged (token presence).

## 10. Mobile app (`mobile/`)

- Register the `econumo` URL scheme: iOS `CFBundleURLTypes` in `Info.plist`;
  Android an intent filter (`VIEW`, `BROWSABLE`, `DEFAULT`, `data
  android:scheme="econumo" android:host="oauth"`) on the existing single-task
  `MainActivity`, so the redirect resumes the running instance.
- At boot (the existing app bootstrap), install one listener on the App
  plugin's `appUrlOpen` through `nativePlugin('App')`, keeping `web/` free of
  the Capacitor dependency. On `econumo://oauth`: close the Browser sheet, then
  dispatch: `handoff` → the same exchange as the web route; `linked` →
  navigate to the linked-accounts page with the marker; `error` → `/login`
  with the code.
- The app's `client = app` state means every redirect lands on the scheme;
  the backend never redirects an app flow to an https SPA route.
- App Store rule 4.8: Sign in with Apple appears wherever Google does. The
  cloud backend configures both slots; the app renders whatever the selected
  backend advertises.
- The app performs local-only logout (§8).

## 11. Analytics and i18n

- `METRICS` keys: `appOauthLoginCompleted`, `appOauthAccountCreated`,
  `appIdentityLinked`, `appIdentityUnlinked`. Fired at the exchange success,
  the link-success marker, and the unlink mutation's `onSuccess`; the provider
  id is an event property. `metrics-coverage.test.ts` enforces each is wired.
- Catalogue keys: `auth.oauth.*` (buttons, divider, callback spinner, the
  local-logout notice, the error codes of §6.3),
  `user.page.settings.profile.linked_accounts.*` (beside `sessions`/`tokens`),
  and `errors.*` entries
  for every new server code (`oauth.provider_not_configured`,
  `oauth.handoff_invalid`, `oauth.last_identity`, ...) registered in
  `errs.AllCodes`. All eleven catalogues carry every key in the same PR.

## 12. Testing

- **`internal/infra/oidc`**: an `httptest` fake issuer serving discovery,
  token, JWKS, userinfo, and end-session endpoints, signing with RSA and ECDSA keys
  generated in the test. Verifier tests for every rejection (issuer,
  audience, expiry, nonce, unknown kid with successful refresh, unsupported
  alg, bad signature); Apple client-secret JWT verified with the test key;
  PKCE challenge/verifier round trip; randomness length and alphabet;
  userinfo fallback (email only in userinfo, `sub` mismatch rejected, ID
  token values win, userinfo failure non-fatal).
- **`internal/oauth`**: table tests over §6.2 against the sqlite test DB
  driving the real callback through the fake issuer: existing identity,
  email drift (mirrored for a passwordless single-identity user; untouched
  when a password is set, a second identity exists, or the address belongs to
  another user, the last with the WARN asserted),
  verified auto-link, trusted auto-link, unverified email rejected (login,
  existing identity, and link intents), provisioning on/off, missing email, inactive user, link intent taken /
  idempotent, unlink last identity, expired and consumed state, expired
  handoff, handoff single use. Repo tests run under the PostgreSQL rerun.
- **User feature**: passwordless login yields "Invalid credentials.";
  reset-password on a passwordless user sets argon2id; `hasPassword` in the
  DTO; `logoutUrl` and `provider` present/absent.
- **apiparity**: scenarios and goldens for every new route; callback goldens
  cover the deterministic error redirects (Location normalised). The success
  callback needs a live issuer and is covered by the `internal/oauth` suite;
  the guard's route count grows by nine. `enginecompare` picks them up.
- **Middleware**: the two allowlisted routes pass the 402 rule; the six new
  public routes need no header.
- **Config**: partial slots, missing `ECONUMO_URL`, bad issuer scheme, bad
  Apple key each fail at boot with the variable named.
- **Frontend (vitest)**: provider row rendering (none / some), start-flow
  navigation on web vs app, callback route exchange and error paths, the
  linked-accounts page states, the app URL-open dispatcher.

## 13. Docs

- CLAUDE.md: the new variables in the configuration list, `internal/oauth` in
  the feature list (thirteen features), the new public routes and allowlist
  entries in API conventions, identities/passwordless/logout under notable
  behaviours.
- `.env.example`: the three slots, commented out.
- `docs/oidc-setup.md`: Google, Apple, a generic IdP with Authentik as the
  worked example, and Cloudflare Access for SaaS; each ends with the exact
  callback URL and, for RP-initiated logout, the post-logout redirect URI.
  The guide states plainly that an issuer whose ID tokens carry no
  `email_verified` claim needs `ECONUMO_OIDC_TRUST_EMAIL=true` or every
  sign-in is rejected with `email_unverified`.
- `docs/regression-test-plan.md`: sign-in per provider (📱 and desktop),
  auto-link, unverified-email rejection, email drift on an SSO-only account,
  link/unlink, last-identity refusal,
  set a password, RP-initiated logout, the local-logout notice, app return
  via the scheme.

## 14. Out of scope (recorded follow-ups)

1. **MCP OAuth authorization server**: Econumo as an OAuth **server** for MCP
   clients (protected-resource metadata, dynamic client registration, consent
   page, authorization + token endpoints, refresh tokens, a third
   `access_tokens` kind). Reuses this design's PKCE helpers, code-table
   pattern, and login page. The `internal/oauth` package name is chosen to
   house both roles.
2. **SSO-only mode** (disable password login instance-wide) with a CLI
   recovery path.
3. **Trusted-proxy auth** from Cloudflare Access's `Cf-Access-Jwt-Assertion`
   header (and the app's service-token problem behind Access).
4. **Email-less identities** (issue #217 Case C): email is the lookup key,
   the reset channel, and NOT NULL.
5. **Native Google / Apple SDKs** in the cloud app.
6. **Back-channel and front-channel logout**; **RP-initiated logout in the
   app**; storing provider refresh tokens.
7. **Several custom OIDC slots**; one is enough until asked.

## 15. Decisions relative to issue #217

Points the issue raises that this design answers differently, on purpose:

- **IdP session expiry is not mirrored.** An Econumo session slides for
  30 days regardless of the ID token's `exp`; without stored refresh tokens
  there is nothing to re-check against. Central revocation therefore reaches
  Econumo only through the user revoking sessions here (or, later, back-channel
  logout, §14).
- **No explicit "link this account?" confirmation.** A verified or trusted
  email auto-links; an unverified one is rejected. The confirmation step the
  issue offers as an alternative adds a screen without adding safety once the
  email is verified, and the trust flag is where the operator makes the call
  for an issuer that does not verify.
- **Env-only configuration.** The product has no admin settings UI, so
  "or UI settings" is not applicable.
- **One setup page**, `docs/oidc-setup.md`, rather than user-guide and
  admin-guide pages; the repo has no such guide structure.
- **Redirect URI is derived**, not configured: one `ECONUMO_URL` drives every
  callback and the post-logout redirect.
- **Login CSRF: every flow is bound to the initiating client** by a
  server-issued flow secret (§4.2, §6.4). The callback carries no client
  credential, so without it an attacker could complete a sign-in with their own
  provider account and feed the resulting handoff to a victim's browser,
  silently landing the victim in the attacker's account. A forged callback now
  yields a handoff the victim's browser cannot redeem.
- **An unknown or expired state always redirects to the web login page.** The
  client (`web`/`app`) and the intent both live in the state row, so before it
  loads there is nothing to route on — a link attempt whose state expired
  reports on the login page rather than in Settings.
