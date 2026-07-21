# Connection Subscription Banner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The global SubscriptionBanner also warns when a connection's (partner's) subscription is ending or already read-only, per the approved spec `docs/superpowers/specs/2026-07-21-connection-subscription-banner-design.md`.

**Architecture:** Frontend-only. A pure derivation helper in `web/src/lib/access.ts` picks the worst connection state; `SubscriptionBanner` feeds it from the existing `useConnections` query (enabled only when billing is enabled) and renders two new dismissible variants below the own-state variants in priority.

**Tech Stack:** React 19, TanStack Query, vitest + testing-library + msw, i18n via `locales/{en,ru}.json` pipe plurals (`pluralPick`).

## Global Constraints

- Priority (highest wins): own `readonly` > own `trial` (≤3 days) > `partner_readonly` > `partner_trial` (≤3 days). Exactly one banner, naming exactly one partner.
- Partner variants require `billingEnabled`; the connections query must not fire when billing is disabled (`useConnections({ enabled: billingEnabled })`).
- Dismissal: the existing single localStorage key `subscriptionBannerDismissedDay` (local calendar day) hides `trial`, `partner_readonly`, and `partner_trial`. Own `readonly` stays permanent and non-dismissible.
- CTA on every variant: existing "Manage subscription" button → `portal.open()` (no `forUserId`).
- Analytics: reuse `METRICS.SUBSCRIPTION_BANNER_SHOW` with `variant` values `partner_readonly` / `partner_trial`; no new metric key.
- Copy (en; ru mirrors with 3-form pipe plurals):
  - `subscription.banner.connection_trial`: `"{name}'s subscription ends in {count} day | {name}'s subscription ends in {count} days"`
  - `subscription.banner.connection_readonly`: `"{name}'s subscription has ended — they can view shared accounts but not edit"`
- All commands run from the repo root unless the step says otherwise; frontend commands run in `web/`.

---

### Task 1: `worstConnectionAttention` derivation helper

**Files:**
- Modify: `web/src/lib/access.ts` (append)
- Test: `web/src/lib/access.test.ts` (append)

**Interfaces:**
- Consumes: `deriveAccessState(level, until)` and `accessDaysLeft(until, now?)`, both already exported from `web/src/lib/access.ts`.
- Produces (Task 2 relies on these exact names/types):

```ts
export interface ConnectionAttention {
  state: 'readonly' | 'trial'
  name: string
  daysLeft: number | null // null for readonly, ≤3 for trial
}
export function worstConnectionAttention(
  connections: readonly { user: { name: string }; accessLevel: 'full' | 'readonly'; accessUntil: string }[],
  now?: Date,
): ConnectionAttention | null
```

- [ ] **Step 1: Write the failing tests**

Append to `web/src/lib/access.test.ts`:

```ts
describe('worstConnectionAttention', () => {
  const now = new Date('2026-07-21T12:00:00Z')
  const conn = (name: string, accessLevel: 'full' | 'readonly', accessUntil: string) => ({
    user: { name },
    accessLevel,
    accessUntil,
  })

  it('returns null when every connection has full access', () => {
    expect(worstConnectionAttention([conn('A', 'full', '')], now)).toBeNull()
    expect(worstConnectionAttention([], now)).toBeNull()
  })

  it('ignores trials ending more than 3 days out', () => {
    expect(worstConnectionAttention([conn('A', 'full', '2026-08-30 00:00:00')], now)).toBeNull()
  })

  it('reports a trial ending within 3 days with the partner name and days left', () => {
    expect(worstConnectionAttention([conn('Megan', 'full', '2026-07-23 00:00:00')], now)).toEqual({
      state: 'trial',
      name: 'Megan',
      daysLeft: 2,
    })
  })

  it('readonly beats an ending trial', () => {
    const result = worstConnectionAttention(
      [conn('A', 'full', '2026-07-22 00:00:00'), conn('B', 'readonly', '')],
      now,
    )
    expect(result).toEqual({ state: 'readonly', name: 'B', daysLeft: null })
  })

  it('an elapsed accessUntil counts as readonly', () => {
    expect(worstConnectionAttention([conn('C', 'full', '2026-07-18 00:00:00')], now)).toEqual({
      state: 'readonly',
      name: 'C',
      daysLeft: null,
    })
  })

  it('among several ending trials the soonest expiry wins', () => {
    const result = worstConnectionAttention(
      [conn('A', 'full', '2026-07-23 00:00:00'), conn('B', 'full', '2026-07-22 00:00:00')],
      now,
    )
    expect(result).toEqual({ state: 'trial', name: 'B', daysLeft: 1 })
  })
})
```

Also add `worstConnectionAttention` to the existing import from `./access` at the top of the test file.

- [ ] **Step 2: Run the tests to verify they fail**

Run (in `web/`): `pnpm vitest run src/lib/access.test.ts`
Expected: FAIL — `worstConnectionAttention` is not exported.

- [ ] **Step 3: Implement the helper**

Append to `web/src/lib/access.ts`:

```ts
// The one connection the SubscriptionBanner should warn about, if any:
// read-only beats ending-soon; among ending trials the soonest expiry wins.
export interface ConnectionAttention {
  state: 'readonly' | 'trial'
  name: string
  daysLeft: number | null
}

export function worstConnectionAttention(
  connections: readonly { user: { name: string }; accessLevel: 'full' | 'readonly'; accessUntil: string }[],
  now: Date = new Date(),
): ConnectionAttention | null {
  let trial: ConnectionAttention | null = null
  for (const c of connections) {
    const state = deriveAccessState(c.accessLevel, c.accessUntil)
    if (state === 'trial') {
      const daysLeft = accessDaysLeft(c.accessUntil, now)
      if (daysLeft <= 0) {
        // Elapsed accessUntil: effectively read-only already.
        return { state: 'readonly', name: c.user.name, daysLeft: null }
      }
      if (daysLeft <= 3 && (trial === null || daysLeft < (trial.daysLeft ?? Infinity))) {
        trial = { state: 'trial', name: c.user.name, daysLeft }
      }
    } else if (state === 'readonly') {
      return { state: 'readonly', name: c.user.name, daysLeft: null }
    }
  }
  return trial
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run (in `web/`): `pnpm vitest run src/lib/access.test.ts`
Expected: PASS (all cases, including the pre-existing ones).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/access.ts web/src/lib/access.test.ts
git commit -m "feat(web): worstConnectionAttention picks the connection the banner warns about"
```

---

### Task 2: Banner variants, i18n strings, and component tests

**Files:**
- Modify: `locales/en.json` (inside `subscription.banner`)
- Modify: `locales/ru.json` (inside `subscription.banner`)
- Modify: `web/src/features/access/SubscriptionBanner.tsx`
- Test: `web/src/features/access/SubscriptionBanner.test.tsx` (append)

**Interfaces:**
- Consumes: `worstConnectionAttention(connections, now?)` from Task 1; `useConnections({ enabled })` from `@/features/connections/queries`; existing `coreHandlers({ connections })` msw fixture override.
- Produces: no new exports — `SubscriptionBanner` keeps its signature.

- [ ] **Step 1: Add the catalogue strings**

In `locales/en.json`, inside `"subscription" > "banner"` after the `"dismiss"` entry, add:

```json
      "connection_trial": "{name}'s subscription ends in {count} day | {name}'s subscription ends in {count} days",
      "connection_readonly": "{name}'s subscription has ended — they can view shared accounts but not edit"
```

In `locales/ru.json`, same position:

```json
      "connection_trial": "Подписка {name} закончится через {count} день | Подписка {name} закончится через {count} дня | Подписка {name} закончится через {count} дней",
      "connection_readonly": "Подписка {name} закончилась — общие счета доступны им только для просмотра"
```

- [ ] **Step 2: Write the failing component tests**

Append to `web/src/features/access/SubscriptionBanner.test.tsx`:

```tsx
function partnerConn(accessLevel: 'full' | 'readonly', accessUntil: string) {
  return [{ user: { id: 'u2', avatar: 'pets:sky', name: 'Megan' }, accessLevel, accessUntil, sharedAccounts: [] }]
}

it('warns when a connection trial ends within 3 days, with the partner name', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ connections: partnerConn('full', utcIn(2)) }))
  renderBanner()
  expect(await screen.findByText("Megan's subscription ends in 2 days")).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Manage subscription' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Dismiss' })).toBeInTheDocument()
  expect(window.dataLayer).toContainEqual(expect.objectContaining({ event: 'appSubscriptionBannerShow' }))
})

it('shows nothing for a connection trial more than 3 days out', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ connections: partnerConn('full', utcIn(30)) }))
  renderBanner()
  await waitFor(() => expect(window.dataLayer).toEqual([]))
})

it('warns dismissibly when a connection is read-only', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ connections: partnerConn('readonly', '') }))
  const user = userEvent.setup()
  renderBanner()
  expect(await screen.findByText(/Megan's subscription has ended/)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Dismiss' }))
  expect(screen.queryByText(/Megan's subscription has ended/)).not.toBeInTheDocument()
  expect(localStorage.getItem('subscriptionBannerDismissedDay')).not.toBeNull()
})

it('own trial outranks a read-only connection', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(
    ...coreHandlers({
      user: { ...fixtureUser, accessUntil: utcIn(2) },
      connections: partnerConn('readonly', ''),
    }),
  )
  renderBanner()
  expect(await screen.findByText('Your subscription ends in 2 days')).toBeInTheDocument()
  expect(screen.queryByText(/Megan's subscription/)).not.toBeInTheDocument()
})

it('shows no partner variants when billing is disabled', async () => {
  server.use(...coreHandlers({ connections: partnerConn('readonly', '') }))
  renderBanner()
  await waitFor(() => expect(window.dataLayer).toEqual([]))
})
```

- [ ] **Step 3: Run the tests to verify the new ones fail**

Run (in `web/`): `pnpm vitest run src/features/access/SubscriptionBanner.test.tsx`
Expected: the five new tests FAIL (no partner banner rendered); the eight pre-existing tests still pass.

- [ ] **Step 4: Implement the banner variants**

Replace the body of `web/src/features/access/SubscriptionBanner.tsx` between the imports and the `cta` construction with:

```tsx
import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { formatDate } from '@/lib/datetime'
import { pluralPick } from '@/lib/plural'
import { METRICS, trackEvent } from '@/lib/metrics'
import { worstConnectionAttention } from '@/lib/access'
import { useAccessState } from '@/features/user/queries'
import { useConnections } from '@/features/connections/queries'
import { useOpenBillingPortal } from './useOpenBillingPortal'

// Dismissal persists for the local calendar day: the banner stays hidden
// across reloads and returns the next day (the countdown has moved by then).
const DISMISSED_KEY = 'subscriptionBannerDismissedDay'

export function SubscriptionBanner() {
  const { t, i18n } = useTranslation()
  const { state, daysLeft, billingEnabled } = useAccessState()
  // Partner warnings only exist where the billing portal can act on them,
  // so self-hosted instances (no BILLING_URL) never fetch connections here.
  const { data: connections = [] } = useConnections({ enabled: billingEnabled })
  const portal = useOpenBillingPortal()
  const [dismissed, setDismissed] = useState(() => localStorage.getItem(DISMISSED_KEY) === formatDate(new Date()))

  const partner = billingEnabled && !dismissed ? worstConnectionAttention(connections) : null

  // One banner; own state outranks any partner's.
  const variant =
    state === 'readonly'
      ? ('readonly' as const)
      : state === 'trial' && billingEnabled && daysLeft !== null && daysLeft <= 3 && !dismissed
        ? ('trial' as const)
        : partner
          ? partner.state === 'readonly'
            ? ('partner_readonly' as const)
            : ('partner_trial' as const)
          : null

  useEffect(() => {
    if (variant) {
      trackEvent(METRICS.SUBSCRIPTION_BANNER_SHOW, { variant })
    }
  }, [variant])

  if (!variant) {
    return null
  }

  const cta = billingEnabled ? (
    <Button type="button" size="sm" variant="outline" disabled={portal.pending} onClick={() => portal.open()}>
      {t('subscription.banner.cta')}
    </Button>
  ) : null

  if (variant === 'readonly') {
    return (
      <div className="flex items-center gap-3 bg-destructive/10 px-4 py-2 text-sm text-destructive">
        <span className="min-w-0 flex-1">{t('subscription.banner.readonly')}</span>
        {cta}
      </div>
    )
  }

  const message =
    variant === 'trial'
      ? pluralPick(t('subscription.banner.trial'), Math.max(daysLeft ?? 0, 0), i18n.language)
      : variant === 'partner_trial'
        ? pluralPick(t('subscription.banner.connection_trial', { name: partner!.name }), partner!.daysLeft ?? 0, i18n.language)
        : t('subscription.banner.connection_readonly', { name: partner!.name })

  return (
    <div className="flex items-center gap-3 bg-primary/10 px-4 py-2 text-sm text-primary">
      <span className="min-w-0 flex-1">{message}</span>
      {cta}
      <button
        type="button"
        aria-label={t('subscription.banner.dismiss')}
        className="shrink-0 hover:opacity-70"
        onClick={() => {
          localStorage.setItem(DISMISSED_KEY, formatDate(new Date()))
          setDismissed(true)
        }}
      >
        <X className="size-4" />
      </button>
    </div>
  )
}
```

(This is the complete new file content.)

- [ ] **Step 5: Run the component tests**

Run (in `web/`): `pnpm vitest run src/features/access/SubscriptionBanner.test.tsx`
Expected: PASS — all 13 tests.

- [ ] **Step 6: Run the full frontend suite, lint, and i18n guards**

Run (in `web/`): `pnpm test` — Expected: all files pass.
Run (in `web/`): `pnpm lint` — Expected: no new warnings.
Run (repo root): `go test ./internal/test/i18ntest/` — Expected: `ok` (new keys have en/ru parity and matching `{name}`/`{count}` placeholder sets).

- [ ] **Step 7: Commit**

```bash
git add locales/en.json locales/ru.json web/src/features/access/SubscriptionBanner.tsx web/src/features/access/SubscriptionBanner.test.tsx
git commit -m "feat(web): SubscriptionBanner warns about connections' ending subscriptions"
```
