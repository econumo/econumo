// Result DTOs for the currency read endpoints (get-currency-list,
// get-currency-rate-list). JSON field names are frozen to the existing API
// wire contract; see CLAUDE.md.
package model

import (
	"strings"
	"time"

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

// CurrencyListItem is a get-currency-list item: the shared currency shape plus
// the caller-relative flags. Anonymous embedding keeps the JSON flat.
// scope is "global" | "own" | "shared"; isHidden/isDeleted are ints 0/1.
// A deleted currency is still listed -- accounts and rates that reference it
// must keep resolving -- so consumers filter on the flag themselves.
type CurrencyListItem struct {
	CurrencyResult
	Scope     string `json:"scope"`
	IsHidden  int    `json:"isHidden"`
	IsDeleted int    `json:"isDeleted"`
}

// GetCurrencyListResult is the get-currency-list response: {items: [...]}.
type GetCurrencyListResult struct {
	Items []CurrencyListItem `json:"items"`
}

// CurrencyRateResult is one rate in the API. rate is a decimal string; updatedAt
// is the published date formatted "Y-m-d 00:00:00"; date is the same day as
// "Y-m-d". Frozen wire shape.
type CurrencyRateResult struct {
	CurrencyId     string `json:"currencyId"`
	BaseCurrencyId string `json:"baseCurrencyId"`
	Rate           string `json:"rate"`
	UpdatedAt      string `json:"updatedAt"`
	Date           string `json:"date"`
}

// MaxCurrencyRateRangeDays caps the inclusive day span of a ranged rate read.
const MaxCurrencyRateRangeDays = 366

const rateDayLayout = "2006-01-02"

// CurrencyRateListRequest is the optional from/to window of
// get-currency-rate-list; both bounds or neither.
type CurrencyRateListRequest struct {
	From, To       string
	HasFrom, HasTo bool
}

func parseRateDay(s string) (time.Time, bool) {
	t, err := time.Parse(rateDayLayout, s)
	if err != nil || t.Format(rateDayLayout) != s {
		return time.Time{}, false
	}
	return t, true
}

func (r CurrencyRateListRequest) Validate() error {
	if !r.HasFrom && !r.HasTo {
		return nil
	}
	var fields []errs.FieldError
	blank := func(key string) {
		fields = append(fields, errs.FieldError{Key: key, Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	invalid := func(key string) {
		fields = append(fields, errs.FieldError{Key: key, Message: "This value is not valid.", Code: errs.CodeInvalidFormat})
	}
	if strings.TrimSpace(r.From) == "" {
		blank("from")
	}
	if strings.TrimSpace(r.To) == "" {
		blank("to")
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	from, okFrom := parseRateDay(r.From)
	to, okTo := parseRateDay(r.To)
	if !okFrom {
		invalid("from")
	}
	if !okTo {
		invalid("to")
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	if to.Before(from) {
		fields = append(fields, errs.FieldError{Key: "to", Message: "The end date must not be before the start date", Code: errs.CodeCurrencyRateRangeOrder})
	} else if int(to.Sub(from).Hours()/24)+1 > MaxCurrencyRateRangeDays {
		fields = append(fields, errs.FieldError{
			Key: "to", Message: "The date range must be at most 366 days", Code: errs.CodeCurrencyRateRangeTooLong,
			Params: map[string]any{"max": MaxCurrencyRateRangeDays},
		})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// Range returns the parsed window; ok is false when no bounds were given.
// Call it after Validate.
func (r CurrencyRateListRequest) Range() (from, to time.Time, ok bool) {
	if !r.HasFrom && !r.HasTo {
		return time.Time{}, time.Time{}, false
	}
	from, _ = parseRateDay(r.From)
	to, _ = parseRateDay(r.To)
	return from, to, true
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
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Code) == "" {
		fields = append(fields, errs.FieldError{Key: "code", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if r.Rate == nil || strings.TrimSpace(*r.Rate) == "" {
		fields = append(fields, errs.FieldError{Key: "rate", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
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
	Rate           string `json:"rate"`
}

func (r UpdateCustomCurrencyRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Symbol) == "" {
		fields = append(fields, errs.FieldError{Key: "symbol", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Rate) == "" {
		fields = append(fields, errs.FieldError{Key: "rate", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type UpdateCustomCurrencyResult struct {
	Item CurrencyListItem `json:"item"`
}

type DeleteCurrencyRequest struct {
	Id string `json:"id"`
}

func (r DeleteCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type DeleteCurrencyResult struct{}

type HideCurrencyRequest struct {
	Id string `json:"id"`
}

func (r HideCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type HideCurrencyResult struct{}

type HideAllCurrenciesResult struct{}

type ShowAllCurrenciesResult struct{}

type ShowCurrencyRequest struct {
	Id string `json:"id"`
}

func (r ShowCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type ShowCurrencyResult struct{}

func validateBlankId(id string) error {
	if strings.TrimSpace(id) == "" {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}
