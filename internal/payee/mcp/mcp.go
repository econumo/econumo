// Package mcp is the payee feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/model"
	apppayee "github.com/econumo/econumo/internal/payee"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

type createPayeeInput struct {
	Name string `json:"name" jsonschema:"payee name, 3-64 characters"`
}

type updatePayeeInput struct {
	ID   string `json:"id" jsonschema:"payee id (UUID), from list_payees"`
	Name string `json:"name" jsonschema:"payee name, 3-64 characters"`
}

type setArchivedInput struct {
	ID       string `json:"id" jsonschema:"payee id (UUID), from list_payees"`
	Archived bool   `json:"archived" jsonschema:"true to archive, false to unarchive"`
}

// archivedResult is a small handler-built confirmation (not a REST DTO): both
// ArchivePayee and UnarchivePayee return empty result structs, and a typed
// MCP handler needs a single Out type.
type archivedResult struct {
	ID       string `json:"id"`
	Archived bool   `json:"archived"`
}

func Register(read *apppayee.ReadService, write *apppayee.Service) webmcp.Register {
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

		sdk.AddTool(s, &sdk.Tool{Name: "create_payee",
			Description: "Create a new payee owned by the caller. See list_payees for existing ids."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createPayeeInput) (*sdk.CallToolResult, model.CreatePayeeResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_payee")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.CreatePayeeResult{}, err
				}
				res, err := write.CreatePayee(ctx, userID, model.CreatePayeeRequest{
					Id:   vo.NewId().String(), // operation id, minted server-side for MCP
					Name: in.Name,
				})
				if err != nil {
					return nil, model.CreatePayeeResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "update_payee",
			Description: "Rename a payee; use list_payees to find its id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in updatePayeeInput) (*sdk.CallToolResult, model.UpdatePayeeResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "update_payee")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.UpdatePayeeResult{}, err
				}
				res, err := write.UpdatePayee(ctx, userID, model.UpdatePayeeRequest{
					Id:   in.ID,
					Name: in.Name,
				})
				if err != nil {
					return nil, model.UpdatePayeeResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "set_payee_archived",
			Description: "Hide an unused payee (archive) or restore it (unarchive); there is no delete. Use list_payees to find its id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in setArchivedInput) (*sdk.CallToolResult, archivedResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "set_payee_archived")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, archivedResult{}, err
				}
				if in.Archived {
					if _, err := write.ArchivePayee(ctx, userID, model.ArchivePayeeRequest{Id: in.ID}); err != nil {
						return nil, archivedResult{}, webmcp.MapErr(ctx, err)
					}
				} else {
					if _, err := write.UnarchivePayee(ctx, userID, model.UnarchivePayeeRequest{Id: in.ID}); err != nil {
						return nil, archivedResult{}, webmcp.MapErr(ctx, err)
					}
				}
				return nil, archivedResult{ID: in.ID, Archived: in.Archived}, nil
			})
	}
}
