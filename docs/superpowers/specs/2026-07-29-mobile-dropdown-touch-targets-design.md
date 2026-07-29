# Mobile dropdown touch targets

## Problem

Picking an account, category, or payee while entering a transaction on a phone is
error-prone: the option rows are too small to hit reliably with a thumb.

Measured on the current code:

| Element | Classes | Height |
| --- | --- | --- |
| `ComboboxItem` (`components/ui/combobox.tsx:159`) | `py-1 … text-sm` | 4 + 20 + 4 = **28px** |
| `SelectItem` (`components/ui/select.tsx:115`) | `py-1 … text-sm` | 4 + 20 + 4 = **28px** |

Both are well under the 44px minimum touch target. Neither file contains a single
`sm:`, `md:`, or `max-md:` variant — the picker renders at desktop density on every
viewport.

A secondary defect surfaced while measuring: the closed picker field
(`features/transactions/EntitySelect.tsx:107`) is `text-sm` (14px). iOS Safari
force-zooms the page when a focused input's font size is below 16px, so tapping any
picker in the transaction form zooms the viewport.

## Precedent

`components/ui/command.tsx` already solves exactly this, and is the pattern this
change follows rather than inventing a second convention:

- `:74` input group — `h-8! … max-md:h-11!`
- `:78` input — `text-sm … max-md:text-base`
- `:158` item — `py-1.5 text-sm … max-md:py-3 max-md:text-base`

`components/CurrencyPickerDialog.tsx:36` sets the matching list cap: `max-h-72 max-md:max-h-96`.

The breakpoint is `max-md:` (below 768px), matching `command.tsx` and the
`useIsMobile` hook. It must be a CSS variant, not a JS hook — these are style-only
changes inside primitives that render on both viewports.

## Scope

Deliberately app-wide at the primitive level, which is contained in practice:

- `ComboboxItem` / `ComboboxList` — one consumer, `features/transactions/EntitySelect.tsx`,
  itself used only by `TransactionDialog.tsx` (account, from/to account, category, payee).
- `Select` — two consumers, `features/budgets/EnvelopeDialog.tsx` and
  `features/connections/SharingRequestsDialog.tsx`.

Fixing the primitives rather than overriding from `EntitySelect` keeps one sizing
convention and covers future pickers automatically.

Out of scope: the tag row (chips, not a dropdown), the date `Popover` + `Calendar`,
and every desktop rendering.

## Changes

### 1. Row height — the core fix

Append to `ComboboxItem` (`components/ui/combobox.tsx:159`) and `SelectItem`
(`components/ui/select.tsx:115`):

```
max-md:py-3 max-md:text-base
```

12 + 24 + 12 = **48px** below 768px, clearing the 44px minimum. Above 768px the class
string is unchanged, so desktop output is byte-identical.

### 2. List height — a required consequence of (1)

`ComboboxList` (`components/ui/combobox.tsx:132`) caps at a fixed 252px:

```
max-h-[min(calc(--spacing(72)---spacing(9)),calc(var(--available-height)---spacing(9)))]
```

At 28px rows that shows 9 options; at 48px rows it would show 5. Taller rows alone
would trade a tapping problem for a scrolling one, so the mobile cap rises with them:

```
max-md:max-h-96
```

384px ≈ 8 rows, preserving roughly today's scannability. The `var(--available-height)`
term in the existing expression is a separate `min()` operand computed by Base UI from
the real viewport, so it still clamps the popup — `max-h-96` cannot overflow the screen.

`SelectContent` needs no equivalent: it already sizes off
`max-h-(--radix-select-content-available-height)` with no fixed cap.

### 3. Closed field — iOS zoom

`EntitySelect.tsx:107` (the `data-slot="entity-select"` row) and `SelectTrigger`
(`components/ui/select.tsx:47`) each gain:

```
max-md:text-base
```

and, for the height, `max-md:h-11` on `EntitySelect` / `max-md:data-[size=default]:h-11`
on `SelectTrigger`.

The height addition is inert inside the transaction form by design. `cardSelectClass`
(`TransactionDialog.tsx:37-38`) already strips the field to `h-auto` via
`[&_[data-slot=entity-select]]:h-auto`, whose descendant selector has higher CSS
specificity (0,2,0) than `.max-md\:h-11` (0,1,0). Specificity outranks media-query
order, so card layouts keep `h-auto` and are unaffected. The class is added for
non-card usages of the primitive.

`text-base` is **not** stripped by `cardSelectClass`, so it does apply inside the
cards. That is the accepted cost: picker text in the transaction form's cards goes
14px → 16px on mobile, in exchange for killing the iOS zoom-on-focus.

## Verification

This is a style-only change; the value is visual, so verification is primarily visual.

1. `cd web && pnpm exec tsc -b` — required before calling the frontend done; vitest and
   oxlint do not type-check.
2. `make web-lint` and `make web-test` — the existing `EntitySelect.test.tsx` must still
   pass. No new class-string assertions: asserting Tailwind strings in a test duplicates
   the source without proving a rendered size.
3. Drive the app at a 390×844 viewport and confirm, in the transaction form:
   option rows measure ≈48px, the open list is taller than before, and focusing a
   picker no longer zooms the page.
4. Spot-check `EnvelopeDialog` and `SharingRequestsDialog` on mobile for the `Select`
   change, and any one desktop viewport to confirm nothing moved.

## Risks

- **Fewer visible options despite (2).** 8 rows instead of 9. Judged acceptable; the
  reported pain is tapping, not scrolling.
- **Card text growth.** Explicitly accepted above; the only visible change to the
  transaction form's closed state.
- **`max-md:` is 768px, `useIsPhone` is 639px.** Small tablets get the larger targets
  while the dialog still renders in its non-full-screen form. Consistent with
  `command.tsx` today, and touch targets on a tablet are not a regression.
