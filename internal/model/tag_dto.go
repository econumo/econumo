// Request/result DTOs for the tag API, with their tier-1 Validate() methods.
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

// TagResult is one tag in the API. isArchived is an int 0/1 (NOT bool);
// createdAt/updatedAt are "2006-01-02 15:04:05" (space separator, no timezone).
// These wire shapes are frozen; see CLAUDE.md.
type TagResult struct {
	Id          string `json:"id"`
	OwnerUserId string `json:"ownerUserId"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	IsArchived  int    `json:"isArchived"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// CreateTagRequest is the create-tag request body. accountId is nullable (a
// pointer) so an absent field is distinct from "".
type CreateTagRequest struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	AccountId *string `json:"accountId"`
}

// Validate enforces the tier-1 NotBlank constraints; the 3-64 name length is
// re-checked tier-2 in the service.
func (r CreateTagRequest) Validate() error {
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

type CreateTagResult struct {
	Item TagResult `json:"item"`
}

type UpdateTagRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func (r UpdateTagRequest) Validate() error {
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

type UpdateTagResult struct {
	Item TagResult `json:"item"`
}

type ArchiveTagRequest struct {
	Id string `json:"id"`
}

func (r ArchiveTagRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

// ArchiveTagResult is the archive-tag response: an empty DTO -> {"data":{}}.
// (Note tag UPDATE, unlike archive, DOES echo the item.)
type ArchiveTagResult struct{}

type UnarchiveTagRequest struct {
	Id string `json:"id"`
}

func (r UnarchiveTagRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type UnarchiveTagResult struct{}

// DeleteTagRequest is the delete-tag request body. Tag delete is unconditional —
// there is no mode/replaceId.
type DeleteTagRequest struct {
	Id string `json:"id"`
}

func (r DeleteTagRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type DeleteTagResult struct{}

type OrderTagListRequest struct {
	Changes []PositionChange `json:"changes"`
}

// Validate enforces a non-empty changes list (an empty list is rejected with
// "Tags list is empty").
func (r OrderTagListRequest) Validate() error {
	if len(r.Changes) == 0 {
		return errs.NewValidation("Tags list is empty")
	}
	return nil
}

type OrderTagListResult struct {
	Items []TagResult `json:"items"`
}

type GetTagListResult struct {
	Items []TagResult `json:"items"`
}
