// Package mcp is the connection feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

func Register(svc *appconnection.Service) webmcp.Register {
	return func(s *sdk.Server) {
		load := func(ctx context.Context, userID vo.Id) (model.GetConnectionListResult, error) {
			result, err := svc.GetConnectionList(ctx, userID)
			if err != nil {
				return model.GetConnectionListResult{}, err
			}
			return *result, nil
		}

		sdk.AddTool(s, &sdk.Tool{Name: "list_connections",
			Description: "Users connected to the current user and the accounts they share."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, model.GetConnectionListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_connections")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetConnectionListResult{}, err
				}
				doc, err := load(ctx, userID)
				if err != nil {
					return nil, model.GetConnectionListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, doc, nil
			})
	}
}
