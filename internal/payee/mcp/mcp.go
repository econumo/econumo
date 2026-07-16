// Package mcp is the payee feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/model"
	apppayee "github.com/econumo/econumo/internal/payee"
	"github.com/econumo/econumo/internal/shared/reqctx"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

func Register(read *apppayee.ReadService) webmcp.Register {
	return func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "list_payees",
			Description: "The user's payees: id, name, isArchived (0/1)."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, model.GetPayeeListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_payees")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetPayeeListResult{}, err
				}
				res, err := read.GetPayeeList(ctx, userID)
				if err != nil {
					return nil, model.GetPayeeListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})
	}
}
