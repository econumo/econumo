// Result DTOs for the currency read endpoints (get-currency-list,
// get-currency-rate-list). JSON field names are frozen to the existing API
// wire contract; see CLAUDE.md.
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

// CurrencyResult is one currency in the API. name is the English display name
// resolved from the Intl table (the stored currencies.name is NULL in practice);
// fractionDigits is an int. Frozen wire shape; see CLAUDE.md.
type CurrencyResult struct {
	Id             string `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Symbol         string `json:"symbol"`
	FractionDigits int    `json:"fractionDigits"`
}

// CurrencyListItem is a get-currency-list item: the shared currency shape
// plus the caller-relative flags. Anonymous embedding keeps the JSON flat, so
// the list endpoint's wire shape is CurrencyResult's fields + scope/isArchived/
// isHidden, while every other endpoint embedding CurrencyResult is untouched.
// scope is "global" | "own" | "shared" (reachable via a shared account/budget,
// not owned by the caller); isArchived/isHidden are ints 0/1.
type CurrencyListItem struct {
	CurrencyResult
	Scope      string `json:"scope"`
	IsArchived int    `json:"isArchived"`
	IsHidden   int    `json:"isHidden"`
}

// GetCurrencyListResult is the get-currency-list response: {items: [...]}.
type GetCurrencyListResult struct {
	Items []CurrencyListItem `json:"items"`
}

// CurrencyRateResult is one rate in the API. rate is a decimal string; updatedAt
// is the published date formatted "Y-m-d 00:00:00". Frozen wire shape.
type CurrencyRateResult struct {
	CurrencyId     string `json:"currencyId"`
	BaseCurrencyId string `json:"baseCurrencyId"`
	Rate           string `json:"rate"`
	UpdatedAt      string `json:"updatedAt"`
}

// GetCurrencyRateListResult is the get-currency-rate-list response: {items:
// [...]}.
type GetCurrencyRateListResult struct {
	Items []CurrencyRateResult `json:"items"`
}

// create-currency. Id is the client-generated operation id (idempotency key);
// the entity gets a fresh server id.
type CreateCurrencyRequest struct {
	Id             string  `json:"id"`
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Symbol         *string `json:"symbol"`
	FractionDigits *int    `json:"fractionDigits"`
	Rate           *string `json:"rate"`
}

func (r CreateCurrencyRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Code) == "" {
		fields = append(fields, errs.FieldError{Key: "code", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type CreateCurrencyResult struct {
	Item CurrencyListItem `json:"item"`
}

// update-currency (the custom-currency lifecycle endpoint under
// /api/v1/currency/, NOT the user-profile /api/v1/user/update-currency,
// which already owns the UpdateCurrencyRequest/Result names in this package):
// full replace of the mutable fields; code is immutable.
type UpdateCustomCurrencyRequest struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	Symbol         string `json:"symbol"`
	FractionDigits int    `json:"fractionDigits"`
}

func (r UpdateCustomCurrencyRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Symbol) == "" {
		fields = append(fields, errs.FieldError{Key: "symbol", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type UpdateCustomCurrencyResult struct {
	Item CurrencyListItem `json:"item"`
}

type ArchiveCurrencyRequest struct {
	Id string `json:"id"`
}

func (r ArchiveCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type ArchiveCurrencyResult struct{}

type UnarchiveCurrencyRequest struct {
	Id string `json:"id"`
}

func (r UnarchiveCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type UnarchiveCurrencyResult struct{}

type DeleteCurrencyRequest struct {
	Id string `json:"id"`
}

func (r DeleteCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type DeleteCurrencyResult struct{}

type SetCurrencyRateRequest struct {
	CurrencyId string  `json:"currencyId"`
	Rate       string  `json:"rate"`
	Date       *string `json:"date"`
}

func (r SetCurrencyRateRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.CurrencyId) == "" {
		fields = append(fields, errs.FieldError{Key: "currencyId", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Rate) == "" {
		fields = append(fields, errs.FieldError{Key: "rate", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type SetCurrencyRateResult struct{}

type HideCurrencyRequest struct {
	Id string `json:"id"`
}

func (r HideCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type HideCurrencyResult struct{}

type ShowCurrencyRequest struct {
	Id string `json:"id"`
}

func (r ShowCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type ShowCurrencyResult struct{}

func validateBlankId(id string) error {
	if strings.TrimSpace(id) == "" {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}
