// Package mcp is the tag feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	apptag "github.com/econumo/econumo/internal/tag"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

func Register(read *apptag.ReadService) webmcp.Register {
	return func(s *sdk.Server) {
		webmcp.AddJSONResource(s, "econumo://tags", "tags",
			"The user's transaction tags: id, name, isArchived (0/1).",
			func(ctx context.Context, userID vo.Id) ([]model.TagResult, error) {
				res, err := read.GetTagList(ctx, userID)
				if err != nil {
					return nil, err
				}
				return res.Items, nil
			})
	}
}
