// Package mcp is the budget feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

func Register(svc *appbudget.Service) webmcp.Register {
	return func(s *sdk.Server) {
		webmcp.AddJSONResource(s, "econumo://budgets", "budgets",
			"The user's budgets (id, name, currency); pass a budget id and month to the get_budget tool for monthly state.",
			func(ctx context.Context, userID vo.Id) ([]model.MetaResult, error) {
				res, err := svc.GetBudgetList(ctx, userID)
				if err != nil {
					return nil, err
				}
				return res.Items, nil
			})
	}
}
