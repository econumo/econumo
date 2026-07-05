// Request/result DTOs for the transaction use cases (tier-1 Validate()).
// JSON field names are frozen to the existing API wire contract; see
// CLAUDE.md.
package model

import (
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// TransactionResult is one transaction in the API. type is the alias string;
// amount/amountRecipient are normalized decimals (amountRecipient falls back to
// amount when null); date is "Y-m-d H:i:s". Optional ids are null when absent.
type TransactionResult struct {
	Id                 string     `json:"id"`
	Author             UserResult `json:"author"`
	Type               string     `json:"type"`
	AccountId          string     `json:"accountId"`
	AccountRecipientId *string    `json:"accountRecipientId"`
	Amount             string     `json:"amount"`
	AmountRecipient    *string    `json:"amountRecipient"`
	CategoryId         *string    `json:"categoryId"`
	Description        string     `json:"description"`
	PayeeId            *string    `json:"payeeId"`
	TagId              *string    `json:"tagId"`
	Date               string     `json:"date"`
}

// CreateTransactionRequest is the create-transaction body. amount/amountRecipient
// are vo.FlexString: the frontend posts them as JSON numbers, the contract treats
// them as decimal strings, and FlexString accepts either (see its doc).
type CreateTransactionRequest struct {
	Id                 string         `json:"id"`
	Type               string         `json:"type"`
	Amount             vo.FlexString  `json:"amount"`
	AmountRecipient    *vo.FlexString `json:"amountRecipient"`
	AccountId          string         `json:"accountId"`
	AccountRecipientId *string        `json:"accountRecipientId"`
	CategoryId         *string        `json:"categoryId"`
	Date               string         `json:"date"`
	Description        *string        `json:"description"`
	PayeeId            *string        `json:"payeeId"`
	TagId              *string        `json:"tagId"`
}

// Validate enforces tier-1 NotBlank on id/type/amount/accountId/date. (For
// non-transfers categoryId is required, but that is re-checked tier-2 in
// buildState.)
func (r CreateTransactionRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"id", r.Id}, {"type", r.Type}, {"amount", r.Amount.String()},
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
	Item     TransactionResult `json:"item"`
	Accounts []AccountResult   `json:"accounts"`
}

// UpdateTransactionRequest is the update-transaction body (same fields as create
// minus the operation-id semantics; id is the transaction id).
type UpdateTransactionRequest struct {
	Id                 string         `json:"id"`
	Type               string         `json:"type"`
	Amount             vo.FlexString  `json:"amount"`
	AmountRecipient    *vo.FlexString `json:"amountRecipient"`
	AccountId          string         `json:"accountId"`
	AccountRecipientId *string        `json:"accountRecipientId"`
	CategoryId         *string        `json:"categoryId"`
	Date               string         `json:"date"`
	Description        *string        `json:"description"`
	PayeeId            *string        `json:"payeeId"`
	TagId              *string        `json:"tagId"`
}

// Validate enforces tier-1 NotBlank on id/type/amount/accountId/date.
func (r UpdateTransactionRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"id", r.Id}, {"type", r.Type}, {"amount", r.Amount.String()},
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
	Item     TransactionResult `json:"item"`
	Accounts []AccountResult   `json:"accounts"`
}

// DeleteTransactionRequest is the delete-transaction body (id NotBlank).
type DeleteTransactionRequest struct {
	Id string `json:"id"`
}

func (r DeleteTransactionRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// DeleteTransactionResult is the delete response: {item, accounts}.
type DeleteTransactionResult struct {
	Item     TransactionResult `json:"item"`
	Accounts []AccountResult   `json:"accounts"`
}

// TransactionListRequest is the get-transaction-list query (all optional): by
// accountId, or by [periodStart, periodEnd), or neither (all visible).
type TransactionListRequest struct {
	AccountId   string `json:"accountId"`
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
}

// Validate: every field is optional, but when present accountId must be a UUID
// and periodStart/periodEnd must match the strict "Y-m-d H:i:s" datetime format.
// The exact messages and field grouping are wire-frozen.
func (r TransactionListRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.AccountId) != "" {
		if _, err := vo.ParseId(r.AccountId); err != nil {
			fields = append(fields, errs.FieldError{Key: "accountId", Message: "This value is not a valid UUID."})
		}
	}
	for _, f := range []struct{ key, val string }{
		{"periodStart", r.PeriodStart},
		{"periodEnd", r.PeriodEnd},
	} {
		if strings.TrimSpace(f.val) == "" {
			continue
		}
		if _, err := time.Parse(datetime.Layout, f.val); err != nil {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value is not a valid datetime."})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Form validation error", fields...)
	}
	return nil
}

// GetTransactionListResult is the response: {items: [...]}.
type GetTransactionListResult struct {
	Items []TransactionResult `json:"items"`
}
