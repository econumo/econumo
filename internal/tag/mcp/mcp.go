// Package mcp is the tag feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	apptag "github.com/econumo/econumo/internal/tag"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

type createTagInput struct {
	Name string `json:"name" jsonschema:"tag name, 3-64 characters"`
}

type updateTagInput struct {
	ID   string `json:"id" jsonschema:"tag id (UUID), from list_tags"`
	Name string `json:"name" jsonschema:"tag name, 3-64 characters"`
}

type setArchivedInput struct {
	ID       string `json:"id" jsonschema:"tag id (UUID), from list_tags"`
	Archived bool   `json:"archived" jsonschema:"true to archive, false to unarchive"`
}

// archivedResult is a small handler-built confirmation (not a REST DTO): both
// ArchiveTag and UnarchiveTag return empty result structs, and a typed MCP
// handler needs a single Out type.
type archivedResult struct {
	ID       string `json:"id"`
	Archived bool   `json:"archived"`
}

func Register(read *apptag.ReadService, write *apptag.Service) webmcp.Register {
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

		sdk.AddTool(s, &sdk.Tool{Name: "create_tag",
			Description: "Create a new transaction tag owned by the caller. See list_tags for existing ids."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createTagInput) (*sdk.CallToolResult, model.CreateTagResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_tag")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.CreateTagResult{}, err
				}
				res, err := write.CreateTag(ctx, userID, model.CreateTagRequest{
					Id:   vo.NewId().String(), // operation id, minted server-side for MCP
					Name: in.Name,
				})
				if err != nil {
					return nil, model.CreateTagResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "update_tag",
			Description: "Rename a tag; use list_tags to find its id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in updateTagInput) (*sdk.CallToolResult, model.UpdateTagResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "update_tag")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.UpdateTagResult{}, err
				}
				res, err := write.UpdateTag(ctx, userID, model.UpdateTagRequest{
					Id:   in.ID,
					Name: in.Name,
				})
				if err != nil {
					return nil, model.UpdateTagResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "set_tag_archived",
			Description: "Hide an unused tag (archive) or restore it (unarchive); there is no delete. Use list_tags to find its id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in setArchivedInput) (*sdk.CallToolResult, archivedResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "set_tag_archived")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, archivedResult{}, err
				}
				if in.Archived {
					if _, err := write.ArchiveTag(ctx, userID, model.ArchiveTagRequest{Id: in.ID}); err != nil {
						return nil, archivedResult{}, webmcp.MapErr(ctx, err)
					}
				} else {
					if _, err := write.UnarchiveTag(ctx, userID, model.UnarchiveTagRequest{Id: in.ID}); err != nil {
						return nil, archivedResult{}, webmcp.MapErr(ctx, err)
					}
				}
				return nil, archivedResult{ID: in.ID, Archived: in.Archived}, nil
			})
	}
}
