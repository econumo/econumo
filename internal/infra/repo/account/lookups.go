// Adapters that satisfy the account service's CurrencyLookup port by
// delegating to the existing currency repository. Lives here (infra) so the
// app layer depends only on its own small interface, not on the currency repo
// package. The UserLookup counterpart lives in internal/server (it needs the
// user feature's Header type, which an infra package cannot import).
package accountrepo

import (
	"context"

	appaccount "github.com/econumo/econumo/internal/app/account"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
)

// CurrencyLookup adapts currencyrepo.Lookup to app/account.CurrencyLookup.
type CurrencyLookup struct {
	inner *currencyrepo.Lookup
}

var _ appaccount.CurrencyLookup = (*CurrencyLookup)(nil)

// NewCurrencyLookup wraps a currencyrepo.Lookup.
func NewCurrencyLookup(inner *currencyrepo.Lookup) *CurrencyLookup {
	return &CurrencyLookup{inner: inner}
}

// GetByID resolves a currency for the account-result embed.
func (l *CurrencyLookup) GetByID(ctx context.Context, id string) (appaccount.CurrencyView, error) {
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
