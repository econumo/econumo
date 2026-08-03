// Request/result DTOs for the payee API, with their tier-1 Validate() methods.
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
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

// MovePayeeRequest is the move-payee request body. AfterId is the id of the
// sibling this payee should land immediately after; null means "move to the
// front". An AfterId that is not one of the caller's own payees appends to the end
// rather than erroring, matching the silent-skip behaviour of the
// absolute-position endpoint this replaces.
type MovePayeeRequest struct {
	Id      string  `json:"id"`
	AfterId *string `json:"afterId"`
}

func (r MovePayeeRequest) Validate() error {
	if _, err := vo.ParseId(r.Id); err != nil {
		return err
	}
	if r.AfterId != nil {
		if _, err := vo.ParseId(*r.AfterId); err != nil {
			return err
		}
	}
	return nil
}

// MovePayeeResult is the move-payee response: {items: [...]}.
type MovePayeeResult struct {
	Items []PayeeResult `json:"items"`
}

type GetPayeeListResult struct {
	Items []PayeeResult `json:"items"`
}
