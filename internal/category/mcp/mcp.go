// Package mcp is the category feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appcategory "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

type createCategoryInput struct {
	Name string `json:"name" jsonschema:"category name, 3-64 characters"`
	Type string `json:"type" jsonschema:"expense or income"`
	Icon string `json:"icon,omitempty" jsonschema:"optional icon name"`
}

type updateCategoryInput struct {
	ID   string `json:"id" jsonschema:"category id (UUID), from list_categories"`
	Name string `json:"name" jsonschema:"category name, 3-64 characters"`
	Icon string `json:"icon" jsonschema:"icon name"`
}

type setArchivedInput struct {
	ID       string `json:"id" jsonschema:"category id (UUID), from list_categories"`
	Archived bool   `json:"archived" jsonschema:"true to archive, false to unarchive"`
}

// archivedResult is a small handler-built confirmation (not a REST DTO): both
// ArchiveCategory and UnarchiveCategory return empty result structs, and a
// typed MCP handler needs a single Out type.
type archivedResult struct {
	ID       string `json:"id"`
	Archived bool   `json:"archived"`
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type mergeCategoryInput struct {
	SourceID string `json:"sourceId" jsonschema:"id (UUID) of the category to absorb and DELETE, from list_categories"`
	TargetID string `json:"targetId" jsonschema:"id (UUID) of the category to keep, from list_categories"`
}

func Register(read *appcategory.ReadService, write *appcategory.Service) webmcp.Register {
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

		sdk.AddTool(s, &sdk.Tool{Name: "create_category",
			Description: "Create a new transaction category owned by the caller. See list_categories for existing ids."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createCategoryInput) (*sdk.CallToolResult, model.CreateCategoryResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_category")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.CreateCategoryResult{}, err
				}
				res, err := write.CreateCategory(ctx, userID, model.CreateCategoryRequest{
					Id:   vo.NewId().String(), // operation id, minted server-side for MCP
					Name: in.Name,
					Type: in.Type,
					Icon: strPtr(in.Icon),
				})
				if err != nil {
					return nil, model.CreateCategoryResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "merge_category",
			Description: "Merge one category into another: moves every transaction and recurring template from sourceId to targetId, then DELETES sourceId. Cannot be undone; confirm with the user before calling."},
			func(ctx context.Context, req *sdk.CallToolRequest, in mergeCategoryInput) (*sdk.CallToolResult, model.MergeCategoryResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "merge_category")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.MergeCategoryResult{}, err
				}
				res, err := write.MergeCategory(ctx, userID, model.MergeCategoryRequest{
					SourceId: in.SourceID,
					TargetId: in.TargetID,
				})
				if err != nil {
					return nil, model.MergeCategoryResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "update_category",
			Description: "Rename a category or change its icon; use list_categories to find its id. Type cannot be changed after creation."},
			func(ctx context.Context, req *sdk.CallToolRequest, in updateCategoryInput) (*sdk.CallToolResult, model.UpdateCategoryResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "update_category")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.UpdateCategoryResult{}, err
				}
				res, err := write.UpdateCategory(ctx, userID, model.UpdateCategoryRequest{
					Id:   in.ID,
					Name: in.Name,
					Icon: in.Icon,
				})
				if err != nil {
					return nil, model.UpdateCategoryResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "set_category_archived",
			Description: "Hide an unused category (archive) or restore it (unarchive); MCP has no delete (the app does). Use list_categories to find its id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in setArchivedInput) (*sdk.CallToolResult, archivedResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "set_category_archived")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, archivedResult{}, err
				}
				if in.Archived {
					if _, err := write.ArchiveCategory(ctx, userID, model.ArchiveCategoryRequest{Id: in.ID}); err != nil {
						return nil, archivedResult{}, webmcp.MapErr(ctx, err)
					}
				} else {
					if _, err := write.UnarchiveCategory(ctx, userID, model.UnarchiveCategoryRequest{Id: in.ID}); err != nil {
						return nil, archivedResult{}, webmcp.MapErr(ctx, err)
					}
				}
				return nil, archivedResult{ID: in.ID, Archived: in.Archived}, nil
			})
	}
}
