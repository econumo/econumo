// Package mcp is the budget feature's MCP edge.
package mcp

import (
	"context"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

func Register(svc *appbudget.Service) webmcp.Register {
	return func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "list_budgets",
			Description: "The user's budgets; pass a budget id and month to the get_budget tool for monthly state."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, model.GetBudgetListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_budgets")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetBudgetListResult{}, err
				}
				res, err := svc.GetBudgetList(ctx, userID)
				if err != nil {
					return nil, model.GetBudgetListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		type getBudgetInput struct {
			BudgetID string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
			Month    string `json:"month,omitempty" jsonschema:"YYYY-MM; defaults to the current month"`
		}

		sdk.AddTool(s, &sdk.Tool{Name: "get_budget",
			Description: "Full monthly budget state: folders, envelopes, categories, tags, limits, spent and available amounts."},
			func(ctx context.Context, req *sdk.CallToolRequest, in getBudgetInput) (*sdk.CallToolResult, model.GetBudgetResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "get_budget")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetBudgetResult{}, err
				}
				date := ""
				if in.Month != "" {
					if _, perr := time.Parse("2006-01", in.Month); perr != nil {
						return nil, model.GetBudgetResult{}, errs.NewValidation("month must be YYYY-MM")
					}
					date = in.Month + "-01"
				}
				res, err := svc.GetBudget(ctx, userID, model.GetBudgetRequest{Id: in.BudgetID, Date: date})
				if err != nil {
					return nil, model.GetBudgetResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})
	}
}
