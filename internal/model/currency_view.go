// Read-side view rows for the currency endpoints (not wire DTOs — the
// currency ReadService maps them into the frozen result shapes).
package model

// CurrencyViewRow is the read-side currency row. Name is the raw (nullable) DB
// value, which is NULL in practice — the service resolves the wire name from the
// Intl display-name table as a fallback. UserID is nil for a global currency,
// set to the owner id for a custom one.
type CurrencyViewRow struct {
	ID             string
	Code           string
	Symbol         string
	Name           *string
	FractionDigits int16
	UserID         *string
	IsArchived     bool
}

// CurrencyRateViewRow is the read-side rate row. UpdatedAt arrives pre-formatted
// "Y-m-d 00:00:00" from the repo.
type CurrencyRateViewRow struct {
	CurrencyID     string
	BaseCurrencyID string
	Rate           string
	UpdatedAt      string
}
