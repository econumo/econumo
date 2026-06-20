// Package transaction is the transaction aggregate's application layer: the
// request/result DTOs (with tier-1 Validate()), the write-side Service (owns the
// tx boundary, builds response DTOs), and the read side (get-transaction-list,
// export).
//
// JSON field names are frozen to the existing API wire contract; see
// COMPATIBILITY.md. create/update/delete results embed the full account list
// (built by the account module's service); the transaction result itself embeds
// the author (minimal user shape).
package transaction

import (
	"strings"

	appaccount "github.com/econumo/econumo/internal/app/account"
	"github.com/econumo/econumo/internal/domain/shared/errs"
)

// AuthorResult is the embedded transaction author: {id, avatar, name}.
type AuthorResult struct {
	Id     string `json:"id"`
	Avatar string `json:"avatar"`
	Name   string `json:"name"`
}

// TransactionResult is one transaction in the API. type is the alias string;
// amount/amountRecipient are normalized decimals (amountRecipient falls back to
// amount when null); date is "Y-m-d H:i:s". Optional ids are null when absent.
type TransactionResult struct {
	Id                 string       `json:"id"`
	Author             AuthorResult `json:"author"`
	Type               string       `json:"type"`
	AccountId          string       `json:"accountId"`
	AccountRecipientId *string      `json:"accountRecipientId"`
	Amount             string       `json:"amount"`
	AmountRecipient    *string      `json:"amountRecipient"`
	CategoryId         *string      `json:"categoryId"`
	Description        string       `json:"description"`
	PayeeId            *string      `json:"payeeId"`
	TagId              *string      `json:"tagId"`
	Date               string       `json:"date"`
}

// ---------------------------------------------------------------------------
// create-transaction
// ---------------------------------------------------------------------------

// CreateTransactionRequest is the create-transaction body. amount/amountRecipient
// accept number or string on the wire (the frontend may send either); decoded as
// json.Number-friendly strings via custom handling in the handler.
type CreateTransactionRequest struct {
	Id                 string  `json:"id"`
	Type               string  `json:"type"`
	Amount             string  `json:"amount"`
	AmountRecipient    *string `json:"amountRecipient"`
	AccountId          string  `json:"accountId"`
	AccountRecipientId *string `json:"accountRecipientId"`
	CategoryId         *string `json:"categoryId"`
	Date               string  `json:"date"`
	Description        *string `json:"description"`
	PayeeId            *string `json:"payeeId"`
	TagId              *string `json:"tagId"`
}

// Validate enforces tier-1 constraints: id/type/amount/accountId/date NotBlank,
// and for non-transfers categoryId is required (the PHP assembler dereferences
// categoryId unconditionally for non-transfers).
func (r CreateTransactionRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"id", r.Id}, {"type", r.Type}, {"amount", r.Amount},
		{"accountId", r.AccountId}, {"date", r.Date},
	} {
		if strings.TrimSpace(f.val) == "" {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// CreateTransactionResult is the create response: {item, accounts}.
type CreateTransactionResult struct {
	Item     TransactionResult          `json:"item"`
	Accounts []appaccount.AccountResult `json:"accounts"`
}

// ---------------------------------------------------------------------------
// update-transaction
// ---------------------------------------------------------------------------

// UpdateTransactionRequest is the update-transaction body (same fields as create
// minus the operation-id semantics; id is the transaction id).
type UpdateTransactionRequest struct {
	Id                 string  `json:"id"`
	Type               string  `json:"type"`
	Amount             string  `json:"amount"`
	AmountRecipient    *string `json:"amountRecipient"`
	AccountId          string  `json:"accountId"`
	AccountRecipientId *string `json:"accountRecipientId"`
	CategoryId         *string `json:"categoryId"`
	Date               string  `json:"date"`
	Description        *string `json:"description"`
	PayeeId            *string `json:"payeeId"`
	TagId              *string `json:"tagId"`
}

// Validate enforces tier-1 NotBlank on id/type/amount/accountId/date.
func (r UpdateTransactionRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"id", r.Id}, {"type", r.Type}, {"amount", r.Amount},
		{"accountId", r.AccountId}, {"date", r.Date},
	} {
		if strings.TrimSpace(f.val) == "" {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// UpdateTransactionResult is the update response: {item, accounts}.
type UpdateTransactionResult struct {
	Item     TransactionResult          `json:"item"`
	Accounts []appaccount.AccountResult `json:"accounts"`
}

// ---------------------------------------------------------------------------
// delete-transaction
// ---------------------------------------------------------------------------

// DeleteTransactionRequest is the delete-transaction body (id NotBlank).
type DeleteTransactionRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r DeleteTransactionRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// DeleteTransactionResult is the delete response: {item, accounts}.
type DeleteTransactionResult struct {
	Item     TransactionResult          `json:"item"`
	Accounts []appaccount.AccountResult `json:"accounts"`
}

// ---------------------------------------------------------------------------
// get-transaction-list
// ---------------------------------------------------------------------------

// GetTransactionListRequest is the get-transaction-list query (all optional):
// by accountId, or by [periodStart, periodEnd), or neither (all visible).
type GetTransactionListRequest struct {
	AccountId   string `json:"accountId"`
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
}

// GetTransactionListResult is the response: {items: [...]}.
type GetTransactionListResult struct {
	Items []TransactionResult `json:"items"`
}
