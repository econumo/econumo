// Request/result DTOs for the payee API, with their tier-1 Validate() methods.
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

// PayeeResult is one payee in the API. isArchived is an int 0/1 (NOT bool);
// createdAt/updatedAt are "2006-01-02 15:04:05" (space separator, no timezone).
// These wire shapes are frozen; see CLAUDE.md.
type PayeeResult struct {
	Id          string `json:"id"`
	OwnerUserId string `json:"ownerUserId"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	IsArchived  int    `json:"isArchived"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// CreatePayeeRequest is the create-payee request body. accountId is nullable (a
// pointer) so an absent field is distinct from "".
type CreatePayeeRequest struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	AccountId *string `json:"accountId"`
}

// Validate enforces the tier-1 NotBlank constraints; the 3-64 name length is
// re-checked tier-2 in the service.
func (r CreatePayeeRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type CreatePayeeResult struct {
	Item PayeeResult `json:"item"`
}

type UpdatePayeeRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func (r UpdatePayeeRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// UpdatePayeeResult is the update-payee response: an EMPTY DTO -> {"data":{}}
// (unlike tag, whose update echoes the item).
type UpdatePayeeResult struct{}

type ArchivePayeeRequest struct {
	Id string `json:"id"`
}

func (r ArchivePayeeRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type ArchivePayeeResult struct{}

type UnarchivePayeeRequest struct {
	Id string `json:"id"`
}

func (r UnarchivePayeeRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type UnarchivePayeeResult struct{}

// DeletePayeeRequest is the delete-payee request body. Payee delete is
// unconditional — there is no mode/replaceId.
type DeletePayeeRequest struct {
	Id string `json:"id"`
}

func (r DeletePayeeRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type DeletePayeeResult struct{}

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

type OrderPayeeListResult struct {
	Items []PayeeResult `json:"items"`
}

type GetPayeeListResult struct {
	Items []PayeeResult `json:"items"`
}
