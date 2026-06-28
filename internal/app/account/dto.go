// Package account is the account+folder aggregate's application layer: the
// request/result DTOs (with tier-1 Validate()), the write-side Service (owns the
// tx boundary, builds response DTOs directly), and the read side.
//
// JSON field names are frozen to the existing API wire contract; see
// CLAUDE.md. The account result embeds the owner (User), the full
// currency, the folder id, the per-user position, the computed balance, and
// sharedAccess (stubbed empty until the connection module lands).
package account

import (
	"strings"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// ---------------------------------------------------------------------------
// Shared result shapes
// ---------------------------------------------------------------------------

// OwnerResult is the embedded account owner: {id, avatar, name} (the minimal
// UserResult shape used by UserToDtoResultAssembler — NOT the login user shape).
type OwnerResult struct {
	Id     string `json:"id"`
	Avatar string `json:"avatar"`
	Name   string `json:"name"`
}

// CurrencyResult is the embedded account currency: {id, code, name, symbol,
// fractionDigits} — identical to the currency module's CurrencyResult.
type CurrencyResult struct {
	Id             string `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Symbol         string `json:"symbol"`
	FractionDigits int    `json:"fractionDigits"`
}

// AccountResult is one account in the API. balance is a normalized decimal
// string; type is the int (1 cash / 2 credit-card); sharedAccess is [] until the
// connection module lands. folderId is the first folder containing the account
// (or null). These wire shapes are frozen; see CLAUDE.md.
type AccountResult struct {
	Id           string         `json:"id"`
	Owner        OwnerResult    `json:"owner"`
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
// (id, avatar, name) + the role alias (admin/user/guest). Mirrors PHP
// SharedAccessItemResultDto via AccountIdToSharedAccessResultAssembler.
type SharedAccess struct {
	User OwnerResult `json:"user"`
	Role string      `json:"role"`
}

// ---------------------------------------------------------------------------
// create-account
// ---------------------------------------------------------------------------

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

// Validate enforces tier-1 NotBlank on id, name, currencyId, icon, folderId. The
// 3-64 name and decimal balance invariants are re-checked tier-2 in the service.
func (r CreateAccountRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"id", r.Id}, {"name", r.Name}, {"currencyId", r.CurrencyId},
		{"icon", r.Icon}, {"folderId", r.FolderId},
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

// CreateAccountResult is the create-account response: {item} ONLY. PHP's
// CreateAccountV1ResultDto has a single public $item property, so the envelope is
// {"data":{"item":{...account...}}}; there is no accounts list (verified vs PHP
// CreateAccountV1ResultAssembler).
type CreateAccountResult struct {
	Item AccountResult `json:"item"`
}

// ---------------------------------------------------------------------------
// update-account
// ---------------------------------------------------------------------------

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
// correction. It is the FULL PHP TransactionResultDto (the update assembler runs
// the correction transaction through TransactionToDtoResultAssembler), so it
// carries author + all the nullable transaction fields, not just a subset. For a
// balance correction: accountRecipientId/categoryId/payeeId/tagId are always
// null and amountRecipient falls back to amount. Field order is irrelevant on the
// wire (canonical compare), but the SET of keys must match PHP exactly.
type CorrectionResult struct {
	Id                 string      `json:"id"`
	Author             OwnerResult `json:"author"`
	Type               string      `json:"type"`
	AccountId          string      `json:"accountId"`
	AccountRecipientId *string     `json:"accountRecipientId"`
	Amount             string      `json:"amount"`
	AmountRecipient    string      `json:"amountRecipient"`
	CategoryId         *string     `json:"categoryId"`
	Description        string      `json:"description"`
	PayeeId            *string     `json:"payeeId"`
	TagId              *string     `json:"tagId"`
	Date               string      `json:"date"`
}

// ---------------------------------------------------------------------------
// delete-account
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// get-account-list / order-account-list
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// folder result + endpoints
// ---------------------------------------------------------------------------

// FolderResult is one folder in the API: {id, name, position, isVisible(int 0/1)}.
type FolderResult struct {
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

// CreateFolderResult is the create-folder response: {item: FolderResult}.
type CreateFolderResult struct {
	Item FolderResult `json:"item"`
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

// UpdateFolderResult is the update-folder response: {item: FolderResult}.
type UpdateFolderResult struct {
	Item FolderResult `json:"item"`
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
	Items []FolderResult `json:"items"`
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
	Items []FolderResult `json:"items"`
}
