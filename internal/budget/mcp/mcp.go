// Package mcp is the budget feature's MCP edge.
package mcp

import (
	"context"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

type createBudgetInput struct {
	Name       string `json:"name" jsonschema:"budget name"`
	CurrencyID string `json:"currency_id,omitempty" jsonschema:"currency id (UUID), from list_currencies; defaults to the user's currency"`
	StartDate  string `json:"start_date,omitempty" jsonschema:"YYYY-MM-DD; defaults to the current month"`
}

type updateBudgetInput struct {
	BudgetID   string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
	Name       string `json:"name" jsonschema:"budget name"`
	CurrencyID string `json:"currency_id" jsonschema:"currency id (UUID), from list_currencies"`
}

type createFolderInput struct {
	BudgetID string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
	Name     string `json:"name" jsonschema:"folder name"`
}

type updateFolderInput struct {
	BudgetID string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
	ID       string `json:"id" jsonschema:"folder id (UUID), from get_budget"`
	Name     string `json:"name" jsonschema:"folder name"`
}

type createEnvelopeInput struct {
	BudgetID    string   `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
	Name        string   `json:"name" jsonschema:"envelope name"`
	Icon        string   `json:"icon" jsonschema:"icon name"`
	CurrencyID  string   `json:"currency_id" jsonschema:"currency id (UUID), from list_currencies"`
	FolderID    string   `json:"folder_id,omitempty" jsonschema:"folder id (UUID), from get_budget; omit to leave the envelope ungrouped"`
	CategoryIDs []string `json:"category_ids,omitempty" jsonschema:"category ids (UUID) grouped under this envelope, from list_categories"`
}

type updateEnvelopeInput struct {
	BudgetID    string   `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
	ID          string   `json:"id" jsonschema:"envelope id (UUID), from get_budget"`
	Name        string   `json:"name" jsonschema:"envelope name"`
	Icon        string   `json:"icon" jsonschema:"icon name"`
	CurrencyID  string   `json:"currency_id" jsonschema:"currency id (UUID), from list_currencies"`
	CategoryIDs []string `json:"category_ids,omitempty" jsonschema:"category ids (UUID) grouped under this envelope, from list_categories; replaces the full set"`
	Archived    bool     `json:"archived" jsonschema:"true to archive the envelope (there is no delete), false to unarchive"`
}

type setLimitInput struct {
	BudgetID  string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
	ElementID string `json:"element_id" jsonschema:"envelope, category or tag id (UUID), from get_budget"`
	Month     string `json:"month" jsonschema:"YYYY-MM; must not be before the budget's start month"`
	Amount    string `json:"amount,omitempty" jsonschema:"decimal string, e.g. 150.00; omit to clear the limit"`
}

type setLimitResult struct {
	BudgetID  string `json:"budget_id"`
	ElementID string `json:"element_id"`
	Month     string `json:"month"`
	Amount    string `json:"amount"`
}

type setAccountIncludedInput struct {
	BudgetID  string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
	AccountID string `json:"account_id" jsonschema:"account id (UUID), from list_accounts"`
	Included  bool   `json:"included" jsonschema:"true to track this account's balance in the budget, false to exclude it"`
}

type accountIncludedResult struct {
	BudgetID  string `json:"budget_id"`
	AccountID string `json:"account_id"`
	Included  bool   `json:"included"`
}

type moveElementItemInput struct {
	ElementID string `json:"element_id" jsonschema:"envelope, tag or category id (UUID), from get_budget"`
	FolderID  string `json:"folder_id,omitempty" jsonschema:"target folder id (UUID), from get_budget; omit to move to the default (ungrouped) area"`
	Position  int    `json:"position" jsonschema:"0-based position within the target folder"`
}

type moveElementInput struct {
	BudgetID string                 `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
	Items    []moveElementItemInput `json:"items" jsonschema:"the elements to move; each names an element_id and its target folder_id + position"`
}

// moveElementResult reports no move count: MoveElementList silently skips any
// item whose element_id isn't found in the budget, so a count would overstate
// what actually happened. Verify the outcome with get_budget.
type moveElementResult struct {
	BudgetID   string   `json:"budget_id"`
	ElementIDs []string `json:"element_ids"`
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func flexPtr(s string) *vo.FlexString {
	if s == "" {
		return nil
	}
	f := vo.NewFlexString(s)
	return &f
}

func Register(svc *appbudget.Service) webmcp.Register {
	return func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "list_budgets",
			Description: "The user's budgets; pass a budget id and month to the get_budget tool for monthly state."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, model.GetBudgetListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_budgets")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetBudgetListResult{}, err
				}
				res, err := svc.GetBudgetList(ctx, userID)
				if err != nil {
					return nil, model.GetBudgetListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		type getBudgetInput struct {
			BudgetID string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
			Month    string `json:"month,omitempty" jsonschema:"YYYY-MM; defaults to the current month"`
		}

		sdk.AddTool(s, &sdk.Tool{Name: "get_budget",
			Description: "Full monthly budget state: folders, envelopes, categories, tags, limits, spent and available amounts."},
			func(ctx context.Context, req *sdk.CallToolRequest, in getBudgetInput) (*sdk.CallToolResult, model.GetBudgetResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "get_budget")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetBudgetResult{}, err
				}
				date := ""
				if in.Month != "" {
					if _, perr := time.Parse("2006-01", in.Month); perr != nil {
						return nil, model.GetBudgetResult{}, errs.NewValidation("month must be YYYY-MM")
					}
					date = in.Month + "-01"
				}
				res, err := svc.GetBudget(ctx, userID, model.GetBudgetRequest{Id: in.BudgetID, Date: date})
				if err != nil {
					return nil, model.GetBudgetResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "create_budget",
			Description: "Create a new budget for the caller, seeded with their existing categories and tags. Use list_currencies for currency_id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createBudgetInput) (*sdk.CallToolResult, model.CreateBudgetResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_budget")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.CreateBudgetResult{}, err
				}
				res, err := svc.CreateBudget(ctx, userID, model.CreateBudgetRequest{
					Id:         vo.NewId().String(), // entity id, minted server-side for MCP
					Name:       in.Name,
					StartDate:  in.StartDate,
					CurrencyId: in.CurrencyID,
				})
				if err != nil {
					return nil, model.CreateBudgetResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "update_budget",
			Description: "Rename a budget or change its currency. Excluded accounts are left untouched (use set_budget_account_included to change them). Use list_budgets to find its id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in updateBudgetInput) (*sdk.CallToolResult, model.UpdateBudgetResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "update_budget")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.UpdateBudgetResult{}, err
				}
				// UpdateBudget's ExcludedAccounts field is authoritative: it replaces the
				// full excluded-account set (internal/budget/crud.go), and account
				// inclusion is this tool surface's own separate concern
				// (set_budget_account_included). Round-trip the CURRENT excluded set here,
				// or an update would silently re-include every previously excluded account.
				current, err := svc.GetBudget(ctx, userID, model.GetBudgetRequest{Id: in.BudgetID})
				if err != nil {
					return nil, model.UpdateBudgetResult{}, webmcp.MapErr(ctx, err)
				}
				res, err := svc.UpdateBudget(ctx, userID, model.UpdateBudgetRequest{
					Id:               in.BudgetID,
					Name:             in.Name,
					CurrencyId:       in.CurrencyID,
					ExcludedAccounts: current.Item.Filters.ExcludedAccountsIds,
				})
				if err != nil {
					return nil, model.UpdateBudgetResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "create_folder",
			Description: "Create a folder to group budget envelopes, categories and tags. Use list_budgets for budget_id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createFolderInput) (*sdk.CallToolResult, model.CreateBudgetFolderResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_folder")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.CreateBudgetFolderResult{}, err
				}
				res, err := svc.CreateFolder(ctx, userID, model.CreateBudgetFolderRequest{
					BudgetId: in.BudgetID,
					Id:       vo.NewId().String(), // entity id, minted server-side for MCP
					Name:     in.Name,
				})
				if err != nil {
					return nil, model.CreateBudgetFolderResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "update_folder",
			Description: "Rename a budget folder. Use get_budget to find its id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in updateFolderInput) (*sdk.CallToolResult, model.UpdateBudgetFolderResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "update_folder")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.UpdateBudgetFolderResult{}, err
				}
				res, err := svc.UpdateFolder(ctx, userID, model.UpdateBudgetFolderRequest{
					BudgetId: in.BudgetID,
					Id:       in.ID,
					Name:     in.Name,
				})
				if err != nil {
					return nil, model.UpdateBudgetFolderResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "create_envelope",
			Description: "Create a budget envelope that groups one or more categories under a shared limit. Use list_currencies for currency_id, list_categories for category_ids, get_budget for folder_id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createEnvelopeInput) (*sdk.CallToolResult, model.CreateEnvelopeResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_envelope")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.CreateEnvelopeResult{}, err
				}
				res, err := svc.CreateEnvelope(ctx, userID, model.CreateEnvelopeRequest{
					BudgetId:   in.BudgetID,
					Id:         vo.NewId().String(), // entity id, minted server-side for MCP
					Name:       in.Name,
					Icon:       in.Icon,
					CurrencyId: in.CurrencyID,
					FolderId:   strPtr(in.FolderID),
					Categories: in.CategoryIDs,
				})
				if err != nil {
					return nil, model.CreateEnvelopeResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "update_envelope",
			Description: "Update an envelope's name/icon/currency/categories, or archive/unarchive it (there is no delete). Send the full field set including category_ids, which replaces the group. Use get_budget for its id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in updateEnvelopeInput) (*sdk.CallToolResult, model.UpdateEnvelopeResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "update_envelope")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.UpdateEnvelopeResult{}, err
				}
				isArchived := 0
				if in.Archived {
					isArchived = 1
				}
				res, err := svc.UpdateEnvelope(ctx, userID, model.UpdateEnvelopeRequest{
					BudgetId:   in.BudgetID,
					Id:         in.ID,
					Name:       in.Name,
					Icon:       in.Icon,
					CurrencyId: in.CurrencyID,
					IsArchived: isArchived,
					Categories: in.CategoryIDs,
				})
				if err != nil {
					return nil, model.UpdateEnvelopeResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "set_limit",
			Description: "Set or clear an envelope/category/tag's limit for one month. Use get_budget for element_id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in setLimitInput) (*sdk.CallToolResult, setLimitResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "set_limit")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, setLimitResult{}, err
				}
				if _, perr := time.Parse("2006-01", in.Month); perr != nil {
					return nil, setLimitResult{}, errs.NewValidation("month must be YYYY-MM")
				}
				_, err = svc.SetLimit(ctx, userID, model.SetLimitRequest{
					BudgetId:  in.BudgetID,
					ElementId: in.ElementID,
					Period:    in.Month + "-01",
					Amount:    flexPtr(in.Amount),
				})
				if err != nil {
					return nil, setLimitResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, setLimitResult{BudgetID: in.BudgetID, ElementID: in.ElementID, Month: in.Month, Amount: in.Amount}, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "move_element",
			Description: "Move budget elements (envelopes, tags, standalone categories) into folders or reorder them. Use get_budget for element_id and folder_id; omit folder_id to move an element to the default ungrouped area."},
			func(ctx context.Context, req *sdk.CallToolRequest, in moveElementInput) (*sdk.CallToolResult, moveElementResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "move_element")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, moveElementResult{}, err
				}
				items := make([]model.MoveElementListItem, 0, len(in.Items))
				for _, it := range in.Items {
					items = append(items, model.MoveElementListItem{
						Id:       it.ElementID,
						FolderId: strPtr(it.FolderID),
						Position: it.Position,
					})
				}
				if _, err := svc.MoveElementList(ctx, userID, model.MoveElementListRequest{
					BudgetId: in.BudgetID,
					Items:    items,
				}); err != nil {
					return nil, moveElementResult{}, webmcp.MapErr(ctx, err)
				}
				elementIDs := make([]string, 0, len(in.Items))
				for _, it := range in.Items {
					elementIDs = append(elementIDs, it.ElementID)
				}
				return nil, moveElementResult{BudgetID: in.BudgetID, ElementIDs: elementIDs}, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "set_budget_account_included",
			Description: "Include or exclude an owned account from a budget's tracked balances. Use list_accounts for account_id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in setAccountIncludedInput) (*sdk.CallToolResult, accountIncludedResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "set_budget_account_included")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, accountIncludedResult{}, err
				}
				if in.Included {
					if _, err := svc.IncludeAccount(ctx, userID, model.IncludeAccountRequest{BudgetId: in.BudgetID, AccountId: in.AccountID}); err != nil {
						return nil, accountIncludedResult{}, webmcp.MapErr(ctx, err)
					}
				} else {
					if _, err := svc.ExcludeAccount(ctx, userID, model.ExcludeAccountRequest{BudgetId: in.BudgetID, AccountId: in.AccountID}); err != nil {
						return nil, accountIncludedResult{}, webmcp.MapErr(ctx, err)
					}
				}
				return nil, accountIncludedResult{BudgetID: in.BudgetID, AccountID: in.AccountID, Included: in.Included}, nil
			})
	}
}
