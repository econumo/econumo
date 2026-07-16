// Package mcp is the tag feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	apptag "github.com/econumo/econumo/internal/tag"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

func Register(read *apptag.ReadService) webmcp.Register {
	return func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "list_tags",
			Description: "The user's transaction tags: id, name, isArchived (0/1)."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, model.GetTagListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_tags")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetTagListResult{}, err
				}
				res, err := read.GetTagList(ctx, userID)
				if err != nil {
					return nil, model.GetTagListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})
	}
}
