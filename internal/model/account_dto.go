// Request/result DTOs for the account+folder use cases (tier-1 Validate()).
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UserResult is the embedded {id, avatar, name} person shape shared by every
// wire embed that refers to a user — an account/folder owner, a shared-access
// grantee, a budget/connection member, or a transaction author.
type UserResult struct {
	Id     string `json:"id"`
	Avatar string `json:"avatar"`
	Name   string `json:"name"`
}

// AccountResult is one account in the API. balance is a normalized decimal
// string; type is the int (1 cash / 2 credit-card); sharedAccess is [] until the
// connection module lands. folderId is the first folder containing the account
// (or null). These wire shapes are frozen; see CLAUDE.md.
type AccountResult struct {
	Id           string         `json:"id"`
	Owner        UserResult     `json:"owner"`
	FolderId     *string        `json:"folderId"`
	Name         string         `json:"name"`
	Position     int            `json:"position"`
	Currency     CurrencyResult `json:"currency"`
	Balance      string         `json:"balance"`
	Type         int            `json:"type"`
	Icon         string         `json:"icon"`
	SharedAccess []SharedAccess `json:"sharedAccess"`
}

// SharedAccess is one accounts_access grant on the account: the granted user
// (id, avatar, name), the role alias (admin/user/guest), and whether the
// grant has been accepted (int 0/1, like isArchived).
type SharedAccess struct {
	User       UserResult `json:"user"`
	Role       string     `json:"role"`
	IsAccepted int        `json:"isAccepted"`
}

// CreateAccountRequest is the create-account body. balance defaults to 0; icon
// has a value-object (non-empty) check tier-2.
type CreateAccountRequest struct {
	Id         string        `json:"id"`
	Name       string        `json:"name"`
	CurrencyId string        `json:"currencyId"`
	Balance    vo.FlexString `json:"balance"`
	Icon       string        `json:"icon"`
	FolderId   string        `json:"folderId"`
}

// Validate enforces tier-1 NotBlank on id, name, currencyId, icon. folderId is
// resolved tier-2 in the service: a blank folderId is accepted for a user's very
// first account (a default folder is created), but rejected once folders exist.
// The 3-64 name and decimal balance invariants are re-checked tier-2.
func (r CreateAccountRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"id", r.Id}, {"name", r.Name}, {"currencyId", r.CurrencyId},
		{"icon", r.Icon},
	} {
		if strings.TrimSpace(f.val) == "" {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// CreateAccountResult is the create-account response: {item, transaction}. item
// is the created account; transaction is the opening-balance correction (same
// full shape as update-account's), or null when the account was created with a
// zero balance. There is no accounts list (frozen wire shape).
type CreateAccountResult struct {
	Item        AccountResult     `json:"item"`
	Transaction *CorrectionResult `json:"transaction"`
}

// UpdateAccountRequest is the update-account body. currencyId is nullable;
// updatedAt is the timestamp the correction transaction is dated with.
type UpdateAccountRequest struct {
	Id         string        `json:"id"`
	Name       string        `json:"name"`
	Balance    vo.FlexString `json:"balance"`
	Icon       string        `json:"icon"`
	CurrencyId *string       `json:"currencyId"`
	UpdatedAt  string        `json:"updatedAt"`
}

// Validate enforces tier-1 NotBlank on id, name, icon, updatedAt.
func (r UpdateAccountRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"id", r.Id}, {"name", r.Name}, {"icon", r.Icon}, {"updatedAt", r.UpdatedAt},
	} {
		if strings.TrimSpace(f.val) == "" {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// UpdateAccountResult is the update-account response: {item, transaction}. item
// is the refreshed account; transaction is the balance-correction transaction
// created (or null when the balance was unchanged).
type UpdateAccountResult struct {
	Item        AccountResult     `json:"item"`
	Transaction *CorrectionResult `json:"transaction"`
}

// CorrectionResult is the transaction shape returned by update-account's balance
// correction. It is the FULL transaction wire shape — author + every nullable
// transaction field, not just a subset. For a balance correction:
// accountRecipientId/categoryId/payeeId/tagId are always null and amountRecipient
// falls back to amount. Field order is irrelevant on the wire (canonical
// compare), but the SET of keys is frozen and must match exactly.
type CorrectionResult struct {
	Id                 string     `json:"id"`
	Author             UserResult `json:"author"`
	Type               string     `json:"type"`
	AccountId          string     `json:"accountId"`
	AccountRecipientId *string    `json:"accountRecipientId"`
	Amount             string     `json:"amount"`
	AmountRecipient    string     `json:"amountRecipient"`
	CategoryId         *string    `json:"categoryId"`
	Description        string     `json:"description"`
	PayeeId            *string    `json:"payeeId"`
	TagId              *string    `json:"tagId"`
	Date               string     `json:"date"`
}

// DeleteAccountRequest is the delete-account body (id NotBlank).
type DeleteAccountRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r DeleteAccountRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// DeleteAccountResult is the delete-account response: an empty object ({}).
type DeleteAccountResult struct{}

// GetAccountListResult is the get-account-list response: {items: [...]}.
type GetAccountListResult struct {
	Items []AccountResult `json:"items"`
}

// AccountPositionChange is one {id, folderId, position} entry in an order
// request — it both repositions the account (accounts_options) and moves it
// between folders.
type AccountPositionChange struct {
	Id       string `json:"id"`
	FolderId string `json:"folderId"`
	Position int    `json:"position"`
}

// OrderAccountListRequest is the order-account-list body.
type OrderAccountListRequest struct {
	Changes []AccountPositionChange `json:"changes"`
}

// Validate enforces a non-empty changes list.
func (r OrderAccountListRequest) Validate() error {
	if len(r.Changes) == 0 {
		return errs.NewValidation("Accounts list is empty")
	}
	return nil
}

// OrderAccountListResult is the order-account-list response: {items: [...]} (not
// reversed, unlike get-account-list).
type OrderAccountListResult struct {
	Items []AccountResult `json:"items"`
}

// AccountFolderResult is one folder in the API: {id, name, position, isVisible(int 0/1)}.
type AccountFolderResult struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Position  int    `json:"position"`
	IsVisible int    `json:"isVisible"`
}

// CreateFolderRequest is the create-folder body (name NotBlank, 3-64 tier-2).
type CreateFolderRequest struct {
	Name string `json:"name"`
}

// Validate enforces name NotBlank.
func (r CreateFolderRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// CreateFolderResult is the create-folder response: {item: AccountFolderResult}.
type CreateFolderResult struct {
	Item AccountFolderResult `json:"item"`
}

// UpdateFolderRequest is the update-folder body (id, name NotBlank).
type UpdateFolderRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// Validate enforces id + name NotBlank.
func (r UpdateFolderRequest) Validate() error {
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

// UpdateFolderResult is the update-folder response: {item: AccountFolderResult}.
type UpdateFolderResult struct {
	Item AccountFolderResult `json:"item"`
}

// HideFolderRequest / ShowFolderRequest carry the folder id (NotBlank).
type HideFolderRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r HideFolderRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// HideFolderResult is the hide-folder response: an empty object ({}).
type HideFolderResult struct{}

// ShowFolderRequest carries the folder id (NotBlank).
type ShowFolderRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r ShowFolderRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// ShowFolderResult is the show-folder response: an empty object ({}).
type ShowFolderResult struct{}

// ReplaceFolderRequest moves a folder's accounts into replaceId and deletes the
// folder.
type ReplaceFolderRequest struct {
	Id        string `json:"id"`
	ReplaceId string `json:"replaceId"`
}

// Validate enforces id + replaceId NotBlank.
func (r ReplaceFolderRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.ReplaceId) == "" {
		fields = append(fields, errs.FieldError{Key: "replaceId", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// ReplaceFolderResult is the replace-folder response: an empty object ({}).
type ReplaceFolderResult struct{}

// GetFolderListResult is the get-folder-list response: {items: [...]}.
type GetFolderListResult struct {
	Items []AccountFolderResult `json:"items"`
}

// FolderPositionChange is one {id, position} entry in an order-folder request.
type FolderPositionChange struct {
	Id       string `json:"id"`
	Position int    `json:"position"`
}

// OrderFolderListRequest is the order-folder-list body.
type OrderFolderListRequest struct {
	Changes []FolderPositionChange `json:"changes"`
}

// Validate enforces a non-empty changes list.
func (r OrderFolderListRequest) Validate() error {
	if len(r.Changes) == 0 {
		return errs.NewValidation("Folders list is empty")
	}
	return nil
}

// OrderFolderListResult is the order-folder-list response: {items: [...]}.
type OrderFolderListResult struct {
	Items []AccountFolderResult `json:"items"`
}

// GrantAccountAccessRequest grants/updates a connected user's role on an owned
// account. New grants start pending (isAccepted 0).
type GrantAccountAccessRequest struct {
	AccountId string `json:"accountId"`
	UserId    string `json:"userId"`
	Role      string `json:"role"`
}

// Validate enforces NotBlank on accountId/userId/role (UUID + role-alias
// validity are checked in the service via the value-object constructors).
func (r GrantAccountAccessRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"accountId", r.AccountId}, {"userId", r.UserId}, {"role", r.Role},
	} {
		if strings.TrimSpace(f.val) == "" {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// GrantAccountAccessResult is the (empty) response.
type GrantAccountAccessResult struct{}

// AcceptAccountAccessRequest accepts a pending grant. folderId picks where the
// account lands; blank is tolerated only when the user has no folders (a
// "General" folder is then created), same as create-account.
type AcceptAccountAccessRequest struct {
	AccountId string `json:"accountId"`
	FolderId  string `json:"folderId"`
}

// Validate enforces accountId NotBlank.
func (r AcceptAccountAccessRequest) Validate() error {
	if strings.TrimSpace(r.AccountId) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "accountId", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// AcceptAccountAccessResult is the (empty) response.
type AcceptAccountAccessResult struct{}

// DeclineAccountAccessRequest removes the caller's own grant (their side of
// the share), pending or accepted.
type DeclineAccountAccessRequest struct {
	AccountId string `json:"accountId"`
}

// Validate enforces accountId NotBlank.
func (r DeclineAccountAccessRequest) Validate() error {
	if strings.TrimSpace(r.AccountId) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "accountId", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// DeclineAccountAccessResult is the (empty) response.
type DeclineAccountAccessResult struct{}

// RevokeAccountAccessRequest removes a connected user's grant on an owned
// account.
type RevokeAccountAccessRequest struct {
	AccountId string `json:"accountId"`
	UserId    string `json:"userId"`
}

// Validate enforces NotBlank on accountId/userId.
func (r RevokeAccountAccessRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"accountId", r.AccountId}, {"userId", r.UserId},
	} {
		if strings.TrimSpace(f.val) == "" {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// RevokeAccountAccessResult is the (empty) response.
type RevokeAccountAccessResult struct{}
