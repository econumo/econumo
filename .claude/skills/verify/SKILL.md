---
name: verify
description: Run a locally built Econumo (Go binary + built SPA) against a scratch SQLite DB, seed data over the API, and drive the SPA with headless Chromium to observe a change end-to-end.
---

# Verifying Econumo changes end-to-end

## Build & launch

```bash
go build -o /tmp/econumo-verify/econumo ./cmd/econumo   # repo root
(cd web && pnpm build)                                   # SPA -> web/dist
cd /tmp/econumo-verify && DATABASE_URL="sqlite:///tmp/econumo-verify/db.sqlite" \
  PORT=8188 ECONUMO_WEB_DIST=<repo>/web/dist \
  ECONUMO_ALLOW_REGISTRATION=true ECONUMO_ANALYTICS=false ./econumo serve   # background
curl -s localhost:8188/health   # {"success":true,...}
```

Pick a free port — other sessions run their own `econumo serve`; when cleaning
up, `kill` your exact PID, never `pkill -f "econumo serve"`.

## Seed

```bash
./econumo user:create "Verifier" v@example.test secret123   # same DATABASE_URL env
```

- Login response is RAW `{token,user}` (no envelope): `POST /api/v1/user/login-user`
  with `{"username","password"}`.
- Everything else is enveloped `{success,message,data}`; writes are `POST`, reads `GET`.
- USD exists after migrations; get its id from `/currency/get-currency-list`.
- Seed order that works: create-account (blank `folderId` is OK for the first
  account; use the **server-returned** id) → create-category (`type:"expense"`,
  needs `accountId`) → create-transaction (amount positive, date `Y-m-d H:i:s`)
  → budget/create-budget (seeds an element per category, sets active budget so
  the SPA lands on it) → budget/create-folder → budget/move-element-list
  (`items:[{id:<elementId>,folderId,position}]`; element ids come from
  `get-budget?id=..&date=YYYY-MM-01` — the param is `date`, NOT `periodStart`,
  wrong params are silently ignored).

## Drive

Headless local Chromium via playwright-core (see the user-level
`debugging-in-browser` skill for the launch recipe; the Playwright MCP plugin is
broken here). The login form does not submit under headless playwright — instead:

```js
await page.goto('http://localhost:8188/')
await page.evaluate((t) => localStorage.setItem('token', t), token)  // eco_ses_… from curl login
await page.goto('http://localhost:8188/budget')
await page.waitForSelector('[data-testid="budget-table"]')
```

Useful hooks: sections are `[data-testid^="budget-folder-"]` (named
`budget-folder-<section name>`); budget edit mode = `button[aria-label="Configure"]`
→ menu item text "Edit structure"; totals `[data-testid="budget-totals"]`.
