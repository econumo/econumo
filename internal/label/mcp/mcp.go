// Package mcp is the label feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	applabel "github.com/econumo/econumo/internal/label"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

type createLabelInput struct {
	Name string `json:"name" jsonschema:"label name, 3-64 characters"`
}

type updateLabelInput struct {
	ID   string `json:"id" jsonschema:"label id (UUID), from list_labels"`
	Name string `json:"name" jsonschema:"label name, 3-64 characters"`
}

type setArchivedInput struct {
	ID       string `json:"id" jsonschema:"label id (UUID), from list_labels"`
	Archived bool   `json:"archived" jsonschema:"true to archive, false to unarchive"`
}

// archivedResult is a small handler-built confirmation (not a REST DTO): both
// ArchiveLabel and UnarchiveLabel return empty result structs, and a typed MCP
// handler needs a single Out type.
type archivedResult struct {
	ID       string `json:"id"`
	Archived bool   `json:"archived"`
}

type mergeLabelInput struct {
	SourceID string `json:"sourceId" jsonschema:"id (UUID) of the reporting tag to absorb and DELETE, from list_labels"`
	TargetID string `json:"targetId" jsonschema:"id (UUID) of the reporting tag to keep, from list_labels"`
}

func Register(read *applabel.ReadService, write *applabel.Service) webmcp.Register {
	return func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "list_labels",
			Description: "The user's reporting labels (cross-cutting, budget-independent classifications): id, name, isArchived (0/1)."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, model.GetLabelListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_labels")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetLabelListResult{}, err
				}
				res, err := read.GetLabelList(ctx, userID)
				if err != nil {
					return nil, model.GetLabelListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "create_label",
			Description: "Create a new reporting label owned by the caller. See list_labels for existing ids."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createLabelInput) (*sdk.CallToolResult, model.CreateLabelResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_label")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.CreateLabelResult{}, err
				}
				res, err := write.CreateLabel(ctx, userID, model.CreateLabelRequest{
					Id:   vo.NewId().String(), // operation id, minted server-side for MCP
					Name: in.Name,
				})
				if err != nil {
					return nil, model.CreateLabelResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "merge_label",
			Description: "Merge one reporting tag into another: moves every transaction and recurring template from sourceId to targetId, then DELETES sourceId. Cannot be undone; confirm with the user before calling."},
			func(ctx context.Context, req *sdk.CallToolRequest, in mergeLabelInput) (*sdk.CallToolResult, model.MergeLabelResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "merge_label")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.MergeLabelResult{}, err
				}
				res, err := write.MergeLabel(ctx, userID, model.MergeLabelRequest{
					SourceId: in.SourceID,
					TargetId: in.TargetID,
				})
				if err != nil {
					return nil, model.MergeLabelResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "update_label",
			Description: "Rename a label; use list_labels to find its id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in updateLabelInput) (*sdk.CallToolResult, model.UpdateLabelResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "update_label")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.UpdateLabelResult{}, err
				}
				res, err := write.UpdateLabel(ctx, userID, model.UpdateLabelRequest{
					Id:   in.ID,
					Name: in.Name,
				})
				if err != nil {
					return nil, model.UpdateLabelResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "set_label_archived",
			Description: "Hide an unused label (archive) or restore it (unarchive); MCP has no delete (the app does). Use list_labels to find its id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in setArchivedInput) (*sdk.CallToolResult, archivedResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "set_label_archived")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, archivedResult{}, err
				}
				if in.Archived {
					if _, err := write.ArchiveLabel(ctx, userID, model.ArchiveLabelRequest{Id: in.ID}); err != nil {
						return nil, archivedResult{}, webmcp.MapErr(ctx, err)
					}
				} else {
					if _, err := write.UnarchiveLabel(ctx, userID, model.UnarchiveLabelRequest{Id: in.ID}); err != nil {
						return nil, archivedResult{}, webmcp.MapErr(ctx, err)
					}
				}
				return nil, archivedResult{ID: in.ID, Archived: in.Archived}, nil
			})
	}
}
