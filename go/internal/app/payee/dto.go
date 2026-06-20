// Package payee is the payee aggregate's application layer: the request/result
// DTOs (with their tier-1 Validate() methods), the write-side Service (which
// owns the tx boundary and builds the response-shaped *Result directly), and the
// read-side ReadService for the pure get-payee-list read.
//
// JSON field names are frozen to the existing API wire contract; see
// COMPATIBILITY.md. The payee result shape has NO type and NO icon field (same
// as tag) — a payee has neither.
package payee

import (
	"strings"

	"github.com/econumo/econumo/internal/domain/shared/errs"
)

// ---------------------------------------------------------------------------
// Shared result shape
// ---------------------------------------------------------------------------

// PayeeResult is one payee in the API. isArchived is an int 0/1 (NOT bool);
// createdAt/updatedAt are "2006-01-02 15:04:05" (space separator, no timezone).
// There is no type or icon field. These wire shapes are frozen; see
// COMPATIBILITY.md.
type PayeeResult struct {
	Id          string `json:"id"`
	OwnerUserId string `json:"ownerUserId"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	IsArchived  int    `json:"isArchived"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// ---------------------------------------------------------------------------
// create-payee
// ---------------------------------------------------------------------------

// CreatePayeeRequest is the create-payee request body. accountId is nullable (a
// pointer) so an absent field is distinct from "".
type CreatePayeeRequest struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	AccountId *string `json:"accountId"`
}

// Validate enforces the tier-1 constraints: id NotBlank, name NotBlank. (The
// name 3-64 length invariant is re-checked tier-2 in the service via the
// value-object constructor, which produces the exact "Payee name must be 3-64
// characters" message.)
func (r CreatePayeeRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// CreatePayeeResult is the create-payee response: {item: PayeeResult}.
type CreatePayeeResult struct {
	Item PayeeResult `json:"item"`
}

// ---------------------------------------------------------------------------
// update-payee
// ---------------------------------------------------------------------------

// UpdatePayeeRequest is the update-payee request body (id, name NotBlank tier-1;
// name 3..64 re-checked tier-2).
type UpdatePayeeRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// Validate enforces the tier-1 NotBlank constraints; the 3-64 name invariant is
// re-checked tier-2 in the service.
func (r UpdatePayeeRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// UpdatePayeeResult is the update-payee response: {item: PayeeResult}.
type UpdatePayeeResult struct {
	Item PayeeResult `json:"item"`
}

// ---------------------------------------------------------------------------
// archive-payee / unarchive-payee
// ---------------------------------------------------------------------------

// ArchivePayeeRequest is the archive-payee request body (id NotBlank).
type ArchivePayeeRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r ArchivePayeeRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// ArchivePayeeResult is the archive-payee response: {item: PayeeResult}.
type ArchivePayeeResult struct {
	Item PayeeResult `json:"item"`
}

// UnarchivePayeeRequest is the unarchive-payee request body (id NotBlank).
type UnarchivePayeeRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r UnarchivePayeeRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// UnarchivePayeeResult is the unarchive-payee response: {item: PayeeResult}.
type UnarchivePayeeResult struct {
	Item PayeeResult `json:"item"`
}

// ---------------------------------------------------------------------------
// delete-payee
// ---------------------------------------------------------------------------

// DeletePayeeRequest is the delete-payee request body. Like tag-delete, payee
// delete is unconditional — there is no mode/replaceId.
type DeletePayeeRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r DeletePayeeRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// DeletePayeeResult is the delete-payee response: an empty object ({}).
type DeletePayeeResult struct{}

// ---------------------------------------------------------------------------
// order-payee-list
// ---------------------------------------------------------------------------

// PositionChange is one {id, position} entry in an order request.
type PositionChange struct {
	Id       string `json:"id"`
	Position int    `json:"position"`
}

// OrderPayeeListRequest is the order-payee-list request body.
type OrderPayeeListRequest struct {
	Changes []PositionChange `json:"changes"`
}

// Validate enforces a non-empty changes list (an empty list is rejected with
// "Payees list is empty").
func (r OrderPayeeListRequest) Validate() error {
	if len(r.Changes) == 0 {
		return errs.NewValidation("Payees list is empty")
	}
	return nil
}

// OrderPayeeListResult is the order-payee-list response: {items: [...]}.
type OrderPayeeListResult struct {
	Items []PayeeResult `json:"items"`
}

// ---------------------------------------------------------------------------
// get-payee-list
// ---------------------------------------------------------------------------

// GetPayeeListResult is the get-payee-list response: {items: [...]}.
type GetPayeeListResult struct {
	Items []PayeeResult `json:"items"`
}
