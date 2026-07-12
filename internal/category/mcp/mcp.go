// Package mcp is the category feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appcategory "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

func Register(read *appcategory.ReadService) webmcp.Register {
	return func(s *sdk.Server) {
		webmcp.AddJSONResource(s, "econumo://categories", "categories",
			"The user's transaction categories: id, name, type (expense|income), isArchived (0/1).",
			func(ctx context.Context, userID vo.Id) ([]model.CategoryResult, error) {
				res, err := read.GetCategoryList(ctx, userID)
				if err != nil {
					return nil, err
				}
				return res.Items, nil
			})
	}
}
