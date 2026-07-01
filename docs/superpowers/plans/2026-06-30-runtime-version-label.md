# Runtime-Configurable UI Version Label Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the SPA's displayed version label be overridden at runtime via `econumo-config.js`, with a build-time fallback so the default build is unchanged.

**Architecture:** Reuse the existing `window.econumoConfig` runtime-config global (set by the static, unbundled `web/public/econumo-config.js`, loaded before app boot). `config.ts` gains a `getVersion()` that prefers `window.econumoConfig.VERSION` and falls back to the build-time `process.env.ECONUMO_VERSION`; `package.ts` uses it.

**Tech Stack:** Vue 3 + Quasar 2 + TypeScript (`web/`). No backend/Go changes.

## Global Constraints

- Frontend has **no JS test framework** (`npm test` is a no-op stub). Verification is ESLint (`make web-lint`) + production build (`make web-bundle`) + manual label check. Do not add a test runner.
- Default build behaviour must stay byte-identical when `VERSION` is absent/`null`/empty: build-time `ECONUMO_VERSION` still wins.
- The `--build-arg ECONUMO_VERSION` Docker path stays unchanged (no Dockerfile edits).
- Precedence, exact expression: `window.econumoConfig?.VERSION || process.env.ECONUMO_VERSION`.

---

### Task 1: Runtime version override via econumo-config.js

**Files:**
- Modify: `web/src/modules/config.ts` (add `VERSION?` to `EconumoConfig` interface; add + export `getVersion()`; add to default export object)
- Modify: `web/src/modules/package.ts` (`getEditionLabel()` calls `config.getVersion()`)
- Modify: `web/public/econumo-config.js` (add `VERSION: null` placeholder)

**Interfaces:**
- Consumes: existing `window.econumoConfig` global and `process.env.ECONUMO_VERSION` (Quasar DefinePlugin constant).
- Produces: `config.getVersion(): string` — returns the effective version label. Consumed by `package.ts`.

- [ ] **Step 1: Add `VERSION` to the config interface**

In `web/src/modules/config.ts`, add the field to the `EconumoConfig` interface:

```typescript
interface EconumoConfig {
  API_URL?: string
  ALLOW_REGISTRATION?: boolean | string
  PAYWALL_ENABLED?: boolean | string
  VERSION?: string
}
```

- [ ] **Step 2: Add and export `getVersion()`**

In `web/src/modules/config.ts`, add this exported function (place it next to `getWebsiteUrl`, which already reads `process.env`):

```typescript
export function getVersion(): string {
  return window.econumoConfig?.VERSION || String(process.env.ECONUMO_VERSION)
}
```

Then add it to the default export object at the bottom of the file:

```typescript
export default {
  selfHosted,
  backendHost,
  isHttps,
  locale,
  getLocaleOptions,
  getWebsiteUrl,
  getVersion,
  isCustomApiAllowed,
  isRegistrationAllowed,
  isPaywallEnabled
}
```

- [ ] **Step 3: Use `getVersion()` in package.ts**

In `web/src/modules/package.ts`, import the named function and use it. The existing import is `import { isPaywallEnabled as isPaywallEnabledConfig } from './config'` — extend it:

```typescript
import { isPaywallEnabled as isPaywallEnabledConfig, getVersion } from './config';
```

Change `getEditionLabel()` from reading `process.env` directly to:

```typescript
function getEditionLabel(): string {
  return getVersion();
}
```

- [ ] **Step 4: Add the `VERSION` placeholder to the runtime config file**

In `web/public/econumo-config.js`, add the field (`null` = fall back to build-time value):

```javascript
window.econumoConfig = {
  API_URL: null,
  LILTAG_CONFIG_URL: 'liltag-config.json',
  LILTAG_CACHE_TTL: 0,
  ALLOW_REGISTRATION: true,
  PAYWALL_ENABLED: false,
  // Override the UI version label at runtime without a rebuild.
  // null / absent => the build-time ECONUMO_VERSION is used.
  VERSION: null
};
```

- [ ] **Step 5: Lint**

Run: `make web-lint`
Expected: PASS (no new ESLint errors).

- [ ] **Step 6: Build the SPA**

Run: `make web-bundle`
Expected: build succeeds; `web/dist/spa/econumo-config.js` exists and contains `VERSION`.

- [ ] **Step 7: Manual verification — runtime override wins**

Edit the built `web/dist/spa/econumo-config.js`, set `VERSION:"demo-override"`, then serve the built SPA (via `make go-run` with `ECONUMO_WEB_DIST` pointing at `web/dist/spa`, or `go run ./cmd/econumo serve`) and hard-refresh. Confirm the version shown in the UI (Settings page footer / login layout) reads `demo-override`.

- [ ] **Step 8: Manual verification — fallback still works**

Set `VERSION:null` (or remove it) in the built `web/dist/spa/econumo-config.js`, hard-refresh, and confirm the label falls back to the build-time `ECONUMO_VERSION` (e.g. `dev`).

- [ ] **Step 9: Commit**

```bash
git add web/src/modules/config.ts web/src/modules/package.ts web/public/econumo-config.js
git commit -m "feat(web): allow runtime version label override via econumo-config.js"
```

---

## Self-Review

- **Spec coverage:** config.ts interface + `getVersion()` (spec change 1) ✓; package.ts uses it (change 2) ✓; `econumo-config.js` placeholder (change 3) ✓; precedence expression verbatim ✓; build-arg/Dockerfile untouched ✓; caching caveat noted as out-of-scope in spec (no task, intentional) ✓.
- **Placeholder scan:** none — every code step shows full content.
- **Type consistency:** `getVersion()` name/signature identical across config.ts definition, default-export entry, and package.ts import/usage.
