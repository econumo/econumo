# React Web Migration — Plan 1 of 6: Foundation + Auth

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the React + Vite + shadcn/ui app in `web-react/` with the full foundation (build, test harness, config, API client, i18n, routing, layouts) and a complete, parity-checked auth feature (login, registration, recovery, logout).

**Architecture:** Feature-folder React SPA per `docs/superpowers/specs/2026-07-01-react-web-migration-design.md`. The Vue app in `web/` stays untouched; `web-react/` is a sibling that talks to the same Go backend. Server data via TanStack Query; forms via react-hook-form (bundled with shadcn's Form); validation messages come from the ported validator functions + i18n catalog so they match the Vue app exactly.

**Tech Stack:** React 19, Vite 7, TypeScript (strict), Tailwind 4, shadcn/ui (full vendored set), react-router v7 (library mode), TanStack Query v5, react-i18next, axios, uuid v11 (`v7()`), jwt-decode v4, Vitest + React Testing Library + MSW.

## Global Constraints

- Package manager is **pnpm**; run all commands from `web-react/` unless stated otherwise. Repo root is `/…/econumo`; the Vue app being ported from is `../web` relative to `web-react/`.
- **API contract is frozen**: exact endpoint paths (`POST /api/v1/user/login-user`, …), the response envelope `{"success": bool, "message": string, "data"|"errors"|"code": …}`, datetimes as `"2006-01-02 15:04:05"`, `isArchived` as int `0`/`1`.
- **All client-generated ids are UUIDv7** (`v7()` from `uuid`). Every API request carries `X-Request-Id: <uuidv7>`, plus `Authorization: Bearer <jwt>` (when a token exists), `Accept: application/json`, `Accept-Language`, and `X-Timezone` (IANA name from `Intl.DateTimeFormat().resolvedOptions().timeZone`).
- **Auth-expiry behavior (approved divergence):** 401 from any endpoint except `login-user` → purge token, hard-redirect to `/login?reason=expired`; the login page shows a visible "session expired" notice. Route guard proactively checks the JWT `exp` claim.
- **Token storage divergence:** the token moves from a cookie (Vue) to `localStorage`. One-time cost at swap: users sign in again. Never read or write the old cookie.
- **i18n strings are ported verbatim** from `web/src/i18n/en-US/index.ts`; i18next is configured with single-brace `{name}` interpolation to match. User-visible copy must match the Vue app except the new session-expired notice.
- **Vendored shadcn components in `src/components/ui/` are never hand-edited.** Always prefer a shadcn component over custom markup.
- No lodash. No new runtime dependencies beyond those named in tasks.
- UI text in components always goes through `t('…')` — no hardcoded English.
- Commit after every task (each task's final step). Conventional-commit messages, `feat(web-react): …` style, ending with the two standard trailer lines used in this repo's recent commits.

---

### Task 1: Scaffold the Vite app + test harness

**Files:**
- Create: `web-react/` (Vite scaffold: `package.json`, `tsconfig*.json`, `vite.config.ts`, `index.html`, `src/main.tsx`, …)
- Create: `web-react/src/test/setup.ts`
- Create: `web-react/src/test/smoke.test.ts`

**Interfaces:**
- Produces: a building, testing React app; path alias `@/` → `web-react/src/`; `pnpm test` runs Vitest; Vite dev server proxies `/api` to the Go backend on `:8181`.

- [x] **Step 1: Scaffold**

Run from the repo root:

```bash
pnpm create vite web-react --template react-ts
cd web-react
pnpm install
pnpm add axios uuid jwt-decode @tanstack/react-query react-router react-i18next i18next
pnpm add -D vitest jsdom @testing-library/react @testing-library/user-event @testing-library/jest-dom msw @types/node
```

- [x] **Step 2: Configure Vite + Vitest + alias + proxy**

Replace `web-react/vite.config.ts`:

```ts
/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, 'src') },
  },
  server: {
    port: 9000,
    proxy: {
      '/api': 'http://localhost:8181',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test/setup.ts',
  },
})
```

Add to `web-react/tsconfig.app.json` under `compilerOptions` (keep the template's strict settings):

```json
"baseUrl": ".",
"paths": { "@/*": ["./src/*"] },
"types": ["vitest/globals", "@testing-library/jest-dom"]
```

Add the same `baseUrl`/`paths` block to the root `web-react/tsconfig.json` if the template split configs (shadcn's CLI reads it from there).

Create `web-react/src/test/setup.ts`:

```ts
import '@testing-library/jest-dom/vitest'
```

Add to `web-react/package.json` scripts:

```json
"test": "vitest run",
"test:watch": "vitest"
```

- [x] **Step 3: Write a smoke test**

Create `web-react/src/test/smoke.test.ts`:

```ts
describe('harness', () => {
  it('runs tests with jsdom and localStorage', () => {
    localStorage.setItem('k', 'v')
    expect(localStorage.getItem('k')).toBe('v')
  })
})
```

- [x] **Step 4: Verify test + build + lint pass**

Run in `web-react/`: `pnpm test` → 1 passed. `pnpm build` → succeeds. `pnpm lint` → clean.

- [x] **Step 5: Commit**

```bash
git add web-react
git commit -m "feat(web-react): scaffold Vite + React 19 + Vitest harness"
```

---

### Task 2: Tailwind 4 + full shadcn/ui vendor

**Files:**
- Modify: `web-react/vite.config.ts`, `web-react/src/index.css`
- Create: `web-react/components.json`, `web-react/src/components/ui/*` (full set), `web-react/src/lib/utils.ts` (shadcn's `cn`)

**Interfaces:**
- Produces: every shadcn component importable as `@/components/ui/<name>`; Tailwind 4 active; theme tokens in `src/index.css`.

- [x] **Step 1: Install Tailwind 4**

```bash
pnpm add tailwindcss @tailwindcss/vite
```

In `vite.config.ts` add `import tailwindcss from '@tailwindcss/vite'` and `tailwindcss()` to `plugins`. Replace `src/index.css` content with:

```css
@import "tailwindcss";
```

Delete `src/App.css` and the template's demo content in `src/App.tsx` (leave `App` returning `<div>econumo</div>` for now; it is replaced in Task 10).

- [x] **Step 2: Init shadcn and vendor the full set**

```bash
pnpm dlx shadcn@latest init
pnpm dlx shadcn@latest add --all
```

Accept defaults (base color: neutral; CSS variables: yes). This creates `components.json`, rewrites `src/index.css` with the token variables, adds `src/lib/utils.ts`, and vendors every component into `src/components/ui/`.

- [x] **Step 3: Verify with a render test**

Create `web-react/src/test/shadcn.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { Button } from '@/components/ui/button'

it('renders a shadcn button', () => {
  render(<Button>Save</Button>)
  expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
})
```

Run: `pnpm test` → all pass. Run `pnpm build` → succeeds.

- [x] **Step 4: Commit**

```bash
git add web-react
git commit -m "feat(web-react): add Tailwind 4 and vendor full shadcn/ui set"
```

---

### Task 3: Static shell — index.html, runtime config, LilTag, public assets

**Files:**
- Modify: `web-react/index.html`
- Create (copies): `web-react/public/econumo-config.js`, `web-react/public/liltag.min.js`, `web-react/public/icons/*`, `web-react/public/manifest.json`, `web-react/public/favicon.ico`, `web-react/public/browserconfig.xml`

**Interfaces:**
- Produces: `window.econumoConfig` available before the app boots; LilTag loads when `LILTAG_CONFIG_URL` is set — identical to the Vue app's `index.template.html`.

- [x] **Step 1: Copy public assets from the Vue app**

Run from `web-react/`:

```bash
cp ../web/public/econumo-config.js ../web/public/liltag.min.js public/
cp ../web/public/favicon.ico ../web/public/manifest.json public/ 2>/dev/null || true
cp -r ../web/public/icons public/ 2>/dev/null || true
cp ../web/public/browserconfig.xml public/ 2>/dev/null || true
rm -f public/vite.svg
```

Check `ls ../web/public/` first and copy every remaining static asset the Vue app ships (e.g. `robots.txt`) — the served file set must match.

- [x] **Step 2: Port index.html**

Replace `web-react/index.html` `<head>` (keep Vite's module script for `/src/main.tsx` in `<body>`):

```html
<!doctype html>
<html lang="en">
  <head>
    <title>Econumo</title>
    <meta charset="utf-8" />
    <meta name="description" content="An open-source budget application" />
    <meta name="format-detection" content="telephone=no" />
    <meta name="msapplication-tap-highlight" content="no" />
    <meta name="viewport" content="user-scalable=no, initial-scale=1, maximum-scale=1, minimum-scale=1, width=device-width" />
    <link rel="apple-touch-icon" sizes="180x180" href="/icons/apple-touch-icon.png" />
    <link rel="icon" type="image/png" sizes="32x32" href="/icons/favicon-32x32.png" />
    <link rel="icon" type="image/png" sizes="16x16" href="/icons/favicon-16x16.png" />
    <link rel="manifest" href="/manifest.json" />
    <link rel="mask-icon" href="/icons/safari-pinned-tab.svg" color="#5bbad5" />
    <link rel="shortcut icon" href="/favicon.ico" />
    <meta name="apple-mobile-web-app-title" content="Econumo" />
    <meta name="application-name" content="Econumo" />
    <meta name="msapplication-TileColor" content="#2b5797" />
    <meta name="msapplication-config" content="/browserconfig.xml" />
    <meta name="theme-color" content="#ffffff" />
    <script src="/econumo-config.js"></script>
    <script>
      (function (config) {
        if (!config.LILTAG_CONFIG_URL) {
          return;
        }
        const script = document.createElement("script");
        script.src = "/liltag.min.js";
        script.onload = function () {
          const lilTag = new LilTag(window.econumoConfig.LILTAG_CONFIG_URL);
          lilTag.enableCache(parseInt(window.econumoConfig.LILTAG_CACHE_TTL));
          lilTag.init();
        };
        document.head.appendChild(script);
      })(window.econumoConfig);
    </script>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [x] **Step 3: Verify**

Run `pnpm build`; confirm `dist/` contains `econumo-config.js`, `liltag.min.js`, `icons/`. Run `pnpm dev` and load `http://localhost:9000` — no console errors, `window.econumoConfig` is defined.

- [x] **Step 4: Commit**

```bash
git add web-react
git commit -m "feat(web-react): port index.html, runtime config and LilTag loader"
```

---

### Task 4: `lib/storage.ts` — token + localStorage helpers (TDD)

**Files:**
- Create: `web-react/src/lib/storage.ts`
- Test: `web-react/src/lib/storage.test.ts`

**Interfaces:**
- Produces: `getToken(): string | null`, `hasToken(): boolean`, `setToken(token: string): void`, `removeToken(): void`, `isTokenExpired(token: string): boolean`, `getItem(key: string): unknown`, `setItem(key: string, value: unknown): void`, `removeItem(key: string): void`.
- Note: the Vue `StorageKeys` cache-key enum is NOT ported — TanStack Query replaces that caching. `config.ts` (Task 5) uses raw keys `selfHosted`, `backendHost`, `locale` via `getItem`/`setItem`.

- [x] **Step 1: Write failing tests**

Create `web-react/src/lib/storage.test.ts`:

```ts
import { getToken, hasToken, setToken, removeToken, isTokenExpired, getItem, setItem } from './storage'

function fakeJwt(payload: object): string {
  const b64 = (o: object) => btoa(JSON.stringify(o)).replace(/=+$/, '')
  return `${b64({ alg: 'RS256', typ: 'JWT' })}.${b64(payload)}.sig`
}

beforeEach(() => localStorage.clear())

describe('token storage', () => {
  it('round-trips the token through localStorage', () => {
    expect(hasToken()).toBe(false)
    setToken('abc')
    expect(getToken()).toBe('abc')
    expect(hasToken()).toBe(true)
    removeToken()
    expect(getToken()).toBeNull()
  })

  it('detects an expired token by the exp claim', () => {
    const past = Math.floor(Date.now() / 1000) - 60
    const future = Math.floor(Date.now() / 1000) + 3600
    expect(isTokenExpired(fakeJwt({ exp: past }))).toBe(true)
    expect(isTokenExpired(fakeJwt({ exp: future }))).toBe(false)
  })

  it('treats a token without exp as not expired, and garbage as expired', () => {
    expect(isTokenExpired(fakeJwt({ id: 'u1' }))).toBe(false)
    expect(isTokenExpired('not-a-jwt')).toBe(true)
  })
})

describe('JSON item storage', () => {
  it('serializes values and parses them back', () => {
    setItem('k', { a: 1 })
    expect(getItem('k')).toEqual({ a: 1 })
  })

  it('returns null for missing keys and unparseable values', () => {
    expect(getItem('missing')).toBeNull()
    localStorage.setItem('bad', '{oops')
    expect(getItem('bad')).toBeNull()
  })
})
```

- [x] **Step 2: Run to verify failure**

Run: `pnpm vitest run src/lib/storage.test.ts` — FAIL (module not found).

- [x] **Step 3: Implement**

Create `web-react/src/lib/storage.ts`:

```ts
import { jwtDecode } from 'jwt-decode'

const TOKEN_KEY = 'token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function hasToken(): boolean {
  return getToken() !== null
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function removeToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

export function isTokenExpired(token: string): boolean {
  try {
    const { exp } = jwtDecode<{ exp?: number }>(token)
    if (!exp) {
      return false
    }
    return exp * 1000 <= Date.now()
  } catch {
    return true
  }
}

export function getItem(key: string): unknown {
  const value = localStorage.getItem(key)
  if (value === null) {
    return null
  }
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

export function setItem(key: string, value: unknown): void {
  localStorage.setItem(key, JSON.stringify(value))
}

export function removeItem(key: string): void {
  localStorage.removeItem(key)
}
```

- [x] **Step 4: Run tests**

Run: `pnpm vitest run src/lib/storage.test.ts` — PASS.

- [x] **Step 5: Commit**

```bash
git add web-react/src/lib/storage.ts web-react/src/lib/storage.test.ts
git commit -m "feat(web-react): token and localStorage helpers"
```

---

### Task 5: `lib/config.ts` + `lib/package.ts` — runtime config port (TDD)

**Files:**
- Create: `web-react/src/lib/config.ts`, `web-react/src/lib/package.ts`
- Test: `web-react/src/lib/config.test.ts`

**Interfaces:**
- Consumes: `getItem`/`setItem` from `@/lib/storage`.
- Produces: `backendHost(value?: string): string`, `selfHosted(value?: boolean): boolean`, `locale(value?: string): string`, `getLocaleOptions()`, `isHttps()`, `getWebsiteUrl()`, `getVersion()`, `isCustomApiAllowed()`, `isRegistrationAllowed()`, `isPaywallEnabled()`, and the `EconumoConfig` global typing (now including `LILTAG_CONFIG_URL?` and `LILTAG_CACHE_TTL?`). `lib/package.ts` exports `econumoPackage: { label, includesConnections, includesSharedAccess, isPaywallEnabled, paywallUrl }`.
- Source of truth for behavior: `web/src/modules/config.ts` and `web/src/modules/package.ts` — same precedence and defaults, with two changes: locale detection uses `navigator.language` instead of Quasar, and build-time env comes from `import.meta.env` instead of `process.env`.

- [x] **Step 1: Write failing tests**

Create `web-react/src/lib/config.test.ts`:

```ts
import { backendHost, selfHosted, locale, isCustomApiAllowed, isRegistrationAllowed, isPaywallEnabled, getVersion } from './config'

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

describe('backendHost', () => {
  it('prefers econumoConfig.API_URL over everything', () => {
    window.econumoConfig = { API_URL: 'https://api.example.test', ALLOW_CUSTOM_API: 'true' }
    expect(backendHost()).toBe('https://api.example.test')
  })

  it('falls back to the page origin when custom API is not allowed', () => {
    window.econumoConfig = { ALLOW_CUSTOM_API: 'false' }
    expect(backendHost()).toBe(window.location.origin)
  })

  it('uses the stored custom host when self-hosted mode is on and custom API allowed', () => {
    window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
    selfHosted(true)
    backendHost('https://my.box.test')
    expect(backendHost()).toBe('https://my.box.test')
  })
})

describe('flags', () => {
  it('parses booleans and strings for ALLOW_CUSTOM_API', () => {
    window.econumoConfig = { ALLOW_CUSTOM_API: true }
    expect(isCustomApiAllowed()).toBe(true)
    window.econumoConfig = { ALLOW_CUSTOM_API: 'false' }
    expect(isCustomApiAllowed()).toBe(false)
  })

  it('defaults registration=true, paywall=false when unset', () => {
    expect(isRegistrationAllowed()).toBe(true)
    expect(isPaywallEnabled()).toBe(false)
  })
})

describe('locale and version', () => {
  it('persists a chosen locale and defaults to en', () => {
    expect(locale()).toBe('en')
    locale('en')
    expect(locale()).toBe('en')
  })

  it('prefers econumoConfig.VERSION for the version label', () => {
    window.econumoConfig = { VERSION: 'v9.9.9' }
    expect(getVersion()).toBe('v9.9.9')
  })
})
```

- [x] **Step 2: Run to verify failure**

Run: `pnpm vitest run src/lib/config.test.ts` — FAIL (module not found).

- [x] **Step 3: Implement**

Create `web-react/src/lib/config.ts` — a line-for-line port of `web/src/modules/config.ts` with the Quasar locale branch replaced and `LILTAG_*` added to the typing:

```ts
import { getItem, setItem } from './storage'

export interface LocaleOption {
  value: string
  label: string
  short: string
}

export interface EconumoConfig {
  API_URL?: string
  ALLOW_REGISTRATION?: boolean | string
  PAYWALL_ENABLED?: boolean | string
  ALLOW_CUSTOM_API?: boolean | string
  VERSION?: string
  LILTAG_CONFIG_URL?: string
  LILTAG_CACHE_TTL?: string
}

declare global {
  interface Window {
    econumoConfig: EconumoConfig
  }
}

export function selfHosted(value?: boolean): boolean {
  if (!isCustomApiAllowed()) {
    return false
  }
  if (value === undefined) {
    return !!getItem('selfHosted')
  }
  setItem('selfHosted', value)
  return value
}

export function backendHost(value?: string): string {
  if (window.econumoConfig.API_URL) {
    return window.econumoConfig.API_URL
  }
  if (!isCustomApiAllowed()) {
    const url = new URL(window.location.href)
    return `${url.protocol}//${url.host}`
  }
  if (value === undefined) {
    const defaultHost = window.location.origin
    if (!selfHosted()) {
      return defaultHost
    }
    return (getItem('backendHost') as string | null) ?? defaultHost
  }
  setItem('backendHost', value)
  return value
}

export function isHttps(): boolean {
  return window.location.protocol === 'https:'
}

export function locale(value?: string): string {
  if (value === undefined) {
    const stored = getItem('locale')
    if (stored) {
      return stored as string
    }
    return (navigator.language || 'en').split('-')[0] || 'en'
  }
  setItem('locale', value)
  return value
}

export function getLocaleOptions(): LocaleOption[] {
  return [{ value: 'en', label: 'English', short: 'Eng' }]
}

export function getWebsiteUrl(): string {
  return import.meta.env.WEBSITE_URL ?? 'https://econumo.com'
}

export function getVersion(): string {
  return window.econumoConfig?.VERSION || String(import.meta.env.ECONUMO_VERSION ?? 'dev')
}

export function isCustomApiAllowed(): boolean {
  const allowCustomApi = window.econumoConfig?.ALLOW_CUSTOM_API
  if (typeof allowCustomApi === 'boolean') {
    return allowCustomApi
  }
  return allowCustomApi === 'true'
}

export function isRegistrationAllowed(): boolean {
  const allowRegistration = window.econumoConfig?.ALLOW_REGISTRATION
  if (allowRegistration === undefined) {
    return true
  }
  if (typeof allowRegistration === 'boolean') {
    return allowRegistration
  }
  return allowRegistration === 'true'
}

export function isPaywallEnabled(): boolean {
  const paywall = window.econumoConfig?.PAYWALL_ENABLED
  if (paywall === undefined) {
    return false
  }
  if (typeof paywall === 'boolean') {
    return paywall
  }
  return paywall === 'true'
}
```

Add to `vite.config.ts` inside `defineConfig({ … })` so the two env names keep working (Vite only exposes `VITE_*` by default):

```ts
envPrefix: ['VITE_', 'ECONUMO_', 'WEBSITE_'],
```

Create `web-react/src/lib/package.ts` (port of `web/src/modules/package.ts` — but computed lazily, because `window.econumoConfig` loads before the bundle):

```ts
import { isPaywallEnabled, getVersion } from './config'

export interface EconumoPackage {
  label: string
  includesConnections: boolean
  includesSharedAccess: boolean
  isPaywallEnabled: boolean
  paywallUrl: string
}

export function econumoPackage(): EconumoPackage {
  const paywall = isPaywallEnabled()
  return {
    label: getVersion(),
    includesConnections: true,
    includesSharedAccess: true,
    isPaywallEnabled: paywall,
    paywallUrl: paywall ? 'https://pay.econumo.com/cloud/' : '',
  }
}
```

- [x] **Step 4: Run tests**

Run: `pnpm vitest run src/lib/config.test.ts` — PASS. Also `pnpm build` (checks the `import.meta.env` typing compiles; if TS complains, add `/// <reference types="vite/client" />` usage is already in `src/vite-env.d.ts` — extend it with:)

```ts
interface ImportMetaEnv {
  readonly ECONUMO_VERSION?: string
  readonly WEBSITE_URL?: string
}
```

- [x] **Step 5: Commit**

```bash
git add web-react/src/lib/config.ts web-react/src/lib/config.test.ts web-react/src/lib/package.ts web-react/vite.config.ts web-react/src/vite-env.d.ts
git commit -m "feat(web-react): port runtime config and package metadata"
```

---

### Task 6: `lib/validation.ts` — validator port (TDD)

**Files:**
- Create: `web-react/src/lib/validation.ts`
- Test: `web-react/src/lib/validation.test.ts`

**Interfaces:**
- Produces: every function from `web/src/modules/helpers/validation.ts` EXCEPT `isValidFormula` and `hasIncompleteFormula` (they depend on the calculator, which is ported in Plan 3 — they move into `lib/validation.ts` then). Signatures: `isValidHttpUrl(value: string): boolean`, `isValidEmail`, `isValidNumber`, `isValidDecimalNumber`, `isValidName`, `isValidFolderName`, `isValidAccountName`, `isValidCategoryName`, `isValidTagName`, `isValidPayeeName`, `isValidBudgetName`, `isValidPassword`, `isValidBudgetFolderName`, `isValidBudgetEnvelopeName`, `isNotEmpty`, `isValidRecoveryCode` — all `(value: string) => boolean`.

- [x] **Step 1: Write failing tests**

Create `web-react/src/lib/validation.test.ts`:

```ts
import { isValidHttpUrl, isValidEmail, isValidName, isValidPassword, isNotEmpty, isValidRecoveryCode, isValidDecimalNumber } from './validation'

it('validates http(s) urls only', () => {
  expect(isValidHttpUrl('https://a.test')).toBe(true)
  expect(isValidHttpUrl('http://a.test')).toBe(true)
  expect(isValidHttpUrl('ftp://a.test')).toBe(false)
  expect(isValidHttpUrl('not a url')).toBe(false)
})

it('validates emails loosely (anything@anything)', () => {
  expect(isValidEmail('a@b')).toBe(true)
  expect(isValidEmail('nope')).toBe(false)
})

it('validates name 2-64, password >= 4, recovery code length 12', () => {
  expect(isValidName('ab')).toBe(true)
  expect(isValidName('a')).toBe(false)
  expect(isValidPassword('1234')).toBe(true)
  expect(isValidPassword('123')).toBe(false)
  expect(isValidRecoveryCode('123456789012')).toBe(true)
  expect(isValidRecoveryCode('123')).toBe(false)
})

it('treats empty as valid decimal and enforces up to 8 fraction digits', () => {
  expect(isValidDecimalNumber('')).toBe(true)
  expect(isValidDecimalNumber('-12.12345678')).toBe(true)
  expect(isValidDecimalNumber('1.123456789')).toBe(false)
})

it('isNotEmpty rejects empty string and null', () => {
  expect(isNotEmpty('x')).toBe(true)
  expect(isNotEmpty('')).toBe(false)
})
```

- [x] **Step 2: Run to verify failure**

Run: `pnpm vitest run src/lib/validation.test.ts` — FAIL.

- [x] **Step 3: Implement**

Create `web-react/src/lib/validation.ts` by copying `web/src/modules/helpers/validation.ts` verbatim, then: delete the top import from `./calculator`, delete `isValidFormula` and `hasIncompleteFormula`, and add explicit `: boolean` return types. (All other function bodies unchanged — they are the message-parity source of truth.)

- [x] **Step 4: Run tests**

Run: `pnpm vitest run src/lib/validation.test.ts` — PASS.

- [x] **Step 5: Commit**

```bash
git add web-react/src/lib/validation.ts web-react/src/lib/validation.test.ts
git commit -m "feat(web-react): port form validators"
```

---

### Task 7: `api/client.ts` — axios instance + interceptors (TDD)

**Files:**
- Create: `web-react/src/api/client.ts`
- Test: `web-react/src/api/client.test.ts`
- Create: `web-react/src/test/msw.ts`

**Interfaces:**
- Consumes: `getToken`, `removeToken` from `@/lib/storage`; `backendHost`, `locale` from `@/lib/config`.
- Produces: `api` (AxiosInstance) with request headers (`Accept`, `Authorization`, `Accept-Language`, `X-Timezone`, `X-Request-Id`) and the 401 handler; `apiUrl(path: string): string` returning `backendHost() + path`.

- [x] **Step 1: MSW test server helper**

Create `web-react/src/test/msw.ts`:

```ts
import { setupServer } from 'msw/node'

export const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())
```

(Import this from individual test files, not from the global setup, so unit tests that need no network don't pay for it.)

- [x] **Step 2: Write failing tests**

Create `web-react/src/api/client.test.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { api, apiUrl } from './client'
import { setToken, getToken } from '@/lib/storage'

const UUID_V7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('sends the standard headers on every request', async () => {
  let captured: Headers | undefined
  server.use(
    http.get('*/api/v1/ping', ({ request }) => {
      captured = request.headers
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  setToken('tok123')
  await api.get(apiUrl('/api/v1/ping'))
  expect(captured!.get('accept')).toBe('application/json')
  expect(captured!.get('authorization')).toBe('Bearer tok123')
  expect(captured!.get('x-timezone')).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone)
  expect(captured!.get('x-request-id')).toMatch(UUID_V7)
})

it('omits Authorization when there is no token', async () => {
  let captured: Headers | undefined
  server.use(
    http.get('*/api/v1/ping', ({ request }) => {
      captured = request.headers
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  await api.get(apiUrl('/api/v1/ping'))
  expect(captured!.get('authorization')).toBeNull()
})

it('on 401 purges the token and redirects to /login?reason=expired', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', {
    value: { ...window.location, assign },
    writable: true,
  })
  server.use(
    http.get('*/api/v1/secure', () =>
      HttpResponse.json({ success: false, message: 'Unauthorized', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  setToken('expired-tok')
  await expect(api.get(apiUrl('/api/v1/secure'))).rejects.toThrow()
  expect(getToken()).toBeNull()
  expect(assign).toHaveBeenCalledWith('/login?reason=expired')
})

it('does NOT redirect on 401 from login-user (invalid credentials case)', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', {
    value: { ...window.location, assign },
    writable: true,
  })
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Invalid credentials.', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  await expect(api.post(apiUrl('/api/v1/user/login-user'), {})).rejects.toThrow()
  expect(assign).not.toHaveBeenCalled()
})
```

- [x] **Step 3: Run to verify failure**

Run: `pnpm vitest run src/api/client.test.ts` — FAIL (module not found).

- [x] **Step 4: Implement**

Create `web-react/src/api/client.ts`:

```ts
import axios from 'axios'
import { v7 as uuidv7 } from 'uuid'
import { getToken, removeToken } from '@/lib/storage'
import { backendHost, locale } from '@/lib/config'

export const api = axios.create()

api.interceptors.request.use((config) => {
  config.headers.Accept = 'application/json'
  const token = getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  config.headers['Accept-Language'] = locale()
  config.headers['X-Timezone'] = Intl.DateTimeFormat().resolvedOptions().timeZone
  config.headers['X-Request-Id'] = uuidv7()
  return config
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    const status = error.response?.status
    const url: string = error.config?.url ?? ''
    if (status === 401 && !url.includes('/api/v1/user/login-user')) {
      removeToken()
      window.location.assign('/login?reason=expired')
    }
    return Promise.reject(error)
  },
)

export function apiUrl(path: string): string {
  return `${backendHost()}${path}`
}
```

- [x] **Step 5: Run tests**

Run: `pnpm vitest run src/api/client.test.ts` — PASS.

- [x] **Step 6: Commit**

```bash
git add web-react/src/api/client.ts web-react/src/api/client.test.ts web-react/src/test/msw.ts
git commit -m "feat(web-react): axios client with auth, timezone and X-Request-Id interceptors"
```

---

### Task 8: user API module + DTOs (TDD)

**Files:**
- Create: `web-react/src/api/types.ts`, `web-react/src/api/dto/user.ts`, `web-react/src/api/user.ts`
- Test: `web-react/src/api/user.test.ts`

**Interfaces:**
- Consumes: `api`, `apiUrl` from `@/api/client`.
- Produces:
  - `types.ts`: `export type Id = string`
  - `dto/user.ts`: `UserDto { id: Id; avatar: string; name: string }`, `CurrentUserDto { id: Id; name: string; email: string; avatar: string; options: UserOptionDto[]; currency: string; reportPeriod: string }`, `UserOptionDto { name: UserOptions; value: string | null }`, `enum UserOptions { CURRENCY='currency', CURRENCY_ID='currency_id', REPORT_PERIOD='report_period', BUDGET='budget', ONBOARDING='onboarding' }`, `UserLoginItemDto { user: CurrentUserDto; token: string }`, `UserLoginResponseDto { data: UserLoginItemDto }`, `CurrentUserResponseDto { data: { user: CurrentUserDto } }`
  - `user.ts`: `login(username, password): Promise<UserLoginItemDto>`, `logout(): Promise<void>`, `register(email, password, name): Promise<void>`, `updateName(name): Promise<void>`, `updatePassword(oldPassword, newPassword): Promise<void>`, `updateCurrency(currency): Promise<void>`, `updateDefaultBudget(budgetId: Id): Promise<void>`, `getUserData(): Promise<CurrentUserDto>`, `remindPassword(username): Promise<void>`, `resetPassword(username, code, password): Promise<void>`, `completeOnboarding(): Promise<void>` — all string params.

- [x] **Step 1: Write failing tests**

Create `web-react/src/api/user.test.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import * as userApi from './user'

const user = {
  id: '01890000-0000-7000-8000-000000000001',
  name: 'Ada',
  email: 'ada@example.test',
  avatar: '',
  options: [],
  currency: 'USD',
  reportPeriod: 'month',
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('login posts username/password to login-user and unwraps data', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/login-user', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user, token: 'jwt-token' } })
    }),
  )
  const result = await userApi.login('ada@example.test', 'secret')
  expect(body).toEqual({ username: 'ada@example.test', password: 'secret' })
  expect(result.token).toBe('jwt-token')
  expect(result.user.name).toBe('Ada')
})

it('login rejects on 401 invalid credentials', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Invalid credentials.', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  await expect(userApi.login('ada@example.test', 'wrong')).rejects.toThrow()
})

it('register posts email/password/name to register-user', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/register-user', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user } })
    }),
  )
  await userApi.register('ada@example.test', 'secret', 'Ada')
  expect(body).toEqual({ email: 'ada@example.test', password: 'secret', name: 'Ada' })
})

it('getUserData unwraps data.user', async () => {
  server.use(
    http.get('*/api/v1/user/get-user-data', () =>
      HttpResponse.json({ success: true, message: '', data: { user } }),
    ),
  )
  await expect(userApi.getUserData()).resolves.toEqual(user)
})

it('remindPassword and resetPassword hit their endpoints', async () => {
  const calls: string[] = []
  server.use(
    http.post('*/api/v1/user/remind-password', () => {
      calls.push('remind')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/user/reset-password', async ({ request }) => {
      calls.push('reset')
      expect(await request.json()).toEqual({ username: 'ada@example.test', code: '123456789012', password: 'newpass' })
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  await userApi.remindPassword('ada@example.test')
  await userApi.resetPassword('ada@example.test', '123456789012', 'newpass')
  expect(calls).toEqual(['remind', 'reset'])
})
```

- [x] **Step 2: Run to verify failure**

Run: `pnpm vitest run src/api/user.test.ts` — FAIL.

- [x] **Step 3: Implement**

Create `web-react/src/api/types.ts`:

```ts
export type Id = string
```

Create `web-react/src/api/dto/user.ts` with exactly the types listed in **Interfaces** above (they are a merge of `web/src/shared/dto/user.dto.ts` and `web/src/modules/api/v1/dto/user.dto.ts`, keeping field names verbatim including the deprecated `currency`/`reportPeriod`).

Create `web-react/src/api/user.ts`:

```ts
import { api, apiUrl } from './client'
import type { Id } from './types'
import type { CurrentUserDto, CurrentUserResponseDto, UserLoginItemDto, UserLoginResponseDto } from './dto/user'

export async function login(username: string, password: string): Promise<UserLoginItemDto> {
  const response = await api.post<UserLoginResponseDto>(apiUrl('/api/v1/user/login-user'), { username, password })
  return response.data.data
}

export async function logout(): Promise<void> {
  await api.post(apiUrl('/api/v1/user/logout-user'))
}

export async function register(email: string, password: string, name: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/register-user'), { email, password, name })
}

export async function updateName(name: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-name'), { name })
}

export async function updatePassword(oldPassword: string, newPassword: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-password'), { oldPassword, newPassword })
}

export async function updateCurrency(currency: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-currency'), { currency })
}

export async function updateDefaultBudget(budgetId: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-budget'), { value: budgetId })
}

export async function getUserData(): Promise<CurrentUserDto> {
  const response = await api.get<CurrentUserResponseDto>(apiUrl('/api/v1/user/get-user-data'))
  return response.data.data.user
}

export async function remindPassword(username: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/remind-password'), { username })
}

export async function resetPassword(username: string, code: string, password: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/reset-password'), { username, code, password })
}

export async function completeOnboarding(): Promise<void> {
  await api.post(apiUrl('/api/v1/user/complete-onboarding'))
}
```

- [x] **Step 4: Run tests**

Run: `pnpm vitest run src/api/user.test.ts` — PASS.

- [x] **Step 5: Commit**

```bash
git add web-react/src/api
git commit -m "feat(web-react): promise-based user API module with frozen DTOs"
```

---

### Task 9: i18n — react-i18next + ported catalog

**Files:**
- Create: `web-react/src/locales/en-US.ts` (copy of `web/src/i18n/en-US/index.ts`), `web-react/src/app/i18n.ts`
- Test: `web-react/src/app/i18n.test.tsx`

**Interfaces:**
- Produces: initialized i18next instance (default export of `@/app/i18n`); components use `useTranslation()` and call `t('modules.user.form.user.email.label')`-style dotted keys. Interpolation uses **single braces** `{name}` to match the ported catalog.
- Adds ONE new key (the session-expired notice): under `modules.user.page.sign_in` add `'session_expired': 'Your session has expired. Please sign in again.'`.

- [x] **Step 1: Copy the catalog**

```bash
mkdir -p src/locales
cp ../web/src/i18n/en-US/index.ts src/locales/en-US.ts
```

Edit `src/locales/en-US.ts`: add the `session_expired` key under `modules.user.page.sign_in` (next to its existing `header` key). Leave everything else byte-identical — including the vue-i18n pipe-plural string in the CSV-import section (it is converted in Plan 6 when that feature is built; it is not referenced before then).

- [x] **Step 2: Write failing test**

Create `web-react/src/app/i18n.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { I18nextProvider, useTranslation } from 'react-i18next'
import i18n from './i18n'

function Probe() {
  const { t } = useTranslation()
  return (
    <>
      <span>{t('modules.user.form.user.email.validation.required_field')}</span>
      <span>{t('elements.form.account.delete_account_modal.question', { account: 'Cash' })}</span>
    </>
  )
}

it('resolves dotted keys and single-brace interpolation', () => {
  render(
    <I18nextProvider i18n={i18n}>
      <Probe />
    </I18nextProvider>,
  )
  expect(screen.getByText('Required field')).toBeInTheDocument()
  expect(screen.getByText('Are you sure you want to remove the account «Cash»?')).toBeInTheDocument()
})
```

Run: `pnpm vitest run src/app/i18n.test.tsx` — FAIL.

- [x] **Step 3: Implement**

Create `web-react/src/app/i18n.ts`:

```ts
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import enUS from '@/locales/en-US'
import { locale } from '@/lib/config'

i18n.use(initReactI18next).init({
  lng: locale(),
  fallbackLng: 'en',
  resources: {
    en: { translation: enUS },
  },
  interpolation: {
    escapeValue: false,
    prefix: '{',
    suffix: '}',
  },
  returnNull: false,
})

export default i18n
```

- [x] **Step 4: Run tests**

Run: `pnpm vitest run src/app/i18n.test.tsx` — PASS. (If the exact assertion strings differ from the copied catalog, fix the TEST to the catalog's actual text — the catalog is the source of truth.)

- [x] **Step 5: Commit**

```bash
git add web-react/src/locales web-react/src/app/i18n.ts web-react/src/app/i18n.test.tsx
git commit -m "feat(web-react): react-i18next with ported en-US catalog"
```

---

### Task 10: Router, auth guard, layouts, app entry (TDD)

**Files:**
- Create: `web-react/src/app/router-pages.ts`, `web-react/src/app/RequireAuth.tsx`, `web-react/src/app/routes.tsx`, `web-react/src/app/layouts/LoginLayout.tsx`, `web-react/src/app/layouts/ApplicationLayout.tsx`, `web-react/src/pages/NotFoundPage.tsx`, `web-react/src/assets/econumo.svg` (copy)
- Modify: `web-react/src/main.tsx`; delete `web-react/src/App.tsx`
- Test: `web-react/src/app/RequireAuth.test.tsx`

**Interfaces:**
- Consumes: `getToken`, `removeToken`, `isTokenExpired` from `@/lib/storage`.
- Produces:
  - `router-pages.ts`: `export const RouterPage = { LOGIN: '/login', REGISTER: '/register', LOGOUT: '/logout', HOME: '/', ACCOUNT: (id: string) => `/account/${id}`, BUDGET: '/budget', ONBOARDING: '/onboarding', SETTINGS: '/settings', SETTINGS_PROFILE: '/settings/profile', SETTINGS_CHANGE_PASSWORD: '/settings/profile/change-password', SETTINGS_ACCOUNTS: '/settings/accounts', SETTINGS_CATEGORIES: '/settings/categories', SETTINGS_PAYEES: '/settings/payees', SETTINGS_TAGS: '/settings/tags', SETTINGS_CONNECTIONS: '/settings/connections', SETTINGS_BUDGETS: '/settings/budgets' } as const` — URL paths (react-router navigates by path, not name; paths are identical to the Vue route table).
  - `RequireAuth`: layout-route component rendering `<Outlet />` when a valid token exists; `<Navigate to="/login" replace />` when no token; purge + `<Navigate to="/login?reason=expired" replace />` when the token's `exp` is past.
  - `routes.tsx`: `createRouter(): ReturnType<typeof createBrowserRouter>` building the full route tree. Pages not yet built (home, account, budget, onboarding, settings…) render `ApplicationLayout` with an empty content area — real pages arrive in Plans 2–6 by swapping imports into this table.
  - `LoginLayout`: logo, Sign in / Sign up tab links (register tab disabled when `!isRegistrationAllowed() && !econumoPackage().isPaywallEnabled`), `<Outlet />`, GitHub/X social links, `© econumo.com` help link — the structure of `web/src/layouts/LoginLayout.vue` (language switcher arrives with the settings feature in Plan 4).
  - `ApplicationLayout`: minimal shell for now — `<div className="flex min-h-svh"><main className="flex-1"><Outlet /></main></div>`; the real sidebar shell is Plan 3's first task.

- [x] **Step 1: Copy the logo**

```bash
mkdir -p src/assets && cp ../web/src/assets/econumo.svg src/assets/
```

- [x] **Step 2: Write failing guard tests**

Create `web-react/src/app/RequireAuth.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { RequireAuth } from './RequireAuth'
import { setToken } from '@/lib/storage'

function fakeJwt(payload: object): string {
  const b64 = (o: object) => btoa(JSON.stringify(o)).replace(/=+$/, '')
  return `${b64({ alg: 'RS256' })}.${b64(payload)}.sig`
}

function renderAt(path: string) {
  const router = createMemoryRouter(
    [
      { path: '/login', element: <div>LOGIN PAGE</div> },
      { element: <RequireAuth />, children: [{ path: '/', element: <div>SECRET</div> }] },
    ],
    { initialEntries: [path] },
  )
  render(<RouterProvider router={router} />)
  return router
}

beforeEach(() => localStorage.clear())

it('renders the protected page with a valid token', () => {
  setToken(fakeJwt({ exp: Math.floor(Date.now() / 1000) + 3600 }))
  renderAt('/')
  expect(screen.getByText('SECRET')).toBeInTheDocument()
})

it('redirects to /login when there is no token', () => {
  const router = renderAt('/')
  expect(screen.getByText('LOGIN PAGE')).toBeInTheDocument()
  expect(router.state.location.search).toBe('')
})

it('redirects to /login?reason=expired and purges an expired token', () => {
  setToken(fakeJwt({ exp: Math.floor(Date.now() / 1000) - 60 }))
  const router = renderAt('/')
  expect(screen.getByText('LOGIN PAGE')).toBeInTheDocument()
  expect(router.state.location.search).toBe('?reason=expired')
  expect(localStorage.getItem('token')).toBeNull()
})
```

Run: `pnpm vitest run src/app/RequireAuth.test.tsx` — FAIL.

- [x] **Step 3: Implement guard, layouts, routes, entry**

Create `web-react/src/app/RequireAuth.tsx`:

```tsx
import { Navigate, Outlet } from 'react-router'
import { getToken, isTokenExpired, removeToken } from '@/lib/storage'

export function RequireAuth() {
  const token = getToken()
  if (!token) {
    return <Navigate to="/login" replace />
  }
  if (isTokenExpired(token)) {
    removeToken()
    return <Navigate to="/login?reason=expired" replace />
  }
  return <Outlet />
}
```

Create `web-react/src/app/router-pages.ts` with the exact object from **Interfaces**.

Create `web-react/src/app/layouts/LoginLayout.tsx`:

```tsx
import { Link, NavLink, Outlet } from 'react-router'
import { useTranslation } from 'react-i18next'
import { isRegistrationAllowed } from '@/lib/config'
import { econumoPackage } from '@/lib/package'
import { RouterPage } from '@/app/router-pages'
import logo from '@/assets/econumo.svg'

export function LoginLayout() {
  const { t } = useTranslation()
  const registerEnabled = econumoPackage().isPaywallEnabled || isRegistrationAllowed()
  const tabClass = ({ isActive }: { isActive: boolean }) =>
    `flex-1 border-b-2 pb-2 text-center text-sm font-medium ${isActive ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground'}`
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-6 p-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex justify-center">
          <img src={logo} width={194} height={20} alt={t('elements.econumo.label')} />
        </div>
        <div className="mb-6 flex">
          <NavLink to={RouterPage.LOGIN} className={tabClass}>
            {t('modules.user.page.sign_in.header')}
          </NavLink>
          {registerEnabled ? (
            <NavLink to={RouterPage.REGISTER} className={tabClass}>
              {t('modules.user.page.sign_up.header')}
            </NavLink>
          ) : (
            <span className="flex-1 border-b-2 border-transparent pb-2 text-center text-sm font-medium text-muted-foreground/50">
              {t('modules.user.page.sign_up.header')}
            </span>
          )}
        </div>
        <Outlet />
      </div>
      <div className="flex gap-4 text-sm text-muted-foreground">
        <a target="_blank" rel="nofollow" href="https://github.com/econumo/" aria-label="GitHub">GitHub</a>
        <a target="_blank" rel="nofollow" href="https://x.com/econumo" aria-label="Twitter">X</a>
      </div>
      <a target="_blank" rel="nofollow" href={t('blocks.help.url')} className="text-sm text-muted-foreground">
        {t('blocks.help.label')}
      </a>
    </div>
  )
}
```

Create `web-react/src/app/layouts/ApplicationLayout.tsx`:

```tsx
import { Outlet } from 'react-router'

export function ApplicationLayout() {
  return (
    <div className="flex min-h-svh">
      <main className="flex-1">
        <Outlet />
      </main>
    </div>
  )
}
```

Create `web-react/src/pages/NotFoundPage.tsx`:

```tsx
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { RouterPage } from '@/app/router-pages'

export function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-4">
      <h1 className="text-5xl font-bold">404</h1>
      <Link to={RouterPage.HOME} className="text-primary underline">
        {t('elements.econumo.label')}
      </Link>
    </div>
  )
}
```

Create `web-react/src/app/routes.tsx`:

```tsx
import { createBrowserRouter, Outlet } from 'react-router'
import { RequireAuth } from './RequireAuth'
import { LoginLayout } from './layouts/LoginLayout'
import { ApplicationLayout } from './layouts/ApplicationLayout'
import { NotFoundPage } from '@/pages/NotFoundPage'

// Pages land here as Plans 2-6 build them; until then guarded paths show the empty shell.
const EmptyPage = () => <div />

export function createRouter() {
  return createBrowserRouter([
    {
      element: <LoginLayout />,
      children: [
        { path: '/login', element: <EmptyPage /> },
        { path: '/register', element: <EmptyPage /> },
      ],
    },
    { path: '/logout', element: <EmptyPage /> },
    {
      element: <RequireAuth />,
      children: [
        {
          element: <ApplicationLayout />,
          children: [
            { path: '/', element: <EmptyPage /> },
            { path: '/account/:id', element: <EmptyPage /> },
            { path: '/budget', element: <EmptyPage /> },
            { path: '/onboarding', element: <EmptyPage /> },
            { path: '/settings', element: <EmptyPage /> },
            { path: '/settings/profile', element: <EmptyPage /> },
            { path: '/settings/profile/change-password', element: <EmptyPage /> },
            { path: '/settings/accounts', element: <EmptyPage /> },
            { path: '/settings/categories', element: <EmptyPage /> },
            { path: '/settings/payees', element: <EmptyPage /> },
            { path: '/settings/tags', element: <EmptyPage /> },
            { path: '/settings/connections', element: <EmptyPage /> },
            { path: '/settings/budgets', element: <EmptyPage /> },
          ],
        },
      ],
    },
    { path: '*', element: <NotFoundPage /> },
  ])
}
```

Replace `web-react/src/main.tsx`:

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import '@/app/i18n'
import './index.css'
import { createRouter } from '@/app/routes'

const queryClient = new QueryClient()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={createRouter()} />
    </QueryClientProvider>
  </StrictMode>,
)
```

Delete `web-react/src/App.tsx`.

- [x] **Step 4: Run tests + build**

Run: `pnpm vitest run src/app/RequireAuth.test.tsx` → PASS. `pnpm test` → all pass. `pnpm build` → succeeds.

- [x] **Step 5: Commit**

```bash
git add web-react/src
git commit -m "feat(web-react): router with auth guard, login/app layouts, app entry"
```

---

### Task 11: Responsive dialog primitive (TDD)

**Files:**
- Create: `web-react/src/components/ResponsiveDialog.tsx`, `web-react/src/hooks/useIsMobile.ts`
- Test: `web-react/src/components/ResponsiveDialog.test.tsx`

**Interfaces:**
- Consumes: shadcn `Dialog`/`Drawer` from `@/components/ui/`.
- Produces:
  - `useIsMobile(): boolean` — `window.matchMedia('(max-width: 767px)')`-backed hook with a change listener.
  - `ResponsiveDialog({ open, onOpenChange, title, description?, children, dismissible = true }: { open: boolean; onOpenChange: (open: boolean) => void; title: string; description?: string; children: ReactNode; dismissible?: boolean })` — renders a shadcn `Dialog` on desktop and a `Drawer` (bottom sheet) on mobile; `dismissible={false}` reproduces Quasar's `no-backdrop-dismiss`.
- Every feature modal in this and later plans renders inside this component.

- [x] **Step 1: Write failing tests**

Create `web-react/src/components/ResponsiveDialog.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { ResponsiveDialog } from './ResponsiveDialog'

function mockMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

it('renders title and children as a dialog on desktop', () => {
  mockMatchMedia(false)
  render(
    <ResponsiveDialog open onOpenChange={() => {}} title="My title">
      <p>body text</p>
    </ResponsiveDialog>,
  )
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  expect(screen.getByText('My title')).toBeInTheDocument()
  expect(screen.getByText('body text')).toBeInTheDocument()
})

it('renders as a drawer on mobile', () => {
  mockMatchMedia(true)
  render(
    <ResponsiveDialog open onOpenChange={() => {}} title="My title">
      <p>body text</p>
    </ResponsiveDialog>,
  )
  expect(screen.getByText('body text')).toBeInTheDocument()
})
```

Run: `pnpm vitest run src/components/ResponsiveDialog.test.tsx` — FAIL.

- [x] **Step 2: Implement**

Create `web-react/src/hooks/useIsMobile.ts`:

```ts
import { useEffect, useState } from 'react'

const QUERY = '(max-width: 767px)'

export function useIsMobile(): boolean {
  const [isMobile, setIsMobile] = useState(() => window.matchMedia(QUERY).matches)
  useEffect(() => {
    const mql = window.matchMedia(QUERY)
    const onChange = (e: MediaQueryListEvent) => setIsMobile(e.matches)
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [])
  return isMobile
}
```

Create `web-react/src/components/ResponsiveDialog.tsx`:

```tsx
import type { ReactNode } from 'react'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { useIsMobile } from '@/hooks/useIsMobile'

interface ResponsiveDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  children: ReactNode
  dismissible?: boolean
}

export function ResponsiveDialog({ open, onOpenChange, title, description, children, dismissible = true }: ResponsiveDialogProps) {
  const isMobile = useIsMobile()
  const handleOpenChange = (next: boolean) => {
    if (!next && !dismissible) {
      return
    }
    onOpenChange(next)
  }
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={handleOpenChange} dismissible={dismissible}>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>{title}</DrawerTitle>
            {description ? <DrawerDescription>{description}</DrawerDescription> : null}
          </DrawerHeader>
          <div className="px-4 pb-4">{children}</div>
        </DrawerContent>
      </Drawer>
    )
  }
  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        onInteractOutside={dismissible ? undefined : (e) => e.preventDefault()}
        showCloseButton={dismissible}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  )
}
```

(If the vendored `DialogContent` has no `showCloseButton` prop in the pulled shadcn version, omit that prop — check `src/components/ui/dialog.tsx` and adapt the call, not the vendored file.)

- [x] **Step 3: Run tests**

Run: `pnpm vitest run src/components/ResponsiveDialog.test.tsx` — PASS.

- [x] **Step 4: Commit**

```bash
git add web-react/src/components/ResponsiveDialog.tsx web-react/src/components/ResponsiveDialog.test.tsx web-react/src/hooks/useIsMobile.ts
git commit -m "feat(web-react): responsive dialog primitive (dialog on desktop, drawer on mobile)"
```

---

### Task 12: Auth mutations — `features/auth/queries.ts` (TDD)

**Files:**
- Create: `web-react/src/features/auth/queries.ts`
- Test: `web-react/src/features/auth/queries.test.tsx`

**Interfaces:**
- Consumes: `@/api/user`, `setToken` from `@/lib/storage`.
- Produces: `useLogin()` — mutation taking `{ username: string; password: string }`, on success stores the token via `setToken(data.token)` and returns `UserLoginItemDto`; `useRegister()` — mutation taking `{ email: string; password: string; name: string }`; `useRemindPassword()` — mutation taking `{ username: string }`; `useResetPassword()` — mutation taking `{ username: string; code: string; password: string }`. All are thin `useMutation` wrappers — pages read `isPending`, `mutateAsync`.

- [x] **Step 1: Write failing test**

Create `web-react/src/features/auth/queries.test.tsx`:

```tsx
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { getToken } from '@/lib/storage'
import { useLogin } from './queries'

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
    {children}
  </QueryClientProvider>
)

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('stores the token after a successful login', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({
        success: true,
        message: '',
        data: { user: { id: 'u1', name: 'Ada', email: 'a@b', avatar: '', options: [], currency: 'USD', reportPeriod: 'month' }, token: 'fresh-jwt' },
      }),
    ),
  )
  const { result } = renderHook(() => useLogin(), { wrapper })
  result.current.mutate({ username: 'a@b', password: 'pw' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(getToken()).toBe('fresh-jwt')
})

it('does not store a token on failed login', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Invalid credentials.', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  const { result } = renderHook(() => useLogin(), { wrapper })
  result.current.mutate({ username: 'a@b', password: 'bad' })
  await waitFor(() => expect(result.current.isError).toBe(true))
  expect(getToken()).toBeNull()
})
```

Run: `pnpm vitest run src/features/auth/queries.test.tsx` — FAIL.

- [x] **Step 2: Implement**

Create `web-react/src/features/auth/queries.ts`:

```ts
import { useMutation } from '@tanstack/react-query'
import * as userApi from '@/api/user'
import { setToken } from '@/lib/storage'

export function useLogin() {
  return useMutation({
    mutationFn: ({ username, password }: { username: string; password: string }) =>
      userApi.login(username, password),
    onSuccess: (data) => {
      setToken(data.token)
    },
  })
}

export function useRegister() {
  return useMutation({
    mutationFn: ({ email, password, name }: { email: string; password: string; name: string }) =>
      userApi.register(email, password, name),
  })
}

export function useRemindPassword() {
  return useMutation({
    mutationFn: ({ username }: { username: string }) => userApi.remindPassword(username),
  })
}

export function useResetPassword() {
  return useMutation({
    mutationFn: ({ username, code, password }: { username: string; code: string; password: string }) =>
      userApi.resetPassword(username, code, password),
  })
}
```

- [x] **Step 3: Run tests**

Run: `pnpm vitest run src/features/auth/queries.test.tsx` — PASS.

- [x] **Step 4: Commit**

```bash
git add web-react/src/features/auth
git commit -m "feat(web-react): auth mutations with token persistence"
```

---

### Task 13: Auth dialogs — SelfHostedInfo, Recovery, Fail (TDD)

**Files:**
- Create: `web-react/src/features/auth/SelfHostedInfoDialog.tsx`, `web-react/src/features/auth/RecoveryDialog.tsx`, `web-react/src/components/FailDialog.tsx`
- Test: `web-react/src/features/auth/RecoveryDialog.test.tsx`

**Interfaces:**
- Consumes: `ResponsiveDialog`, shadcn `Button`/`Input`/`Label`, `useRemindPassword`/`useResetPassword`, validators, `t()`.
- Produces:
  - `FailDialog({ open, onClose, title, description }: { open: boolean; onClose: () => void; title: string; description: string })` — shared OK-button failure dialog (used by login + registration; the OK label is `t('elements.button.ok.label')`).
  - `SelfHostedInfoDialog({ open, onClose }: { open: boolean; onClose: () => void })` — shows `t('modules.app.modal.self_hosted.information')` + OK.
  - `RecoveryDialog({ open, onClose }: { open: boolean; onClose: () => void })` — two-step recovery, non-dismissible by backdrop (`dismissible={false}` mirrors the Vue `no-backdrop-dismiss`): step 1 email → `useRemindPassword` → step 2 adds code (12 chars) + new password → `useResetPassword` → `onClose()`. Field validation mirrors `RecoveryModal.vue` (required/`isValidEmail`, required/`isValidRecoveryCode`, required/`isValidPassword`) with the same i18n message keys.

- [x] **Step 1: Write failing test**

Create `web-react/src/features/auth/RecoveryDialog.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { RecoveryDialog } from './RecoveryDialog'

function renderDialog(onClose = vi.fn()) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <RecoveryDialog open onClose={onClose} />
    </QueryClientProvider>,
  )
  return onClose
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('walks the two-step recovery flow', async () => {
  const user = userEvent.setup()
  const calls: string[] = []
  server.use(
    http.post('*/api/v1/user/remind-password', () => {
      calls.push('remind')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/user/reset-password', () => {
      calls.push('reset')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const onClose = renderDialog()

  await user.type(screen.getByLabelText(/e-?mail/i), 'ada@example.test')
  await user.click(screen.getByRole('button', { name: /recover/i }))
  expect(await screen.findByLabelText(/code/i)).toBeInTheDocument()

  await user.type(screen.getByLabelText(/code/i), '123456789012')
  await user.type(screen.getByLabelText(/password/i), 'newpass1')
  await user.click(screen.getByRole('button', { name: /change password/i }))

  await vi.waitFor(() => expect(onClose).toHaveBeenCalled())
  expect(calls).toEqual(['remind', 'reset'])
})

it('validates the email before sending', async () => {
  const user = userEvent.setup()
  renderDialog()
  await user.type(screen.getByLabelText(/e-?mail/i), 'not-an-email')
  await user.click(screen.getByRole('button', { name: /recover/i }))
  expect(await screen.findByText(/invalid e-?mail/i)).toBeInTheDocument()
})
```

The accessible-name regexes must match the catalog's actual label strings under `modules.user.form.user.*` and `modules.user.form.access_recovery.action.*` — after copying the catalog in Task 9, adjust the regexes to the real copy if they differ.

Run: `pnpm vitest run src/features/auth/RecoveryDialog.test.tsx` — FAIL.

- [x] **Step 2: Implement**

Create `web-react/src/components/FailDialog.tsx`:

```tsx
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

interface FailDialogProps {
  open: boolean
  onClose: () => void
  title: string
  description: string
}

export function FailDialog({ open, onClose, title, description }: FailDialogProps) {
  const { t } = useTranslation()
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title} description={description}>
      <Button className="w-full" onClick={onClose}>
        {t('elements.button.ok.label')}
      </Button>
    </ResponsiveDialog>
  )
}
```

Create `web-react/src/features/auth/SelfHostedInfoDialog.tsx`:

```tsx
import { useTranslation } from 'react-i18next'
import { FailDialog } from '@/components/FailDialog'

export function SelfHostedInfoDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <FailDialog
      open={open}
      onClose={onClose}
      title={t('elements.econumo.label')}
      description={t('modules.app.modal.self_hosted.information')}
    />
  )
}
```

Create `web-react/src/features/auth/RecoveryDialog.tsx`:

```tsx
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { isNotEmpty, isValidEmail, isValidPassword, isValidRecoveryCode } from '@/lib/validation'
import { useRemindPassword, useResetPassword } from './queries'

interface RecoveryForm {
  email: string
  code: string
  password: string
}

export function RecoveryDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const [isCodeSent, setIsCodeSent] = useState(false)
  const remind = useRemindPassword()
  const reset = useResetPassword()
  const form = useForm<RecoveryForm>({ mode: 'onTouched', defaultValues: { email: '', code: '', password: '' } })
  const { register, handleSubmit, getValues, formState: { errors } } = form

  const sendCode = handleSubmit(async ({ email }) => {
    await remind.mutateAsync({ username: email })
    setIsCodeSent(true)
  })

  const changePassword = handleSubmit(async ({ email, code, password }) => {
    await reset.mutateAsync({ username: email, code, password })
    onClose()
  })

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('modules.user.modal.access_recovery.header')}
      description={t('modules.user.modal.access_recovery.information')}
      dismissible={false}
    >
      <form onSubmit={isCodeSent ? changePassword : sendCode} className="flex flex-col gap-4" noValidate>
        <div className="flex flex-col gap-2">
          <Label htmlFor="recovery-email">{t('modules.user.form.user.email.placeholder')}</Label>
          <Input
            id="recovery-email"
            type="email"
            disabled={isCodeSent}
            autoFocus={!isCodeSent}
            {...register('email', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.email.validation.required_field'),
                email: (v) => isValidEmail(v) || t('modules.user.form.user.email.validation.invalid_email'),
              },
            })}
          />
          {errors.email ? <p className="text-sm text-destructive">{errors.email.message}</p> : null}
        </div>

        {isCodeSent ? (
          <>
            <p className="text-sm text-muted-foreground">{t('modules.user.modal.access_recovery.instruction')}</p>
            <div className="flex flex-col gap-2">
              <Label htmlFor="recovery-code">{t('modules.user.form.user.code.placeholder')}</Label>
              <Input
                id="recovery-code"
                autoFocus
                {...register('code', {
                  validate: {
                    required: (v) => isNotEmpty(v) || t('modules.user.form.user.code.validation.required_field'),
                    code: (v) => isValidRecoveryCode(v) || t('modules.user.form.user.code.validation.invalid_code'),
                  },
                })}
              />
              {errors.code ? <p className="text-sm text-destructive">{errors.code.message}</p> : null}
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="recovery-password">{t('modules.user.form.user.password.placeholder')}</Label>
              <Input
                id="recovery-password"
                type="password"
                {...register('password', {
                  validate: {
                    required: (v) => isNotEmpty(v) || t('modules.user.form.user.password.validation.required_field'),
                    password: (v) => isValidPassword(v) || t('modules.user.form.user.password.validation.invalid_password'),
                  },
                })}
              />
              {errors.password ? <p className="text-sm text-destructive">{errors.password.message}</p> : null}
            </div>
            <Button type="submit" className="w-full" disabled={reset.isPending}>
              {t('modules.user.form.access_recovery.action.change_password.label')}
            </Button>
          </>
        ) : (
          <Button type="submit" className="w-full" disabled={remind.isPending}>
            {t('modules.user.form.access_recovery.action.recover.label')}
          </Button>
        )}
      </form>
    </ResponsiveDialog>
  )
}
```

(`react-hook-form` was installed by shadcn's `add --all` in Task 2; if `pnpm ls react-hook-form` shows it missing, `pnpm add react-hook-form`. `getValues` import is unused — drop it if the linter flags it.)

- [x] **Step 3: Run tests**

Run: `pnpm vitest run src/features/auth/RecoveryDialog.test.tsx` — PASS.

- [x] **Step 4: Commit**

```bash
git add web-react/src/features/auth web-react/src/components/FailDialog.tsx
git commit -m "feat(web-react): recovery, self-hosted info and fail dialogs"
```

---

### Task 14: Login page (TDD)

**Files:**
- Create: `web-react/src/features/auth/LoginPage.tsx`
- Modify: `web-react/src/app/routes.tsx` (swap `/login` element)
- Test: `web-react/src/features/auth/LoginPage.test.tsx`

**Interfaces:**
- Consumes: `useLogin`, `RecoveryDialog`, `SelfHostedInfoDialog`, `FailDialog`, validators, `config.isCustomApiAllowed`/`selfHosted`/`backendHost`, `getToken` + `isTokenExpired` from storage.
- Produces: `LoginPage` — behavior parity with `web/src/pages/Login.vue`:
  - email + password fields (same validation keys), submit → `useLogin`;
  - success → `window.location.assign('/')` (full reload, matching the Vue app);
  - failure → `FailDialog` with `modules.user.modal.sign_in_failed.{header,information}`;
  - "forgot password" button opens `RecoveryDialog`;
  - when `isCustomApiAllowed()`: self-hosted checkbox (persists via `config.selfHosted(v)`) and, when checked, a host URL field (persists via `config.backendHost(v)`), plus the hint opening `SelfHostedInfoDialog`;
  - `?reason=expired` → shadcn `Alert` with `t('modules.user.page.sign_in.session_expired')`;
  - already authenticated (valid token) on mount → redirect to `/`.

- [x] **Step 1: Write failing tests**

Create `web-react/src/features/auth/LoginPage.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { LoginPage } from './LoginPage'

function renderLogin(path = '/login') {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  const router = createMemoryRouter([{ path: '/login', element: <LoginPage /> }], { initialEntries: [path] })
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('logs in and stores the token', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({
        success: true, message: '',
        data: { user: { id: 'u1', name: 'Ada', email: 'a@b', avatar: '', options: [], currency: 'USD', reportPeriod: 'month' }, token: 'jwt' },
      }),
    ),
  )
  const user = userEvent.setup()
  renderLogin()
  await user.type(screen.getByLabelText(/e-?mail/i), 'ada@example.test')
  await user.type(screen.getByLabelText(/password/i), 'secret')
  await user.click(screen.getByRole('button', { name: /sign in/i }))
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/'))
  expect(localStorage.getItem('token')).toBe('jwt')
})

it('shows the failure dialog on invalid credentials', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Invalid credentials.', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  const user = userEvent.setup()
  renderLogin()
  await user.type(screen.getByLabelText(/e-?mail/i), 'ada@example.test')
  await user.type(screen.getByLabelText(/password/i), 'wrong')
  await user.click(screen.getByRole('button', { name: /sign in/i }))
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
})

it('shows the session-expired notice when reason=expired', () => {
  renderLogin('/login?reason=expired')
  expect(screen.getByText(/session has expired/i)).toBeInTheDocument()
})

it('hides the self-hosted section when custom API is not allowed', () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'false' }
  renderLogin()
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
})

it('shows the self-hosted section when custom API is allowed', () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
  renderLogin()
  expect(screen.getByRole('checkbox')).toBeInTheDocument()
})
```

(As in Task 13: the name/label regexes must match the ported catalog copy; adjust to the real strings.)

Run: `pnpm vitest run src/features/auth/LoginPage.test.tsx` — FAIL.

- [x] **Step 2: Implement**

Create `web-react/src/features/auth/LoginPage.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { FailDialog } from '@/components/FailDialog'
import * as config from '@/lib/config'
import { getToken, isTokenExpired } from '@/lib/storage'
import { isNotEmpty, isValidEmail, isValidHttpUrl } from '@/lib/validation'
import { RecoveryDialog } from './RecoveryDialog'
import { SelfHostedInfoDialog } from './SelfHostedInfoDialog'
import { useLogin } from './queries'

interface LoginForm {
  username: string
  password: string
  selfHosted: boolean
  host: string
}

export function LoginPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const login = useLogin()
  const [failOpen, setFailOpen] = useState(false)
  const [recoveryOpen, setRecoveryOpen] = useState(false)
  const [infoOpen, setInfoOpen] = useState(false)
  const sessionExpired = searchParams.get('reason') === 'expired'
  const customApiAllowed = config.isCustomApiAllowed()

  const { register, handleSubmit, control, watch, formState: { errors } } = useForm<LoginForm>({
    mode: 'onTouched',
    defaultValues: {
      username: '',
      password: '',
      selfHosted: config.selfHosted(),
      host: config.backendHost() || '',
    },
  })
  const selfHostedChecked = watch('selfHosted')

  useEffect(() => {
    const token = getToken()
    if (token && !isTokenExpired(token)) {
      window.location.assign('/')
    }
  }, [])

  const onSubmit = handleSubmit(async ({ username, password, selfHosted, host }) => {
    if (customApiAllowed) {
      config.selfHosted(selfHosted)
      if (selfHosted && host) {
        config.backendHost(host)
      }
    }
    try {
      const result = await login.mutateAsync({ username, password })
      if (!result.token) {
        setFailOpen(true)
        return
      }
      window.location.assign('/')
    } catch {
      setFailOpen(true)
    }
  })

  return (
    <div className="flex w-full flex-col gap-4">
      {sessionExpired ? (
        <Alert variant="destructive">
          <AlertDescription>{t('modules.user.page.sign_in.session_expired')}</AlertDescription>
        </Alert>
      ) : null}

      <form onSubmit={onSubmit} className="flex flex-col gap-4" aria-label="Login form" noValidate>
        <div className="flex flex-col gap-2">
          <Label htmlFor="login-email">{t('modules.user.form.user.email.label')}</Label>
          <Input
            id="login-email"
            type="email"
            placeholder={t('modules.user.form.user.email.placeholder')}
            aria-required="true"
            {...register('username', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.email.validation.required_field'),
                email: (v) => isValidEmail(v) || t('modules.user.form.user.email.validation.invalid_email'),
              },
            })}
          />
          {errors.username ? <p className="text-sm text-destructive">{errors.username.message}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="login-password">{t('modules.user.form.user.password.label')}</Label>
          <Input
            id="login-password"
            type="password"
            placeholder={t('modules.user.form.user.password.placeholder')}
            aria-required="true"
            {...register('password', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.password.validation.required_field'),
              },
            })}
          />
          {errors.password ? <p className="text-sm text-destructive">{errors.password.message}</p> : null}
        </div>

        {customApiAllowed ? (
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2">
              <Controller
                control={control}
                name="selfHosted"
                render={({ field }) => (
                  <Checkbox id="login-self-hosted" checked={field.value} onCheckedChange={field.onChange} />
                )}
              />
              <Label htmlFor="login-self-hosted">{t('modules.user.form.user.server_host.self_hosted')}</Label>
              <button
                type="button"
                className="text-sm text-muted-foreground underline"
                onClick={() => setInfoOpen(true)}
                aria-label={t('modules.app.modal.self_hosted.information')}
              >
                ?
              </button>
            </div>
            {selfHostedChecked ? (
              <div className="flex flex-col gap-2">
                <Label htmlFor="login-host">{t('modules.user.form.user.server_host.label')}</Label>
                <Input
                  id="login-host"
                  type="url"
                  placeholder={t('modules.user.form.user.server_host.placeholder')}
                  {...register('host', {
                    validate: {
                      required: (v) => isNotEmpty(v) || t('modules.user.form.user.server_host.validation.required_field'),
                      url: (v) => isValidHttpUrl(v) || t('modules.user.form.user.server_host.validation.invalid_url'),
                    },
                  })}
                />
                {errors.host ? <p className="text-sm text-destructive">{errors.host.message}</p> : null}
              </div>
            ) : null}
          </div>
        ) : null}

        <Button type="submit" className="w-full" disabled={login.isPending}>
          {t('modules.user.form.sign_in.action.sign_in')}
        </Button>
        <Button type="button" variant="secondary" className="w-full" onClick={() => setRecoveryOpen(true)}>
          {t('modules.user.form.sign_in.action.forget_password')}
        </Button>
      </form>

      <FailDialog
        open={failOpen}
        onClose={() => setFailOpen(false)}
        title={t('modules.user.modal.sign_in_failed.header')}
        description={t('modules.user.modal.sign_in_failed.information')}
      />
      <SelfHostedInfoDialog open={infoOpen} onClose={() => setInfoOpen(false)} />
      {recoveryOpen ? <RecoveryDialog open onClose={() => setRecoveryOpen(false)} /> : null}
    </div>
  )
}
```

In `web-react/src/app/routes.tsx`, import `LoginPage` and replace the `/login` element: `{ path: '/login', element: <LoginPage /> }`.

- [x] **Step 3: Run tests**

Run: `pnpm vitest run src/features/auth/LoginPage.test.tsx` — PASS. Then `pnpm test` — all pass.

- [x] **Step 4: Commit**

```bash
git add web-react/src/features/auth/LoginPage.tsx web-react/src/features/auth/LoginPage.test.tsx web-react/src/app/routes.tsx
git commit -m "feat(web-react): login page with recovery, self-hosted mode and session-expired notice"
```

---

### Task 15: Registration page (TDD)

**Files:**
- Create: `web-react/src/features/auth/RegistrationPage.tsx`
- Modify: `web-react/src/app/routes.tsx` (swap `/register` element)
- Test: `web-react/src/features/auth/RegistrationPage.test.tsx`

**Interfaces:**
- Consumes: `useRegister`, `FailDialog`, `SelfHostedInfoDialog`, validators, config, `econumoPackage`.
- Produces: `RegistrationPage` — behavior parity with `web/src/pages/Registration.vue`:
  - when `econumoPackage().isPaywallEnabled`: render the paywall block instead of the form — `modules.user.page.sign_up.paywall.{header,text,action,next_steps}` (header/text via `dangerouslySetInnerHTML`, matching the Vue `v-html`) with the action linking to `econumoPackage().paywallUrl` (`target="_blank" rel="noopener noreferrer"`);
  - otherwise: name (required + `isValidName`), email (required + `isValidEmail`), password (required + `isValidPassword`), password-retry (required + must equal password, message key `modules.user.form.user.password_retry.validation.not_equals`), the same self-hosted section as LoginPage, privacy text (`modules.user.page.sign_up.privacy.text` via `dangerouslySetInnerHTML`), submit → `useRegister`;
  - success → navigate to `/login` (react-router `useNavigate`); failure → `FailDialog` with `modules.user.modal.sign_up_failed.{header,information}`;
  - valid token on mount → `window.location.assign('/')`.

- [x] **Step 1: Write failing tests**

Create `web-react/src/features/auth/RegistrationPage.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { RegistrationPage } from './RegistrationPage'

function renderPage() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  const router = createMemoryRouter(
    [
      { path: '/register', element: <RegistrationPage /> },
      { path: '/login', element: <div>LOGIN PAGE</div> },
    ],
    { initialEntries: ['/register'] },
  )
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('registers and navigates to the login page', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/register-user', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: { id: 'u1', name: 'Ada', avatar: '' } } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.type(screen.getByLabelText(/name/i), 'Ada')
  await user.type(screen.getByLabelText(/e-?mail/i), 'ada@example.test')
  const pwFields = screen.getAllByLabelText(/password/i)
  await user.type(pwFields[0], 'secret1')
  await user.type(pwFields[1], 'secret1')
  await user.click(screen.getByRole('button', { name: /sign up/i }))
  expect(await screen.findByText('LOGIN PAGE')).toBeInTheDocument()
  expect(body).toEqual({ email: 'ada@example.test', password: 'secret1', name: 'Ada' })
})

it('rejects mismatched password retry', async () => {
  const user = userEvent.setup()
  renderPage()
  const pwFields = screen.getAllByLabelText(/password/i)
  await user.type(pwFields[0], 'secret1')
  await user.type(pwFields[1], 'different')
  await user.click(screen.getByRole('button', { name: /sign up/i }))
  expect(await screen.findAllByText(/./, { selector: 'p.text-destructive, p.text-sm.text-destructive' })).not.toHaveLength(0)
})

it('shows the paywall instead of the form when enabled', () => {
  window.econumoConfig = { PAYWALL_ENABLED: 'true' }
  renderPage()
  expect(screen.queryByRole('button', { name: /sign up/i })).not.toBeInTheDocument()
  expect(screen.getByRole('link')).toHaveAttribute('href', 'https://pay.econumo.com/cloud/')
})
```

(Adjust label regexes to the catalog copy; the password-retry mismatch assertion should target the exact `not_equals` message text from the catalog once known.)

Run: `pnpm vitest run src/features/auth/RegistrationPage.test.tsx` — FAIL.

- [x] **Step 2: Implement**

Create `web-react/src/features/auth/RegistrationPage.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { FailDialog } from '@/components/FailDialog'
import * as config from '@/lib/config'
import { econumoPackage } from '@/lib/package'
import { getToken, isTokenExpired } from '@/lib/storage'
import { isNotEmpty, isValidEmail, isValidHttpUrl, isValidName, isValidPassword } from '@/lib/validation'
import { RouterPage } from '@/app/router-pages'
import { SelfHostedInfoDialog } from './SelfHostedInfoDialog'
import { useRegister } from './queries'

interface RegistrationForm {
  name: string
  email: string
  password: string
  passwordRetry: string
  selfHosted: boolean
  host: string
}

export function RegistrationPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const registerMutation = useRegister()
  const [failOpen, setFailOpen] = useState(false)
  const [infoOpen, setInfoOpen] = useState(false)
  const customApiAllowed = config.isCustomApiAllowed()
  const pkg = econumoPackage()

  const { register, handleSubmit, control, watch, formState: { errors } } = useForm<RegistrationForm>({
    mode: 'onTouched',
    defaultValues: {
      name: '',
      email: '',
      password: '',
      passwordRetry: '',
      selfHosted: config.selfHosted(),
      host: config.backendHost() || '',
    },
  })
  const selfHostedChecked = watch('selfHosted')

  useEffect(() => {
    const token = getToken()
    if (token && !isTokenExpired(token)) {
      window.location.assign('/')
    }
  }, [])

  if (pkg.isPaywallEnabled) {
    return (
      <div className="flex flex-col items-center gap-4 text-center">
        <div dangerouslySetInnerHTML={{ __html: t('modules.user.page.sign_up.paywall.header') }} />
        <div dangerouslySetInnerHTML={{ __html: t('modules.user.page.sign_up.paywall.text') }} />
        <Button asChild size="lg">
          <a href={pkg.paywallUrl} target="_blank" rel="noopener noreferrer">
            {t('modules.user.page.sign_up.paywall.action')}
          </a>
        </Button>
        <p className="text-sm text-muted-foreground">{t('modules.user.page.sign_up.paywall.next_steps')}</p>
      </div>
    )
  }

  const onSubmit = handleSubmit(async ({ name, email, password, selfHosted, host }) => {
    if (customApiAllowed) {
      config.selfHosted(selfHosted)
      if (selfHosted && host) {
        config.backendHost(host)
      }
    }
    try {
      await registerMutation.mutateAsync({ email, password, name })
      navigate(RouterPage.LOGIN)
    } catch {
      setFailOpen(true)
    }
  })

  return (
    <div className="flex w-full flex-col gap-4">
      <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
        <div className="flex flex-col gap-2">
          <Label htmlFor="reg-name">{t('modules.user.form.user.name.label')}</Label>
          <Input
            id="reg-name"
            placeholder={t('modules.user.form.user.name.placeholder')}
            {...register('name', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.name.validation.required_field'),
                name: (v) => isValidName(v) || t('modules.user.form.user.name.validation.invalid_name'),
              },
            })}
          />
          {errors.name ? <p className="text-sm text-destructive">{errors.name.message}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="reg-email">{t('modules.user.form.user.email.label')}</Label>
          <Input
            id="reg-email"
            type="email"
            placeholder={t('modules.user.form.user.email.placeholder')}
            {...register('email', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.email.validation.required_field'),
                email: (v) => isValidEmail(v) || t('modules.user.form.user.email.validation.invalid_email'),
              },
            })}
          />
          {errors.email ? <p className="text-sm text-destructive">{errors.email.message}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="reg-password">{t('modules.user.form.user.password.label')}</Label>
          <Input
            id="reg-password"
            type="password"
            placeholder={t('modules.user.form.user.password.placeholder')}
            {...register('password', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.password.validation.required_field'),
                password: (v) => isValidPassword(v) || t('modules.user.form.user.password.validation.invalid_password'),
              },
            })}
          />
          {errors.password ? <p className="text-sm text-destructive">{errors.password.message}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="reg-password-retry">{t('modules.user.form.user.password_retry.label')}</Label>
          <Input
            id="reg-password-retry"
            type="password"
            placeholder={t('modules.user.form.user.password_retry.placeholder')}
            {...register('passwordRetry', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.password_retry.validation.invalid_password'),
                equals: (v, values) => v === values.password || t('modules.user.form.user.password_retry.validation.not_equals'),
              },
            })}
          />
          {errors.passwordRetry ? <p className="text-sm text-destructive">{errors.passwordRetry.message}</p> : null}
        </div>

        {customApiAllowed ? (
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2">
              <Controller
                control={control}
                name="selfHosted"
                render={({ field }) => (
                  <Checkbox id="reg-self-hosted" checked={field.value} onCheckedChange={field.onChange} />
                )}
              />
              <Label htmlFor="reg-self-hosted">{t('modules.user.form.user.server_host.self_hosted')}</Label>
              <button
                type="button"
                className="text-sm text-muted-foreground underline"
                onClick={() => setInfoOpen(true)}
                aria-label={t('modules.app.modal.self_hosted.information')}
              >
                ?
              </button>
            </div>
            {selfHostedChecked ? (
              <div className="flex flex-col gap-2">
                <Label htmlFor="reg-host">{t('modules.user.form.user.server_host.label')}</Label>
                <Input
                  id="reg-host"
                  type="url"
                  placeholder={t('modules.user.form.user.server_host.placeholder')}
                  {...register('host', {
                    validate: {
                      required: (v) => isNotEmpty(v) || t('modules.user.form.user.server_host.validation.required_field'),
                      url: (v) => isValidHttpUrl(v) || t('modules.user.form.user.server_host.validation.invalid_url'),
                    },
                  })}
                />
                {errors.host ? <p className="text-sm text-destructive">{errors.host.message}</p> : null}
              </div>
            ) : null}
          </div>
        ) : null}

        <Button type="submit" className="w-full" disabled={registerMutation.isPending}>
          {t('modules.user.form.sign_up.action.sign_up')}
        </Button>

        <div className="text-xs text-muted-foreground" dangerouslySetInnerHTML={{ __html: t('modules.user.page.sign_up.privacy.text') }} />
      </form>

      <FailDialog
        open={failOpen}
        onClose={() => setFailOpen(false)}
        title={t('modules.user.modal.sign_up_failed.header')}
        description={t('modules.user.modal.sign_up_failed.information')}
      />
      <SelfHostedInfoDialog open={infoOpen} onClose={() => setInfoOpen(false)} />
    </div>
  )
}
```

In `routes.tsx` swap `/register` to `<RegistrationPage />`.

- [x] **Step 3: Run tests**

Run: `pnpm vitest run src/features/auth/RegistrationPage.test.tsx` — PASS. `pnpm test` — all pass. `pnpm build` — succeeds.

- [x] **Step 4: Commit**

```bash
git add web-react/src/features/auth/RegistrationPage.tsx web-react/src/features/auth/RegistrationPage.test.tsx web-react/src/app/routes.tsx
git commit -m "feat(web-react): registration page with paywall branch"
```

---

### Task 16: Logout page (TDD)

**Files:**
- Create: `web-react/src/features/auth/LogoutPage.tsx`
- Modify: `web-react/src/app/routes.tsx` (swap `/logout` element)
- Test: `web-react/src/features/auth/LogoutPage.test.tsx`

**Interfaces:**
- Consumes: `logout` from `@/api/user`, `removeToken`, `hasToken`.
- Produces: `LogoutPage` — on mount: if a token exists, fire `logout()` (ignore any error), always `removeToken()`, then `window.location.assign('/login')` (full reload clears all in-memory query caches).

- [x] **Step 1: Write failing test**

Create `web-react/src/features/auth/LogoutPage.test.tsx`:

```tsx
import { render } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { setToken } from '@/lib/storage'
import { LogoutPage } from './LogoutPage'

it('calls logout, purges the token and redirects to /login', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  let called = false
  server.use(
    http.post('*/api/v1/user/logout-user', () => {
      called = true
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  setToken('tok')
  const router = createMemoryRouter([{ path: '/logout', element: <LogoutPage /> }], { initialEntries: ['/logout'] })
  render(<RouterProvider router={router} />)
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
  expect(called).toBe(true)
  expect(localStorage.getItem('token')).toBeNull()
})
```

Run: `pnpm vitest run src/features/auth/LogoutPage.test.tsx` — FAIL.

- [x] **Step 2: Implement**

Create `web-react/src/features/auth/LogoutPage.tsx`:

```tsx
import { useEffect } from 'react'
import { logout } from '@/api/user'
import { hasToken, removeToken } from '@/lib/storage'

export function LogoutPage() {
  useEffect(() => {
    const run = async () => {
      if (hasToken()) {
        try {
          await logout()
        } catch {
          // best effort; the token is purged regardless
        }
      }
      removeToken()
      window.location.assign('/login')
    }
    void run()
  }, [])
  return null
}
```

In `routes.tsx` swap `/logout` to `<LogoutPage />`.

- [x] **Step 3: Run tests**

Run: `pnpm vitest run src/features/auth/LogoutPage.test.tsx` — PASS.

- [x] **Step 4: Commit**

```bash
git add web-react/src/features/auth/LogoutPage.tsx web-react/src/features/auth/LogoutPage.test.tsx web-react/src/app/routes.tsx
git commit -m "feat(web-react): logout page"
```

---

### Task 17: Make targets for web-react

**Files:**
- Modify: `Makefile` (repo root)

**Interfaces:**
- Produces: `make web-react-install`, `web-react-dev`, `web-react-test`, `web-react-lint`, `web-react-bundle` — mirroring the existing `web-*` targets' style (look at how `web-install`/`web-dev`/`web-bundle`/`web-lint` are written in the root `Makefile` and copy that pattern with `cd web-react`).

- [x] **Step 1: Add targets**

Append to the root `Makefile`, matching the existing `web-*` targets' formatting:

```makefile
web-react-install:
	cd web-react && pnpm install

web-react-dev:
	cd web-react && pnpm dev

web-react-test:
	cd web-react && pnpm test

web-react-lint:
	cd web-react && pnpm lint

web-react-bundle:
	cd web-react && pnpm build
```

Also add the new target names to the `.PHONY` line if the Makefile declares one.

- [x] **Step 2: Verify**

Run from repo root: `make web-react-test` → all tests pass; `make web-react-lint` → clean; `make web-react-bundle` → builds.

- [x] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore(web-react): make targets for install/dev/test/lint/bundle"
```

---

### Task 18: Auth parity check (manual, gate for Plan 1)

**Files:** none (verification only; fix any divergence found, with a test, before closing this task)

- [x] **Step 1: Run all three apps**

```bash
# terminal 1 — Go backend (ensure .env has PORT=8181 and a DATABASE_URL)
make go-run
# terminal 2 — Vue app
make web-dev
# terminal 3 — React app
make web-react-dev
```

Create a test user if needed: `go run ./cmd/econumo user:create "Parity Tester" parity@example.test secret123`.

- [x] **Step 2: Walk every auth flow in BOTH apps, at three widths**

Use browser devtools responsive mode at 1280px (desktop), 820px (tablet), 375px (mobile). For each flow confirm the React app matches the Vue app (same data, same validation messages, same navigation):

1. Login with valid credentials → lands on `/`.
2. Login with wrong password → failure dialog with the same title/text.
3. Field validation: empty email, invalid email, empty password — same messages, shown on blur/submit.
4. Forgot password → recovery dialog: send code (backend logs the email to stdout with the default console mailer — copy the 12-char code), enter code + new password → dialog closes; login with the new password works.
5. Registration: name/email/password/retry validation (incl. mismatch message), successful registration → lands on login; then `ECONUMO_ALLOW_REGISTRATION=false` → register tab disabled in both apps.
6. Self-hosted section: with `ALLOW_CUSTOM_API` false in `public/econumo-config.js` the checkbox is absent; with `'true'` the checkbox + host field appear and persist across reloads.
7. Logout → back to login; token gone from devtools storage.
8. React-only (approved divergence): with a logged-in session, hand-edit the stored token to an expired one (or wait) and navigate — redirected to `/login?reason=expired` with the visible notice.
9. Deep link while logged out (`/settings`) → redirected to login in both apps.

- [x] **Step 3: Record the result**

Note any divergences found and fixed in the final commit message. When the checklist is clean:

```bash
git commit --allow-empty -m "chore(web-react): auth parity check vs Vue app passed (desktop/tablet/mobile)"
```

---

## Plan sequence

This is Plan 1 of 6. Subsequent plans (written after each phase completes, against the then-current code):

2. App shell + accounts (sidebar, folders, account page, transactions, transaction modal, drag-reorder)
3. Shared widgets it depends on (calculator input, month picker, currency select) — folded into Plan 2/3 task lists as needed
4. Settings cluster (profile, change-password, accounts, categories, payees, tags, budgets list)
5. Budget (page split, envelopes, limits, widgets, modals)
6. Connections + onboarding + CSV import/export + the `web/` → swap commit (Makefile, Dockerfile, delete Vue app)
