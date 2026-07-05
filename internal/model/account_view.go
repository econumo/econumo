// View-row shapes the account+folder service's port interfaces exchange with
// their adapters (not wire DTOs — no JSON tags).
package model

// CurrencyView is the embeddable currency shape an account result needs.
type CurrencyView struct {
	ID             string
	Code           string
	Name           string
	Symbol         string
	FractionDigits int
}

// OwnerView is the minimal owner shape an account/budget/connection result
// embeds.
type OwnerView struct {
	ID     string
	Name   string
	Avatar string
}

// SharedAccessView is one accounts_access grant on an account: the granted
// user's id + the role alias. The owner embed (name/avatar) is resolved by
// the account service via UserLookup.
type SharedAccessView struct {
	UserID string
	Role   string
}
