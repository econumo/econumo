# Reporting tags: one noun, purpose-phrased creation, budget folder

**Date:** 2026-08-05
**Status:** Design approved. Ready for implementation planning.
**Amends:** [2026-08-02-multi-tag-labels-design.md](2026-08-02-multi-tag-labels-design.md)

## Why this amendment

The multi-tag labels spec shipped a hybrid vocabulary: two backend entities
(`tags` + `labels`), unified in the UI under the umbrella word "Tags" but
naming each kind — "label" — wherever the two had to be told apart. Building it
out revealed the cost: a user meets the word "label" on the settings page, in
the budget view, in CSV import, and in error messages, and nothing in the
product ever teaches what a label is or why it differs from a tag. The name
describes the mechanism (it labels things) rather than the purpose (reporting).

This amendment settles the vocabulary question and, with it, two smaller
decisions that were left as "hardcoded per kind" in v1, plus a reshape of the
budget surfacing that the flat list could not deliver.

**Nothing in the data model changes.** Migrations, tables, join tables, wire
field names (`labelIds`), the `internal/label/` package, MCP tools, analytics
keys, and the CSV column layout are all untouched. This is a copy, dialog,
color, validation, and budget-rendering change on top of the branch as built.

## Concept

Everything the user creates is a **tag**. A tag has a **purpose**, chosen at
creation:

- **budgeting** — one per transaction, carries limits, participates in envelope
  math and carryover (today's tag, unchanged);
- **reporting** — several per transaction, budget-neutral, surfaces spend for
  "where did the money go" questions (today's label).

The word "label" disappears from every user-facing surface in all 12 languages.
Backend naming stays exactly as it is — `labels` table, `internal/label/`,
`labelIds` on the wire — so this costs no schema or contract work. The
divergence between backend and UI naming is deliberate and worth stating: the
backend name records the *mechanism* (a separate table that structurally cannot
enter budget math), the UI name records the *purpose*.

Where the two kinds must be distinguished in the UI, they are distinguished by
**qualifier, not by a second noun**: "budget tag" and "reporting tag" — the way
"checking account" and "savings account" are both accounts.

## Create/edit dialog

The dialog teaches the distinction at the moment of choice, phrased by purpose
rather than by mechanism. Name input, then a radio with two options and **no
legend heading** (the options explain themselves):

> ○ **Budget money for this tag** — set a limit and track it in your budget. One per transaction.
> ○ **Label transactions for reporting** — see where money went without affecting your budget. Several per transaction.

- The **icon preview** sits next to the name field and swaps live with the radio
  selection: `#` (Material ligature `tag`) for budgeting, the label shape
  (ligature `label`) for reporting. This is the existing live-preview behavior,
  retained.
- **On edit the radio is disabled**, with a short note that purpose cannot be
  changed after creation. This matches the existing non-goal: there is no
  in-place conversion path on the backend, and a user who needs to convert
  re-creates and re-assigns.
- The primary/secondary description text under each option is a catalogue
  string per language, not a hardcoded English sentence.

## Color

**One accent color for both kinds**, replacing the current per-kind pair. The
`#` and label-shape icons are distinct enough on their own, and a single color
reads calmer in a dense budget table.

`kindAccentClass()` (`web/src/lib/classificationKind.ts`) collapses to a single
constant but is **kept as a function**, so call sites do not churn and a future
icon/color picker can reintroduce variation without re-threading every consumer.

## Wording across surfaces

| Surface | Now | Becomes |
|---|---|---|
| Settings menu item + page title | "Tags" | "Tags" (unchanged) |
| Settings sections | "Tags" / "Labels" | "Budget tags" / "Reporting tags" |
| Settings info box | mentions labels | rewritten around the two purposes |
| Create button | "Create tag or label" | "Create tag" |
| Budget block | "Labels" section | "Reporting tags" folder (see below) |
| Budget overlap note | "several labels" | "several reporting tags" |
| Transaction preview field | "Tags and labels" | "Tags" |
| CSV import fields | "Tag" / "Labels" | "Budget tag" / "Reporting tags" |
| CSV separator dialog | "Label separator" | "Tag separator" |
| Import preview count | "N new labels" | "N new tags" |
| Errors (`errors.label.*`) | "Label already exists" | "Tag already exists" |

Russian follows the same shape — «Теги для бюджета» / «Теги для отчётов», never
«метки» — and so do the other ten catalogues. This is a values-only change per
language; the key structure barely moves.

### Catalogue key handling

- Two currently-dead keys are **reused rather than deleted**:
  `classifications.tags.pages.settings.info` becomes the new two-purpose info
  text, and `.create_tag` becomes the plain "Create tag" button.
- `tags_and_labels_info` and `create_tag_or_label` are then unused and are
  **removed** (all 12 languages).
- The `classifications.labels.*` subtree **keeps its keys** — the code paths
  still reference them for section captions, delete dialogs, and validation —
  but its **values** change to the qualified wording.
- New keys for the two radio option descriptions.

## Cross-kind name uniqueness

Because both kinds are "tags" to the user, a budget tag `trip` coexisting with a
reporting tag `trip` reads as a duplicate-name bug — in pickers, in the settings
list, and most sharply in CSV import's name matching, where it is genuinely
ambiguous which one a cell means.

**Creating or renaming a tag of either kind fails if any tag of either kind
already has that name for the owner**, compared **case-insensitively**.

Note this is *not* what the existing per-kind check does: `ensureNameUnique` in
both `internal/tag/usecase.go` and `internal/label/usecase.go` compares exactly
(`t.Name == name`), so today "Trip" and "trip" coexist within a kind. **Both the
same-kind and the cross-kind comparison move to case-insensitive**
(`strings.EqualFold`), so the rule is one sentence a user can hold: one name,
one tag, regardless of case or purpose. This aligns with CSV import, which
already matches names case-insensitively — today a CSV cell `trip` matching an
existing `Trip` reuses it, while the create endpoint would have allowed a
duplicate.

The same-kind change is a **behavior change for existing data**: a user who
already has "Trip" and "trip" as separate tags keeps both (nothing is migrated
or merged), but renaming either onto a case-variant collision now fails. This is
acceptable — the pair was always confusing — and needs its own test.

The error is the existing
`errors.tag.already_exists` ("Tag already exists"), so
`errors.label.already_exists` becomes unused and is **removed**, along with its
code in `internal/shared/errs/codes.go` and its entry in all 12 catalogues (the
`i18ntest` two-way coverage guard enforces that all three move together).

**Mechanically**, this is a cross-feature lookup and follows the dependency rule
the codebase already uses: `internal/tag/ports.go` gains a name-exists port
implemented over the label repo, wired by a `glue_` file in `internal/server`;
symmetrically for `internal/label`. No shared table, no schema change, no
feature importing a feature.

**CSV import inherits this** for free: its find-or-create resolves a name
against one kind and now collides with the other kind rather than silently
creating a twin. The collision surfaces as a **per-row error** through the
existing row-error reporting — the honest outcome, since the CSV asked for a
reporting tag and the user already has a budget tag by that name.

**Existing data:** the feature is unreleased, so no production data carries a
same-name pair. No migration, no backfill. The check applies from the moment it
ships.

## Budget view: reporting tags become a folder

### Shape

Today the reporting block is a bordered section headed "Labels" holding a flat,
one-line-per-label list. It becomes a **folder named "Reporting tags"**,
rendered like the budget's real folders and **collapsed by default**. Expanding
it reveals one row per reporting tag; each of those rows is **itself
expandable**, and expanding one shows its **category breakdown** — the same
nested child rows a budget tag element shows today.

The hierarchy therefore gains a level:

```
Reporting tags            (ephemeral folder, collapsed by default)
└── kid-A                 (reporting tag, collapsed by default)
    ├── Groceries         (category child, spend only)
    └── Uncategorized     (category child, spend only)
```

This answers "where did kid A's money actually go" inside the budget view,
which the flat list could only do by opening the drill-down dialog.

### Ephemeral means it is not real

The folder exists **only in the response**. It has no `budgets_folders` row,
cannot be renamed, moved, deleted, or receive dragged elements, and does not
appear in the edit-structure surface. It renders **after the real folders and
after Uncategorized**, at the bottom.

### Collapse state

Real folders and elements persist their unfolded state in `budgetStore.ts`. The
reporting-tags folder and its tag rows use **that same store** under reserved
synthetic ids, so a user who expands a tag keeps it expanded across period
changes — consistent with every other row in the table. Default is collapsed,
both for the folder and for each tag inside it.

### Amounts and columns

Unchanged in meaning. A reporting tag row shows **only its period spend** — no
budgeted, no available, because it has no limit and that is the entire point.
Category children likewise show spend only. The **overlap note stays**, moved to
just inside the expanded folder, so the caveat appears exactly where the numbers
do.

A reporting tag's children **do** sum to that tag's total. The tags still
overlap with **each other** — unchanged from the original spec, now visible two
levels deep, which is why the note remains load-bearing.

### Drill-down

The spend figure on a reporting tag row keeps opening the transactions dialog
filtered to that tag. **Category children get the same treatment**, filtered to
tag + category together, mirroring how a budget tag's child rows already pass a
`parent` in their click target.

This is **more than reusing an existing branch**, and the current code actively
forbids it. `internal/budget/txlist.go` guards `labelId` as mutually exclusive
with every other selector — combined with `categoryId`, `tagId`, `envelopeId`,
or `uncategorized` it is rejected up front with
`CodeBudgetTransactionFilterRequired`, deliberately, so that no combination
silently picks one selector. (The existing combined tag+category branch in the
switch serves **budget** tags, a different path.) Enabling category-child
drill-down therefore requires:

- **relaxing that guard** to permit exactly `labelId + categoryId` and
  `labelId + uncategorized`, while still rejecting `labelId` with `tagId` or
  `envelopeId` — the combinations that remain genuinely ambiguous;
- **new switch cases** for those two combinations;
- **new read-repo methods** (`BudgetTransactionsByLabelAndCategory` and the
  uncategorized variant) mirroring `BudgetTransactionsByLabel`, joining
  `transactions_labels` and filtering on category, for both engines;
- `BudgetTransactionsParams` on the frontend carrying both fields together.

The relaxed guard needs its own tests in both directions: the newly-permitted
pairs succeed, and `labelId + tagId` / `labelId + envelopeId` still fail with the
same code as today.

### Backend work

- `LabelSpendResult` gains a `children` array of the existing
  `ChildElementResult` shape.
- `CountSpendingByLabel` currently groups by `(label_id, currency_id)`; it gains
  `category_id` to the grouping. Its independence from `CountSpending` — the
  reason the many-to-many fan-out is safe — is **unchanged and still
  load-bearing**.
- The builder rolls those rows up into per-tag totals plus per-category
  children, the same two-level assembly the tag path performs, through the
  **same bulk currency conversion**.
- Visibility rules carry over unchanged: a reporting tag appears only with
  non-zero spend (it has no limit that could keep it on screen), and a category
  child only with non-zero spend.
- Transactions with no category land in an **uncategorized child**, reusing the
  existing uncategorized id and display-name handling rather than introducing a
  new concept.

## Testing

**Frontend.** `TagsPage.test.tsx` currently asserts "Tags" renders exactly twice
(title plus section caption); that becomes one title plus two qualified
captions. `TagDialog` tests gain radio-copy and disabled-on-edit assertions. Any
per-kind color assertion in `ClassificationChips` is dropped. New `BudgetTable`
tests cover default-collapsed, expand-to-tags, expand-to-categories, and that
the drill-down target carries the category.

**Backend.** New unit tests for both directions of the uniqueness check (create
budget tag colliding with a reporting tag, the reverse, and the rename path), an
`apiparity` scenario for the resulting 400, and a CSV-import test that a
cross-kind name collision becomes a row error. New tests for the two-level
budget rollup, the uncategorized child, and that a multi-tag transaction
contributes its full amount to each tag's children **without** double-counting
anything in envelope math.

**Goldens.** Budget goldens change (nested children in the labels block);
`apiparity` goldens shift where error message text changed. Regenerate with
`UPDATE_GOLDEN=1` and **inspect the diff** — a golden change means observable
behavior changed. `enginecompare` must stay byte-identical across engines.
`i18ntest` parity guards cover the catalogue changes and the removed error code.

## Decisions log

- **Vocabulary:** one user-facing noun, "tag". "Label" is removed from the UI in
  all 12 languages; backend naming (`labels`, `internal/label/`, `labelIds`) is
  deliberately left alone.
- **Distinguishing the kinds:** qualifiers ("budget tag" / "reporting tag"), not
  a second noun.
- **Creation:** purpose-phrased radio with descriptions, no legend heading;
  disabled on edit (no conversion path).
- **Color:** one accent color for both kinds; `kindAccentClass()` kept as a
  function for future variation.
- **Uniqueness:** one namespace across both kinds, case-insensitive, per owner;
  `errors.label.already_exists` and its code removed.
- **Budget surfacing:** ephemeral "Reporting tags" folder, collapsed by default,
  with expandable per-tag category breakdown; renders last; state persisted in
  `budgetStore` under reserved synthetic ids.
- **Drill-down:** extended to category children (tag + category), via the
  existing combined branch in `txlist.go`.
- **Unchanged:** data model, migrations, join tables, wire field names, MCP
  surface, analytics keys, CSV column layout.
