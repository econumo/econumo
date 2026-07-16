// Package mcp is the account feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

func Register(svc *appaccount.Service) webmcp.Register {
	return func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "list_accounts",
			Description: "The user's accounts with current balances."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, model.GetAccountListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_accounts")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetAccountListResult{}, err
				}
				items, err := svc.AccountListForUser(ctx, userID)
				if err != nil {
					return nil, model.GetAccountListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, model.GetAccountListResult{Items: items}, nil
			})
	}
}
