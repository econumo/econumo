// Package mcp is the payee feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/model"
	apppayee "github.com/econumo/econumo/internal/payee"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

func Register(read *apppayee.ReadService) webmcp.Register {
	return func(s *sdk.Server) {
		webmcp.AddJSONResource(s, "econumo://payees", "payees",
			"The user's payees: id, name, isArchived (0/1).",
			func(ctx context.Context, userID vo.Id) ([]model.PayeeResult, error) {
				res, err := read.GetPayeeList(ctx, userID)
				if err != nil {
					return nil, err
				}
				return res.Items, nil
			})
	}
}
