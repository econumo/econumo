// Request/result DTOs for the category API, with their tier-1 Validate()
// methods.
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CategoryResult is one category in the API. type is the alias string
// ("expense"/"income"); isArchived is an int 0/1 (NOT bool); createdAt/updatedAt
// are "2006-01-02 15:04:05" (space separator, no timezone). These wire shapes
// are frozen; see CLAUDE.md.
type CategoryResult struct {
	Id          string `json:"id"`
	OwnerUserId string `json:"ownerUserId"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	Type        string `json:"type"`
	Icon        string `json:"icon"`
	IsArchived  int    `json:"isArchived"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// CreateCategoryRequest is the create-category request body. accountId and icon
// are nullable (pointers) so an absent field is distinct from "".
type CreateCategoryRequest struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	AccountId *string `json:"accountId"`
	Icon      *string `json:"icon"`
}

// Validate enforces the tier-1 constraints: id NotBlank, name NotBlank, type
// NotBlank. (The name 3-64 length and the expense|income type invariants are
// re-checked tier-2 in the service via the value-object constructors, which
// produce the exact "Category name must be 3-64 characters" message.)
func (r CreateCategoryRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Type) == "" {
		fields = append(fields, errs.FieldError{Key: "type", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// CreateCategoryResult is the create-category response: {item: CategoryResult}.
type CreateCategoryResult struct {
	Item CategoryResult `json:"item"`
}

// UpdateCategoryRequest is the update-category request body (id, name, icon all
// NotBlank tier-1; name 3..64 and non-empty icon re-checked tier-2).
type UpdateCategoryRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

// Validate enforces the tier-1 NotBlank constraints; the 3-64 name and non-empty
// icon invariants are re-checked tier-2 in the service.
func (r UpdateCategoryRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Icon) == "" {
		fields = append(fields, errs.FieldError{Key: "icon", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// UpdateCategoryResult is the update-category response: an empty body
// ({"data":{}}) — no item is echoed back (frozen wire shape).
type UpdateCategoryResult struct{}

// ArchiveCategoryRequest is the archive-category request body (id NotBlank).
type ArchiveCategoryRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r ArchiveCategoryRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

// ArchiveCategoryResult is the archive-category response: an empty body
// ({"data":{}}).
type ArchiveCategoryResult struct{}

// UnarchiveCategoryRequest is the unarchive-category request body (id NotBlank).
type UnarchiveCategoryRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r UnarchiveCategoryRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

// UnarchiveCategoryResult is the unarchive-category response: an empty body
// ({"data":{}}).
type UnarchiveCategoryResult struct{}

// Delete modes. "replace" was removed in favour of merge-category, which does
// the same job completely (it also moves recurring templates and budget limits);
// it now falls through to the invalid-choice branch below rather than silently
// degrading into a destructive plain delete.
const (
	ModeDelete = "delete"
)

// DeleteCategoryRequest is the delete-category request body.
type DeleteCategoryRequest struct {
	Id   string `json:"id"`
	Mode string `json:"mode"`
}

// Validate enforces id NotBlank and mode NotBlank + "delete".
func (r DeleteCategoryRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	switch r.Mode {
	case ModeDelete:
	case "":
		fields = append(fields, errs.FieldError{Key: "mode", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	default:
		fields = append(fields, errs.FieldError{Key: "mode", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// DeleteCategoryResult is the delete-category response: an empty object ({}).
type DeleteCategoryResult struct{}

// PositionChange is one {id, position} entry in an order request.
type PositionChange struct {
	Id       string `json:"id"`
	Position int    `json:"position"`
}

// MoveCategoryRequest is the move-category request body. AfterId is the id of
// the sibling this category should land immediately after; null means "move to
// the front". An AfterId that is not one of the caller's own categories appends
// to the end rather than erroring, matching the silent-skip behaviour of the
// absolute-position endpoint this replaces.
type MoveCategoryRequest struct {
	Id      string  `json:"id"`
	AfterId *string `json:"afterId"`
}

func (r MoveCategoryRequest) Validate() error {
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

// SortCategoryListRequest is the sort-category-list request body: the caller's desired
// order as a full list of ids. A drag is one MoveCategory; sorting the whole list
// (alphabetically, say) is this, because no single relative move can express an
// n-item reorder.
type SortCategoryListRequest struct {
	Ids []string `json:"ids"`
}

func (r SortCategoryListRequest) Validate() error {
	if len(r.Ids) == 0 {
		return &errs.ValidationError{Msg: "Ids list is empty", MsgCode: errs.CodeIsBlank}
	}
	for _, id := range r.Ids {
		if _, err := vo.ParseId(id); err != nil {
			return err
		}
	}
	return nil
}

// SortCategoryListResult is the sort-category-list response: {items: [...]}.
type SortCategoryListResult struct {
	Items []CategoryResult `json:"items"`
}

// MoveCategoryResult is the move-category response: {items: [...]}.
type MoveCategoryResult struct {
	Items []CategoryResult `json:"items"`
}

// GetCategoryListResult is the get-category-list response: {items: [...]}.
type GetCategoryListResult struct {
	Items []CategoryResult `json:"items"`
}

// MergeCategoryRequest is the merge-category request body: sourceId is absorbed into
// targetId and then deleted. The fields are named rather than reusing the usual
// "id" because this also ships as an MCP tool, where which side gets destroyed
// must be unambiguous to a model choosing arguments.
type MergeCategoryRequest struct {
	SourceId string `json:"sourceId"`
	TargetId string `json:"targetId"`
}

func (r MergeCategoryRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.SourceId) == "" {
		fields = append(fields, errs.FieldError{Key: "sourceId", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.TargetId) == "" {
		fields = append(fields, errs.FieldError{Key: "targetId", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type MergeCategoryResult struct{}
