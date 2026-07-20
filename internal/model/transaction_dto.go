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
// are vo.FlexString: the contract treats these as decimal strings, but FlexString
// keeps accepting the deprecated JSON-number form from third-party clients (see
// vo.FlexString's doc).
type CreateTransactionRequest struct {
	Id                 string         `json:"id"`
	Type               string         `json:"type"`
	Amount             vo.FlexString  `json:"amount" swaggertype:"string"`
	AmountRecipient    *vo.FlexString `json:"amountRecipient" swaggertype:"string"`
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
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: errs.CodeIsBlank})
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
	Amount             vo.FlexString  `json:"amount" swaggertype:"string"`
	AmountRecipient    *vo.FlexString `json:"amountRecipient" swaggertype:"string"`
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
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: errs.CodeIsBlank})
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
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

// DeleteTransactionResult is the delete response: {item, accounts}.
type DeleteTransactionResult struct {
	Item     TransactionResult `json:"item"`
	Accounts []AccountResult   `json:"accounts"`
}

// TransactionListRequest is the get-transaction-list query (all optional): by
// accountId, or by [periodStart, periodEnd), or neither (all visible). The
// classification filters (Uncategorized, CategoryId, PayeeId, TagId) are
// MCP-only — REST's get-transaction-list never populates them (see
// internal/transaction/api/transactionlist.go), so their zero values MUST
// leave GetTransactionList's behavior byte-identical to before they existed.
type TransactionListRequest struct {
	AccountId     string `json:"accountId"`
	PeriodStart   string `json:"periodStart"`
	PeriodEnd     string `json:"periodEnd"`
	Uncategorized bool   `json:"uncategorized,omitempty"`
	CategoryId    string `json:"categoryId,omitempty"`
	PayeeId       string `json:"payeeId,omitempty"`
	TagId         string `json:"tagId,omitempty"`
}

// Validate: every field is optional, but when present accountId/categoryId/
// payeeId/tagId must be UUIDs and periodStart/periodEnd must match the strict
// "Y-m-d H:i:s" datetime format. uncategorized and categoryId are mutually
// exclusive (uncategorized means categoryId IS NULL). The exact messages and
// field grouping for accountId/periodStart/periodEnd are wire-frozen.
func (r TransactionListRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.AccountId) != "" {
		if _, err := vo.ParseId(r.AccountId); err != nil {
			fields = append(fields, errs.FieldError{Key: "accountId", Message: "This value is not a valid UUID.", Code: errs.CodeInvalidUUID})
		}
	}
	for _, f := range []struct{ key, val string }{
		{"categoryId", r.CategoryId},
		{"payeeId", r.PayeeId},
		{"tagId", r.TagId},
	} {
		if strings.TrimSpace(f.val) == "" {
			continue
		}
		if _, err := vo.ParseId(f.val); err != nil {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value is not a valid UUID.", Code: errs.CodeInvalidUUID})
		}
	}
	if r.Uncategorized && strings.TrimSpace(r.CategoryId) != "" {
		fields = append(fields, errs.FieldError{Key: "categoryId", Message: "This value should not be provided when uncategorized is true.", Code: errs.CodeInvalidChoice})
	}
	for _, f := range []struct{ key, val string }{
		{"periodStart", r.PeriodStart},
		{"periodEnd", r.PeriodEnd},
	} {
		if strings.TrimSpace(f.val) == "" {
			continue
		}
		if _, err := time.Parse(datetime.Layout, f.val); err != nil {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value is not a valid datetime.", Code: errs.CodeInvalidDatetime})
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

// BulkUpdateTransactionsRequest is the MCP-only bulk_update_transactions
// input: re-classify (set or clear category/payee/tag on) an explicit list of
// transaction ids in one all-or-nothing call. There is no REST route for
// this. Amount/date/account/type are never touched. A nil pointer field means
// "leave unchanged"; the matching Clear* flag means "clear to NULL" — setting
// both for the same field is rejected.
type BulkUpdateTransactionsRequest struct {
	Ids           []string `json:"ids"`
	CategoryId    *string  `json:"categoryId"`
	PayeeId       *string  `json:"payeeId"`
	TagId         *string  `json:"tagId"`
	ClearCategory bool     `json:"clearCategory"`
	ClearPayee    bool     `json:"clearPayee"`
	ClearTag      bool     `json:"clearTag"`
}

// Validate enforces tier-1 shape: at least one id, at least one requested
// change, and no field both set and cleared at once. The id cap (100), UUID
// parsing, and the per-transaction access/reference/type invariants are
// tier-2 in Service.BulkUpdateTransactions — MCP tools call the service
// directly and never run Validate() (see internal/web/mcp), so that tier-2
// pass is the one MCP callers actually hit.
func (r BulkUpdateTransactionsRequest) Validate() error {
	var fields []errs.FieldError
	if len(r.Ids) == 0 {
		fields = append(fields, errs.FieldError{Key: "ids", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	for _, f := range []struct {
		key   string
		set   *string
		clear bool
	}{
		{"categoryId", r.CategoryId, r.ClearCategory},
		{"payeeId", r.PayeeId, r.ClearPayee},
		{"tagId", r.TagId, r.ClearTag},
	} {
		if f.set != nil && f.clear {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be provided together with the matching clear flag.", Code: errs.CodeInvalidChoice})
		}
	}
	if r.CategoryId == nil && r.PayeeId == nil && r.TagId == nil && !r.ClearCategory && !r.ClearPayee && !r.ClearTag {
		fields = append(fields, errs.FieldError{Key: "categoryId", Message: "At least one classification change is required.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// BulkUpdateTransactionsResult is the response: {updated: <count>}.
type BulkUpdateTransactionsResult struct {
	Updated int `json:"updated"`
}
