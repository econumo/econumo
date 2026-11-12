# Debts belong to a budget, with several payment rules

**Date:** 2026-11-12 · **Status:** design, awaiting approval · builder-checked
**Builds on:** `2026-10-29-budget-debts-design.md` and `2026-11-07-debt-setup-redesign-design.md`
(branch `feature/budget-debts`, PR #424, not released). Everything not changed here stays as those
designs and the code on the branch describe it.

## Why

A couple shares a mortgage. Sometimes he pays it, sometimes she does; each records the payment from
their own account under their own "Mortgage" category. Today a debt belongs to one user and matches
one of that user's categories, so neither of them can track the shared loan: his debt never sees her
payments, and she cannot track his category at all ("Track as debt" is offered only on your own
category). The loan is the household's, and the household's place in Econumo is the shared budget.

## Decisions (agreed with the owner)

1. **A debt belongs to one budget**, not to a user. There is no owner. A loan tracked in two budgets
   is two independent debts; the user creates the second one by hand. Nothing links them.
2. **Access follows the budget's roles.** Anyone who can plan in the budget (owner, admin, user) can
   create, finish setup, edit, reconcile, mark paid off and delete its debts. A guest sees the debts
   read-only.
3. **A debt has one or more payment rules.** A rule is what a single debt matched before: a
   category, optionally narrowed by a payee and by a tag or a reporting tag (tag OR label, as now).
   The category may belong to any participant of the budget; its payee/tag/label are that same
   participant's (classifications are per user). A payment counts when it matches ANY rule.
4. **Only the budget's accounts count.** A payment matches only when it is on one of the budget's
   member accounts. A payment on an account outside the budget does not exist for the debt, exactly
   as it does not exist for the budget.
5. **"Track as debt" works on any participant's expense category** in the budget, and the row menu
   also offers **"Add to a debt…"** (shown when the budget has at least one debt the caller can
   edit): it opens a modal listing the budget's debts to pick from; picking one adds the category
   as another rule, and the category row's plans move onto the debt row, adding to what is there.
6. Settings → Debts lists the debts of every budget the user is in, grouped by budget.
7. When a participant leaves the budget, their accounts leave with them, so their payments stop
   counting. Their rules stay on the debt and simply match nothing.

## Model

- `debts` loses `user_id`, `category_id`, `payee_id`, `tag_id`, `label_id`; gains `budget_id`
  (FK `budgets`, ON DELETE CASCADE, indexed). `created_day` is the creation day in the CREATING
  user's stored timezone (UTC when none), still fixed at creation.
- **Schema changes go in a NEW migration** (both engines, same version, numbered above every
  migration on `main` and above `20261110000500`), never by editing `20261110000500`: the `dev`
  image published from this branch has already applied it to real databases. The new migration
  **deletes every existing debt** (agreed: the feature is unreleased), with its snapshots and
  splits, and every budget element row of type `ElementDebt` with its plans and comments, then
  reshapes the tables. Nothing is converted.
- New `debts_rules`: `id`, `debt_id` (FK, cascade), `category_id`, `payee_id`, `tag_id`, `label_id`
  (FKs NO ACTION as today, so the coded in-use check refuses a delete), `created_at`; indexed by
  debt and by each classification. A debt is created with at least one rule and an edit may not
  remove the last one; only a classification delete (below) can leave a debt with none, and such a
  debt matches nothing until someone adds a category.
- The start date stays on the debt (one loan, one start). A debt keeps ONE date range: when a later
  debt of the same budget has a rule overlapping any of its rules, the earlier debt stops matching
  at the later debt's start date for ALL its rules (the refinancing case: both Mortgage categories
  move to the new loan). *My call.*
- `debts_payments` keeps one row per (debt, transaction): its primary key becomes
  `(debt_id, transaction_id)`, because one transaction can now count for debts in two different
  budgets. Within one budget a transaction still counts on exactly one debt.

## Matching

A transaction is a payment of debt D (budget B) when it is an expense, on a member account of B
(soft-deleted members included, the same population the budget counts), dated inside D's range, and
matches at least one of D's rules (category, and payee/tag/label when the rule names them; a rule
with a label needs the transaction to carry it).

- **Conflicts are checked within a budget, per rule**, with today's rules applied rule by rule:
  overlapping keys (category alone vs category + payee, labelled vs unlabelled) are refused
  (`debt.key_conflict`), two debts whose overlapping rules name different labels coexist, identical
  keys need distinct start dates (`debt.start_date_taken`). Two rules of the same debt that overlap
  are refused too. Debts of different budgets never conflict.
- **Tiebreak** within a budget: a transaction matching several debts belongs to the older debt
  (`created_at`, then `id`), as now.

## Budget integration

- A debt has exactly one row (`ElementDebt`), in its own budget only (today: in every budget of the
  owner). Attribution, Paid, the drill-down, planning bounds, shifted periods, the red hint and
  totals are unchanged, now scoped to that budget.
- **Plan moves are per rule.** A rule with no payee, tag or label, whose category no other debt of
  the budget covers, is a whole-category rule; creating the debt, adding such a rule ("Add to") or
  editing a rule into one moves that category row's plans and comments onto the debt row (in this
  budget only; a month both rows plan adds up), within the debt's planning life, as today.
- **Removing a rule** leaves the debt row's plans where they are (they cannot be split back by
  category). *My call.*
- **Deleting a debt** hands its plans back only when the debt has exactly one rule and that rule is
  whole-category (to that category, as today); otherwise its plans and comments are dropped after
  `confirmDataRemoval`. *My call.*
- Deleting the budget deletes its debts. `clone-budget` copies the debts (rules, snapshots, start
  date, terms) with new ids, and their rows with them; splits rebuild on the next read. *My call.*
- An archived budget's debts are read-only, like everything in it.

## Classifications

- **Deleting a classification a rule uses is no longer refused** (agreed): `category.used_by_debt`,
  `payee.used_by_debt`, `tag.used_by_debt` and `label.used_by_debt` go away. Deleting a category
  removes every rule naming it, in every budget, in the same transaction (the delete never depends
  on whether its owner can edit those debts: someone who left the budget or is a guest must still be
  able to clean up their own categories). Deleting a payee, tag or label a rule narrows by removes
  that narrowing from the rule (*my call*: the rule then covers more, which is visible on the
  shared debt; the alternative of dropping the rule would silently lose payments). If that makes
  two rules of one debt identical, one is dropped; if it makes rules of two debts overlap, the
  OLDER debt keeps the rule and the newer debt's rule is removed (*my call*).
- Payments already split keep their splits; the next sync drops splits of payments that no longer
  match, as for any edit.
- A merge moves the rules to the target classification; a rule that would then duplicate or
  conflict is resolved like a delete above, never refused (the merging user may have no access to
  the debt). Merges therefore never return `debt.key_conflict`.
- Archiving a category is allowed, as now.

## Currency

A new debt takes the converted category row's currency, else the budget's currency (today: the
user's). Changing it stays as now.

## Locking

The debt sync and every debt write lock the BUDGET row (today: the owner's user row), taken before
any budget element row. *Left to the builder:* keep a single global lock order and recheck the
places that took the owner's lock (merges' `PrepareMerge`, partner revocation, onboarding).

## API (unreleased, so the wire may change)

- Every debt route checks the caller's access to the debt's budget: read for `get-debt`,
  `get-debt-list`, `get-debt-forecast`; plan access (owner/admin/user) for every write. No access =
  not found, as today for another user's debt.
- `get-debt-list` returns the debts of every budget the caller can read, each with `budgetId` and
  `budgetName`.
- `create-debt` takes `budgetId` and `rules: [{categoryId, payeeId?, tagId?, labelId?}]` (1..10)
  instead of the single category/payee/tag/label; `update-debt` takes the full `rules` list
  (replace-set, ids for kept rules optional) and may not change `budgetId`.
- New `add-debt-rule` (`{id, rule}`) behind "Add to a debt…", returning the debt. *My call:* a
  separate route keeps the menu action a single request with its own plan move.
- `DebtResult` replaces `categoryId/payeeId/tagId/labelId` with `rules[]` (each with the category's
  name/icon and its owner's display name for the SPA) and adds `budgetId`, `budgetName`, `canEdit`.
- Budget rows: `structure.debts[].ownerUserId` goes away; the SPA reads `canEdit` from the budget's
  access instead.

## SPA

- **Create form:** "Payments from" becomes a list of rule sentences, one per category ("Payments in
  [her Mortgage] · from [any payee] · tagged [any tag]"), with "+ Add a category" and a remove button
  on each line but the last. The category picker lists every participant's expense categories,
  grouped by participant (own first), with `rankByName`. From Settings → Debts the form also asks
  for the budget (default: the default budget); from a budget row the budget is that row's.
- **Row menu** (any plan-capable participant): Details, Mark as paid off, Delete as now, plus on
  category rows "Track as debt" and "Add to a debt…". The latter opens a modal (`ResponsiveDialog`,
  a `PickerSheet` on a phone) listing the budget's debts with their icon, name and current rules
  ("Payments in his Mortgage, her Mortgage"), searchable with `rankByName` when long; a debt whose
  rules already cover the category, or that a new rule would conflict with, is shown disabled with
  the reason. Picking one sends `add-debt-rule` and closes; a refusal shows inline.
- **Review screen:** opens for every participant; guests see it read-only (no Settings, Balance
  form or Danger zone actions). "Which payments count" lists the rules, each naming the category's
  owner when it is not the viewer ("her Mortgage" style: the owner's display name).
- **Settings → Debts:** grouped by budget, each group headed by the budget name.
- **Onboarding** auto-detection creates its debts in the budget the step created or reused (the
  default budget), one rule each. The plan handover (`TakeCategoryPlans`) works per budget.
- Analytics: `DEBT_RULE_ADD` (a rule added from the "Add to a debt…" modal); the others keep firing.

## Non-goals

- Linking one loan across budgets.
- Per-person shares of a debt (who paid how much) beyond what the payments list shows.
- Matching transfers.

## Left to the builder

- Wire field details beyond the names above, rule ordering, the rule limit (10 suggested).
- Exact lock order and the archtest/ports changes (the debt feature now needs budget access through
  a port, the budget feature already reads debts through `DebtSource`).
- How the review screen's Settings form edits the rule list (inline sentences vs a small list).
- Updating CLAUDE.md, both earlier specs' pointers, the regression test plan, the 11 locales,
  apiparity/mcpparity goldens, revisionguard.
