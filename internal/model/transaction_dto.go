// Request/result DTOs for the transaction use cases (tier-1 Validate()).
// JSON field names are frozen to the existing API wire contract; see
// CLAUDE.md.
package model

import (
	"strconv"
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

// TransactionListRequest is the get-transaction-list query (all optional).
// Modes: accountId alone (full per-account list), periodStart+periodEnd
// (window across visible accounts), accountId+limit[+cursor] (keyset page),
// perAccountLimit (newest N per visible account), or nothing (all visible).
type TransactionListRequest struct {
	AccountId       string `json:"accountId"`
	PeriodStart     string `json:"periodStart"`
	PeriodEnd       string `json:"periodEnd"`
	Limit           string `json:"limit"`
	Cursor          string `json:"cursor"`
	PerAccountLimit string `json:"perAccountLimit"`
}

// boundedInt reports whether v parses as an integer in [1, 500].
func boundedInt(v string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	return err == nil && n >= 1 && n <= 500
}

// LimitValue returns the parsed limit; call only after Validate succeeded.
func (r TransactionListRequest) LimitValue() int {
	n, _ := strconv.Atoi(strings.TrimSpace(r.Limit))
	return n
}

// PerAccountLimitValue returns the parsed perAccountLimit; call only after
// Validate succeeded.
func (r TransactionListRequest) PerAccountLimitValue() int {
	n, _ := strconv.Atoi(strings.TrimSpace(r.PerAccountLimit))
	return n
}

// Validate: every field is optional, but when present accountId must be a UUID,
// periodStart/periodEnd must match the strict "Y-m-d H:i:s" datetime format,
// limit/perAccountLimit must be integers in [1,500], and the paging params must
// form a consistent mode (see the struct doc). The exact messages and field
// grouping are wire-frozen.
func (r TransactionListRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.AccountId) != "" {
		if _, err := vo.ParseId(r.AccountId); err != nil {
			fields = append(fields, errs.FieldError{Key: "accountId", Message: "This value is not a valid UUID.", Code: errs.CodeInvalidUUID})
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
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value is not a valid datetime.", Code: errs.CodeInvalidDatetime})
		}
	}
	if r.PerAccountLimit != "" {
		if !boundedInt(r.PerAccountLimit) {
			fields = append(fields, errs.FieldError{Key: "perAccountLimit", Message: "This value should be an integer between 1 and 500."})
		}
		if r.AccountId != "" || r.Limit != "" || r.Cursor != "" || r.PeriodStart != "" || r.PeriodEnd != "" {
			fields = append(fields, errs.FieldError{Key: "perAccountLimit", Message: "perAccountLimit cannot be combined with other parameters."})
		}
	}
	if r.Limit != "" {
		if !boundedInt(r.Limit) {
			fields = append(fields, errs.FieldError{Key: "limit", Message: "This value should be an integer between 1 and 500."})
		}
		if r.AccountId == "" {
			fields = append(fields, errs.FieldError{Key: "limit", Message: "limit requires accountId."})
		}
		if r.PeriodStart != "" || r.PeriodEnd != "" {
			fields = append(fields, errs.FieldError{Key: "limit", Message: "limit cannot be combined with periodStart or periodEnd."})
		}
	}
	if r.Cursor != "" && r.Limit == "" {
		fields = append(fields, errs.FieldError{Key: "cursor", Message: "cursor requires limit."})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Form validation error", fields...)
	}
	return nil
}

// TransactionPageResult is the page-mode pagination block.
type TransactionPageResult struct {
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

// TransactionAccountPageResult is one account's pagination state in boot mode.
type TransactionAccountPageResult struct {
	Id         string  `json:"id"`
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

// GetTransactionListResult is the response: {items: [...]}. page appears only
// in page mode, accounts only in boot mode; both omitted otherwise so legacy
// responses stay byte-identical.
type GetTransactionListResult struct {
	Items    []TransactionResult            `json:"items"`
	Page     *TransactionPageResult         `json:"page,omitempty"`
	Accounts []TransactionAccountPageResult `json:"accounts,omitempty"`
}
