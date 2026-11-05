# Apple Wallet Setup Guide Clarity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the two Shortcuts-app steps of the Apple Wallet setup guide (3: create the automation, 5: switch to Run Immediately) hard to get wrong or skip, and shorten step 2.

**Architecture:** SPA-only change in `web/src/features/imports/AppleWalletSetup.tsx`: steps 3 and 5 get a version-neutral checklist/text, an "Open Shortcuts" button (iOS only) and two links into the website article's iOS 26 / iOS 27 sections; step 2's token explanation moves behind a disclosure. No server, shortcut or step-list change (`STEPS` stays 5 ids).

**Tech Stack:** React 19, react-i18next, vitest + Testing Library + msw, sonner toasts, `locales/*.json` (11 languages).

**Spec:** the decisions below (agreed in chat, 2026-10-24); no separate spec doc.

## Decisions (the spec)

- Keep all 5 steps; step 5 stays (unlock-before-first-payment was tried and failed in practice).
- No in-app screenshots: steps 3 and 5 link to the documentation article instead.
- iOS version: no detection, no toggle. Version-neutral wording + two links per step.
- Shortcut reminder, Inbox reminder and the first-payment toast/scroll are out of scope.
- Article anchors (verified live on econumo.com): step 3 → `#ios-26`, `#ios-27`; step 5 → `#on-ios-26`, `#on-ios-27`, appended to `WALLET_GUIDE_URL`.

## Global Constraints

- Every new user-facing action fires a `METRICS` event (`web/src/lib/metrics.ts`, `app`-prefixed camelCase); `metrics-coverage.test.ts` must stay green.
- Every new catalogue key exists in all 11 `locales/<lang>.json` with identical `{var}` sets (`internal/test/i18ntest`); keep `en` as the copy below, translate the other 10.
- Every link/button that leaves the page opens in a new window (`ExternalButton` / `target="_blank" rel="noopener"`), because a same-window navigation from the home-screen PWA has no way back.
- Buttons that show the shortcut/app names keep `normal-case! tracking-normal!` (as `ExternalButton` already does).
- `docs/regression-test-plan.md` §5a items for steps 2, 3, 5 are updated in the same branch.
- Comments sparingly (CLAUDE.md "Comments — write sparingly").

## Review Focus

- Desktop (not iOS): "Open Shortcuts" must not render; the two guide links must still render — tested in Task 1.
- Step 5 before the first payment: the new button row must keep Done disabled — covered by the existing `step 5 requires the first payment` test, which must stay green.
- Long translations on a 320 px phone: the link row and buttons must wrap (`flex-wrap`), never overflow — manual check, regression plan item in Task 3.

---

### Task 1: Steps 3 and 5 — checklist, Open Shortcuts, iOS 26/27 guide links

**Files:**
- Modify: `web/src/features/imports/AppleWalletSetup.tsx` (`body()` cases; two small components next to `ExternalButton`)
- Modify: `web/src/lib/metrics.ts` (two keys after `IMPORT_SETUP_RESTART`)
- Modify: `locales/*.json` (11 files)
- Test: `web/src/features/imports/AppleWalletSetup.test.tsx`

**Interfaces:**
- Produces: `METRICS.IMPORT_SETUP_GUIDE_OPEN = 'appImportSetupGuideOpen'` fired with `{ provider: 'apple-wallet', step: 'automate' | 'immediate', ios: '26' | '27' }`; `METRICS.IMPORT_SETUP_SHORTCUTS_OPEN = 'appImportSetupShortcutsOpen'` fired with `{ provider: 'apple-wallet', step: 'automate' | 'immediate' }`.
- Produces: `function GuideLinks({ step }: { step: 'automate' | 'immediate' })` and `function OpenShortcutsButton({ step }: { step: 'automate' | 'immediate' })` (module-private).
- Produces: `export const SHORTCUTS_APP_URL = 'shortcuts://'`.

**Copy (`en`, keys under `imports.apple_wallet.steps`):**

| Key | Value |
|---|---|
| `automate.text` | `In the Shortcuts app, create a Wallet automation:` |
| `automate.list.open` | `Open Automation (on iOS 27: Library → Automation) and tap +` |
| `automate.list.wallet` | `Choose Wallet` |
| `automate.list.cards` | `Keep all cards and categories selected` |
| `automate.list.confirm` | `Keep confirmation on: Run After Confirmation (iOS 26) or Confirm Before Run (iOS 27)` |
| `automate.list.shortcut` | `Run the econumo shortcut (on iOS 27: add Run Shortcut → econumo, with Transaction as its input)` |
| `immediate.text` | `Turn off the confirmation so payments import without asking: open the automation in Shortcuts and choose Run Immediately (iOS 26) or turn off Confirm Before Run (iOS 27).` |
| `guide_title` | `Step-by-step guide with screenshots:` |
| `guide_ios26` | `iOS 26` |
| `guide_ios27` | `iOS 27` |
| `open_shortcuts` | `Open Shortcuts` |

`automate.list.*` render as an `<ol>` (numbered, `list-decimal pl-5`, muted text) after `automate.text`. Remove nothing else; the old one-sentence `automate.text` / `immediate.text` values are replaced.

- [ ] **Step 1: Write the failing tests** in `AppleWalletSetup.test.tsx`

```tsx
it.each([
  ['Create the automation', ['install', 'configure'], '#ios-26', '#ios-27', 'automate'],
  ['Switch the automation to Run Immediately', ['install', 'configure', 'automate'], '#on-ios-26', '#on-ios-27', 'immediate'],
] as const)('%s links to the iOS 26 and iOS 27 sections of the guide in a new window', async (title, ticked, a26, a27, stepId) => {
  // step 5 needs a received payment to be current
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(ticked))
  mockIsIOS.value = true
  renderSetup(stepId === 'immediate' ? { ...source, cards: [card] } : source)
  const body = step(title)
  for (const [name, anchor] of [['iOS 26', a26], ['iOS 27', a27]] as const) {
    const link = within(body).getByRole('link', { name })
    expect(link).toHaveAttribute('href', `https://econumo.com/docs/user-guide/apple-wallet${anchor}`)
    expect(link).toHaveAttribute('target', '_blank')
  }
  expect(within(body).getByRole('link', { name: 'Open Shortcuts' })).toHaveAttribute('href', 'shortcuts://')
})

it('step 3 lists the automation checklist in order', () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install', 'configure']))
  renderSetup(source)
  const items = within(step('Create the automation')).getAllByRole('listitem').map((li) => li.textContent)
  expect(items).toEqual([
    'Open Automation (on iOS 27: Library → Automation) and tap +',
    'Choose Wallet',
    'Keep all cards and categories selected',
    'Keep confirmation on: Run After Confirmation (iOS 26) or Confirm Before Run (iOS 27)',
    'Run the econumo shortcut (on iOS 27: add Run Shortcut → econumo, with Transaction as its input)',
  ])
})

it('desktop shows the guide links but no Open Shortcuts button', () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install', 'configure']))
  renderSetup(source)
  expect(within(step('Create the automation')).getByRole('link', { name: 'iOS 27' })).toBeInTheDocument()
  expect(screen.queryByRole('link', { name: 'Open Shortcuts' })).not.toBeInTheDocument()
})
```

Note: `getAllByRole('listitem')` inside the step body must find only the checklist `<li>`s — the step itself is an `<li>`, but `within(stepLi)` excludes the container. If the selector picks up nested step lists, scope by `getByRole('list')` first.

Add a test in `walletSetupAnalytics.test.tsx` (it already mocks `trackEvent`; follow its harness): clicking `iOS 27` on step 3 tracks `appImportSetupGuideOpen` with `{ provider: 'apple-wallet', step: 'automate', ios: '27' }`; clicking `Open Shortcuts` on iOS tracks `appImportSetupShortcutsOpen` with `{ provider: 'apple-wallet', step: 'automate' }`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm vitest run src/features/imports/AppleWalletSetup.test.tsx src/features/imports/walletSetupAnalytics.test.tsx`
Expected: the new tests FAIL (no `iOS 26` link / no checklist items); existing tests pass.

- [ ] **Step 3: Implement**

Add the metrics keys, the catalogue keys (all 11 languages), `SHORTCUTS_APP_URL`, `GuideLinks` (muted `guide_title` text, then two `<a>` links in a `flex flex-wrap gap-x-4 gap-y-1` row styled like the existing "Setup instructions" link with `ExternalLink` icon, href `${WALLET_GUIDE_URL}${anchor}`), and `OpenShortcutsButton` (`ExternalButton` with `href={SHORTCUTS_APP_URL}`, rendered only when `isIOS()`). In `body()`: `case 'automate'` renders text, checklist, `<div className="flex flex-wrap items-center gap-3">` holding `OpenShortcutsButton` and the Done button (when not done), then `GuideLinks`; `immediate` takes the default branch today — give it its own case with text, the same button row (Done stays disabled until the payment, as now) and `GuideLinks`.

- [ ] **Step 4: Run the tests, lint and i18n guards**

Run: `cd web && pnpm vitest run src/features/imports src/lib/metrics-coverage.test.ts && pnpm lint`
Run: `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/imports web/src/lib/metrics.ts locales
git commit -m "feat: Apple Wallet steps 3 and 5 get a checklist, Open Shortcuts and iOS 26/27 guide links"
```

### Task 2: Step 2 — one-line instruction, details behind "What does this do?"

**Files:**
- Modify: `web/src/features/imports/AppleWalletSetup.tsx` (`case 'configure'`)
- Modify: `locales/*.json`
- Test: `web/src/features/imports/AppleWalletSetup.test.tsx`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: catalogue keys `steps.configure.text` (replaced), `steps.configure.details_toggle`, `steps.configure.details`.

**Copy (`en`):**

| Key | Value |
|---|---|
| `configure.text` | `Tap Connect the Shortcut, then tap Always Allow on every prompt.` |
| `configure.details_toggle` | `What does this do?` |
| `configure.details` | `It creates an access token for this iPhone. The Shortcut saves your server address and the token, then checks in with Econumo. The automation needs the permissions you allow here.` |

- [ ] **Step 1: Write the failing test**

```tsx
it('step 2 keeps one instruction visible and the token details behind a toggle', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  const user = userEvent.setup()
  renderSetup(source)
  const body = step('Connect the Shortcut')
  expect(within(body).getByText('Tap Connect the Shortcut, then tap Always Allow on every prompt.')).toBeVisible()
  expect(within(body).queryByText(/creates an access token/)).not.toBeInTheDocument()
  const toggle = within(body).getByRole('button', { name: 'What does this do?' })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await user.click(toggle)
  expect(toggle).toHaveAttribute('aria-expanded', 'true')
  expect(within(body).getByText(/creates an access token/)).toBeVisible()
})
```

Note: `step('Connect the Shortcut')` resolves via `stepToggle`, which already filters to the `aria-expanded` step header; the new disclosure button also carries `aria-expanded` but a different name, so the helper is unaffected.

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm vitest run src/features/imports/AppleWalletSetup.test.tsx -t "step 2 keeps"`
Expected: FAIL (old long text, no toggle).

- [ ] **Step 3: Implement** a local `useState(false)` disclosure in `Stepper` (not in `body()`, which is not a component): a `text-xs text-econumo-link` button with `aria-expanded`, showing `configure.details` as muted `text-xs` below when open. Update all 11 catalogues.

- [ ] **Step 4: Run** `cd web && pnpm vitest run src/features/imports && pnpm lint` and the i18ntest command from Task 1. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/imports locales
git commit -m "feat: shorten Apple Wallet step 2, move token details behind a toggle"
```

### Task 3: Regression plan and full verification

**Files:**
- Modify: `docs/regression-test-plan.md` §5a (the step 2, step 3/4, step 5 items around lines 496–540)

- [ ] **Step 1: Update the checklist items**
  - Step 2: text is one line ("Tap Connect the Shortcut, then tap Always Allow on every prompt."); "What does this do?" expands the token explanation.
  - Step 3: numbered 5-item checklist (version-neutral); "Open Shortcuts" (iOS only 📱) opens the Shortcuts app; "iOS 26" / "iOS 27" open the article at `#ios-26` / `#ios-27` in a new window; desktop shows the links but not the button.
  - Step 5: same button and links (`#on-ios-26` / `#on-ios-27`); Done still disabled until the first payment.
  - New item: on a 320 px-wide phone the link row and buttons in steps 3 and 5 wrap without horizontal scrolling, in German and Russian too 📱.

- [ ] **Step 2: Full verification**

Run: `cd web && pnpm test && pnpm lint && pnpm build`
Run: `GOTOOLCHAIN=go1.27.1 PATH=/usr/local/go/bin:$PATH make go-test`
Expected: all PASS (go-test covers i18ntest; no Go code changed).

- [ ] **Step 3: Commit**

```bash
git add docs/regression-test-plan.md
git commit -m "docs: regression items for the clearer Apple Wallet setup guide"
```
