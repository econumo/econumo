# Budget MCP prompts + `move_element` tool — design

Date: 2026-07-22

## Problem

The three budget MCP prompts (`budget-setup`, `budget-review`, `budget-update` in
`internal/web/mcp/prompts.go`) are too thin. They do not explain what
`get_budget`'s fields mean, so a model reading them mis-interprets the
multi-currency and cumulative fields. Specifically:

- `available` is a **cumulative carryover** (all prior limits minus all spending
  from budget start through the end of the selected month), not "limit − spent
  this month" — but nothing tells the model that.
- `spent` is in the element's **own currency**; `budgetSpent` is the same amount
  converted to the **budget's base currency**. Comparing a limit against `spent`
  is wrong when currencies differ.
- `balances[]` / `currencyRates[]` (per-currency account summary) and
  `meta.access[]` / `ownerUserId` (multi-user) are undocumented in the prompts.

Beyond field docs, the prompts encode structure rules that no longer match what
we want:

1. `budget-setup` says "leave tags alone" — but tags are budget elements too and
   should be filed into folders alongside envelopes.
2. Nothing drives the **default (ungrouped) folder to empty**; elements are left
   scattered.
3. `budget-setup` allows a single-category envelope ("rent can stay its own
   envelope"). We want envelopes only for grouping **multiple** categories.
4. `budget-update` is a broad "reconcile + restructure" flow; we want it narrowed
   to **revise-only**, pushing big changes to `budget-setup`.

### Blocker discovered during research

Filing a **tag or a standalone category** into a folder is **impossible through
the current MCP tools**. The budget MCP edge exposes no move tool; only
envelopes get a `folder_id`, and only at *creation* (`update_envelope` has no
`folder_id`). So "tags in folders" and "empty default folder" are unreachable by
prompt wording alone. The service layer already supports it — `MoveElementList`
(`internal/budget/move.go`) moves any element (envelope/tag/category, keyed by
external id) between folders — it is simply not surfaced on MCP.

## Field semantics (verified from source)

From `internal/budget/builder_structure_build.go` and `builder_summary.go`, and
confirmed against `internal/test/mcpparity/testdata/golden/budget_write.golden`:

Per parent element (`ParentElementResult`) and child (`ChildElementResult`):

- `budgeted` = **this month's** limit only.
- `budgetedBefore` (internal; surfaced via `available`) = summed limits of all
  prior months.
- `spent` = spending in the **selected month**, in the element's **own**
  `currencyId`.
- `budgetSpent` = the same month's spending converted to the **budget base
  currency** (`meta.currencyId`). Children carry both `spent` and `budgetSpent`.
- `available` = `budgetedBefore − spentBefore − spent` = **cumulative running
  balance** of the element from budget start through end of the selected month,
  in the element's own currency. Negative ⇒ behind overall; positive ⇒ banked.
- `currencyId` = the element's own display currency (may differ from budget
  currency).
- `ownerUserId` = who owns the element/child (multi-user budgets).
- `folderId`, `position` = where the element sits in the folder layout.

Top-level:

- `balances[]` — one row per currency the budget touches, **budget currency
  first**. `startBalance`/`endBalance` are point-in-time account totals (`null`
  until the period has started/ended); `income`/`expenses`/`exchanges` are
  period flows; `holdings` is net cross-currency transfer movement. Account-level,
  separate from envelope spending.
- `currencyRates[]` — average rate per currency over the (snapped) period.
- `meta.access[]` — owner + collaborators, each `{user, role, isAccepted}`.

## Design

Five parts: two new tools (`move_element`, `create_account`), a shared glossary,
the three reworked prompts, and one new onboarding prompt (`budget-quick-start`).

### A. New MCP tool: `move_element` (`internal/budget/mcp/mcp.go`)

Thin wrapper over `svc.MoveElementList`, following the existing tool pattern in
the file (`webmcp.UserID`, `webmcp.MapErr`, `reqctx.AddLogAttr`).

- **Input struct** `moveElementInput`:
  - `budget_id` — budget id (UUID), from `list_budgets`.
  - `items[]` of `{ element_id, folder_id, position }`:
    - `element_id` — the element `id` from `get_budget` (envelope/tag/category
      external id; the service keys moves by external id, no type needed).
    - `folder_id` — target folder id from `get_budget`; **omit to move to the
      default (ungrouped) area**.
    - `position` — 0-based position within the target group.
- **Behavior:** build `model.MoveElementListRequest{BudgetId, Items:[…]}` and
  call `svc.MoveElementList`. The service renumbers positions contiguously per
  folder as its last step, so callers need not pack positions perfectly.
- **Output** `moveElementResult`: `{ budget_id, moved: <len(items)> }` (the
  service returns an empty result; echo the request for a useful tool response).
- **Registration:** add inside `Register`'s returned closure alongside the other
  `sdk.AddTool` calls. Description names the tool's purpose and points at
  `get_budget` for `element_id`/`folder_id`.
- **Note:** an envelope is an element too, so `move_element` also refiles
  envelopes between folders — `update_envelope` need not gain `folder_id`.

### A2. New MCP tool: `create_account` (`internal/account/mcp/mcp.go`)

Currently the account MCP surface has only `list_accounts`. A budget needs at
least one account to track balances/spending, so the quick-start onboarding
prompt cannot seed a from-scratch user without this. Thin wrapper over
`svc.CreateAccount`, following the file's existing pattern.

- **Input struct** `createAccountInput`:
  - `name` — account name.
  - `currency_id` — currency id (UUID), from `list_currencies`; optional →
    defaults to the user's currency (mirror how other create tools fall back).
  - `balance` — optional decimal string opening balance (defaults to 0; a
    non-zero value writes a balance-correction transaction, per the service).
  - `icon` — optional icon name (default a sensible value; the tool must always
    send a non-blank icon, which `CreateAccountRequest.Validate` requires).
  - No `folder_id`: a blank folder is tolerated for a user's first account (the
    service auto-creates a default "General" account folder), which is exactly
    the quick-start case. Accounts stay ungrouped otherwise.
- **id semantics:** `CreateAccountRequest.Id` is the **operation/idempotency
  id**, not the entity id (the service mints a fresh account id). Mint a UUID
  server-side for it, as the budget tools do.
- **Output:** return the created account's id + name + currency (built from
  `CreateAccountResult.Item`), enough for the prompt to reference it.
- **Registration + tests:** register in the account `Register` closure; add a
  scenario to `internal/test/mcpparity/catalogue.go`; add unit coverage in
  `internal/account/mcp/mcp_test.go` following `list_accounts`' test.

### B. Shared "reading get_budget" glossary (`internal/web/mcp/prompts.go`)

A package-level helper, e.g. `budgetFieldGlossary() string`, returning a compact
text block. Injected into `budget-setup`, `budget-review`, and `budget-update`
(all read `get_budget`). It documents exactly the fields above that a model
otherwise mis-reads: cumulative `available`; `budgeted` = this month only; `spent`
(element currency) vs `budgetSpent` (budget currency); `balances[]`;
`currencyRates[]`; and the multi-user `access[]` / `ownerUserId`. Keep it tight —
it is guidance, not a schema dump.

### C. `budget-setup` (extend)

Keep the ordered flow. Base/Additional split stays the **default, never forced**
(a deliberate existing layout is respected). Changes:

1. Inject the glossary.
2. **Envelopes group multiple categories only.** Never wrap a single category in
   its own envelope — reverse the current "rent can stay its own envelope" line.
   A lone category stays a **standalone category element**, filed directly into
   its folder via `move_element`.
3. **Cross-owner grouping (shared budgets).** Use `ownerUserId` (from
   `list_categories` / `get_budget`) to spot similar categories across owners
   (e.g. each partner's own "Groceries") and propose merging each such set into
   one shared envelope. This is the primary reason to create an envelope here.
4. **Tags go into folders too** (remove "leave tags alone"): file tags into the
   same folders as envelopes/categories via `move_element`.
5. **End state: default folder empty.** Add an explicit final move step — every
   participating envelope, tag, and standalone category sits in a named folder;
   nothing meaningful left ungrouped.
6. Multi-currency caution when totaling mixed-currency history (total in budget
   currency, i.e. reason in `budgetSpent` terms).

### D. `budget-review` (extend)

Keep the 5 steps; inject the glossary. Sharpen the comparison step:

- Compare limits against **`budgetSpent`** (budget currency), not `spent`.
- Flag both **this-month** overspend (`budgetSpent > budgeted`) and **cumulative**
  shortfall (`available < 0`).
- ≥90% threshold applies to the monthly figure.
- Note multi-user attribution (`ownerUserId`) where relevant.

### E. `budget-update` (rewrite → reconcile-only)

Narrow to **revise, don't rebuild**:

1. `list_budgets` (ask which if >1); `get_budget` + `list_categories` /
   `list_tags`; inject the glossary.
2. **Reconcile only.** File into the **existing** structure any envelope, tag, or
   standalone category sitting in the default/ungrouped area — matching how
   siblings are already grouped — via `move_element`. Correct stale or
   clearly-wrong limits against 2–3 months of history (`list_transactions`). Flag
   archived/dead elements still occupying a folder. Apply the same
   envelopes-only-for-multiple-categories rule (a lone default-folder category
   just moves in as a standalone element).
3. If many elements sit in the default folder, offer to arrange them into the
   existing groups the way the rest are structured. **End state: default folder
   empty.**
4. **Guardrail.** If the change would need significant restructuring (new
   top-level groups, re-splitting Base/Additional, wholesale regrouping), **stop
   and advise creating a new budget via `budget-setup`.** Update never rebuilds.
5. Short change list → wait for approval → apply (`move_element`, `set_limit`,
   `update_envelope`'s full category-set caveat) → re-`get_budget` → report.
6. Update the prompt `Description` to the narrower scope.

### F. New prompt: `budget-quick-start` (`internal/web/mcp/prompts.go`)

A one-shot onboarding flow for a user with little or no data: seed a starter set
of categories (and a couple of tags), ensure an account exists, then build a
budget on top. Optional `name` argument (budget name), like `budget-setup`.

1. **Check emptiness.** Call `list_accounts`, `list_categories`, `list_tags`,
   `list_budgets`, `list_currencies`. If the user already has substantial data
   (existing budget, or many categories/transactions), say so and point them at
   `budget-setup` instead — quick-start is for empty/near-empty users.
2. **Propose a starter set, confirm, then create.** Propose a sensible default
   set of expense + income categories (and maybe 1–2 tags), tailored lightly to
   anything the user said. Show it and **wait for confirmation** before creating
   anything. On approval, create them via `create_category` / `create_tag`.
3. **Ensure an account.** If `list_accounts` is empty, propose one starter
   account (name, currency, optional opening balance), confirm, and create it via
   `create_account`. If an account already exists, use it.
4. **Build the budget** following the same rules as `budget-setup`: Base/
   Additional folders (default, not forced); envelopes only for grouping multiple
   categories; tags and standalone categories filed into folders via
   `move_element`; default folder empty. Since a fresh user has no spending
   history, propose limits from the user's stated rough figures (or leave limits
   unset and say so) rather than inventing averages.
5. Call `get_budget`, confirm the result, and give one or two next steps (log a
   first expense, adjust a limit).

Ask before creating anything that changes structure. Reply in the user's
language. Reuses the glossary block.

## Testing

- **New tools (`move_element`, `create_account`):** add a scenario for each to
  `internal/test/mcpparity/catalogue.go` (the guard requires every registered
  tool to have a scenario and forbids the scenario/route counts from shrinking).
  Add unit coverage in `internal/budget/mcp/mcp_test.go` and
  `internal/account/mcp/mcp_test.go` following the sibling tools' tests.
- **Prompts:** prompt text and descriptions are golden-captured in
  `internal/test/mcpparity/testdata/golden/prompts.golden`; `prompts-list` and
  `tools-list` capture descriptions/registration. Add a `prompts/get` scenario
  for the new `budget-quick-start` prompt to the catalogue. Regenerate:
  `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/`, then **inspect the diff** —
  only the changed prompt strings/descriptions and the two new tool entries plus
  the new prompt should move. Never hand-edit a golden.
- Run `make go-test` (build, vet, gofmt, OpenAPI-fresh, unit/integration,
  coverage gate). MCP changes don't touch REST, so `apiparity` should be
  unaffected.

## Out of scope

- No change to `update_envelope` (folder moves go through `move_element`).
- No REST-side changes; this is MCP edge + prompts only.
- No new analytics events (MCP prompts/tools, not SPA user-facing features).
