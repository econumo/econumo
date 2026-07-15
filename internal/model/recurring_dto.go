// Request/result DTOs for the recurring transaction use cases (tier-1 Validate()).
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

// RecurringTransactionResult is one recurring transaction in the API. type is
// the alias string; amount is a normalized decimal; nextPaymentAt is
// "2006-01-02 15:04:05" (space separator, no timezone). Optional ids are null
// when absent.
type RecurringTransactionResult struct {
	Id                 string  `json:"id"`
	OwnerUserId        string  `json:"ownerUserId"`
	Type               string  `json:"type"`
	AccountId          string  `json:"accountId"`
	AccountRecipientId *string `json:"accountRecipientId"`
	Amount             string  `json:"amount"`
	CategoryId         *string `json:"categoryId"`
	PayeeId            *string `json:"payeeId"`
	TagId              *string `json:"tagId"`
	Description        string  `json:"description"`
	Schedule           string  `json:"schedule"`
	NextPaymentAt      string  `json:"nextPaymentAt"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

// GetRecurringTransactionListResult is the response: {items: [...]}.
type GetRecurringTransactionListResult struct {
	Items []RecurringTransactionResult `json:"items"`
}

// CreateRecurringTransactionRequest is the create-recurring-transaction body.
// amount is vo.FlexString: the frontend posts it as a JSON number, the contract
// treats it as a decimal string, and FlexString accepts either (see its doc).
type CreateRecurringTransactionRequest struct {
	Id                 string        `json:"id"`
	Type               string        `json:"type"`
	Amount             vo.FlexString `json:"amount"`
	AccountId          string        `json:"accountId"`
	AccountRecipientId *string       `json:"accountRecipientId"`
	CategoryId         *string       `json:"categoryId"`
	PayeeId            *string       `json:"payeeId"`
	TagId              *string       `json:"tagId"`
	Description        *string       `json:"description"`
	Schedule           string        `json:"schedule"`
	NextPaymentAt      string        `json:"nextPaymentAt"`
}

// Validate enforces tier-1 NotBlank on id/type/amount/accountId/schedule/nextPaymentAt
// and checks that schedule is valid and nextPaymentAt matches the datetime format.
func (r CreateRecurringTransactionRequest) Validate() error {
	return validateRecurringFields(r.Id, r.Type, r.Amount.String(), r.AccountId, r.Schedule, r.NextPaymentAt)
}

// CreateRecurringTransactionResult is the create response: {item}.
type CreateRecurringTransactionResult struct {
	Item RecurringTransactionResult `json:"item"`
}

// UpdateRecurringTransactionRequest is the update-recurring-transaction body
// (same fields as create; id is the recurring transaction id).
type UpdateRecurringTransactionRequest struct {
	Id                 string        `json:"id"`
	Type               string        `json:"type"`
	Amount             vo.FlexString `json:"amount"`
	AccountId          string        `json:"accountId"`
	AccountRecipientId *string       `json:"accountRecipientId"`
	CategoryId         *string       `json:"categoryId"`
	PayeeId            *string       `json:"payeeId"`
	TagId              *string       `json:"tagId"`
	Description        *string       `json:"description"`
	Schedule           string        `json:"schedule"`
	NextPaymentAt      string        `json:"nextPaymentAt"`
}

// Validate enforces tier-1 NotBlank on id/type/amount/accountId/schedule/nextPaymentAt
// and checks that schedule is valid and nextPaymentAt matches the datetime format.
func (r UpdateRecurringTransactionRequest) Validate() error {
	return validateRecurringFields(r.Id, r.Type, r.Amount.String(), r.AccountId, r.Schedule, r.NextPaymentAt)
}

// UpdateRecurringTransactionResult is the update response: {item}.
type UpdateRecurringTransactionResult struct {
	Item RecurringTransactionResult `json:"item"`
}

// DeleteRecurringTransactionRequest is the delete-recurring-transaction body
// (id NotBlank).
type DeleteRecurringTransactionRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r DeleteRecurringTransactionRequest) Validate() error {
	return validateNotBlank([]blankCheck{{"id", r.Id}}, nil)
}

// DeleteRecurringTransactionResult is the delete response (empty).
type DeleteRecurringTransactionResult struct{}

// PostRecurringTransactionRequest is the post-recurring-transaction body
// (posts/materializes a single instance of a recurring transaction).
type PostRecurringTransactionRequest struct {
	RecurringId        string         `json:"recurringId"`
	Id                 string         `json:"id"`
	Type               string         `json:"type"`
	Amount             vo.FlexString  `json:"amount"`
	AmountRecipient    *vo.FlexString `json:"amountRecipient"`
	AccountId          string         `json:"accountId"`
	AccountRecipientId *string        `json:"accountRecipientId"`
	CategoryId         *string        `json:"categoryId"`
	PayeeId            *string        `json:"payeeId"`
	TagId              *string        `json:"tagId"`
	Description        *string        `json:"description"`
	Date               string         `json:"date"`
}

// Validate enforces NotBlank on recurringId/id/type/amount/accountId/date.
func (r PostRecurringTransactionRequest) Validate() error {
	return validateNotBlank([]blankCheck{
		{"recurringId", r.RecurringId}, {"id", r.Id}, {"type", r.Type},
		{"amount", r.Amount.String()}, {"accountId", r.AccountId}, {"date", r.Date},
	}, nil)
}

// PostRecurringTransactionResult is the post response: {item, accounts, nextPaymentAt}.
type PostRecurringTransactionResult struct {
	Item          TransactionResult `json:"item"`
	Accounts      []AccountResult   `json:"accounts"`
	NextPaymentAt string            `json:"nextPaymentAt"`
}

// SkipRecurringTransactionRequest is the skip-recurring-transaction body.
type SkipRecurringTransactionRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r SkipRecurringTransactionRequest) Validate() error {
	return validateNotBlank([]blankCheck{{"id", r.Id}}, nil)
}

// SkipRecurringTransactionResult is the skip response: {item}.
type SkipRecurringTransactionResult struct {
	Item RecurringTransactionResult `json:"item"`
}

// blankCheck holds a field key and its value for blank validation.
type blankCheck struct {
	key string
	val string
}

// validateNotBlank checks that all fields in checks are not blank (after trim),
// and returns a ValidationError if any fail or if extra field errors are provided.
func validateNotBlank(checks []blankCheck, extra []errs.FieldError) error {
	var fields []errs.FieldError
	for _, c := range checks {
		if strings.TrimSpace(c.val) == "" {
			fields = append(fields, errs.FieldError{Key: c.key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
	}
	fields = append(fields, extra...)
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// validateRecurringFields checks that id/type/amount/accountId/schedule/nextPaymentAt
// are not blank, and that schedule is a valid RecurringSchedule and nextPaymentAt
// matches the datetime format.
func validateRecurringFields(id, typ, amount, accountID, schedule, nextPaymentAt string) error {
	var extra []errs.FieldError
	if strings.TrimSpace(schedule) != "" {
		if _, ok := ParseRecurringSchedule(schedule); !ok {
			extra = append(extra, errs.FieldError{Key: "schedule", Message: "The value you selected is not a valid choice.", Code: "INVALID_CHOICE_ERROR"})
		}
	}
	if strings.TrimSpace(nextPaymentAt) != "" {
		if _, err := time.Parse(datetime.Layout, nextPaymentAt); err != nil {
			extra = append(extra, errs.FieldError{Key: "nextPaymentAt", Message: "This value is not valid.", Code: "INVALID_FORMAT_ERROR"})
		}
	}
	return validateNotBlank([]blankCheck{
		{"id", id}, {"type", typ}, {"amount", amount},
		{"accountId", accountID}, {"schedule", schedule}, {"nextPaymentAt", nextPaymentAt},
	}, extra)
}
