// Package mcp is the category feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appcategory "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

func Register(read *appcategory.ReadService) webmcp.Register {
	return func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "list_categories",
			Description: "The user's transaction categories: id, name, type (expense|income), isArchived (0/1)."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, model.GetCategoryListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_categories")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetCategoryListResult{}, err
				}
				res, err := read.GetCategoryList(ctx, userID)
				if err != nil {
					return nil, model.GetCategoryListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})
	}
}
