// Package currency is the currency aggregate's application layer. Both currency
// endpoints (get-currency-list, get-currency-rate-list) are pure reads, so this
// module is read-only: a ReadService plus its result DTOs and the ReadModel
// port. There is no write Service.
//
// JSON field names are frozen to the existing API wire contract; see
// CLAUDE.md.
package currency

// ---------------------------------------------------------------------------
// get-currency-list
// ---------------------------------------------------------------------------

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

// GetCurrencyListResult is the get-currency-list response: {items: [...]}.
type GetCurrencyListResult struct {
	Items []CurrencyResult `json:"items"`
}

// ---------------------------------------------------------------------------
// get-currency-rate-list
// ---------------------------------------------------------------------------

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
