// Package mcp is the account feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

func Register(svc *appaccount.Service) webmcp.Register {
	return func(s *sdk.Server) {
		webmcp.AddJSONResource(s, "econumo://accounts", "accounts",
			"The user's accounts with current balances (as of end of the user's today): id, name, currency, balance, owner, sharedAccess.",
			func(ctx context.Context, userID vo.Id) ([]model.AccountResult, error) {
				return svc.AccountListForUser(ctx, userID)
			})

		sdk.AddTool(s, &sdk.Tool{Name: "list_accounts",
			Description: "The user's accounts with current balances. Same data as econumo://accounts."},
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
