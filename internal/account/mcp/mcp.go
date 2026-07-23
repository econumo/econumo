// Package mcp is the account feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type emptyInput struct{}

type createAccountInput struct {
	Name       string `json:"name" jsonschema:"account name"`
	CurrencyID string `json:"currency_id" jsonschema:"currency id (UUID), from list_currencies"`
	Balance    string `json:"balance,omitempty" jsonschema:"opening balance as a decimal string, e.g. 100.00; defaults to 0"`
	Icon       string `json:"icon,omitempty" jsonschema:"optional icon name; defaults to 'wallet'"`
	FolderID   string `json:"folder_id,omitempty" jsonschema:"account folder id (UUID), from list_accounts; omit to use the user's default folder (new users get one created automatically)"`
}

type createAccountResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CurrencyID string `json:"currency_id"`
	Balance    string `json:"balance"`
}

func Register(svc *appaccount.Service) webmcp.Register {
	return func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "list_accounts",
			Description: "The user's accounts with current balances."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, model.GetAccountListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_accounts")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetAccountListResult{}, err
				}
				items, err := svc.AccountListForUser(ctx, userID)
				if err != nil {
					return nil, model.GetAccountListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, model.GetAccountListResult{Items: items}, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "create_account",
			Description: "Create an account for the caller. currency_id is required (use list_currencies). Optional opening balance and icon. New users get a default account folder automatically."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createAccountInput) (*sdk.CallToolResult, createAccountResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_account")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, createAccountResult{}, err
				}
				icon := in.Icon
				if icon == "" {
					icon = "wallet"
				}
				balance := in.Balance
				if balance == "" {
					balance = "0"
				}
				res, err := svc.CreateAccount(ctx, userID, model.CreateAccountRequest{
					Id:         vo.NewId().String(), // operation id; the service mints the entity id
					Name:       in.Name,
					CurrencyId: in.CurrencyID,
					Balance:    vo.NewFlexString(balance),
					Icon:       icon,
					FolderId:   in.FolderID,
					// A blank FolderId is tolerated for a first account.
				})
				if err != nil {
					return nil, createAccountResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, createAccountResult{
					ID:         res.Item.Id,
					Name:       res.Item.Name,
					CurrencyID: res.Item.Currency.Id,
					Balance:    res.Item.Balance,
				}, nil
			})
	}
}
