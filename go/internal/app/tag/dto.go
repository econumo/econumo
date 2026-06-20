// Package tag is the tag aggregate's application layer: the request/result DTOs
// (with their tier-1 Validate() methods), the write-side Service (which owns the
// tx boundary and builds the response-shaped *Result directly), and the
// read-side ReadService for the pure get-tag-list read.
//
// JSON field names are frozen to the existing API wire contract; see
// COMPATIBILITY.md. Note the tag result shape has NO type and NO icon field
// (unlike category) — a tag has neither.
package tag

import (
	"strings"

	"github.com/econumo/econumo/internal/domain/shared/errs"
)

// ---------------------------------------------------------------------------
// Shared result shape
// ---------------------------------------------------------------------------

// TagResult is one tag in the API. isArchived is an int 0/1 (NOT bool);
// createdAt/updatedAt are "2006-01-02 15:04:05" (space separator, no timezone).
// There is no type or icon field. These wire shapes are frozen; see
// COMPATIBILITY.md.
type TagResult struct {
	Id          string `json:"id"`
	OwnerUserId string `json:"ownerUserId"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	IsArchived  int    `json:"isArchived"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// ---------------------------------------------------------------------------
// create-tag
// ---------------------------------------------------------------------------

// CreateTagRequest is the create-tag request body. accountId is nullable (a
// pointer) so an absent field is distinct from "".
type CreateTagRequest struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	AccountId *string `json:"accountId"`
}

// Validate enforces the tier-1 constraints: id NotBlank, name NotBlank. (The
// name 3-64 length invariant is re-checked tier-2 in the service via the
// value-object constructor, which produces the exact "Tag name must be 3-64
// characters" message.)
func (r CreateTagRequest) Validate() error {
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

// CreateTagResult is the create-tag response: {item: TagResult}.
type CreateTagResult struct {
	Item TagResult `json:"item"`
}

// ---------------------------------------------------------------------------
// update-tag
// ---------------------------------------------------------------------------

// UpdateTagRequest is the update-tag request body (id, name NotBlank tier-1;
// name 3..64 re-checked tier-2).
type UpdateTagRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// Validate enforces the tier-1 NotBlank constraints; the 3-64 name invariant is
// re-checked tier-2 in the service.
func (r UpdateTagRequest) Validate() error {
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

// UpdateTagResult is the update-tag response: {item: TagResult}.
type UpdateTagResult struct {
	Item TagResult `json:"item"`
}

// ---------------------------------------------------------------------------
// archive-tag / unarchive-tag
// ---------------------------------------------------------------------------

// ArchiveTagRequest is the archive-tag request body (id NotBlank).
type ArchiveTagRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r ArchiveTagRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// ArchiveTagResult is the archive-tag response: {item: TagResult}.
type ArchiveTagResult struct {
	Item TagResult `json:"item"`
}

// UnarchiveTagRequest is the unarchive-tag request body (id NotBlank).
type UnarchiveTagRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r UnarchiveTagRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// UnarchiveTagResult is the unarchive-tag response: {item: TagResult}.
type UnarchiveTagResult struct {
	Item TagResult `json:"item"`
}

// ---------------------------------------------------------------------------
// delete-tag
// ---------------------------------------------------------------------------

// DeleteTagRequest is the delete-tag request body. Unlike category-delete, tag
// delete is unconditional — there is no mode/replaceId.
type DeleteTagRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r DeleteTagRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// DeleteTagResult is the delete-tag response: an empty object ({}).
type DeleteTagResult struct{}

// ---------------------------------------------------------------------------
// order-tag-list
// ---------------------------------------------------------------------------

// PositionChange is one {id, position} entry in an order request.
type PositionChange struct {
	Id       string `json:"id"`
	Position int    `json:"position"`
}

// OrderTagListRequest is the order-tag-list request body.
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

// OrderTagListResult is the order-tag-list response: {items: [...]}.
type OrderTagListResult struct {
	Items []TagResult `json:"items"`
}

// ---------------------------------------------------------------------------
// get-tag-list
// ---------------------------------------------------------------------------

// GetTagListResult is the get-tag-list response: {items: [...]}.
type GetTagListResult struct {
	Items []TagResult `json:"items"`
}
