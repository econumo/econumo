package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/shared/reqctx"
)

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
		Description: "Create a budget from existing categories and tags, split into base (essential) and additional expenses.",
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

Work in this order:

1. Survey what exists before creating anything. Call list_categories, list_tags,
   list_accounts and list_currencies. Call list_budgets too — if I already have a
   budget, show me what it is and ask whether to add another before continuing.
2. Look at how I actually spend. Call list_transactions for the last 2-3 full
   months and total each expense category, so the plan reflects my real spending
   rather than a generic template. Say so if the history is too thin to judge.
3. Sort my expense categories into exactly two groups:
   - BASE EXPENSES — the ones I cannot live without: housing, utilities,
     groceries, transport to work, insurance, healthcare, debt payments,
     childcare, essential subscriptions (phone, internet).
   - ADDITIONAL EXPENSES — everything else: dining out, entertainment, hobbies,
     travel, gifts, shopping, non-essential subscriptions.
   Judge by how I use a category, not by its name alone. Anything genuinely
   ambiguous goes in ADDITIONAL and gets flagged to me — never silently promote
   something into BASE.
4. Show me the proposed split and the monthly figure behind each group, and WAIT
   for my confirmation. Do not create anything yet.
5. On my approval, build it:
   - create_budget (name, currency, start date).
   - create_folder twice: "Base expenses" and "Additional expenses".
   - Group related categories into envelopes with create_envelope, passing
     folder_id and the member category_ids — e.g. an envelope "Housing" holding
     rent, utilities and maintenance. Only group where it aids reading: a single
     large category like rent can stay its own envelope. Do not invent
     categories that do not exist; suggest them to me instead.
   - set_limit per envelope for the current month, using the averages from step 2
     (round to a sensible figure). Limits go on the envelope, not on each member
     category.
6. Leave tags alone unless I ask — tags cut across the folder structure (a tag
   can carry its own limit) and are better added once the base structure settles.
   Tell me which tags you saw and what they could track.
7. Call get_budget and confirm the result: folders, envelopes, limits, and the
   base-vs-additional totals. Flag anything I should revisit.

Ask me before guessing on anything that changes the structure. Reply in my language.`,
			name)
		return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{
			{Role: "user", Content: &sdk.TextContent{Text: text}},
		}}, nil
	})

	s.AddPrompt(&sdk.Prompt{
		Name:        "budget-update",
		Description: "Bring an existing budget back in line: new categories, stale limits, base vs additional drift.",
		Arguments: []*sdk.PromptArgument{{
			Name: "month", Description: "YYYY-MM; defaults to the current month", Required: false,
		}},
	}, func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
		reqctx.AddLogAttr(ctx, "prompt", "budget-update")
		month := req.Params.Arguments["month"]
		if month == "" {
			month = "the current month"
		}
		text := fmt.Sprintf(`Bring my Econumo budget up to date for %s.

This is a reconciliation, not a rebuild: keep the structure that already works
and change only what has drifted.

1. Call list_budgets; if I have more than one, ask which. Call get_budget for it,
   plus list_categories and list_tags.
2. Find the gaps:
   - Categories and tags that exist but sit in no envelope — including ones
     created since the budget was set up.
   - Envelopes whose limit is missing, or clearly wrong against the last 2-3
     months (check with list_transactions).
   - Categories whose real usage no longer matches where they sit: an
     "Additional" category that has become unavoidable, or a "Base" one that has
     turned discretionary.
   - Archived or dead categories still occupying an envelope.
3. Classify anything new as BASE EXPENSES (cannot live without: housing,
   utilities, groceries, commuting, insurance, healthcare, debt, childcare,
   essential subscriptions) or ADDITIONAL EXPENSES (everything else). Ambiguous
   goes to ADDITIONAL and gets flagged.
4. Show me the proposed changes as a short list — what moves, what limit changes
   and from what to what, what gets created — and WAIT for my approval.
5. On my approval, apply it:
   - create_folder only if a needed group is genuinely missing.
   - create_envelope for new groupings; update_envelope to add categories to an
     existing one. NOTE: update_envelope replaces the envelope's FULL category
     set, so include the existing category_ids alongside the new ones or you will
     silently unlink them.
   - set_limit for missing or corrected limits (omit the amount to clear one).
   - Retire an envelope that no longer applies by archiving it via
     update_envelope — there is no delete.
6. Call get_budget again and report what changed, with the new base-vs-additional
   split and anything still needing my decision.

Never reorganize a structure that is working just to fit the two-group model —
if my existing layout is deliberate, say so and leave it. Reply in my language.`,
			month)
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

Follow these steps:
1. Call list_budgets; if I have more than one budget, ask which one.
2. Call get_budget with the budget_id (and the month, if not current).
3. Compare limits against spending per envelope/category: flag anything overspent, and anything above 90%% of its limit.
4. If something looks unusual, sample the underlying activity with list_transactions.
5. Reply with a short structured review in my language: overall position, top overspends, notable items, one concrete suggestion.`,
			month)
		return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{
			{Role: "user", Content: &sdk.TextContent{Text: text}},
		}}, nil
	})
}
