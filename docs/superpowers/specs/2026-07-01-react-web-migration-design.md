# Web frontend migration: Vue 3 + Quasar → React + Vite + shadcn/ui

**Date:** 2026-07-01
**Branch:** `react-web` (cut from `golang`; all migration work lands here)
**Status:** Approved design

## Goals

Replace the Vue 3 + Quasar 2 SPA in `web/` with a React + Vite + shadcn/ui app that:

- preserves every screen, workflow, route, and URL exactly as they are today;
- works on desktop, tablet, and mobile as first-class targets;
- is simpler and more maintainable than the current codebase;
- leaves the Go backend, its API contract, and the deployment model (one binary
  serving the SPA from `ECONUMO_WEB_DIST`) untouched.

## Non-goals

- No API changes.
- No visual redesign beyond adopting shadcn's native styling (see UI fidelity).
- No new features.
- No changes to the runtime-config mechanism: `econumo-config.js` /
  `window.econumoConfig` keeps working identically for existing deployments.

## Key decisions

| Decision | Choice |
|---|---|
| UI fidelity | Same structure, layouts, and workflows; components adopt shadcn's native look (no pixel-mimicry of Quasar Material). Icons move to lucide-react. |
| Strategy | Parallel rewrite in `web-react/`, swapped into `web/` in one commit at feature parity. No Vue/React interop; the Vue app stays untouched until the swap. |
| State layer | TanStack Query for all server data (replaces the Pinia stores and the hand-rolled `sync.ts` orchestration); one small zustand store or local state for UI-only state. |
| i18n | react-i18next; the en-US catalog ports as-is, keeping the door open for community locales. |
| Testing | Broad coverage: Vitest + React Testing Library + MSW, including page-level integration tests for every page and modal flow. |
| Code organization | Feature folders + targeted cleanup (Approach A): identical UX, cleaner internals — split oversized files, one shared dialog system, mixins dissolve into hooks. |
| shadcn usage | Vendor the **full** shadcn component set upfront (`pnpm dlx shadcn@latest add --all` into `components/ui/`); always reach for a shadcn component before writing anything custom. Vendored files are never hand-edited. |
| Parity rule | Every feature migration ends with a side-by-side comparison against the Vue app (same backend) covering all of that feature's user flows at desktop, tablet, and mobile widths. |
| Auth expiry fix | An intentional behavior change: an expired/invalid token no longer fails silently — the user is redirected to the login screen with a visible "session expired" notice. |
| UUIDv7 | All client-generated IDs use UUIDv7 (the Vue app uses v4), matching the backend's convention for new ids. Every API request also carries an `X-Request-Id: <uuidv7>` header so the backend can later adopt client-supplied request ids. |

## Library mapping

| Today (Vue) | New (React) | Note |
|---|---|---|
| Quasar 2 + webpack (`@quasar/app-webpack`) | Vite + React 19 + TypeScript | |
| Quasar components | shadcn/ui (Radix + Tailwind 4) | copied-in source, not a dependency |
| vue-router | react-router v7 (library mode) | same route table and auth-guard meta |
| Pinia (16 stores) | TanStack Query + small zustand store | server cache vs. UI state |
| vue-i18n | react-i18next | |
| vue-chartjs | react-chartjs-2 | chart.js itself stays; charts look identical |
| vuedraggable / vue-draggable-plus | dnd-kit | account/category/envelope reordering |
| axios + `modules/api/v1` DTO layer | ported unchanged | typed TS, moves nearly verbatim |
| lodash / lodash-es | dropped | modern JS covers current usage |
| jwt-decode | kept | |
| uuid v9 (`v4()`) | uuid v11+ (`v7()`) | UUIDv7 for all client-generated ids |
| — (no tests) | Vitest + React Testing Library + MSW | MSW mocks at the network layer |

Package manager stays pnpm.

## Project structure

The new app lives in `web-react/` during the migration:

```
web-react/src/
├── app/                    # composition root: providers, router, layouts
│   ├── layouts/            # ApplicationLayout, LoginLayout (ports of the two Vue layouts)
│   └── routes.tsx          # same route table + auth guard
├── components/ui/          # full vendored shadcn set (never hand-edited)
├── components/             # shared app components (MonthPicker, CurrencySelect, dialog system…)
├── features/
│   ├── auth/               # login, registration, recovery, logout
│   ├── accounts/           # account list, account page, folders, account modal
│   ├── transactions/       # transaction modal (split), CSV import/export
│   ├── budget/             # budget page (split), envelopes, limits, widgets
│   ├── categories/
│   ├── payees/
│   ├── tags/
│   ├── connections/
│   ├── settings/
│   └── onboarding/
├── api/                    # ported modules/api/v1 + DTOs (axios client, auth interceptor)
├── lib/                    # config.ts, storage.ts, metrics.ts, money/calculator helpers, i18n setup
└── locales/en-US/          # ported string catalog
```

Each feature colocates its pages, components, and query hooks.

Targeted cleanup during the port:

- `Budget.vue` (1,241 lines) splits into page + table + folder-row + totals +
  per-modal files.
- `TransactionModal.vue` (1,076 lines) splits into form, calculator-input
  wiring, and transfer/expense/income parts.
- The 8 Options-API mixins dissolve into plain hooks or are deleted.
- The ~25 bespoke modal components become content components rendered inside
  one shared dialog primitive (see UI layer).

## Data layer

- Each API module gets a thin query-hook file (e.g. `features/accounts/queries.ts`):
  `useAccounts()`, `useCreateAccount()`, … wrapping the ported axios functions
  with TanStack Query.
- Mutations invalidate the affected query keys; this fully replaces the
  hand-rolled `sync.ts` refresh orchestration.
- One query-key convention: `['accounts']`, `['transactions', accountId]`,
  `['budget', budgetId, period]`.
- Auth flow: JWT in localStorage via `lib/storage.ts`, attached by an axios
  interceptor. **Behavior fix over the Vue app** (which silently fails on an
  expired token): any 401 response → purge the token, redirect to `/login`,
  and show a visible "Your session has expired, please sign in again" notice
  on the login screen. Additionally, the `exp` claim (via jwt-decode) is
  checked on app load and route changes, so an already-expired token redirects
  proactively instead of waiting for a failed API call.
- UI-only state (open modal, selected budget month, collapsed folders) lives in
  one small zustand store or local component state, whichever is closest to use.
- **IDs are UUIDv7.** Client-generated entity ids (transactions, categories,
  tags, payees, accounts, budgets/envelopes…) use `v7()` from the `uuid`
  package (v11+), replacing the Vue app's v4. Existing ids are never rewritten.
- **`X-Request-Id` on every request.** The axios request interceptor attaches a
  fresh UUIDv7 as `X-Request-Id` to each API call. The backend currently mints
  its own request id and ignores incoming ones; this header prepares for a
  later backend improvement to honor client-supplied ids (out of scope here).

## UI layer

**Layouts & navigation.** `LoginLayout` (centered card) and `ApplicationLayout`
(app shell: sidebar/drawer with account list and navigation, header) port
one-to-one. Same routes, URLs, `requireAuth` redirect logic, and 404 catch-all —
bookmarks survive the swap.

**Responsive.** Desktop, tablet, and mobile are first-class:

- App shell: persistent sidebar on desktop; collapsible/overlay drawer on
  tablet and mobile (matching current behavior).
- Dialog primitive: desktop dialog ↔ mobile bottom-sheet.
- Tailwind's standard breakpoints (`sm`/`md`/`lg`) drive layout.

**Dialog system.** One shared responsive dialog primitive built on shadcn's
`Dialog`/`Drawer`, plus shared `ConfirmDialog` and `PromptDialog`. Feature
modals keep their current fields and flows; the open/close/responsive plumbing
is written once.

**Styling.** Tailwind 4 with shadcn theme tokens. The current `css/` directory
(~6,800 lines of SCSS, mostly Quasar overrides) is not ported. Econumo's core
brand colors map onto the shadcn CSS-variable theme; purpose-built styling
(e.g. the budget table) is rebuilt with Tailwind utilities.

**Runtime config — unchanged contract.** `index.html` loads `econumo-config.js`
exactly as today. `lib/config.ts` is a near-verbatim port reading
`window.econumoConfig` (`API_URL`, `ALLOW_REGISTRATION`, `ALLOW_CUSTOM_API`,
`PAYWALL_ENABLED`, `VERSION`) with the same precedence and defaults; the Quasar
locale-detection is replaced by `navigator.language`. Build-time env
(`ECONUMO_VERSION`, `WEBSITE_URL`) moves from webpack `process.env` to Vite
`import.meta.env` under the same names. `metrics.ts` (dataLayer events) ports
as-is.

**LilTag.** The vendored tag-manager (`public/liltag.min.js`) and its
conditional loader snippet in `index.html` port unchanged: when
`window.econumoConfig.LILTAG_CONFIG_URL` is set, the script is injected and
initialized with `LILTAG_CONFIG_URL` + `LILTAG_CACHE_TTL`, exactly as today.
Both keys stay in the `EconumoConfig` typing.

**Error handling.** API validation errors surface on form fields (the
envelope's `errors` map); global failures show the error modal; a React error
boundary wraps the app shell. Loading states keep current patterns (blocking
loading modal, inline spinners) built from shadcn primitives.

## Testing

Vitest + React Testing Library + MSW, wired into `make` targets from day one
(new `web-test` target):

- **Unit tests** for all ported logic: calculator, money/decimal formatting,
  validation, `config.ts` precedence, month/date math.
- **Component tests** for shared building blocks: dialog system, MonthPicker,
  CurrencySelect, CalculatorInput.
- **Page-level integration tests for every page and modal flow**, with MSW
  answering using real envelope-shaped responses (`{"success": true, …}`, the
  exact `"2006-01-02 15:04:05"` datetime format, `isArchived` as `0`/`1`).
- Where mobile behavior differs from desktop (e.g. dialog vs. bottom-sheet),
  tests exercise both renderings.

## Migration order

Foundation first, then features in dependency order:

1. **Foundation**: Vite scaffold, Tailwind + full shadcn vendor, ported `api/` +
   `lib/` + i18n catalog, test harness, providers, router, both layouts.
2. **Auth**: login, registration, recovery, logout, self-hosted/custom-API modal.
3. **App shell + accounts**: sidebar with account list/folders, account page
   with transactions, transaction modal, drag-reorder.
4. **Settings cluster**: profile, change-password, accounts, categories,
   payees, tags, budgets list.
5. **Budget**: the page, envelopes, limits, widgets, budget modals.
6. **Connections + onboarding + CSV import/export + 404.**

### Per-feature definition of done

1. Page tests pass.
2. Side-by-side parity check: the Vue app and the React app run simultaneously
   against the same local backend; every user flow of the feature is walked in
   both and behaves identically (same data shown, same validation messages,
   same navigation) — at desktop, tablet, and mobile widths. Approved
   divergences: the auth-expiry fix (user-visible) and UUIDv7 ids +
   `X-Request-Id` (invisible to the user).
3. Any divergence is fixed before moving to the next feature.

## The swap

When all features pass parity, one commit:

- `web-react/` replaces `web/` (Vue app deleted, directory renamed).
- Makefile targets repointed: `web-install`, `web-dev`, `web-bundle`,
  `web-lint`, plus the new `web-test`.
- `deployment/docker/Dockerfile` updated to the Vite build; output stays a
  static `dist/` served via `ECONUMO_WEB_DIST` — no backend change.
- `econumo-config.js` keeps its name and location, so existing self-hosted
  deployments' runtime config keeps working with no user action.
