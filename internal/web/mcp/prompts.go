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
1. Read econumo://accounts, econumo://categories and econumo://payees.
2. Work out the type (expense unless clearly income or a transfer), the amount (decimal string), the date (default: today), and the best-matching account, category and payee (payee may be omitted).
3. Call create_transaction.
4. Confirm in one line what you logged: amount, currency, category, account, date.
If the amount or the account is ambiguous, ask me before creating anything.`,
			req.Params.Arguments["description"])
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
1. Read econumo://budgets; if I have more than one budget, ask which one.
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
