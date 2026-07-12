// Package mcp is the account feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

func Register(svc *appaccount.Service) webmcp.Register {
	return func(s *sdk.Server) {
		webmcp.AddJSONResource(s, "econumo://accounts", "accounts",
			"The user's accounts with current balances (as of end of the user's today): id, name, currency, balance, owner, sharedAccess.",
			func(ctx context.Context, userID vo.Id) ([]model.AccountResult, error) {
				return svc.AccountListForUser(ctx, userID)
			})
	}
}
