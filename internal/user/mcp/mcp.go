// Package mcp is the user feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

func Register(read *appuser.ReadService) webmcp.Register {
	return func(s *sdk.Server) {
		load := func(ctx context.Context, userID vo.Id) (model.GetUserDataResult, error) {
			u, err := read.GetUserData(ctx, userID)
			if err != nil {
				return model.GetUserDataResult{}, err
			}
			return *u, nil
		}

		sdk.AddTool(s, &sdk.Tool{Name: "get_user",
			Description: "The authenticated user's profile: id, name, email, avatar, base currency."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, model.GetUserDataResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "get_user")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetUserDataResult{}, err
				}
				doc, err := load(ctx, userID)
				if err != nil {
					return nil, model.GetUserDataResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, doc, nil
			})
	}
}
