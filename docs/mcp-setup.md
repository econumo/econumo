# Connect Claude or Codex to Econumo (MCP)

Econumo serves an [MCP](https://modelcontextprotocol.io) endpoint at
`<your Econumo URL>/mcp`. Add that address to an MCP client, sign in to
Econumo in the browser when asked, approve the app, and its tools work with
your data. There is nothing to copy or paste: no token, no client id.

Under the hood this is OAuth 2.1 (authorization code with PKCE, dynamic client
registration, resource indicators), so any client that implements the MCP
authorization spec can connect the same way.

## Prerequisites

- Set `ECONUMO_URL` to the public https URL of your instance (for example
  `https://econumo.example.com`). It is the issuer of the sign-in flow and the
  address the clients are told to use. With it unset, the browser sign-in is
  off and `/mcp` accepts personal access tokens only (see below).
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

One scope, `mcp`: everything your own personal access token could do on
`/mcp`, which is reading and changing all of your Econumo data. The consent
page says so. The app never sees your password, and its token works only on
`/mcp`, not on the rest of the API.

The access token lasts one hour and the app renews it silently with a
rotating refresh token. A connection expires 90 days after its last refresh (clients
refresh at least hourly while in use), and the app then asks you to sign in
again.

## Revoking an app

Settings → Profile → Connected apps lists every app you approved with when it
was connected and last used. Revoke signs it out immediately; its next tool
call is rejected and it has to be approved again.

Resetting your password through the emailed link (or an operator changing it
with `user:change-password`) disconnects every app, as does deactivating the
account. Changing your password from Settings keeps them connected.

## Alternative: a personal access token

Clients that do not support the browser sign-in can send a personal access
token instead. Create one in Settings → Profile → API tokens (a token
created there has full access) and configure the client to send
`Authorization: Bearer eco_pat_...` to `<your Econumo URL>/mcp`. This works
with or without `ECONUMO_URL`.

## Troubleshooting

- The client says the server does not support authorization, or never opens a
  browser: check `ECONUMO_URL` is set, is the exact address the client uses
  (https, no typo, no different host) and that
  `<your Econumo URL>/.well-known/oauth-authorization-server` returns JSON.
- Behind a reverse proxy, make sure `/.well-known/*`, `/oauth/*` and `/mcp`
  are passed through to Econumo, not only `/api`.
- "This app sent an invalid request" on the approval page: go back to the app
  and start the connection again.
