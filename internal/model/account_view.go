// View-row shapes the account+folder service's port interfaces exchange with
// their adapters (not wire DTOs — no JSON tags).
package model

import "time"

// CurrencyView is the embeddable currency shape an account result needs.
type CurrencyView struct {
	ID             string
	Code           string
	Name           string
	Symbol         string
	FractionDigits int
}

// OwnerView is the minimal owner shape an account/budget/connection result
// embeds. AccessLevel/AccessUntil are the raw stored columns (see
// model.Header); only the connection list reads them.
type OwnerView struct {
	ID          string
	Name        string
	Avatar      string
	AccessLevel AccessLevel
	AccessUntil *time.Time
}
