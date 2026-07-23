package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/shared/reqctx"
)

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

func addPrompts(s *sdk.Server) {
	s.AddPrompt(&sdk.Prompt{
		Name:        "log-expense",
		Description: "Log a transaction in Econumo from a free-text description.",
		Arguments: []*sdk.PromptArgument{{
			Name: "description", Description: "free text, e.g. '27.50 groceries at Lidl yesterday'", Required: true,
		}},
	}, func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
		reqctx.AddLogAttr(ctx, "prompt", "log-expense")
		text := fmt.Sprintf(`Log this in my Econumo finance tracker: %s

Follow these steps:
1. Call list_accounts, list_categories and list_payees.
2. Work out the type (expense unless clearly income or a transfer), the amount (decimal string), the date (default: today), and the best-matching account, category and payee (payee may be omitted).
3. Call create_transaction.
4. Confirm in one line what you logged: amount, currency, category, account, date.
If the amount or the account is ambiguous, ask me before creating anything.`,
			req.Params.Arguments["description"])
		return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{
			{Role: "user", Content: &sdk.TextContent{Text: text}},
		}}, nil
	})

	// The two budget-structure prompts below share one model: spending splits
	// into "Base expenses" (the ones you cannot live without) and "Additional
	// expenses" (everything else). Setup builds that split; update reconciles an
	// existing budget against it without reorganizing what already works.
	s.AddPrompt(&sdk.Prompt{
		Name:        "budget-setup",
		Description: "Create a budget from existing categories and tags, organized into folders, split into base (essential) and additional expenses.",
		Arguments: []*sdk.PromptArgument{{
			Name: "name", Description: "budget name; defaults to a name you propose", Required: false,
		}},
	}, func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
		reqctx.AddLogAttr(ctx, "prompt", "budget-setup")
		name := req.Params.Arguments["name"]
		if name == "" {
			name = "(none given — propose one)"
		}
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
		return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{
			{Role: "user", Content: &sdk.TextContent{Text: text}},
		}}, nil
	})

	s.AddPrompt(&sdk.Prompt{
		Name:        "budget-update",
		Description: "Revise an existing budget: file stray categories/tags into the existing folders and fix stale limits. For big changes, create a new budget instead.",
		Arguments: []*sdk.PromptArgument{{
			Name: "month", Description: "YYYY-MM; defaults to the current month", Required: false,
		}},
	}, func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
		reqctx.AddLogAttr(ctx, "prompt", "budget-update")
		month := req.Params.Arguments["month"]
		if month == "" {
			month = "the current month"
		}
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
		return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{
			{Role: "user", Content: &sdk.TextContent{Text: text}},
		}}, nil
	})

	s.AddPrompt(&sdk.Prompt{
		Name:        "budget-review",
		Description: "Review the monthly budget: limits vs spending, overspends, notable items.",
		Arguments: []*sdk.PromptArgument{{
			Name: "month", Description: "YYYY-MM; defaults to the current month", Required: false,
		}},
	}, func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
		reqctx.AddLogAttr(ctx, "prompt", "budget-review")
		month := req.Params.Arguments["month"]
		if month == "" {
			month = "the current month"
		}
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
		return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{
			{Role: "user", Content: &sdk.TextContent{Text: text}},
		}}, nil
	})

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
   create_account needs a folder_id unless I have no folders at all — take one
   from an existing account's folderId in list_accounts.
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
}
