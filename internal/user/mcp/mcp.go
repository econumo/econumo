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

// ConnectionLister is the consumer-side port for the connection feature
// (features never import features; server wires the concrete service, whose
// GetConnectionList satisfies this directly).
type ConnectionLister interface {
	GetConnectionList(ctx context.Context, userID vo.Id) (*model.GetConnectionListResult, error)
}

type userDoc struct {
	User        model.CurrentUserResult  `json:"user"`
	Connections []model.ConnectionResult `json:"connections"`
}

type emptyInput struct{}

func Register(read *appuser.ReadService, connections ConnectionLister) webmcp.Register {
	return func(s *sdk.Server) {
		load := func(ctx context.Context, userID vo.Id) (userDoc, error) {
			u, err := read.GetUserData(ctx, userID)
			if err != nil {
				return userDoc{}, err
			}
			conns, err := connections.GetConnectionList(ctx, userID)
			if err != nil {
				return userDoc{}, err
			}
			return userDoc{User: u.User, Connections: conns.Items}, nil
		}

		webmcp.AddJSONResource(s, "econumo://user", "user",
			"The authenticated user's profile (id, name, email, avatar, base currency) and connected users with shared-account access.",
			load)

		sdk.AddTool(s, &sdk.Tool{Name: "get_user",
			Description: "The authenticated user's profile and connected users. Same data as econumo://user."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, userDoc, error) {
				reqctx.AddLogAttr(ctx, "tool", "get_user")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, userDoc{}, err
				}
				doc, err := load(ctx, userID)
				if err != nil {
					return nil, userDoc{}, webmcp.MapErr(ctx, err)
				}
				return nil, doc, nil
			})
	}
}
