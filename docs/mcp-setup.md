# Connect Claude or Codex to Econumo (MCP)

Econumo serves an [MCP](https://modelcontextprotocol.io) endpoint at
`<your Econumo URL>/mcp`. Add that address to an MCP client, sign in to
Econumo in the browser when asked, approve the app, and its tools work with
your data. There is nothing to copy or paste: no token, no client id.

Under the hood this is OAuth 2.1 (authorization code with PKCE, dynamic client
registration, resource indicators), so any client that implements the MCP
authorization spec can connect the same way.

Econumo's hosted instance is also submitted to Claude's connector directory,
where it appears as a one-click connector for `app.econumo.com`. A self-hosted
instance is added by URL as described below; see
`docs/mcp-directory-submission.md` for why the directory listing covers the
hosted instance only.

## Prerequisites

- Set `ECONUMO_URL` to the public https origin of your instance, with no path
  (for example `https://econumo.example.com`; `https://example.com/econumo` does
  not work, because the sign-in endpoints and discovery are served from the
  root). It is the issuer of the sign-in flow and the
  address the clients are told to use. The browser sign-in is off, and `/mcp`
  accepts personal access tokens only (see below), when it is unset, has a path, or is a
  plain `http://` address (plain http is accepted only for `localhost`,
  `127.0.0.1` and `[::1]`). Switching the sign-in off also stops `/mcp` from
  accepting access tokens that were issued earlier.
- Claude on the web, desktop and mobile connects from Anthropic's servers, so
  the instance must be reachable from the internet. Claude Code and Codex run
  on your computer and only need to reach the URL from there.
- The account must have full access. A read-only account (trial ended) cannot
  approve an app, and `/mcp` is closed to it.

## Claude (claude.ai, desktop, mobile)

1. Settings, Connectors, Add custom connector.
2. Enter `<your Econumo URL>/mcp` and add it.
3. Connect: your browser opens Econumo. Sign in if you are not already, check
   the app name and where it will send you, and press Allow.

## Claude Code

```bash
claude mcp add --transport http econumo <your Econumo URL>/mcp
```

Then run `/mcp` inside Claude Code, pick `econumo` and choose Authenticate.
The browser opens the Econumo approval page.

## Codex

```bash
codex mcp add econumo --url <your Econumo URL>/mcp
codex mcp login econumo
```

`codex mcp login` opens the browser for the approval page. Flags can change
between Codex releases; `codex mcp add --help` shows the current ones.

## What approving gives the app

By default the scope is `mcp`: everything your own personal access token could
do on `/mcp`, which is reading and changing all of your Econumo data. The
consent page says so. The app never sees your password, and its token works
only on `/mcp`, not on the rest of the API.

The access token lasts one hour and the app renews it silently with a
rotating refresh token. A connection expires 90 days after its last refresh (clients
refresh at least hourly while in use), and the app then asks you to sign in
again.

An app that registered itself (Claude, Codex, Claude Code) belongs to the
Econumo account that approves it first. If someone else opens its consent link,
Econumo shows "This app is connected to a different Econumo account" and
connects nothing; that person adds Econumo in their own client, which registers
a separate app for them. Disconnecting the app does not release it: the owner
can always connect it again. Apps the server operator registered with the CLI
(official apps) are open to every user.

## Read-only connections

An app can ask for the `read` scope instead, by requesting `read` and not `mcp`
at authorization. (A request with no scope, with `mcp`, or with both gets
`mcp`.) The consent page then says the app only reads: it can see your profile
(name and email), accounts, transactions, categories, tags, labels and
currencies, and change nothing. Accounts and transactions include the names of
people you share them with. A user whose access is read-only can approve such an
app, but not an `mcp` one.

A `read` token answers only these `GET` routes; everything else, `/mcp` included,
is a 401:

- `/api/v1/user/get-user-data`
- `/api/v1/account/get-account-list`
- `/api/v1/category/get-category-list`
- `/api/v1/tag/get-tag-list`
- `/api/v1/label/get-label-list`
- `/api/v1/currency/get-currency-list`
- `/api/v1/currency/get-currency-rate-list`
- `/api/v1/transaction/get-account-transaction-list`
- `/api/v1/transaction/get-transaction-list`

This needs nothing configured beyond the OAuth server itself, and turning that
server off stops every token it issued, `read` ones included. Discovery lists both
scopes on the authorization server (`/.well-known/oauth-authorization-server`)
and only `mcp` on the `/mcp` resource. A read-only app shows under Connected apps
and is revoked there like any other.

## Revoking an app

Settings → Profile → Connected apps lists every app you approved with when it
was connected and last used. Revoke signs it out immediately; its next tool
call is rejected and it has to be approved again. Approving the same app again
replaces its earlier connection instead of adding a second one.

Resetting your password through the emailed link (or an operator changing it
with `user:change-password`) disconnects every app, as does deactivating the
account. Changing your password from Settings keeps them connected.

## For operators: official apps

An app a client registers for itself belongs to the one Econumo account that
approves it first. To offer an app to every user of your instance, register it
as an official app with the CLI (`docker exec <container> /app/econumo ...` in
the image):

```bash
econumo app:register "Claude" https://claude.ai/api/mcp/auth_callback
econumo app:register "Econumo Share" https://share.econumo.com/auth/callback
```

The first is for a Claude directory listing whose credentials Anthropic holds,
or for your organization's own OAuth client; the second for the share-reports
app. Each prints a `client_id` and a `client_secret` (shown only once; add
`--public` for a client with no secret that relies on PKCE alone). Official
apps carry an "Official" badge on the consent page and in Connected apps.

A client registered with `curl` against `/oauth/register` is a user app and
works for one account only. `econumo app:list` shows `N (M from other users)`
for a user app that several people already share; `econumo app:make-official
<id>` makes it open to everyone (this cannot be undone). The other commands
are `app:show`, `app:update` (official apps: name and redirect URIs),
`app:rotate-secret`, `app:disconnect [--user <user-id>]` and `app:delete`;
disconnecting or deleting signs the app out at once. They work while the
OAuth server is off (`ECONUMO_URL` is not an https origin, or an http loopback origin), and `app:register` warns that
the app only works once it is on.

## Alternative: a personal access token

Clients that do not support the browser sign-in can send a personal access
token instead. Create one in Settings → Profile → API tokens (a token
created there has full access) and configure the client to send
`Authorization: Bearer eco_pat_...` to `<your Econumo URL>/mcp`. This works
with or without `ECONUMO_URL`.

## Troubleshooting

- The client says the server does not support authorization, or never opens a
  browser: check `ECONUMO_URL` is set, is the exact address the client uses
  (https, with no path, no typo, no different host) and that
  `<your Econumo URL>/.well-known/oauth-authorization-server` returns JSON.
- Behind a reverse proxy, make sure `/.well-known/*`, `/oauth/*` and `/mcp`
  are passed through to Econumo, not only `/api`.
- "This app sent an invalid request" on the approval page: go back to the app
  and start the connection again.
- "This app's registration has expired or is unknown" on the approval page: the
  app is holding a client registration Econumo no longer has (a registration
  that is never approved is deleted after 30 days, or sooner once 1,000 such
  registrations are waiting, or the app was registered
  against another Econumo instance). Make the app register again:
  - Claude Code: run `/mcp`, pick `econumo` and choose Clear authentication,
    then Authenticate; or `claude mcp remove econumo` and add it again.
  - Claude: remove the Econumo connector and add it again.
  - Codex: `codex mcp logout econumo`, then `codex mcp login econumo`.
