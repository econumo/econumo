// Account glue: every adapter satisfying a port that the account feature
// declares (see internal/account/ports.go). Features must not import each
// other (archtest); the composition root bridges them here.
package server

import (
	"context"

	appaccount "github.com/econumo/econumo/internal/account"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
)

// AccountCurrencyLookup adapts currencyrepo.Lookup to account.CurrencyLookup.
// Lives here, not in internal/account, because account and currency are
// separate features that must not import each other (see archtest); only the
// composition root may bridge them.
type AccountCurrencyLookup struct {
	inner *currencyrepo.Lookup
}

var _ appaccount.CurrencyLookup = (*AccountCurrencyLookup)(nil)

// NewAccountCurrencyLookup wraps a currencyrepo.Lookup.
func NewAccountCurrencyLookup(inner *currencyrepo.Lookup) *AccountCurrencyLookup {
	return &AccountCurrencyLookup{inner: inner}
}

// GetByID resolves a currency for the account-result embed.
func (l *AccountCurrencyLookup) GetByID(ctx context.Context, id string) (model.CurrencyView, error) {
	v, err := l.inner.GetByID(ctx, id)
	if err != nil {
		return model.CurrencyView{}, err
	}
	return model.CurrencyView{
		ID:             v.ID,
		Code:           v.Code,
		Name:           v.Name,
		Symbol:         v.Symbol,
		FractionDigits: v.FractionDigits,
	}, nil
}
