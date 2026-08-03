// Request/result DTOs for the label API, with their tier-1 Validate() methods.
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

// LabelResult is one label in the API. isArchived is an int 0/1 (NOT bool) and
// the timestamps are "2006-01-02 15:04:05" - the same frozen wire shapes as
// TagResult.
type LabelResult struct {
	Id          string `json:"id"`
	OwnerUserId string `json:"ownerUserId"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Position    int    `json:"position"`
	IsArchived  int    `json:"isArchived"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// CreateLabelRequest carries the same optional accountId as CreateTagRequest: a
// label added in the context of a shared account belongs to the account OWNER,
// not the caller.
type CreateLabelRequest struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	AccountId *string `json:"accountId"`
}

func (r CreateLabelRequest) Validate() error {
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

type CreateLabelResult struct {
	Item LabelResult `json:"item"`
}

type UpdateLabelRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func (r UpdateLabelRequest) Validate() error {
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

type UpdateLabelResult struct {
	Item LabelResult `json:"item"`
}

type ArchiveLabelRequest struct {
	Id string `json:"id"`
}

func (r ArchiveLabelRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

// ArchiveLabelResult is the archive-label response: an empty DTO -> {"data":{}}.
// (Note label UPDATE, unlike archive, DOES echo the item.)
type ArchiveLabelResult struct{}

type UnarchiveLabelRequest struct {
	Id string `json:"id"`
}

func (r UnarchiveLabelRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type UnarchiveLabelResult struct{}

// DeleteLabelRequest is the delete-label request body. Label delete is
// unconditional — there is no mode/replaceId.
type DeleteLabelRequest struct {
	Id string `json:"id"`
}

func (r DeleteLabelRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type DeleteLabelResult struct{}

type OrderLabelListRequest struct {
	Changes []PositionChange `json:"changes"`
}

// Validate enforces a non-empty changes list (an empty list is rejected with
// "Labels list is empty").
func (r OrderLabelListRequest) Validate() error {
	if len(r.Changes) == 0 {
		return &errs.ValidationError{Msg: "Labels list is empty", MsgCode: errs.CodeLabelListEmpty}
	}
	var fields []errs.FieldError
	for _, c := range r.Changes {
		fields = append(fields, validatePositionField("position", c.Position)...)
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type OrderLabelListResult struct {
	Items []LabelResult `json:"items"`
}

type GetLabelListResult struct {
	Items []LabelResult `json:"items"`
}
