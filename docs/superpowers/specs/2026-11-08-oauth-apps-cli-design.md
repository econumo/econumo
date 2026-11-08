# OAuth apps: `app:*` CLI, official apps and single-user user apps — design

Status: agreed in conversation 2026-11-07/08; written 2026-11-08. Branch `feature/oauth-app-cli`,
its own PR from `main`, independent of #417 (share reports).

## 1. Intent

Two kinds of OAuth apps (clients of Econumo's MCP OAuth server, `internal/authserver`):

- **User apps** (self-registered). What Claude, Codex, Cursor and any MCP client create through
  `POST /oauth/register` when a person adds Econumo. Each belongs to **one Econumo user**: the
  journey is "I open Claude → it registers → I approve → it's mine"; every other user's client
  registers its own.
- **Official apps.** Registered by the operator from the command line, usable by **every user** of the
  instance. Needed on the cloud only: Econumo Share (`share.econumo.com`), a Claude connector-directory
  listing with Anthropic-held client credentials, a client an organization admin enters in Claude
  ("Use your own OAuth client").

Plus operator tooling to register and manage apps from the CLI, so nobody runs `curl` against
`/oauth/register` again.

**Terms.** "Official" and "user" are the user-facing names (CLI output, UI, docs). In code and the
database an official app is `is_global = true`; the column keeps that name. "Self-registered" is used
below only where the registration path matters.

## 2. Today on `main` (what changes)

`oauth_clients` has `id, name, redirect_uris, secret_hash, created_at, last_used_at`, no owner. Any
signed-in user can approve any client id. Registration is anonymous (it happens before anyone signs
in). Never-approved clients (`last_used_at IS NULL`) are purged after 30 days, and at most 1,000
unapproved ones are kept (`MaxUnusedClients`), both on each registration. Grants are per user
(`oauth_grants.user_id`); a successful code exchange revokes the user's other grants for the same
client. In practice each person's MCP client registers its own app (Claude: "registers a new client on
every fresh connection" when the server has no CIMD), so apps are already de facto single-user; nothing
enforces it.

## 3. Goals, non-goals, anti-goals

Goals: the owner rule for user apps; official apps created only by the CLI; the `app:*`
commands; a migration that changes nothing for people already connected.

Non-goals: a web UI for operators; CIMD (Client ID Metadata Documents) support; per-app scope limits;
showing a connection's scope in Connected apps; renaming scopes (`mcp`, `read` stay).

Anti-goals (failure even if it works):
- Anyone already connected through Claude/Codex has to reconnect.
- The error page for someone else's app reveals who owns it.
- An official app can be created or claimed through the public API.
- A user disconnecting an official app deletes it or affects other users.
- Deleting an app leaves live access tokens behind.

## 4. Data model

Migration (both engines; version must sort after main's newest SQL and command migration at merge
time; renumber if main moves):

- `oauth_clients.owner_user_id` UUID NULL, FK `users(id) ON DELETE CASCADE`, indexed.
- `oauth_clients.is_global` BOOLEAN NOT NULL DEFAULT false.
- Backfill: for each client with grants, `owner_user_id` := the `user_id` of its earliest grant
  (`MIN(created_at)`, ties by grant id), revoked grants included. Clients without grants stay NULL.
  `is_global` stays false for every existing row.

Rules:
- A self-registered client is `is_global = false`; `owner_user_id` is NULL until the first approval.
- An official client is `is_global = true` and `owner_user_id` is always NULL.
- Purge (age and cap) applies only to `is_global = false AND last_used_at IS NULL` rows. Official apps
  are never purged. Approve already sets `last_used_at` in its transaction (`MarkClientUsed`), so a
  claimed client is never purgeable; the existing partial index on unused clients needs `is_global` in
  its predicate or the queries must stay index-friendly (builder's call).

## 5. The owner rule

- `validate` (used by describe, approve and decline) refuses a non-official client whose
  `owner_user_id` is set and is not the signed-in user, with the same treatment as an unknown client:
  an error page that never redirects. New error code `authserver.app_not_yours` (all 11 locales), text
  along the lines of "This app is connected to a different Econumo account. Remove Econumo from the app
  and add it again." It must not name, hint at or confirm the owner.
- Approve claims an unowned non-official client atomically inside its transaction:
  `UPDATE oauth_clients SET owner_user_id = $user WHERE id = $client AND is_global = false AND
  owner_user_id IS NULL`, then re-read; if the owner is someone else (a concurrent approval won), fail
  with the same error and write nothing. Keep the existing lock order (user row first).
- Decline never claims.
- Exchange and refresh do not re-check the owner: a code can only come from an approval that passed
  the check, and existing grants of non-owners left over from before the migration keep working for as
  long as they stay in use (refresh slides the 90-day idle window, so an active one never lapses) or
  until revoked. That is intended: nobody connected is cut off. Such users cannot approve that client
  again; if they ever need to, their MCP client registers a new app.
- `app:list` flags such clients ("N connections from other users") so the operator can spot a client
  that several people genuinely share and convert it with `app:make-official` (section 6).
- The owner disconnecting (Connected apps) revokes the grant only; the client stays theirs and they
  can reconnect.
- Owner deactivated: grants are revoked as today; the client stays owned (dead). Owner deleted: the
  client cascades away.

## 6. CLI: `app:*`

All in `internal/cli`, wired through the container like the user commands; ids only in log lines;
secrets printed once to stdout, never logged. Commands work whether or not the OAuth server is enabled
(`ECONUMO_URL`), but `app:register` warns when it is disabled.

| Command | Behaviour |
|---|---|
| `app:register <name> <redirect-uri>… [--public]` | Creates an **official** app. Validation as `/oauth/register` (name ≤ 100 runes, invisible runes stripped; 1–10 redirect URIs, each ≤ 2048 bytes, `ValidateRedirectURI`). Confidential by default (generates a secret; `client_secret_basic` and `client_secret_post` both work, as for self-registered confidential clients); `--public` = no secret, PKCE only. Prints `client_id` and `client_secret`. |
| `app:list [--official] [--user]` | One line per app: id, kind (`official`/`user`), name, first redirect host, owner user id (user apps) or `-`, created, last used, live connections. |
| `app:show <id>` | Everything above plus every redirect URI, auth (public/confidential), and live connections: user id, connected at, last used. (After #417 merges, also each connection's scope; whichever PR merges second adds it.) |
| `app:update <id> [--name <name>] [--redirect-uri <uri>]…` | Official apps only (a self-registered app's redirect list belongs to its user's client; rewriting it could send their codes elsewhere). Renames and/or replaces the redirect URI list (same validation); live grants are kept. |
| `app:make-official <id>` | Converts a user app into an official one (`is_global = true`, owner cleared; grants kept). For the rare existing client that several users genuinely share. One-way. |
| `app:rotate-secret <id>` | Confidential apps only. New secret printed once; the old one stops working immediately (no grace). Existing grants and refresh tokens stay valid with the new secret. |
| `app:disconnect <id> [--user <user-id>]` | Revokes every live grant (or one user's) and their access tokens, in the same order as `revokeGrant` (user lock, grant, tokens). The app stays. |
| `app:delete <id>` | Disconnects everyone as above, then deletes the client (codes, grants and spent refresh hashes cascade). `access_tokens.grant_id` has no FK, so tokens must be revoked before the delete. |

There is no reverse of `app:make-official`. Duplicate names and redirect URIs are allowed for official apps.
`app:list`/`app:show` print 0 connections and "never" for an app never used. Failures (unknown id,
wrong kind, `app:rotate-secret` on a public app, validation) print an error and exit non-zero, as the
`user:*` commands do. Revocations take effect on the next request: per-request authentication reads
the access token from the database (no token cache), so a CLI process revoking tokens needs no signal
to a running server; the builder confirms this in `internal/user/authenticate.go`.

## 6a. API and UI: the "Official" mark

A user app's name is whatever its client sent to `/oauth/register`, so anyone can register an app
called "Claude". Official apps are vouched for by the operator, and the UI says which is which.

- API (additive, no other wire change): `GET /api/v1/authserver/get-authorization-request` and
  `GET /api/v1/authserver/get-connected-app-list` each gain a boolean `isOfficial` (true when
  `is_global`). Goldens change additively.
- Consent page: an official app shows an "Official" badge next to its name. A user app shows a muted
  line above the existing redirect-host line: "Registered by the app itself. Econumo doesn't verify
  its name; check the address below."
- Connected apps: the same "Official" badge next to an official app's name; nothing extra for user apps.
- New i18n keys in all 11 locales; existing shadcn `Badge` patterns; no bold for hierarchy (badge,
  size and muted color only). Vitest for both components; regression test plan items for both screens.

## 7. Claude and other MCP clients

- Econumo advertises no CIMD, so Claude uses DCR, registering per connection: every connection becomes
  its user's app. Claude Code does the same (CIMD falls back to DCR).
- A Claude connector-directory listing can switch to **Anthropic-held client credentials** with one
  official app: `app:register "Claude" https://claude.ai/api/mcp/auth_callback`, id and secret entered in
  Anthropic's developer portal. Claude Code keeps registering per user.
- An organization admin's "Use your own OAuth client" in Claude must be an official app.
- If CIMD is ever added: a CIMD client id (e.g. `https://claude.ai/oauth/...metadata`) is shared by all
  users of that client and must be treated as official, or it works for one person only.
- Other MCP clients (Codex, ChatGPT, Cursor) were not verified; per-connection registration is the
  common pattern. A client that reuses one registration across users would work for its first user
  only; the remedy is an official app.

## 8. Docs and tests

Docs: CLAUDE.md (MCP endpoint / MCP OAuth section and the CLI command list), docs/mcp-setup.md
(user apps are per user; official apps via `app:register`; Claude directory note),
regression test plan (the "Official" badge and the unverified-name line on the consent page and
Connected apps; another account opening someone else's app link gets the error page; the owner
reconnects fine), .env.example untouched.

Tests: owner claimed on first approval; second user refused on describe, approve and decline with the
generic error and nothing written; concurrent approvals (one wins); owner reconnect; decline doesn't
claim; official app usable by two users; purge skips official apps; migration backfill (none / one user /
several users → earliest); pre-migration grants of a non-owner keep refreshing; every CLI command incl.
validation errors, `--public`, rotate (old secret refused, new accepted, grants intact), disconnect
(all / one user), delete (tokens dead immediately, rows gone); apiparity goldens for the consent error;
both engines (`make test-repo-pgsql`, `make test-engines`), `make test-upgrade`.

## 9. Follow-ups outside this PR

- share repo README: replace the `curl` registration with
  `econumo app:register "Econumo Share" https://share.econumo.com/auth/callback` (keep `curl` only as a
  note for instances without the command; a curl-registered app would now be single-user, so it is no
  longer a valid way to register Share).
- Claude listing: switch from DCR to Anthropic-held credentials once this ships (email
  directory@anthropic.com for a published listing).

## 10. Left to the builder

Package layout inside authserver/cli, exact output formatting of the commands, query shapes, the
exact error wording (within the constraint above), migration version.
