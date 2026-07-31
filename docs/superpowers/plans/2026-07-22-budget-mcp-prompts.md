# Budget MCP prompts + tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the budget MCP prompts detailed (field semantics, multi-currency, multi-user, cumulative fields), add a `move_element` and a `create_account` MCP tool, and add a `budget-quick-start` onboarding prompt.

**Architecture:** All changes live on the MCP edge. Two new thin tools wrap existing services (`svc.MoveElementList`, `svc.CreateAccount`). A shared Go helper produces a "reading get_budget" glossary injected into the four budget prompts. Prompt bodies in `internal/web/mcp/prompts.go` are rewritten. Golden-file suites (`mcpparity`) capture tool registration + prompt text and are regenerated.

**Tech Stack:** Go, `github.com/modelcontextprotocol/go-sdk/mcp`, sqlc, the project's dbtest/fixture/mcptest test helpers.

## Global Constraints

- Comment sparingly: only non-obvious *why*, never restating names. No references to the removed PHP/Symfony code. (CLAUDE.md)
- MCP tools follow the file's existing pattern: `webmcp.UserID(ctx)`, `webmcp.MapErr(ctx, err)`, `reqctx.AddLogAttr(ctx, "tool", "<name>")`; entity/operation ids are minted server-side with `vo.NewId()`.
- Golden files are never hand-edited: regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/` and inspect the diff.
- Every registered MCP tool MUST have a scenario in `internal/test/mcpparity/catalogue.go`; the guard forbids the scenario/tool counts from shrinking.
- Full check before done: `make go-test` (build, vet, gofmt, OpenAPI-fresh, sqlite unit/integration, coverage gate ≥78).
- Prompts must instruct the model to reply in the user's language and to confirm before structural changes (existing convention).

---

## File Structure

- `internal/budget/mcp/mcp.go` — add `move_element` tool (input/result structs + `sdk.AddTool` in `Register`).
- `internal/budget/mcp/mcp_test.go` — add a `move_element` unit test.
- `internal/account/mcp/mcp.go` — add `create_account` tool (input/result structs + `sdk.AddTool` in `Register`).
- `internal/account/mcp/mcp_test.go` — add a `create_account` unit test.
- `internal/web/mcp/prompts.go` — add `budgetFieldGlossary()` helper; rewrite `budget-setup`, `budget-review`, `budget-update`; add `budget-quick-start`.
- `internal/test/mcpparity/catalogue.go` — add `move_element` + `create_account` tool scenarios and a `budget-quick-start` prompt scenario.
- `internal/test/mcpparity/testdata/golden/*.golden` — regenerated, not hand-edited.

---

## Task 1: `move_element` MCP tool

**Files:**
- Modify: `internal/budget/mcp/mcp.go`
- Test: `internal/budget/mcp/mcp_test.go`

**Interfaces:**
- Consumes: `svc.MoveElementList(ctx, userID vo.Id, model.MoveElementListRequest) (*model.MoveElementListResult, error)`; `model.MoveElementListRequest{BudgetId string, Items []model.MoveElementListItem}`; `model.MoveElementListItem{Id string, FolderId *string, Position int}`; helper `strPtr(string) *string` (already in the file).
- Produces: MCP tool `move_element`.

- [ ] **Step 1: Write the failing test**

Add to `internal/budget/mcp/mcp_test.go`. It builds a budget with a folder and a standalone (unfoldered) category element, then moves that element into the folder and asserts `get_budget` reports the new `folderId`. Reuses `newBudgetService`, `connectBudgetSession`, `structured` from the file.

```go
func TestMoveElementTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	accountID := f.Account(fixture.Account{UserID: userID})
	f.AccountOption(accountID, userID, 0)
	categoryID := f.Category(fixture.Category{UserID: userID, Type: 0})

	svc := newBudgetService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectBudgetSession(t, ctx, svc)

	createBudgetRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_budget",
		Arguments: map[string]any{"name": "B", "currency_id": fixture.USD, "start_date": "2024-04-01"},
	})
	if err != nil || createBudgetRes.IsError {
		t.Fatalf("create_budget: %v %#v", err, createBudgetRes)
	}
	budgetID, _ := structured(t, createBudgetRes)["item"].(map[string]any)["meta"].(map[string]any)["id"].(string)

	folderRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_folder",
		Arguments: map[string]any{"budget_id": budgetID, "name": "Bills"},
	})
	if err != nil || folderRes.IsError {
		t.Fatalf("create_folder: %v %#v", err, folderRes)
	}
	folderID, _ := structured(t, folderRes)["item"].(map[string]any)["id"].(string)

	moveRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "move_element",
		Arguments: map[string]any{
			"budget_id": budgetID,
			"items":     []any{map[string]any{"element_id": categoryID, "folder_id": folderID, "position": 0}},
		},
	})
	if err != nil {
		t.Fatalf("move_element: transport error: %v", err)
	}
	if moveRes.IsError {
		t.Fatalf("move_element: unexpected error: %#v", moveRes.Content)
	}
	if got := structured(t, moveRes)["moved"]; got != float64(1) {
		t.Fatalf("move_element: moved = %#v, want 1", got)
	}

	getRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "get_budget",
		Arguments: map[string]any{"budget_id": budgetID, "month": "2024-04"},
	})
	if err != nil || getRes.IsError {
		t.Fatalf("get_budget: %v %#v", err, getRes)
	}
	structure := structured(t, getRes)["item"].(map[string]any)["structure"].(map[string]any)
	elements, _ := structure["elements"].([]any)
	found := false
	for _, e := range elements {
		el := e.(map[string]any)
		if el["id"] == categoryID {
			found = true
			if el["folderId"] != folderID {
				t.Fatalf("element folderId = %#v, want %q", el["folderId"], folderID)
			}
		}
	}
	if !found {
		t.Fatalf("moved category not found in structure: %#v", elements)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/budget/mcp/ -run TestMoveElementTool -v`
Expected: FAIL — `move_element` is not a registered tool (error like `tool "move_element" not found` / `IsError`).

- [ ] **Step 3: Add the input/result structs**

Add near the other input structs in `internal/budget/mcp/mcp.go` (after `setAccountIncludedInput`/`accountIncludedResult`):

```go
type moveElementItemInput struct {
	ElementID string `json:"element_id" jsonschema:"envelope, tag or category id (UUID), from get_budget"`
	FolderID  string `json:"folder_id,omitempty" jsonschema:"target folder id (UUID), from get_budget; omit to move to the default (ungrouped) area"`
	Position  int    `json:"position" jsonschema:"0-based position within the target folder"`
}

type moveElementInput struct {
	BudgetID string                 `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
	Items    []moveElementItemInput `json:"items" jsonschema:"the elements to move; each names an element_id and its target folder_id + position"`
}

type moveElementResult struct {
	BudgetID string `json:"budget_id"`
	Moved    int    `json:"moved"`
}
```

- [ ] **Step 4: Register the tool**

Add inside `Register`'s returned closure in `internal/budget/mcp/mcp.go` (e.g. after the `set_limit` tool):

```go
sdk.AddTool(s, &sdk.Tool{Name: "move_element",
	Description: "Move budget elements (envelopes, tags, standalone categories) into folders or reorder them. Use get_budget for element_id and folder_id; omit folder_id to move an element to the default ungrouped area."},
	func(ctx context.Context, req *sdk.CallToolRequest, in moveElementInput) (*sdk.CallToolResult, moveElementResult, error) {
		reqctx.AddLogAttr(ctx, "tool", "move_element")
		userID, err := webmcp.UserID(ctx)
		if err != nil {
			return nil, moveElementResult{}, err
		}
		items := make([]model.MoveElementListItem, 0, len(in.Items))
		for _, it := range in.Items {
			items = append(items, model.MoveElementListItem{
				Id:       it.ElementID,
				FolderId: strPtr(it.FolderID),
				Position: it.Position,
			})
		}
		if _, err := svc.MoveElementList(ctx, userID, model.MoveElementListRequest{
			BudgetId: in.BudgetID,
			Items:    items,
		}); err != nil {
			return nil, moveElementResult{}, webmcp.MapErr(ctx, err)
		}
		return nil, moveElementResult{BudgetID: in.BudgetID, Moved: len(in.Items)}, nil
	})
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/budget/mcp/ -run TestMoveElementTool -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/budget/mcp/mcp.go internal/budget/mcp/mcp_test.go
git commit -m "feat(mcp): add move_element budget tool"
```

---

## Task 2: `create_account` MCP tool

**Files:**
- Modify: `internal/account/mcp/mcp.go`
- Test: `internal/account/mcp/mcp_test.go`

**Interfaces:**
- Consumes: `svc.CreateAccount(ctx, userID vo.Id, model.CreateAccountRequest) (*model.CreateAccountResult, error)`; `model.CreateAccountRequest{Id, Name, CurrencyId string, Balance vo.FlexString, Icon, FolderId string}`; `model.CreateAccountResult{Item model.AccountResult}` where `AccountResult{Id, Name string, Currency model.CurrencyResult, Balance string}`; `vo.NewId()`, `vo.NewFlexString(string) vo.FlexString`.
- Produces: MCP tool `create_account`.
- **Decision (deviation from spec):** `currency_id` is **required**. The account service does not resolve a default currency the way `create_budget` does, and adding that plumbing is out of scope; the quick-start prompt fetches the id from `list_currencies`. `folder_id` is omitted entirely — a blank folder is tolerated for a user's first account (service auto-creates a "General" folder). `req.Id` is the operation/idempotency id (service mints the entity id).

- [ ] **Step 1: Write the failing test**

Add to `internal/account/mcp/mcp_test.go`. It creates an account and asserts the result echoes the name + currency, then that `list_accounts` shows it. Add imports as needed (`fixture`, `mcptest` already imported; add `strings` only if used — the test below does not need it).

```go
func TestCreateAccountTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	svc := newAccountService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	accountmcp.Register(svc)(srv)

	ctx := mcptest.CtxWithUser(t, userID)
	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "c", Version: "t"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "create_account",
		Arguments: map[string]any{
			"name":        "Checking",
			"currency_id": fixture.USD,
			"balance":     "100.00",
		},
	})
	if err != nil {
		t.Fatalf("create_account: transport error: %v", err)
	}
	if res.IsError {
		t.Fatalf("create_account: unexpected error: %#v", res.Content)
	}
	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("create_account: structuredContent is not a map: %#v", res.StructuredContent)
	}
	if m["name"] != "Checking" {
		t.Fatalf("create_account: name = %#v, want Checking", m["name"])
	}
	if m["id"] == "" || m["id"] == nil {
		t.Fatalf("create_account: empty id: %#v", m)
	}

	listRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_accounts", Arguments: map[string]any{}})
	if err != nil || listRes.IsError {
		t.Fatalf("list_accounts: %v %#v", err, listRes)
	}
	items, _ := listRes.StructuredContent.(map[string]any)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("list_accounts: want 1 account, got %#v", items)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/account/mcp/ -run TestCreateAccountTool -v`
Expected: FAIL — `create_account` not a registered tool.

- [ ] **Step 3: Add imports, input/result structs**

In `internal/account/mcp/mcp.go`, ensure these imports exist (add the missing ones): `"github.com/econumo/econumo/internal/shared/reqctx"`, `"github.com/econumo/econumo/internal/shared/vo"`, `"github.com/econumo/econumo/internal/model"`, `webmcp "github.com/econumo/econumo/internal/web/mcp"` (mirror the budget mcp file's import block). Add the structs after `emptyInput`:

```go
type createAccountInput struct {
	Name       string `json:"name" jsonschema:"account name"`
	CurrencyID string `json:"currency_id" jsonschema:"currency id (UUID), from list_currencies"`
	Balance    string `json:"balance,omitempty" jsonschema:"opening balance as a decimal string, e.g. 100.00; defaults to 0"`
	Icon       string `json:"icon,omitempty" jsonschema:"optional icon name; defaults to 'wallet'"`
}

type createAccountResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CurrencyID string `json:"currency_id"`
	Balance    string `json:"balance"`
}
```

- [ ] **Step 4: Register the tool**

Add inside `Register`'s returned closure, after the `list_accounts` tool:

```go
sdk.AddTool(s, &sdk.Tool{Name: "create_account",
	Description: "Create an account for the caller. currency_id is required (use list_currencies). Optional opening balance and icon. New users get a default account folder automatically."},
	func(ctx context.Context, req *sdk.CallToolRequest, in createAccountInput) (*sdk.CallToolResult, createAccountResult, error) {
		reqctx.AddLogAttr(ctx, "tool", "create_account")
		userID, err := webmcp.UserID(ctx)
		if err != nil {
			return nil, createAccountResult{}, err
		}
		icon := in.Icon
		if icon == "" {
			icon = "wallet"
		}
		balance := in.Balance
		if balance == "" {
			balance = "0"
		}
		res, err := svc.CreateAccount(ctx, userID, model.CreateAccountRequest{
			Id:         vo.NewId().String(), // operation id; the service mints the entity id
			Name:       in.Name,
			CurrencyId: in.CurrencyID,
			Balance:    vo.NewFlexString(balance),
			Icon:       icon,
			// FolderId omitted: a blank folder is tolerated for a first account.
		})
		if err != nil {
			return nil, createAccountResult{}, webmcp.MapErr(ctx, err)
		}
		return nil, createAccountResult{
			ID:         res.Item.Id,
			Name:       res.Item.Name,
			CurrencyID: res.Item.Currency.Id,
			Balance:    res.Item.Balance,
		}, nil
	})
```

Note: `model.CurrencyResult` has field `Id string` (verified), so `res.Item.Currency.Id` is correct.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/account/mcp/ -run TestCreateAccountTool -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/account/mcp/mcp.go internal/account/mcp/mcp_test.go
git commit -m "feat(mcp): add create_account tool"
```

---

## Task 3: Shared glossary helper + rewritten prompts

**Files:**
- Modify: `internal/web/mcp/prompts.go`

**Interfaces:**
- Consumes: `sdk.Server.AddPrompt`, `reqctx.AddLogAttr`, `fmt.Sprintf`.
- Produces: `budgetFieldGlossary() string`; updated `budget-setup`/`budget-review`/`budget-update` bodies + descriptions; new `budget-quick-start` prompt.

- [ ] **Step 1: Add the glossary helper**

Add a package-level function in `internal/web/mcp/prompts.go`:

```go
// budgetFieldGlossary explains the get_budget fields a model otherwise mis-reads:
// the cumulative carryover, the per-currency vs budget-currency spent, the
// account balances block, and the multi-user attribution.
func budgetFieldGlossary() string {
	return `Reading get_budget — what the numbers mean:
- available: a CUMULATIVE carryover, not this month's leftover. It is every prior
  month's limit minus all spending from the budget's start through the end of the
  selected month, in the element's own currency. Negative = the element is behind
  overall; positive = banked room. Do NOT read it as "limit minus spent this month".
- budgeted: THIS month's limit only.
- spent: this month's spending in the ELEMENT's own currency (its currencyId).
- budgetSpent: the same spending converted to the BUDGET's base currency
  (meta.currencyId). When currencies differ, compare limits against budgetSpent,
  not spent. Child categories carry both spent and budgetSpent.
- balances[]: one row per currency the budget touches (budget currency first) —
  account-level, separate from envelope spending. startBalance/endBalance are
  point-in-time totals (null until the period starts/ends); income/expenses/
  exchanges are period flows; holdings is net cross-currency transfer movement.
- currencyRates[]: the average rate per currency over the period.
- Multi-user: meta.access[] lists the owner and collaborators (role, isAccepted);
  ownerUserId on each element/child says who owns it. In a shared budget, do not
  assume every element is the caller's.`
}
```

- [ ] **Step 2: Rewrite `budget-setup`**

Replace the `budget-setup` `AddPrompt` block's `Description` and `text`. New `Description`:

```
"Create a budget from existing categories and tags, organized into folders, split into base (essential) and additional expenses."
```

New `text` (the `fmt.Sprintf` body), keeping the `name` arg:

```go
text := fmt.Sprintf(`Set up a new budget in my Econumo finance tracker. Requested name: %s

%s

Work in this order:

1. Survey what exists before creating anything. Call list_categories, list_tags,
   list_accounts and list_currencies. Call list_budgets too — if I already have a
   budget, show me what it is and ask whether to add another before continuing.
2. Look at how I actually spend. Call list_transactions for the last 2-3 full
   months and total each expense category. Total in the budget's currency (reason
   in budgetSpent terms) if my categories span currencies. Say so if the history
   is too thin to judge.
3. Sort my expense categories into exactly two groups:
   - BASE EXPENSES — the ones I cannot live without: housing, utilities,
     groceries, transport to work, insurance, healthcare, debt payments,
     childcare, essential subscriptions (phone, internet).
   - ADDITIONAL EXPENSES — everything else: dining out, entertainment, hobbies,
     travel, gifts, shopping, non-essential subscriptions.
   Judge by how I use a category, not its name. Anything genuinely ambiguous goes
   in ADDITIONAL and gets flagged — never silently promote something into BASE.
   This two-group split is the default; if I clearly want a different structure,
   follow mine instead.
4. Show me the proposed split and the monthly figure behind each group, and WAIT
   for my confirmation. Do not create anything yet.
5. On my approval, build it:
   - create_budget (name, currency, start date).
   - create_folder twice: "Base expenses" and "Additional expenses".
   - Group categories into envelopes with create_envelope ONLY to combine two or
     more categories under one line — pass folder_id and the member category_ids.
     Never wrap a single category in its own envelope; a lone category stays a
     standalone category element and is filed straight into its folder (step 6).
   - In a SHARED budget, look for similar categories across owners (use
     ownerUserId from list_categories) — e.g. each partner's own "Groceries" — and
     propose merging each such set into ONE shared envelope. This is the main
     reason to create an envelope here.
   - Do not invent categories that do not exist; suggest them to me instead.
   - set_limit per envelope (and per standalone category, if I want a limit) for
     the current month, using the averages from step 2 (round sensibly). Limits go
     on the envelope, not on each member category.
6. Put EVERYTHING in a folder. Use move_element to file tags AND standalone
   categories into "Base expenses" or "Additional expenses" alongside the
   envelopes. Classify each tag the same BASE/ADDITIONAL way. The goal end state:
   the default (ungrouped) area is empty — every envelope, tag and standalone
   category sits in a named folder.
7. Call get_budget and confirm the result: folders, envelopes, tags, limits, and
   the base-vs-additional totals. On a brand-new budget "available" is ~0 for
   everything — that is expected. Flag anything I should revisit.

Ask me before guessing on anything that changes the structure. Reply in my language.`,
	name, budgetFieldGlossary())
```

- [ ] **Step 3: Rewrite `budget-review`**

Replace its `text` (keep the `month` arg and `Description`). New `text`:

```go
text := fmt.Sprintf(`Review my Econumo budget for %s.

%s

Follow these steps:
1. Call list_budgets; if I have more than one budget, ask which one.
2. Call get_budget with the budget_id (and the month, if not current).
3. Compare limits against spending per envelope/category. Use budgetSpent (budget
   currency) for the comparison, not spent, so mixed-currency elements are fair.
   Flag two things separately: THIS month's overspend (budgetSpent > budgeted) and
   anything above 90%% of its monthly limit; and CUMULATIVE trouble (available < 0),
   which means the element is behind across the whole budget so far.
4. If something looks unusual, sample the underlying activity with
   list_transactions. In a shared budget, note who owns the element (ownerUserId).
5. Reply with a short structured review in my language: overall position, top
   monthly overspends, any cumulative shortfalls, notable items, one concrete
   suggestion.`,
	month, budgetFieldGlossary())
```

- [ ] **Step 4: Rewrite `budget-update` (reconcile-only)**

Replace its `Description` and `text` (keep the `month` arg). New `Description`:

```
"Revise an existing budget: file stray categories/tags into the existing folders and fix stale limits. For big changes, create a new budget instead."
```

New `text`:

```go
text := fmt.Sprintf(`Revise my Econumo budget for %s.

This is a revision, not a rebuild: keep the structure that already works and change
only what has drifted. If it needs significant restructuring, do NOT do it here —
tell me to create a new budget with the budget-setup prompt instead.

%s

1. Call list_budgets; if I have more than one, ask which. Call get_budget for it,
   plus list_categories and list_tags.
2. Reconcile only:
   - Any envelope, tag or standalone category sitting in the default (ungrouped)
     area — including ones created since setup — gets filed into the EXISTING
     folder that matches how its siblings are already grouped, via move_element.
     Follow the envelopes-only-for-multiple-categories rule: a lone default-folder
     category just moves in as a standalone element; do not wrap it in an envelope.
   - Envelopes whose limit is missing or clearly wrong against the last 2-3 months
     (check list_transactions) get corrected with set_limit.
   - Archived or dead categories still occupying a folder get flagged.
3. If lots of elements sit in the default area, offer to arrange them into the
   existing groups the way the rest are structured — nothing more. Goal end state:
   the default (ungrouped) area is empty.
4. STOP-AND-ADVISE: if the fix would need new top-level groups, re-splitting
   Base/Additional, or wholesale regrouping, do not attempt it — tell me to run
   budget-setup and create a fresh budget. Update never rebuilds.
5. Show me the proposed changes as a short list — what moves, what limit changes
   and from what to what — and WAIT for my approval.
6. On my approval, apply it: move_element for the moves, set_limit for limits
   (omit the amount to clear one). To retire an envelope, archive it via
   update_envelope (there is no delete). NOTE: update_envelope replaces the
   envelope's FULL category set, so include the existing category_ids alongside
   any new ones or you will silently unlink them.
7. Call get_budget again and report what changed, with the new base-vs-additional
   split and anything still needing my decision.

Reply in my language.`,
	month, budgetFieldGlossary())
```

- [ ] **Step 5: Add the `budget-quick-start` prompt**

Add a new `AddPrompt` block in `addPrompts` (after `budget-review`):

```go
s.AddPrompt(&sdk.Prompt{
	Name:        "budget-quick-start",
	Description: "Onboard a new/empty user: seed starter categories, ensure an account, and build a first budget.",
	Arguments: []*sdk.PromptArgument{{
		Name: "name", Description: "budget name; defaults to a name you propose", Required: false,
	}},
}, func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
	reqctx.AddLogAttr(ctx, "prompt", "budget-quick-start")
	name := req.Params.Arguments["name"]
	if name == "" {
		name = "(none given — propose one)"
	}
	text := fmt.Sprintf(`Give me a quick start in my Econumo finance tracker. Requested budget name: %s

This is for a new or nearly-empty account: seed a starter set of categories, make
sure I have an account, then build a first budget.

%s

1. Check what I have. Call list_accounts, list_categories, list_tags,
   list_budgets and list_currencies. If I already have a budget or lots of
   categories/transactions, say so and point me at the budget-setup prompt instead
   — quick-start is for empty/near-empty accounts.
2. Propose a starter set of categories, then WAIT for my confirmation before
   creating anything: a sensible default of expense categories (housing,
   groceries, transport, utilities, dining, entertainment) plus a couple of income
   categories (salary, other income), lightly tailored to anything I tell you.
   Maybe 1-2 tags. On my approval, create them with create_category / create_tag.
3. Ensure an account. If list_accounts is empty, propose ONE starter account
   (name, currency from list_currencies, optional opening balance), confirm, and
   create it with create_account. If I already have an account, use it.
4. Build the budget like budget-setup does: create_budget; "Base expenses" and
   "Additional expenses" folders (default, not forced); envelopes ONLY to group
   two or more categories (never a single-category envelope); file tags and
   standalone categories into folders with move_element so the default area ends
   up empty. I have no spending history yet, so set limits from rough figures I
   give you, or leave limits unset and tell me — do not invent averages.
5. Call get_budget, confirm the result, and give me one or two next steps (log a
   first expense, adjust a limit).

Ask before creating anything that changes structure. Reply in my language.`,
		name, budgetFieldGlossary())
	return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{
		{Role: "user", Content: &sdk.TextContent{Text: text}},
	}}, nil
})
```

- [ ] **Step 6: Build + run the mcp package tests**

Run: `go build ./... && go test ./internal/web/mcp/ -v`
Expected: build succeeds; `internal/web/mcp` tests pass (the prompt-registration test there counts prompts — if it asserts an exact count, update it to include `budget-quick-start`; see Step 7).

- [ ] **Step 7: Verify prompt/tool tests still pass**

`internal/web/mcp/mcp_test.go` uses substring checks (`strings.Contains` for `log-expense`, `budget-review`, `get_budget`, "the current month"), not an exact prompt count — the `budget-review` rewrite keeps `get_budget` and "the current month", so no count fix is needed. Just confirm: `go test ./internal/web/mcp/ -v` → PASS. If any substring assertion references text you removed, update the assertion to match the new wording.

- [ ] **Step 8: Commit**

```bash
git add internal/web/mcp/
git commit -m "feat(mcp): detailed budget prompts + budget-quick-start, shared get_budget glossary"
```

---

## Task 4: mcpparity scenarios + golden regeneration

**Files:**
- Modify: `internal/test/mcpparity/catalogue.go`
- Regenerate: `internal/test/mcpparity/testdata/golden/*.golden`

**Interfaces:**
- Consumes: the `Scenario`/`Step` types + `register(...)`, `CaptureAs`/`MCPCapturePath`/`{{...}}` substitution, `apiparity.USD`, `apiparity.CatFood`, `apiparity.OwnerFolder`, `apiparity.OwnerAccount` constants (as used by the existing `budget_write`/`reference_tools_write` scenarios).

- [ ] **Step 1: Add a `move_element` step to the `budget_write` scenario**

In `internal/test/mcpparity/catalogue.go`, inside the existing `budget_write` scenario's `Steps`, after the `create-envelope` step (which captures `element_id` — verify its `CaptureAs: "element_id"`) and before or after `set-limit`, add a step that moves the envelope element into the folder. Use the already-captured `{{budget_id}}`, `{{folder_id}}`, `{{element_id}}`:

```go
{Label: "move-element",
	RPC: `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"move_element","arguments":{"budget_id":"{{budget_id}}","items":[{"element_id":"{{element_id}}","folder_id":"{{folder_id}}","position":0}]}}}`},
```

Pick a JSON-RPC `id` not already used in that scenario (the closing error step uses 10; use 11). Place it after `create-envelope` so `element_id`/`folder_id` are captured.

- [ ] **Step 2: Add a `create_account` scenario**

Add a new scenario (near `reference_tools_write`). It creates an account via MCP and then lists accounts to confirm shape:

```go
register(Scenario{Name: "account_write", Steps: []Step{
	{Label: "create-account",
		RPC: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_account","arguments":{"name":"MCP Made Account","currency_id":"` + apiparity.USD + `","balance":"100.00","icon":"wallet"}}}`},
	{Label: "list-accounts-after",
		RPC: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_accounts","arguments":{}}}`},
}})
```

- [ ] **Step 3: Add the `budget-quick-start` prompt scenario**

In the existing `prompts` scenario's `Steps` (the block with `get-log-expense`, `get-budget-review`, etc.), add:

```go
{Label: "get-budget-quick-start", RPC: `{"jsonrpc":"2.0","id":5,"method":"prompts/get","params":{"name":"budget-quick-start","arguments":{}}}`},
```

Use a JSON-RPC `id` (5) not already used in that scenario.

- [ ] **Step 4: Regenerate goldens**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/`
Expected: PASS, goldens rewritten.

- [ ] **Step 5: Inspect the diff**

Run: `git status --short internal/test/mcpparity/testdata/golden/ && git diff internal/test/mcpparity/testdata/golden/`
Expected: only these changes — `tools-list` gains `move_element` + `create_account`; `prompts-list` reflects the three changed descriptions + the new `budget-quick-start`; `prompts.golden` reflects the new/changed prompt text; the `budget_write`, new `account_write`, and `prompts` scenario goldens gain their new steps. **No unrelated golden should move.** If anything else changed, investigate before proceeding.

- [ ] **Step 6: Run the mcpparity suite clean (no UPDATE_GOLDEN)**

Run: `go test ./internal/test/mcpparity/ -v`
Expected: PASS against the committed goldens.

- [ ] **Step 7: Commit**

```bash
git add internal/test/mcpparity/
git commit -m "test(mcpparity): scenarios + goldens for move_element, create_account, budget-quick-start"
```

---

## Task 5: Full suite + finish

**Files:** none (verification only).

- [ ] **Step 1: Run the smoke suite**

Run: `make go-test`
Expected: PASS — build, vet, gofmt, OpenAPI-fresh, sqlite unit/integration, coverage gate ≥78. If gofmt flags anything, run `gofmt -w` on the touched files and re-run.

- [ ] **Step 2: Fix and re-run if needed**

If coverage dipped below the gate, the new tools' tests (Tasks 1–2) should cover them; add an assertion or a small extra case to the tool tests rather than lowering the gate. Re-run `make go-test`.

- [ ] **Step 3: Final commit (only if fixes were made)**

```bash
git add -A
git commit -m "chore: gofmt + coverage touch-ups for budget mcp changes"
```

---

## Self-Review notes

- **Spec coverage:** move_element (Task 1), create_account (Task 2), glossary + budget-setup/review/update rewrites + budget-quick-start (Task 3), tests/goldens (Task 4), full suite (Task 5). All spec sections map to a task.
- **Deviation logged:** `create_account.currency_id` is required (service has no default-currency resolution); the quick-start prompt fetches it from `list_currencies`. Recorded in Task 2's Interfaces block.
- **Type consistency:** `moveElementResult.Moved` (int) asserted as `float64(1)` in Task 1's test (JSON numbers decode to float64) — consistent. `create_account` result fields (`id`/`name`/`currency_id`/`balance`) match the struct and the test assertions. `budgetFieldGlossary()` signature used identically in all four prompts.
- **Verified pre-write:** `create-envelope` in `budget_write` captures `element_id` (`CaptureAs: "element_id"`, catalogue.go:127); `model.CurrencyResult.Id` exists; `internal/web/mcp/mcp_test.go` uses substring checks (no exact prompt-count assertion), and the `budget-review` rewrite preserves its checked substrings (`get_budget`, "the current month").
