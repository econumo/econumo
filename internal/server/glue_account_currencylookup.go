package server

import (
	"context"

	appaccount "github.com/econumo/econumo/internal/app/account"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
)

// AccountCurrencyLookup adapts currencyrepo.Lookup to app/account.CurrencyLookup.
// Lives here (not in infra/repo/account) because once the currency feature owns
// its repo package, an infra leaf can no longer import it directly.
type AccountCurrencyLookup struct {
	inner *currencyrepo.Lookup
}

var _ appaccount.CurrencyLookup = (*AccountCurrencyLookup)(nil)

// NewAccountCurrencyLookup wraps a currencyrepo.Lookup.
func NewAccountCurrencyLookup(inner *currencyrepo.Lookup) *AccountCurrencyLookup {
	return &AccountCurrencyLookup{inner: inner}
}

// GetByID resolves a currency for the account-result embed.
func (l *AccountCurrencyLookup) GetByID(ctx context.Context, id string) (appaccount.CurrencyView, error) {
	v, err := l.inner.GetByID(ctx, id)
	if err != nil {
		return appaccount.CurrencyView{}, err
	}
	return appaccount.CurrencyView{
		ID:             v.ID,
		Code:           v.Code,
		Name:           v.Name,
		Symbol:         v.Symbol,
		FractionDigits: v.FractionDigits,
	}, nil
}
