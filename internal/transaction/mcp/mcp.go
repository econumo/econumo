// Package mcp is the transaction feature's MCP edge: the write tools plus
// the filtered list read.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type listInput struct {
	AccountID   string `json:"account_id,omitempty" jsonschema:"filter by account id (UUID); may be combined with a full period window"`
	PeriodStart string `json:"period_start,omitempty" jsonschema:"inclusive start, YYYY-MM-DD or 'YYYY-MM-DD HH:MM:SS'; must be paired with period_end; may be combined with account_id"`
	PeriodEnd   string `json:"period_end,omitempty" jsonschema:"inclusive end, YYYY-MM-DD or 'YYYY-MM-DD HH:MM:SS'; must be paired with period_start; may be combined with account_id"`
}

type txFields struct {
	Type               string `json:"type" jsonschema:"expense, income or transfer"`
	Amount             string `json:"amount" jsonschema:"decimal string, e.g. 12.50"`
	AccountID          string `json:"account_id" jsonschema:"source account id (UUID)"`
	Date               string `json:"date" jsonschema:"YYYY-MM-DD or 'YYYY-MM-DD HH:MM:SS'"`
	CategoryID         string `json:"category_id,omitempty" jsonschema:"category id (UUID); required unless type is transfer"`
	AccountRecipientID string `json:"account_recipient_id,omitempty" jsonschema:"transfer target account id (UUID)"`
	AmountRecipient    string `json:"amount_recipient,omitempty" jsonschema:"received amount for cross-currency transfers"`
	Description        string `json:"description,omitempty"`
	PayeeID            string `json:"payee_id,omitempty" jsonschema:"payee id (UUID)"`
	TagID              string `json:"tag_id,omitempty" jsonschema:"tag id (UUID)"`
}

type createInput struct{ txFields }

type updateInput struct {
	ID string `json:"id" jsonschema:"transaction id (UUID)"`
	txFields
}

type deleteInput struct {
	ID string `json:"id" jsonschema:"transaction id (UUID)"`
}

// expand widens a date-only value to the wire datetime; end=true lands on the
// last second of the day so date-only ranges are inclusive.
func expand(s string, end bool) string {
	if len(s) == len(datetime.DateLayout) {
		if end {
			return s + " 23:59:59"
		}
		return s + " 00:00:00"
	}
	return s
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

func (f txFields) toRequestFields() (typ string, amount vo.FlexString, accountID, date string,
	categoryID, accountRecipientID *string, amountRecipient *vo.FlexString, description, payeeID, tagID *string) {
	return f.Type, vo.NewFlexString(f.Amount), f.AccountID, expand(f.Date, false),
		strPtr(f.CategoryID), strPtr(f.AccountRecipientID), flexPtr(f.AmountRecipient),
		strPtr(f.Description), strPtr(f.PayeeID), strPtr(f.TagID)
}

func Register(svc *apptransaction.Service) webmcp.Register {
	return func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "list_transactions",
			Description: "List the user's transactions, optionally filtered by account_id and/or a full period (period_start and period_end together); the filters compose."},
			func(ctx context.Context, req *sdk.CallToolRequest, in listInput) (*sdk.CallToolResult, model.GetTransactionListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_transactions")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetTransactionListResult{}, err
				}
				if (in.PeriodStart != "") != (in.PeriodEnd != "") {
					return nil, model.GetTransactionListResult{}, errs.NewValidation("period_start and period_end must be provided together")
				}
				res, err := svc.GetTransactionList(ctx, userID, model.TransactionListRequest{
					AccountId:   in.AccountID,
					PeriodStart: expand(in.PeriodStart, false),
					PeriodEnd:   expand(in.PeriodEnd, true),
				})
				if err != nil {
					return nil, model.GetTransactionListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "create_transaction",
			Description: "Record a new expense, income or transfer. Look up account/category/payee/tag ids with list_accounts/list_categories/list_payees/list_tags first."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createInput) (*sdk.CallToolResult, model.CreateTransactionResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_transaction")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.CreateTransactionResult{}, err
				}
				typ, amount, accountID, date, categoryID, accountRecipientID, amountRecipient, description, payeeID, tagID := in.toRequestFields()
				res, err := svc.CreateTransaction(ctx, userID, model.CreateTransactionRequest{
					Id:                 vo.NewId().String(), // operation id, minted server-side for MCP
					Type:               typ,
					Amount:             amount,
					AccountId:          accountID,
					AccountRecipientId: accountRecipientID,
					AmountRecipient:    amountRecipient,
					CategoryId:         categoryID,
					Date:               date,
					Description:        description,
					PayeeId:            payeeID,
					TagId:              tagID,
				})
				if err != nil {
					return nil, model.CreateTransactionResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "update_transaction",
			Description: "Update an existing transaction; send the full new field set (type, amount, account_id, date, ...)."},
			func(ctx context.Context, req *sdk.CallToolRequest, in updateInput) (*sdk.CallToolResult, model.UpdateTransactionResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "update_transaction")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.UpdateTransactionResult{}, err
				}
				typ, amount, accountID, date, categoryID, accountRecipientID, amountRecipient, description, payeeID, tagID := in.toRequestFields()
				res, err := svc.UpdateTransaction(ctx, userID, model.UpdateTransactionRequest{
					Id:                 in.ID,
					Type:               typ,
					Amount:             amount,
					AccountId:          accountID,
					AccountRecipientId: accountRecipientID,
					AmountRecipient:    amountRecipient,
					CategoryId:         categoryID,
					Date:               date,
					Description:        description,
					PayeeId:            payeeID,
					TagId:              tagID,
				})
				if err != nil {
					return nil, model.UpdateTransactionResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "delete_transaction",
			Description: "Delete a transaction by id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in deleteInput) (*sdk.CallToolResult, model.DeleteTransactionResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "delete_transaction")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.DeleteTransactionResult{}, err
				}
				res, err := svc.DeleteTransaction(ctx, userID, model.DeleteTransactionRequest{Id: in.ID})
				if err != nil {
					return nil, model.DeleteTransactionResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})
	}
}
