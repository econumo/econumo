# Mobile App Wrapper (Capacitor) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Package the existing React SPA as iOS + Android apps via a Capacitor shell in a new `mobile/` directory, per `docs/superpowers/specs/2026-08-03-mobile-app-wrapper-design.md`.

**Architecture:** `web/` stays the single frontend; every app-specific behavior branches on one `isNativeApp()` helper that probes the `window.Capacitor` global (zero new dependencies in `web/`). The Capacitor project in `mobile/` bundles `web/dist` and supplies the native plugins; web code reaches them through the injected `window.Capacitor.Plugins` bridge via a typed accessor.

**Tech Stack:** Capacitor (latest, via pnpm) + plugins (preferences, filesystem, share, browser, app, status-bar, splash-screen); React 19 / Vite / vitest in `web/` unchanged.

## Global Constraints

- **No new runtime dependencies in `web/package.json`.** Native plugins are reached through `window.Capacitor.Plugins`; TypeScript interfaces for them are declared locally where used.
- **Web build behavior must be unchanged** — every app-mode branch is dead code when `window.Capacitor` is absent. The full existing suite (`make web-test`, `make web-lint`) must pass untouched except where a task explicitly modifies a test.
- Cloud default host: exactly `https://app.econumo.com`.
- App id: `com.econumo.app`; app name: `Econumo`.
- Runtime server-config merge allowlist: exactly `ALLOW_REGISTRATION` and `ANALYTICS` into `window.econumoConfig`; the server's `VERSION` goes to a separate store, never into `window.econumoConfig`.
- Minimum supported server version constant: `v1.3.0`.
- Package manager is pnpm everywhere. Work happens on branch `feature/mobile-app-wrapper`. Commit messages follow the repo style (`feat:`, `docs:`, `chore:` prefixes, lowercase summary).
- New translatable strings must be added to **all 11** catalogues (`locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json`) with identical `{var}` placeholder sets (enforced by `go test ./internal/test/i18ntest/`).

---

### Task 1: Platform detection (`platform.ts`)

**Files:**
- Create: `web/src/lib/platform.ts`
- Test: `web/src/lib/platform.test.ts`

**Interfaces:**
- Produces: `isNativeApp(): boolean`; `nativePlugin<T>(name: string): T | null` — used by every later task.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/platform.test.ts
import { afterEach, expect, it } from 'vitest'
import { isNativeApp, nativePlugin } from './platform'

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
})

it('is not native without the Capacitor global', () => {
  expect(isNativeApp()).toBe(false)
  expect(nativePlugin('Preferences')).toBeNull()
})

it('is native when Capacitor reports a native platform', () => {
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Preferences: { get: () => {} } } }
  expect(isNativeApp()).toBe(true)
  expect(nativePlugin('Preferences')).not.toBeNull()
  expect(nativePlugin('Missing')).toBeNull()
})

it('is not native for the Capacitor web runtime', () => {
  window.Capacitor = { isNativePlatform: () => false, Plugins: {} }
  expect(isNativeApp()).toBe(false)
  expect(nativePlugin('Preferences')).toBeNull()
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/lib/platform.test.ts`
Expected: FAIL — `Cannot find module './platform'`.

- [ ] **Step 3: Write the implementation**

```ts
// web/src/lib/platform.ts
// The Capacitor runtime injects window.Capacitor into the WebView; probing it
// keeps web/ free of any Capacitor npm dependency. Plugin call surfaces are
// typed locally at each call site.
interface CapacitorGlobal {
  isNativePlatform?: () => boolean
  Plugins?: Record<string, unknown>
}

declare global {
  interface Window {
    Capacitor?: CapacitorGlobal
  }
}

export function isNativeApp(): boolean {
  return window.Capacitor?.isNativePlatform?.() === true
}

export function nativePlugin<T>(name: string): T | null {
  if (!isNativeApp()) {
    return null
  }
  return (window.Capacitor?.Plugins?.[name] as T | undefined) ?? null
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/lib/platform.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/platform.ts web/src/lib/platform.test.ts
git commit -m "feat: app-mode platform detection via the Capacitor global"
```

---

### Task 2: App-mode backend connection defaults (`config.ts`)

**Files:**
- Modify: `web/src/lib/config.ts` (functions `isCustomApiAllowed`, `backendHost`)
- Test: `web/src/lib/config.test.ts` (append)

**Interfaces:**
- Consumes: `isNativeApp()` from Task 1.
- Produces: unchanged signatures; new behavior — in app mode `isCustomApiAllowed()` is always `true` and `backendHost()` defaults to `https://app.econumo.com`.

- [ ] **Step 1: Write the failing tests** (append to `web/src/lib/config.test.ts`; mirror the file's existing setup/teardown style)

```ts
describe('app mode', () => {
  afterEach(() => {
    delete (window as { Capacitor?: unknown }).Capacitor
    localStorage.clear()
  })

  it('always allows the custom API in the native app', () => {
    window.econumoConfig = { ALLOW_CUSTOM_API: false }
    window.Capacitor = { isNativePlatform: () => true }
    expect(isCustomApiAllowed()).toBe(true)
  })

  it('defaults backendHost to Econumo Cloud in the native app', () => {
    window.econumoConfig = {}
    window.Capacitor = { isNativePlatform: () => true }
    expect(backendHost()).toBe('https://app.econumo.com')
  })

  it('keeps a stored self-hosted backendHost in the native app', () => {
    window.econumoConfig = {}
    window.Capacitor = { isNativePlatform: () => true }
    selfHosted(true)
    backendHost('https://my.server.example')
    expect(backendHost()).toBe('https://my.server.example')
  })
})
```

- [ ] **Step 2: Run tests to verify the new ones fail**

Run: `cd web && pnpm vitest run src/lib/config.test.ts`
Expected: the three new tests FAIL (custom API false, host = `http://localhost:9000`); all pre-existing tests still PASS.

- [ ] **Step 3: Implement**

In `web/src/lib/config.ts`, add the import and a constant, then edit the two functions:

```ts
import { isNativeApp } from './platform'

const CLOUD_HOST = 'https://app.econumo.com'
```

`isCustomApiAllowed` — add as the first statement:

```ts
export function isCustomApiAllowed(): boolean {
  if (isNativeApp()) {
    return true
  }
  const allowCustomApi = window.econumoConfig?.ALLOW_CUSTOM_API
  // ...rest unchanged
```

`backendHost` — replace `const defaultHost = window.location.origin` with:

```ts
    // Inside the WebView the origin is capacitor://localhost — meaningless as
    // an API host, so the app defaults to Econumo Cloud instead.
    const defaultHost = isNativeApp() ? CLOUD_HOST : window.location.origin
```

(The `!isCustomApiAllowed()` branch above it is unreachable in app mode and stays as is.)

- [ ] **Step 4: Run the whole lib suite**

Run: `cd web && pnpm vitest run src/lib/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/config.ts web/src/lib/config.test.ts
git commit -m "feat: cloud-default backend host and forced custom API in app mode"
```

---

### Task 3: Durable credential storage (Preferences mirror)

**Files:**
- Create: `web/src/lib/appStorage.ts`
- Modify: `web/src/lib/storage.ts` (all four write functions)
- Test: `web/src/lib/appStorage.test.ts`

**Interfaces:**
- Consumes: `isNativeApp`, `nativePlugin` (Task 1).
- Produces: `mirrorWrite(key: string, value: string | null): void`; `restoreNativeStorage(): Promise<void>` (called by Task 5's boot).

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/appStorage.test.ts
import { afterEach, expect, it, vi } from 'vitest'
import { mirrorWrite, restoreNativeStorage } from './appStorage'

function installPrefs(store: Map<string, string>) {
  const prefs = {
    get: vi.fn(async ({ key }: { key: string }) => ({ value: store.get(key) ?? null })),
    set: vi.fn(async ({ key, value }: { key: string; value: string }) => {
      store.set(key, value)
    }),
    remove: vi.fn(async ({ key }: { key: string }) => {
      store.delete(key)
    }),
  }
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Preferences: prefs } }
  return prefs
}

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  localStorage.clear()
})

it('mirrors writes and removals of mirrored keys to Preferences', () => {
  const store = new Map<string, string>()
  const prefs = installPrefs(store)
  mirrorWrite('token', 'eco_ses_abc')
  mirrorWrite('token', null)
  expect(prefs.set).toHaveBeenCalledWith({ key: 'token', value: 'eco_ses_abc' })
  expect(prefs.remove).toHaveBeenCalledWith({ key: 'token' })
})

it('ignores non-mirrored keys and does nothing on the web', () => {
  const store = new Map<string, string>()
  const prefs = installPrefs(store)
  mirrorWrite('locale', 'en')
  expect(prefs.set).not.toHaveBeenCalled()
  delete (window as { Capacitor?: unknown }).Capacitor
  mirrorWrite('token', 't') // must not throw
})

it('restores lost keys from Preferences without clobbering live ones', async () => {
  const store = new Map([
    ['token', 'eco_ses_native'],
    ['backendHost', '"https://my.server.example"'],
  ])
  installPrefs(store)
  localStorage.setItem('token', 'eco_ses_live')
  await restoreNativeStorage()
  expect(localStorage.getItem('token')).toBe('eco_ses_live')
  expect(localStorage.getItem('backendHost')).toBe('"https://my.server.example"')
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/lib/appStorage.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `appStorage.ts`**

```ts
// web/src/lib/appStorage.ts
import { isNativeApp, nativePlugin } from './platform'

interface PreferencesPlugin {
  get(o: { key: string }): Promise<{ value: string | null }>
  set(o: { key: string; value: string }): Promise<void>
  remove(o: { key: string }): Promise<void>
}

// WKWebView may evict localStorage under storage pressure; these are the keys
// whose loss logs the user out or strands them on the wrong server. Values are
// mirrored verbatim (storage.ts JSON-encodes non-token keys), so restore is a
// straight copy back.
const MIRRORED_KEYS: readonly string[] = ['token', 'backendHost', 'selfHosted']

function prefs(): PreferencesPlugin | null {
  return nativePlugin<PreferencesPlugin>('Preferences')
}

export function mirrorWrite(key: string, value: string | null): void {
  if (!MIRRORED_KEYS.includes(key)) {
    return
  }
  const p = prefs()
  if (!p) {
    return
  }
  void (value === null ? p.remove({ key }) : p.set({ key, value }))
}

// Runs before first render (the splash screen covers it). localStorage wins
// when it still has the key — the native copy only fills eviction holes.
export async function restoreNativeStorage(): Promise<void> {
  if (!isNativeApp()) {
    return
  }
  const p = prefs()
  if (!p) {
    return
  }
  for (const key of MIRRORED_KEYS) {
    if (localStorage.getItem(key) !== null) {
      continue
    }
    const { value } = await p.get({ key })
    if (value !== null) {
      localStorage.setItem(key, value)
    }
  }
}
```

- [ ] **Step 4: Wire `storage.ts`**

Add `import { mirrorWrite } from './appStorage'` and one line to each write path:

```ts
export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
  mirrorWrite(TOKEN_KEY, token)
}

export function removeToken(): void {
  localStorage.removeItem(TOKEN_KEY)
  mirrorWrite(TOKEN_KEY, null)
}

export function setItem(key: string, value: unknown): void {
  const encoded = JSON.stringify(value)
  localStorage.setItem(key, encoded)
  mirrorWrite(key, encoded)
}

export function removeItem(key: string): void {
  localStorage.removeItem(key)
  mirrorWrite(key, null)
}
```

- [ ] **Step 5: Run the suite**

Run: `cd web && pnpm vitest run src/lib/`
Expected: PASS (mirroring is a no-op in every existing test — no Capacitor global).

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/appStorage.ts web/src/lib/appStorage.test.ts web/src/lib/storage.ts
git commit -m "feat: mirror credentials to native Preferences in app mode"
```

---

### Task 4: Runtime server config (`appConfig.ts`)

**Files:**
- Create: `web/src/lib/appConfig.ts`
- Test: `web/src/lib/appConfig.test.ts`

**Interfaces:**
- Consumes: `backendHost` (Task 2).
- Produces: `fetchServerConfig(): Promise<void>`; `evalConfigScript(text: string): Record<string, unknown> | null`; `useServerConfig` (zustand store, state `{ serverVersion: string | null }`); `MIN_SERVER_VERSION = 'v1.3.0'`. Used by Tasks 5 and 11.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/appConfig.test.ts
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { evalConfigScript, fetchServerConfig, useServerConfig } from './appConfig'

beforeEach(() => {
  window.econumoConfig = { ALLOW_REGISTRATION: true, ANALYTICS: true, BILLING_URL: '' }
  useServerConfig.setState({ serverVersion: null })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const SERVED = `window.econumoConfig = {
  ALLOW_REGISTRATION: true,
  ANALYTICS: true,
  VERSION: null,
};
Object.assign(window.econumoConfig, {"ALLOW_REGISTRATION":false,"ANALYTICS":false,"VERSION":"v1.4.2","BILLING_URL":"https://x"});
`

it('evaluates the served config script including the server suffix', () => {
  expect(evalConfigScript(SERVED)).toMatchObject({
    ALLOW_REGISTRATION: false,
    ANALYTICS: false,
    VERSION: 'v1.4.2',
  })
  expect(evalConfigScript('not js {')).toBeNull()
})

it('merges only the allowlist and stores the server version separately', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(SERVED, { status: 200 })))
  await fetchServerConfig()
  expect(window.econumoConfig.ALLOW_REGISTRATION).toBe(false)
  expect(window.econumoConfig.ANALYTICS).toBe(false)
  expect(window.econumoConfig.BILLING_URL).toBe('') // not on the allowlist
  expect(window.econumoConfig.VERSION).toBeUndefined() // never merged
  expect(useServerConfig.getState().serverVersion).toBe('v1.4.2')
})

it('is non-fatal on network failure', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => {
    throw new Error('offline')
  }))
  await fetchServerConfig()
  expect(window.econumoConfig.ALLOW_REGISTRATION).toBe(true)
  expect(useServerConfig.getState().serverVersion).toBeNull()
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/lib/appConfig.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
// web/src/lib/appConfig.ts
import { create } from 'zustand'
import { backendHost } from './config'

// The app bundles a static econumo-config.js; the user's server holds the
// instance truth. Only these keys may cross over — everything else keeps its
// bundled default by design.
const MERGED_KEYS = ['ALLOW_REGISTRATION', 'ANALYTICS'] as const

// Minimum server version the bundled SPA is developed against; older servers
// get a warning banner (never a hard block). Documented in mobile/README.md.
export const MIN_SERVER_VERSION = 'v1.3.0'

export const useServerConfig = create<{ serverVersion: string | null }>(() => ({
  serverVersion: null,
}))

// The served file is executable JS (`window.econumoConfig = {...}` plus an
// Object.assign suffix the server appends), not JSON — run it against a stub
// window instead of parsing it.
export function evalConfigScript(text: string): Record<string, unknown> | null {
  try {
    const stub: { econumoConfig: Record<string, unknown> } = { econumoConfig: {} }
    new Function('window', text)(stub)
    return stub.econumoConfig
  } catch {
    return null
  }
}

export async function fetchServerConfig(): Promise<void> {
  try {
    const res = await fetch(`${backendHost()}/econumo-config.js`, { cache: 'no-store' })
    if (!res.ok) {
      return
    }
    const cfg = evalConfigScript(await res.text())
    if (!cfg) {
      return
    }
    const target = window.econumoConfig as Record<string, unknown>
    for (const key of MERGED_KEYS) {
      if (key in cfg) {
        target[key] = cfg[key]
      }
    }
    useServerConfig.setState({
      serverVersion: typeof cfg.VERSION === 'string' ? cfg.VERSION : null,
    })
  } catch {
    // Non-fatal: offline boot keeps bundled defaults and cached data.
  }
}
```

- [ ] **Step 4: Run test + lint** (oxlint may flag `new Function`; if so, add `// oxlint-disable-next-line` with the reported rule name on that line)

Run: `cd web && pnpm vitest run src/lib/appConfig.test.ts && pnpm lint`
Expected: PASS, no new lint errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/appConfig.ts web/src/lib/appConfig.test.ts
git commit -m "feat: fetch server-owned config at runtime in app mode"
```

---

### Task 5: App boot orchestration (`appBoot.ts` + `main.tsx`)

**Files:**
- Create: `web/src/lib/appBoot.ts`
- Modify: `web/src/main.tsx`
- Test: `web/src/lib/appBoot.test.ts`

**Interfaces:**
- Consumes: `restoreNativeStorage` (Task 3), `fetchServerConfig` (Task 4), `nativePlugin`/`isNativeApp` (Task 1).
- Produces: `bootNativeApp(): Promise<void>` (storage restore, then fire-and-forget config fetch); `hideSplash(): void`. Task 8 and Task 10 add their installers next to these.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/appBoot.test.ts
import { afterEach, expect, it, vi } from 'vitest'
import { bootNativeApp, hideSplash } from './appBoot'

vi.mock('./appStorage', () => ({ restoreNativeStorage: vi.fn(async () => {}) }))
vi.mock('./appConfig', () => ({ fetchServerConfig: vi.fn(async () => {}) }))
import { restoreNativeStorage } from './appStorage'
import { fetchServerConfig } from './appConfig'

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  vi.clearAllMocks()
})

it('is a no-op on the web', async () => {
  await bootNativeApp()
  hideSplash()
  expect(restoreNativeStorage).not.toHaveBeenCalled()
  expect(fetchServerConfig).not.toHaveBeenCalled()
})

it('restores storage then fetches server config in app mode', async () => {
  const hide = vi.fn(async () => {})
  window.Capacitor = { isNativePlatform: () => true, Plugins: { SplashScreen: { hide } } }
  await bootNativeApp()
  expect(restoreNativeStorage).toHaveBeenCalledOnce()
  expect(fetchServerConfig).toHaveBeenCalledOnce()
  hideSplash()
  expect(hide).toHaveBeenCalledOnce()
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/lib/appBoot.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
// web/src/lib/appBoot.ts
import { isNativeApp, nativePlugin } from './platform'
import { restoreNativeStorage } from './appStorage'
import { fetchServerConfig } from './appConfig'

interface SplashScreenPlugin {
  hide(): Promise<void>
}

// Storage restore must finish before the first render (auth state reads the
// token synchronously); the config fetch must NOT block first paint.
export async function bootNativeApp(): Promise<void> {
  if (!isNativeApp()) {
    return
  }
  await restoreNativeStorage()
  void fetchServerConfig()
}

export function hideSplash(): void {
  void nativePlugin<SplashScreenPlugin>('SplashScreen')?.hide()
}
```

- [ ] **Step 4: Wire `main.tsx`** — wrap the existing render call:

```tsx
import { bootNativeApp, hideSplash } from '@/lib/appBoot'

void bootNativeApp().then(() => {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <PersistQueryClientProvider
        client={queryClient}
        persistOptions={createPersistOptions()}
        onSuccess={() => refreshRestoredQueries(queryClient)}
      >
        <RouterProvider router={createRouter()} />
      </PersistQueryClientProvider>
    </StrictMode>,
  )
  hideSplash()
})
```

- [ ] **Step 5: Run the full web suite**

Run: `cd web && pnpm test && pnpm lint`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/appBoot.ts web/src/lib/appBoot.test.ts web/src/main.tsx
git commit -m "feat: native app boot sequence (storage restore, config fetch, splash)"
```

---

### Task 6: Capacitor scaffold (`mobile/`) + Makefile targets

**Files:**
- Create: `mobile/package.json`, `mobile/capacitor.config.ts`, `mobile/.gitignore`, `mobile/README.md`
- Create (generated): `mobile/ios/`, `mobile/android/`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `web/dist` (any prior build).
- Produces: a syncable Capacitor project; `make mobile-install | mobile-sync | mobile-ios | mobile-android`.

- [ ] **Step 1: Create the project**

```bash
mkdir mobile && cd mobile
pnpm init
pnpm add @capacitor/core @capacitor/preferences @capacitor/filesystem @capacitor/share @capacitor/browser @capacitor/app @capacitor/status-bar @capacitor/splash-screen
pnpm add -D @capacitor/cli
pnpm add @capacitor/ios @capacitor/android
```

- [ ] **Step 2: Write `mobile/capacitor.config.ts`**

```ts
import type { CapacitorConfig } from '@capacitor/cli'

const config: CapacitorConfig = {
  appId: 'com.econumo.app',
  appName: 'Econumo',
  webDir: '../web/dist',
  plugins: {
    // Route fetch/XHR through the native HTTP stack: requests carry no browser
    // origin, so CORS never applies and self-hosted servers need zero config.
    CapacitorHttp: { enabled: true },
    // appBoot.hideSplash() dismisses it after the first render.
    SplashScreen: { launchAutoHide: false },
  },
}

export default config
```

- [ ] **Step 3: Build the SPA and generate the native projects**

```bash
cd web && pnpm build && cd ../mobile
pnpm exec cap add ios
pnpm exec cap add android
pnpm exec cap sync
```

Expected: both `cap add` commands create their directories and `cap sync` reports all eight plugins found for both platforms. (If `cap add ios` warns that CocoaPods is missing, install it or note the warning — the project is still generated; `cap sync` completes pod install when CocoaPods is available.)

- [ ] **Step 4: Write `mobile/.gitignore`** (Capacitor's `cap add` also drops platform-level .gitignore files — keep those)

```
node_modules/
ios/App/Pods/
ios/App/output/
ios/DerivedData/
android/.gradle/
android/app/build/
android/build/
```

- [ ] **Step 5: Write `mobile/README.md`**

```markdown
# Econumo mobile (Capacitor)

A thin native shell around the web SPA (`web/`). See
`docs/superpowers/specs/2026-08-03-mobile-app-wrapper-design.md`.

## Build

    make mobile-install   # once: pnpm install here
    make mobile-sync      # build web/ and copy into both native projects
    make mobile-ios       # sync + open in Xcode
    make mobile-android   # sync + open in Android Studio

Release builds label the UI with the app version:

    ECONUMO_VERSION=v1.0.0 make mobile-sync

## Minimum supported server version

**v1.3.0** — mirrored by `MIN_SERVER_VERSION` in `web/src/lib/appConfig.ts`.
Older servers show a non-blocking warning banner in the app. Bump both
together when the bundled SPA starts depending on newer API behavior.
```

- [ ] **Step 6: Add Makefile targets** (append near the `web-*` targets; add all four to `.PHONY`)

```makefile
mobile-install:
	cd mobile && pnpm install

mobile-sync:
	cd web && pnpm build
	cd mobile && pnpm exec cap sync

mobile-ios: mobile-sync
	cd mobile && pnpm exec cap open ios

mobile-android: mobile-sync
	cd mobile && pnpm exec cap open android
```

- [ ] **Step 7: Verify**

Run: `make mobile-sync`
Expected: web build succeeds, `cap sync` completes for ios and android.

Manual checkpoint (pause here for human verification): `make mobile-ios`, run on an iPhone simulator — the app boots to the login page, the custom-server section is present, and logging into a cloud/test account works.

- [ ] **Step 8: Commit**

```bash
git add mobile Makefile
git commit -m "feat: Capacitor mobile project scaffold with iOS and Android shells"
```

---

### Task 7: 401 redirect via router navigation

**Files:**
- Create: `web/src/app/routerRef.ts`
- Modify: `web/src/main.tsx`, `web/src/api/client.ts:31`
- Test: `web/src/api/client.test.ts` (append one test)

**Interfaces:**
- Consumes: nothing app-mode-specific (improves web too).
- Produces: `setRouter(r: ReturnType<typeof createBrowserRouter> | null): void`; `navigateTo(path: string): void` (falls back to `window.location.assign` when no router is registered).

- [ ] **Step 1: Write the failing test** (append to `web/src/api/client.test.ts`)

```ts
it('on 401 navigates via the registered router instead of a page load', async () => {
  const navigate = vi.fn(async () => {})
  setRouter({ navigate } as unknown as Parameters<typeof setRouter>[0])
  const assign = vi.fn()
  Object.defineProperty(window, 'location', {
    value: { ...window.location, assign },
    writable: true,
  })
  server.use(http.get('*/api/v1/x', () => new HttpResponse(null, { status: 401 })))
  setToken('expired-tok')
  await expect(api.get('/api/v1/x')).rejects.toThrow()
  expect(navigate).toHaveBeenCalledWith('/login?reason=expired')
  expect(assign).not.toHaveBeenCalled()
  setRouter(null)
})
```

(Adapt the msw handler registration to the file's existing pattern — reuse the same `server.use` idiom the neighbouring 401 test uses.)

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm vitest run src/api/client.test.ts`
Expected: the new test FAILS (`setRouter` not defined); existing tests PASS.

- [ ] **Step 3: Implement `routerRef.ts`**

```ts
// web/src/app/routerRef.ts
import type { createBrowserRouter } from 'react-router'

type AppRouter = ReturnType<typeof createBrowserRouter>

// Lets non-React modules (the axios 401 interceptor) navigate through the SPA
// router. Full page loads to non-root paths are fragile in a packaged WebView
// and waste a reload on the web.
let router: AppRouter | null = null

export function setRouter(r: AppRouter | null): void {
  router = r
}

export function navigateTo(path: string): void {
  if (router) {
    void router.navigate(path)
    return
  }
  window.location.assign(path)
}
```

- [ ] **Step 4: Wire it** — in `web/src/main.tsx`, register the router:

```tsx
import { setRouter } from '@/app/routerRef'
// inside the bootNativeApp().then(() => { ... }) block, before render:
const router = createRouter()
setRouter(router)
// pass this router instance to <RouterProvider router={router} />
```

In `web/src/api/client.ts`, replace `window.location.assign('/login?reason=expired')` with `navigateTo('/login?reason=expired')` and add `import { navigateTo } from '@/app/routerRef'`.

- [ ] **Step 5: Run the suite** — the pre-existing 401 test still passes because no router is registered in that test, so the fallback assigns.

Run: `cd web && pnpm vitest run src/api/client.test.ts && pnpm test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/app/routerRef.ts web/src/api/client.ts web/src/api/client.test.ts web/src/main.tsx
git commit -m "feat: route 401 session expiry through the SPA router"
```

---

### Task 8: External links open the system browser

**Files:**
- Create: `web/src/lib/externalLinks.ts`
- Modify: `web/src/lib/appBoot.ts` (call the installer)
- Test: `web/src/lib/externalLinks.test.ts`

**Interfaces:**
- Consumes: `isNativeApp`, `nativePlugin` (Task 1).
- Produces: `installExternalLinkInterceptor(): void` — one document-level listener; no per-anchor changes anywhere.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/externalLinks.test.ts
import { afterEach, expect, it, vi } from 'vitest'
import { installExternalLinkInterceptor } from './externalLinks'

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  document.body.innerHTML = ''
})

function clickAnchor(href: string): MouseEvent {
  const a = document.createElement('a')
  a.href = href
  document.body.appendChild(a)
  const event = new MouseEvent('click', { bubbles: true, cancelable: true })
  a.dispatchEvent(event)
  return event
}

it('sends absolute http(s) links to the native browser', () => {
  const open = vi.fn(async () => {})
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { open } } }
  installExternalLinkInterceptor()
  const event = clickAnchor('https://econumo.com/docs')
  expect(open).toHaveBeenCalledWith({ url: 'https://econumo.com/docs' })
  expect(event.defaultPrevented).toBe(true)
})

it('leaves in-app relative links alone', () => {
  const open = vi.fn(async () => {})
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { open } } }
  installExternalLinkInterceptor()
  const event = clickAnchor('/settings')
  expect(open).not.toHaveBeenCalled()
  expect(event.defaultPrevented).toBe(false)
})

it('installs nothing on the web', () => {
  installExternalLinkInterceptor()
  const event = clickAnchor('https://econumo.com')
  expect(event.defaultPrevented).toBe(false)
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm vitest run src/lib/externalLinks.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
// web/src/lib/externalLinks.ts
import { isNativeApp, nativePlugin } from './platform'

interface BrowserPlugin {
  open(o: { url: string }): Promise<void>
}

// In-app navigation uses relative hrefs (react-router), so any absolute
// http(s) anchor is by definition external and must leave the WebView.
export function installExternalLinkInterceptor(): void {
  if (!isNativeApp()) {
    return
  }
  document.addEventListener('click', (e) => {
    if (e.defaultPrevented) {
      return
    }
    const anchor = (e.target as Element | null)?.closest?.('a[href]')
    if (!anchor) {
      return
    }
    const href = anchor.getAttribute('href') ?? ''
    if (!/^https?:\/\//.test(href)) {
      return
    }
    e.preventDefault()
    void nativePlugin<BrowserPlugin>('Browser')?.open({ url: href })
  })
}
```

- [ ] **Step 4: Call it from boot** — in `web/src/lib/appBoot.ts`, inside `bootNativeApp()` after `restoreNativeStorage()`:

```ts
import { installExternalLinkInterceptor } from './externalLinks'
// ...
  await restoreNativeStorage()
  installExternalLinkInterceptor()
  void fetchServerConfig()
```

- [ ] **Step 5: Run the lib suite**

Run: `cd web && pnpm vitest run src/lib/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/externalLinks.ts web/src/lib/externalLinks.test.ts web/src/lib/appBoot.ts
git commit -m "feat: open external links in the system browser in app mode"
```

---

### Task 9: CSV export via the native share sheet

**Files:**
- Modify: `web/src/lib/download.ts`
- Test: `web/src/lib/download.test.ts` (create)

**Interfaces:**
- Consumes: `isNativeApp`, `nativePlugin` (Task 1).
- Produces: `downloadBlob(blob, filename)` keeps its signature; in app mode it writes to the cache directory and opens the Share sheet (anchor-click blob downloads do nothing in WKWebView).

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/download.test.ts
import { afterEach, expect, it, vi } from 'vitest'
import { downloadBlob } from './download'

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
})

it('writes the file and opens the share sheet in app mode', async () => {
  const writeFile = vi.fn(async () => ({ uri: 'file:///cache/transactions.csv' }))
  const share = vi.fn(async () => ({}))
  window.Capacitor = {
    isNativePlatform: () => true,
    Plugins: { Filesystem: { writeFile }, Share: { share } },
  }
  downloadBlob(new Blob(['a,b\n1,2'], { type: 'text/csv' }), 'transactions.csv')
  await vi.waitFor(() => expect(share).toHaveBeenCalled())
  expect(writeFile).toHaveBeenCalledWith({
    path: 'transactions.csv',
    data: expect.any(String),
    directory: 'CACHE',
  })
  expect(share).toHaveBeenCalledWith({ url: 'file:///cache/transactions.csv' })
})

it('keeps the anchor download on the web', () => {
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  downloadBlob(new Blob(['x']), 'x.csv')
  expect(click).toHaveBeenCalled()
  click.mockRestore()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm vitest run src/lib/download.test.ts`
Expected: the app-mode test FAILS (share never called); the web test PASSES.

- [ ] **Step 3: Implement** — extend `web/src/lib/download.ts`:

```ts
import { isNativeApp, nativePlugin } from './platform'

interface FilesystemPlugin {
  writeFile(o: { path: string; data: string; directory: string }): Promise<{ uri: string }>
}
interface SharePlugin {
  share(o: { url: string }): Promise<unknown>
}

export function downloadBlob(blob: Blob, filename: string): void {
  if (isNativeApp()) {
    void shareNative(blob, filename)
    return
  }
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

async function shareNative(blob: Blob, filename: string): Promise<void> {
  const fs = nativePlugin<FilesystemPlugin>('Filesystem')
  const share = nativePlugin<SharePlugin>('Share')
  if (!fs || !share) {
    return
  }
  const { uri } = await fs.writeFile({
    path: filename,
    data: await blobToBase64(blob),
    directory: 'CACHE',
  })
  await share.share({ url: uri })
}

function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    // result is a data: URL; Filesystem.writeFile wants the bare base64 payload
    reader.onload = () => resolve(String(reader.result).split(',')[1] ?? '')
    reader.onerror = () => reject(reader.error ?? new Error('blob read failed'))
    reader.readAsDataURL(blob)
  })
}
```

- [ ] **Step 4: Run the suite**

Run: `cd web && pnpm vitest run src/lib/download.test.ts && pnpm test`
Expected: PASS (the CSV export dialog test keeps passing — web path unchanged).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/download.ts web/src/lib/download.test.ts
git commit -m "feat: CSV export through the native share sheet in app mode"
```

---

### Task 10: Android hardware back button

**Files:**
- Modify: `web/src/lib/appBoot.ts`
- Test: `web/src/lib/appBoot.test.ts` (append)

**Interfaces:**
- Consumes: `nativePlugin` (Task 1).
- Produces: `installBackHandler(): void`, called from `bootNativeApp()`. At the navigation root (`/`, `/login`) the app minimizes; anywhere else it goes back in history.

- [ ] **Step 1: Write the failing test** (append to `web/src/lib/appBoot.test.ts`)

```ts
import { installBackHandler } from './appBoot'

it('maps hardware back to history-back, minimizing at the root', () => {
  let handler: (() => void) | undefined
  const minimizeApp = vi.fn(async () => {})
  const addListener = vi.fn((_ev: string, cb: () => void) => {
    handler = cb
  })
  window.Capacitor = {
    isNativePlatform: () => true,
    Plugins: { App: { addListener, minimizeApp } },
  }
  installBackHandler()
  expect(addListener).toHaveBeenCalledWith('backButton', expect.any(Function))

  const back = vi.spyOn(window.history, 'back').mockImplementation(() => {})
  window.history.replaceState(null, '', '/settings')
  handler!()
  expect(back).toHaveBeenCalledOnce()
  expect(minimizeApp).not.toHaveBeenCalled()

  window.history.replaceState(null, '', '/')
  handler!()
  expect(minimizeApp).toHaveBeenCalledOnce()
  back.mockRestore()
  window.history.replaceState(null, '', '/')
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm vitest run src/lib/appBoot.test.ts`
Expected: new test FAILS (`installBackHandler` not exported).

- [ ] **Step 3: Implement** — add to `web/src/lib/appBoot.ts` and call it inside `bootNativeApp()` after `installExternalLinkInterceptor()`:

```ts
interface AppPlugin {
  addListener(ev: 'backButton', cb: () => void): unknown
  minimizeApp(): Promise<void>
}

const ROOT_PATHS = new Set(['/', '/login'])

export function installBackHandler(): void {
  const app = nativePlugin<AppPlugin>('App')
  if (!app) {
    return
  }
  app.addListener('backButton', () => {
    if (ROOT_PATHS.has(window.location.pathname)) {
      void app.minimizeApp()
      return
    }
    window.history.back()
  })
}
```

- [ ] **Step 4: Run the suite**

Run: `cd web && pnpm vitest run src/lib/appBoot.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/appBoot.ts web/src/lib/appBoot.test.ts
git commit -m "feat: Android hardware back navigates the SPA, minimizing at the root"
```

---

### Task 11: Server-version warning banner (+ translations)

**Files:**
- Create: `web/src/components/ServerVersionNotice.tsx`, `web/src/components/ServerVersionNotice.test.tsx`
- Modify: `web/src/app/layouts/ApplicationLayout.tsx` (mount), all 11 `locales/*.json`

**Interfaces:**
- Consumes: `useServerConfig`, `fetchServerConfig`, `MIN_SERVER_VERSION` (Task 4); `isNewerVersion` (`web/src/lib/version.ts`); `isNativeApp` (Task 1).
- Produces: `<ServerVersionNotice />`, mounted in the authenticated layout; i18n key `common.serverUpdate.notice` with a `{version}` placeholder.

- [ ] **Step 1: Add the catalogue key** — in each of `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json`, inside the top-level `"common"` object next to the existing `"update"` object, add:

| lang | `common.serverUpdate.notice` |
|------|------------------------------|
| en | `Your Econumo server ({version}) is older than this app supports. Please update your server.` |
| de | `Dein Econumo-Server ({version}) ist älter als von dieser App unterstützt. Bitte aktualisiere deinen Server.` |
| es | `Tu servidor de Econumo ({version}) es más antiguo de lo que admite esta aplicación. Actualiza tu servidor.` |
| fr | `Votre serveur Econumo ({version}) est plus ancien que ce que cette application prend en charge. Veuillez le mettre à jour.` |
| it | `Il tuo server Econumo ({version}) è più vecchio di quanto supportato da questa app. Aggiorna il server.` |
| nl | `Je Econumo-server ({version}) is ouder dan deze app ondersteunt. Werk je server bij.` |
| pl | `Twój serwer Econumo ({version}) jest starszy niż obsługiwany przez tę aplikację. Zaktualizuj serwer.` |
| pt | `O seu servidor Econumo ({version}) é mais antigo do que esta aplicação suporta. Atualize o servidor.` |
| ru | `Ваш сервер Econumo ({version}) старее, чем поддерживает это приложение. Обновите сервер.` |
| uk | `Ваш сервер Econumo ({version}) старіший, ніж підтримує цей застосунок. Оновіть сервер.` |
| zh | `您的 Econumo 服务器（{version}）版本过旧，此应用不再支持。请更新服务器。` |

- [ ] **Step 2: Run the i18n guards to confirm parity**

Run: `go test ./internal/test/i18ntest/`
Expected: FAIL only on the "frontend t() coverage" direction if it requires the key to be used (if it passes now, fine — the component lands next); no parity/placeholder failures.

- [ ] **Step 3: Write the failing component test**

```tsx
// web/src/components/ServerVersionNotice.test.tsx
import { afterEach, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ServerVersionNotice } from './ServerVersionNotice'
import { useServerConfig } from '@/lib/appConfig'

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  useServerConfig.setState({ serverVersion: null })
})

it('renders nothing on the web or when the server is current', () => {
  useServerConfig.setState({ serverVersion: 'v0.9.0' })
  const { container } = render(<ServerVersionNotice />)
  expect(container).toBeEmptyDOMElement()

  window.Capacitor = { isNativePlatform: () => true }
  useServerConfig.setState({ serverVersion: 'v99.0.0' })
  const { container: current } = render(<ServerVersionNotice />)
  expect(current).toBeEmptyDOMElement()
})

it('warns when the app runs against an outdated server', () => {
  window.Capacitor = { isNativePlatform: () => true }
  useServerConfig.setState({ serverVersion: 'v0.9.0' })
  render(<ServerVersionNotice />)
  expect(screen.getByText(/v0\.9\.0/)).toBeInTheDocument()
})
```

- [ ] **Step 4: Run to verify it fails**

Run: `cd web && pnpm vitest run src/components/ServerVersionNotice.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 5: Implement**

```tsx
// web/src/components/ServerVersionNotice.tsx
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlert } from 'lucide-react'
import { isNativeApp } from '@/lib/platform'
import { MIN_SERVER_VERSION, fetchServerConfig, useServerConfig } from '@/lib/appConfig'
import { isNewerVersion } from '@/lib/version'

// App-mode only: the store app freezes a SPA build while the user's server
// version varies; warn (never block) when the server is older than the
// bundled SPA supports. Refetches on mount so a post-login server switch is
// picked up without an app restart.
export function ServerVersionNotice() {
  const { t } = useTranslation()
  const serverVersion = useServerConfig((s) => s.serverVersion)
  const native = isNativeApp()

  useEffect(() => {
    if (native) {
      void fetchServerConfig()
    }
  }, [native])

  if (!native || !serverVersion || !isNewerVersion(MIN_SERVER_VERSION, serverVersion)) {
    return null
  }
  return (
    <div className="flex items-center gap-2 bg-amber-100 px-3 py-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
      <TriangleAlert className="size-3.5 shrink-0" />
      <span className="min-w-0 flex-1">
        {t('common.serverUpdate.notice', { version: serverVersion })}
      </span>
    </div>
  )
}
```

- [ ] **Step 6: Mount it** — in `web/src/app/layouts/ApplicationLayout.tsx`, directly after the `<SubscriptionBanner />` line inside the root `div`:

```tsx
import { ServerVersionNotice } from '@/components/ServerVersionNotice'
// ...
      <SubscriptionBanner />
      <ServerVersionNotice />
```

- [ ] **Step 7: Run everything**

Run: `cd web && pnpm test && pnpm lint && cd .. && go test ./internal/test/i18ntest/`
Expected: PASS all (catalogue parity, placeholder parity, t() coverage).

- [ ] **Step 8: Commit**

```bash
git add web/src/components/ServerVersionNotice.tsx web/src/components/ServerVersionNotice.test.tsx web/src/app/layouts/ApplicationLayout.tsx locales/
git commit -m "feat: warn when the app runs against an outdated server"
```

---

### Task 12: App icons & splash assets

**Files:**
- Create: `mobile/assets/logo.png`, `mobile/assets/logo-dark.png`
- Modify (generated): icon/splash resources under `mobile/ios/` and `mobile/android/`

- [ ] **Step 1: Prepare the source logo** (the `@capacitor/assets` generator wants a 1024×1024 source; upscale the existing 512 icon)

```bash
mkdir -p mobile/assets
sips -z 1024 1024 web/public/icons/android-chrome-512x512-precomposed.png --out mobile/assets/logo.png
cp mobile/assets/logo.png mobile/assets/logo-dark.png
```

- [ ] **Step 2: Generate**

```bash
cd mobile && pnpm dlx @capacitor/assets generate --iconBackgroundColor '#ffffff' --iconBackgroundColorDark '#111111' --splashBackgroundColor '#ffffff' --splashBackgroundColorDark '#111111'
```

Expected: icons and splash screens written into both native projects.

- [ ] **Step 3: Verify visually** — `make mobile-ios`, confirm the home-screen icon and launch splash on the simulator. Manual checkpoint.

- [ ] **Step 4: Commit**

```bash
git add mobile
git commit -m "feat: app icons and splash screens for both platforms"
```

---

### Task 13: Docs, `ios` branch removal, final verification

**Files:**
- Modify: `CLAUDE.md`, `mobile/README.md`

- [ ] **Step 1: Add a `mobile/` section to `CLAUDE.md`** — under "Development Commands", after the frontend section:

```markdown
### Mobile app (Capacitor) — in `mobile/`

The iOS/Android apps are a Capacitor shell around the `web/` SPA — `web/` stays
the single frontend. App-specific behavior branches on `isNativeApp()`
(`web/src/lib/platform.ts`, probes the injected `window.Capacitor` global — no
Capacitor npm dependency in `web/`) and is dead code on the web. In app mode
the SPA fetches `econumo-config.js` from the selected backend and merges ONLY
`ALLOW_REGISTRATION` and `ANALYTICS` into `window.econumoConfig` (a fixed
allowlist; the server VERSION goes to a separate store for the outdated-server
banner). Minimum supported server version: `MIN_SERVER_VERSION` in
`web/src/lib/appConfig.ts`, documented in `mobile/README.md` — bump together.

    make mobile-install   # cd mobile && pnpm install
    make mobile-sync      # build web/ + cap sync into ios/ and android/
    make mobile-ios       # sync + open in Xcode
    make mobile-android   # sync + open in Android Studio
```

- [ ] **Step 2: Extend `mobile/README.md`** with the store submission checklist:

```markdown
## Store submission checklist

- App id `com.econumo.app` on both stores.
- Privacy labels: declare PostHog product analytics (disabled instance-wide
  when the server sets `ECONUMO_ANALYTICS=false`); no ad tracking.
- Review notes: include a demo account on Econumo Cloud so reviewers can
  sign in without registering.
- Release builds: `ECONUMO_VERSION=vX.Y.Z make mobile-sync`, then archive
  from Xcode (iOS) / build an AAB from Android Studio.
```

- [ ] **Step 3: Delete the superseded native prototype branch** (local-only, approved in the spec)

```bash
git branch -D ios
```

- [ ] **Step 4: Full verification**

```bash
make web-test && make web-lint
make go-test
make mobile-sync
```

Expected: all green; `go-test` covers the i18n guards over the changed catalogues.

Manual checkpoint: run the app on both an iPhone simulator and an Android emulator against Econumo Cloud **and** a local backend (`make go-run`): login, offline relaunch shows cached data, create a transaction, CSV export opens the share sheet, an external settings link opens the system browser, Android back minimizes at the home screen.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md mobile/README.md
git commit -m "docs: mobile app build commands and store checklist"
```
