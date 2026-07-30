# Optional Transaction Category — Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the transaction form save without a category and clear an existing one, label categoryless transactions "Uncategorized", and render the backend's Uncategorized budget element as a read-only row whose spent drills down correctly.

**Architecture:** Every piece of plumbing already exists. `buildPayload` already sends `categoryId: null`, `EntitySelect` already has a `clearable` prop that emits null, and `BudgetTable` already has a per-row mechanism for read-only rows (the archive section). The work is removing one validation branch, adding one prop, one label helper, and one request flag.

**Tech Stack:** React 19 + TypeScript, vitest + testing-library + msw, oxlint, i18next.

This is the frontend half of `docs/superpowers/specs/2026-07-27-optional-transaction-category-design.md`. The backend half (`2026-07-28-optional-category-backend.md`) must be complete first — this plan consumes the wire contract it produces.

## Global Constraints

- Branch: `feature/optional-transaction-category` (shared with the backend plan).
- `pnpm exec tsc -b` MUST pass. vitest and oxlint do **not** type-check.
- `pnpm lint` (oxlint) clean.
- Translation catalogues are at the **repo root** (`locales/en.json`, `locales/ru.json`), shared by both stacks. Every key must exist in **both**, with identical `{var}` placeholder sets — `internal/test/i18ntest` fails the Go suite otherwise. Adding a key to only one file breaks `make go-test`.
- Deleting a key means deleting every `t()` call that uses it, in the same change — `i18ntest` checks both directions.
- Comments only for non-obvious *why*. Never reference the removed PHP/Symfony implementation.
- No new `METRICS` key: `TRANSACTION_CREATE` / `TRANSACTION_UPDATE` already fire in `web/src/features/transactions/queries.ts:42,53`, and `metrics-coverage.test.ts` fails on an unfired catalogue key.
- The wire `spent` value is positive and must never be rendered negated.

## Wire contract this plan consumes

The backend emits a presentation-only budget element:

| field | value |
| --- | --- |
| `id` | `"uncategorized"` (not a UUID) |
| `type` | `1` (CATEGORY) |
| `name` | `"Uncategorized"` (English literal) |
| `icon` | `"question_mark"` |
| `budgeted` | `"0"` |
| `folderId` | `null`, with a large `position` so it sorts last in the no-folder section |
| `children` | `[]` |

The same id also appears as a **child** of a tag element, for spending that is tagged but uncategorized.

The drill-down request gains `uncategorized: boolean`, which composes with `tagId`:
- `{uncategorized: true}` → categoryless, untagged transactions
- `{uncategorized: true, tagId: T}` → categoryless transactions carrying tag T
- `{uncategorized: true, categoryId: X}` → a validation error; never send both

**Localization decision.** The wire `name` stays an English literal — the budget response is data and carries no language, and MCP clients read the same payload. The SPA substitutes a translated label whenever it sees the sentinel id. That keeps the wire stable and language-free.

## File Structure

| File | Responsibility | Change |
| --- | --- | --- |
| `web/src/features/transactions/TransactionDialog.tsx` | Transaction form | Modify — drop `:162-164`, add `clearable` |
| `web/src/features/transactions/useAccountTransactions.ts` | Row title derivation | Modify — `:133` |
| `web/src/features/transactions/ViewTransactionDialog.tsx` | Detail sheet | Modify — `:33` |
| `web/src/api/dto/budget.ts` | Budget wire types | Modify — export the sentinel id |
| `web/src/api/budget.ts` | Budget API client | Modify — `:114-131` |
| `web/src/features/budgets/BudgetTable.tsx` | Budget rows | Modify — `:301`, name display |
| `web/src/features/budgets/BudgetTransactionsDialog.tsx` | Drill-down | Modify — `:60-78`, title |
| `locales/en.json`, `locales/ru.json` | Catalogues | Modify — add `common.uncategorized`, delete the required-field key |

No new files except tests.

---

### Task 1: Let the form save without a category, and clear one

**Files:**
- Modify: `web/src/features/transactions/TransactionDialog.tsx:150-167` (`validate`) and the category `EntitySelect` at `:362-380`
- Modify: `locales/en.json:345-347`, `locales/ru.json:345-347`
- Test: `web/src/features/transactions/` — the existing `TransactionDialog` test file

**Interfaces:**
- Consumes: `EntitySelect`'s `clearable?: boolean` prop (`web/src/features/transactions/EntitySelect.tsx:26`). When set and a value is present, it renders a `—` row (`:66`) whose selection calls `onChange(null)` (`:84`). Nothing else is needed — `onChange` is already typed `(value: string | null) => void` (`:21`).
- `buildPayload` already sends `categoryId: isTransfer ? null : form.categoryId` (`useTransactionForm.ts:96`) — **no change needed there**.

- [ ] **Step 1: Write the failing tests**

Read the existing `TransactionDialog` test file first and match its harness. Add two tests:

1. **Submitting an expense with no category succeeds** — open the dialog, fill in the required fields (amount, account, date), leave the category empty, submit. Assert the create request fired and its body has `categoryId: null`. Assert no validation message appears. Capture the request body via msw, the way sibling tests do.
2. **An existing category can be cleared** — open the dialog on a transaction that has a category, clear it via the select's `—` row, submit. Assert the update request body has `categoryId: null`.

Assert on the **request body**, not merely that the dialog closed. A test that only checks for the absence of an error message would pass even if the form silently dropped the field.

- [ ] **Step 2: Run to verify they FAIL**

Run: `cd web && pnpm exec vitest run src/features/transactions/TransactionDialog.test.tsx`
Expected: test 1 FAILS on the "Category is required" validation message; test 2 FAILS because the select offers no way to clear (no `—` row).

- [ ] **Step 3: Remove the validation branch**

In `web/src/features/transactions/TransactionDialog.tsx`, delete these three lines at `:162-164`:

```ts
    if (!isTransfer && !form.categoryId) {
      next.category = t('transactions.modal.form.category.validation.required_field')
    }
```

Check whether `next.category` is read anywhere else in the file after this deletion; if that error slot becomes entirely unused, remove it from the error-state type too rather than leaving a dead field.

- [ ] **Step 4: Make the category select clearable**

In the same file, add `clearable` to the category `EntitySelect` (`:363-379`), placed between the `options` prop and `onCreate` to match how the payee select at `:383-400` orders it:

```tsx
                      clearable
```

- [ ] **Step 5: Delete the now-unused catalogue key**

The key `transactions.modal.form.category.validation.required_field` now has no `t()` call, and `i18ntest` fails on a catalogue key with no frontend usage.

In **both** `locales/en.json` and `locales/ru.json` the structure is identical and line-aligned:

```
343    "category": {
344      "label": "Category",        <- trailing comma must go
345      "validation": {
346        "required_field": "..."
347      }
348    }
```

Delete lines 345-347 and remove the trailing comma on line 344, leaving `"category": { "label": ... }`. Make the identical edit in both files. Verify both remain valid JSON.

- [ ] **Step 6: Run the tests**

Run: `cd web && pnpm exec vitest run src/features/transactions/` then `pnpm exec tsc -b && pnpm lint`
Expected: all pass, clean.

- [ ] **Step 7: Verify the i18n guards**

Run: `go test ./internal/test/i18ntest/ -v`
Expected: PASS. This is the guard that catches a key deleted from one catalogue but not the other, or a key with no remaining usage.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/transactions/TransactionDialog.tsx web/src/features/transactions/TransactionDialog.test.tsx locales/en.json locales/ru.json
git commit -m "feat(web): allow saving a transaction without a category

The payload already sent null and EntitySelect already had a clearable
mode; only the form's own validation branch forbade it."
```

---

### Task 2: Label categoryless transactions

**Files:**
- Modify: `web/src/features/transactions/useAccountTransactions.ts:133`
- Modify: `web/src/features/transactions/ViewTransactionDialog.tsx:33`
- Modify: `locales/en.json`, `locales/ru.json` (add `common.uncategorized`)
- Test: the existing test files for those two units

**Interfaces:**
- Produces: the catalogue key `common.uncategorized`, consumed again by Task 3.
- `transactionTitleInfo(tx, pageAccountId, t)` already takes `t` as a parameter (`useAccountTransactions.ts:105-109`) — no signature change.

- [ ] **Step 1: Add the catalogue key**

Add `"uncategorized": "Uncategorized"` to the `common` object in `locales/en.json`, and the Russian equivalent (`"Без категории"`) at the identical position in `locales/ru.json`.

`common` spans lines 2-151 and currently contains only nested objects — no flat scalar. Place the new key deliberately and identically in both files (immediately after the `econumo` block, which ends at `:22`, is a reasonable spot). The key takes **no `{var}` placeholders**, so placeholder parity is trivially satisfied.

- [ ] **Step 2: Write the failing tests**

Add to the existing test files:

1. **Transaction row** — a transaction with no category, no description, no tag and no payee renders "Uncategorized" as its title. Today it renders an empty title.
2. **Fallbacks unchanged** — a transaction with no category but **with** a description still shows the description, not "Uncategorized". This guards the existing fallback chain (category → description → tag → payee), which must not change.
3. **Detail sheet** — `ViewTransactionDialog` for a categoryless non-transfer shows "Uncategorized" as its hero name.

- [ ] **Step 3: Run to verify they FAIL**

Run: `cd web && pnpm exec vitest run src/features/transactions/`
Expected: tests 1 and 3 FAIL; test 2 should already PASS (it guards against over-correcting).

- [ ] **Step 4: Label the row title**

In `web/src/features/transactions/useAccountTransactions.ts`, replace the final return at `:133`:

```ts
  return { text: t('common.uncategorized'), source: 'none' }
```

Keep `source: 'none'` unchanged — `TransactionRow` gates its description/tag/payee sub-lines on `title.source` (`:60-74`), and this branch is only reached when all of those are absent anyway.

- [ ] **Step 5: Label the detail sheet**

In `web/src/features/transactions/ViewTransactionDialog.tsx:33`, the hero name currently falls back to the **type label** ("Expense"/"Income") when there is no category:

```ts
  const heroName = isTransfer ? typeLabel : (tx.category?.name ?? typeLabel)
```

Change the non-transfer fallback to the new label, leaving the transfer branch alone:

```ts
  const heroName = isTransfer ? typeLabel : (tx.category?.name ?? t('common.uncategorized'))
```

Confirm `t` is in scope in that component; if it is not, take it from `useTranslation()` the way the rest of the file does.

- [ ] **Step 6: Run the tests and the guards**

Run: `cd web && pnpm exec vitest run src/features/transactions/ && pnpm exec tsc -b && pnpm lint`
Then: `go test ./internal/test/i18ntest/`
Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/transactions/ locales/en.json locales/ru.json
git commit -m "feat(web): label a categoryless transaction Uncategorized

Applies where the title chain previously fell through to an empty
string, and where the detail sheet fell back to the bare type label."
```

---

### Task 3: The read-only Uncategorized budget row

**Files:**
- Modify: `web/src/api/dto/budget.ts` (export the sentinel)
- Modify: `web/src/api/budget.ts:114-131`
- Modify: `web/src/features/budgets/BudgetTable.tsx:301` and the name display
- Modify: `web/src/features/budgets/BudgetTransactionsDialog.tsx:60-78` and the title
- Test: `web/src/features/budgets/BudgetTable.test.tsx`, `web/src/features/budgets/BudgetTransactionsDialog.test.tsx`

**Interfaces:**
- Consumes: `common.uncategorized` from Task 2; the wire contract at the top of this plan.
- Produces: `UNCATEGORIZED_ID` exported from `web/src/api/dto/budget.ts`.

- [ ] **Step 1: Export the sentinel**

In `web/src/api/dto/budget.ts`, next to `BudgetElementType` (`:22-23`):

```ts
/** the presentation-only element the backend emits for spending with no category */
export const UNCATEGORIZED_ID = 'uncategorized'
```

- [ ] **Step 2: Write the failing tests**

In `BudgetTable.test.tsx`, using the `renderTable(mutator, extras)` pattern (a mutator pushes elements onto `budget.structure.elements`):

1. **Read-only** — push an element with `id: UNCATEGORIZED_ID`, `type: 1`, `folderId: null`, `budgeted: '0'`, `spent: '30'`. Assert it renders, that its budget cell is **not** an editor (no `renderBudgetCell` output), that no row actions appear, and that the available pill is not clickable — mirroring how archive rows behave.
2. **Spent is still clickable** — clicking its spent amount calls `onSpentClick` with that element as the target.
3. **Translated name** — the row displays the translated label, not the wire's raw `name`.
4. **Normal rows unaffected** — a sibling normal element in the same section still gets its budget cell and actions. This is the regression guard: the read-only treatment must be per-row, not per-section.

In `BudgetTransactionsDialog.test.tsx`, extending the request-URL capture already in that file:

5. **Top-level uncategorized** — target `{id: UNCATEGORIZED_ID, type: 1}` produces a request with `uncategorized=1` and **no** `categoryId`.
6. **Tag's uncategorized child** — target `{id: UNCATEGORIZED_ID, type: 1, parent: {id: 'tag-x', type: 2}}` produces `uncategorized=1` **and** `tagId=tag-x`, and **no** `categoryId`.
7. **Normal category unchanged** — a normal category target still sends `categoryId` and **no** `uncategorized`.

- [ ] **Step 3: Run to verify they FAIL**

Run: `cd web && pnpm exec vitest run src/features/budgets/`
Expected: 1, 2, 3, 5, 6 FAIL; 4 and 7 PASS (they guard existing behavior).

- [ ] **Step 4: Send the flag**

In `web/src/api/budget.ts`, add to `BudgetTransactionsParams` (`:114-120`):

```ts
  uncategorized?: boolean
```

and serialize it in `getBudgetTransactions` (`:122-131`). The existing lines all set string values, so a boolean needs its own line after the `envelopeId` line:

```ts
  if (params.uncategorized) query.set('uncategorized', '1')
```

- [ ] **Step 5: Build the right params**

In `web/src/features/budgets/BudgetTransactionsDialog.tsx`, the params block at `:60-78` currently folds the parent tag inside the CATEGORY branch. Keep that structure and swap what the CATEGORY branch contributes for the sentinel:

```tsx
  const isUncategorized = element?.id === UNCATEGORIZED_ID
  const parentTagId = element?.parent?.type === BudgetElementType.TAG ? element.parent.id : undefined
  const params = element
    ? {
        budgetId: budget.meta.id,
        periodStart: selectedDate,
        // The uncategorized element is presentation-only and has no id the
        // backend can look up, so it is selected by flag instead. It composes
        // with the parent tag exactly as a real category does.
        ...(element.type === BudgetElementType.CATEGORY
          ? {
              ...(isUncategorized ? { uncategorized: true } : { categoryId: element.id }),
              ...(parentTagId ? { tagId: parentTagId } : {}),
            }
          : {}),
        ...(element.type === BudgetElementType.TAG ? { tagId: element.id } : {}),
        ...(element.type === BudgetElementType.ENVELOPE ? { envelopeId: element.id } : {}),
      }
    : null
```

Never send `uncategorized` together with `categoryId` — the backend rejects that combination.

Also substitute the translated label for the dialog title (`:155`, `title={element.name}`) and the per-row fallback label (`:184`) when the element is the sentinel.

- [ ] **Step 6: Make the row read-only**

In `web/src/features/budgets/BudgetTable.tsx`, line `:301` already strips the editing hooks for the archive section:

```tsx
                  extras={isArchiveSection ? { onSpentClick: extras.onSpentClick } : extras}
```

Extend the same mechanism per-row, so a normal element beside it keeps its full extras:

```tsx
                  extras={isArchiveSection || element.id === UNCATEGORIZED_ID ? { onSpentClick: extras.onSpentClick } : extras}
```

This drops `renderBudgetCell`, `renderActions`, `renderRowWrapper` and `onAvailableClick` for that row, so the budget cell falls back to a plain value and the available pill renders un-clickable. It also means `useSetLimit`'s optimistic patch (`queries.ts:128-130`) can never be triggered for it.

Substitute the translated label where the row renders `element.name`.

- [ ] **Step 7: Run the tests and guards**

Run: `cd web && pnpm exec vitest run src/features/budgets/ && pnpm exec tsc -b && pnpm lint`
Then: `go test ./internal/test/i18ntest/`
Expected: all pass.

- [ ] **Step 8: Prove the tests discriminate**

Temporarily revert Step 6's condition back to `isArchiveSection ? ... : extras` and re-run test 1 — it MUST fail. Restore it and confirm the suite is green. Then temporarily change `{ uncategorized: true }` to `{ categoryId: element.id }` in Step 5 and re-run test 5 — it MUST fail. Restore. Include both outputs in your report.

Verify with `git diff` that both files are restored before committing.

- [ ] **Step 9: Commit**

```bash
git add web/src/api/dto/budget.ts web/src/api/budget.ts web/src/features/budgets/
git commit -m "feat(web): render the Uncategorized budget row read-only

Reuses the archive section's extras-stripping mechanism, applied
per-row. The row has no backend element id, so its drill-down selects
by flag and composes with a parent tag like a real category does."
```

---

### Task 4: Full verification

**Files:** none — verification only.

- [ ] **Step 1: Whole web suite under CI's timezone**

Run: `cd web && TZ=UTC pnpm exec vitest run`

**Use `TZ=UTC`.** This machine is PDT and CI is UTC; a date-dependent test recently passed locally and failed in CI purely because of that gap.

Expected: all pass.

- [ ] **Step 2: Type-check and lint**

Run: `cd web && pnpm exec tsc -b && pnpm lint`
Expected: clean. `tsc -b` is the only real type gate.

- [ ] **Step 3: Full stack suite**

Run: `make test`

Needs a PostgreSQL; a container named `econumo-verify-pg` may already be running from the backend plan. If not, see that plan's Task 5 for the command. Expected: PASS, including `enginecompare`, `i18ntest` and the frontend suite.

- [ ] **Step 4: Golden state**

Run: `git status --porcelain internal/test/apiparity/testdata/golden/ internal/test/mcpparity/testdata/golden/`
Expected: empty. This plan changes no backend response; any golden movement here means something is wrong.

---

## Self-Review

**Spec coverage.** Implements spec §2.6 (Tasks 1-3) and §2.7 (Tasks 1-2). Spec tests 38-39 → Task 1; 40-42 → Task 2; 43-44 → Task 3. Backend items are in the backend plan.

**Corrections to the spec made in this plan:**
- The spec said `ViewTransactionDialog` should show the label "instead of blank/broken". It is not blank: the category is the sheet's **hero**, and a null category already falls back to the type label ("Expense"/"Income"). Task 2 Step 5 changes that specific fallback rather than filling an empty field.
- The spec did not say how the element's name gets localized. The wire carries an English literal, so the SPA substitutes a translated label on the sentinel id; the wire stays language-free.
- The spec's "reuse the archive mechanism" is right, but the archive check is **per-section** (`BudgetTable.tsx:301`) while the uncategorized row lives in the ordinary no-folder section. Task 3 Step 6 makes the check per-row, and test 4 guards that a normal sibling row is unaffected.
- The spec listed suppressing `useSetLimit`'s optimistic patch as separate work. It follows for free from stripping the extras — there is no affordance left to fire it.

**Placeholder scan.** Test steps describe required assertions rather than transcribing bodies, because they depend on each file's existing harness (msw handlers, `renderTable`, `renderDialog`) which the implementer must read first. Every production edit contains the actual code.

**Type consistency.** `UNCATEGORIZED_ID` is defined in Task 3 Step 1 and used in Steps 5 and 6. `common.uncategorized` is added in Task 2 Step 1 and used in Task 2 Steps 4-5 and Task 3 Steps 5-6. `uncategorized?: boolean` on `BudgetTransactionsParams` matches the `{ uncategorized: true }` object built in Step 5.
