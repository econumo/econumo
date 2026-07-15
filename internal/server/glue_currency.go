// Currency glue: adapters for ports the currency feature declares.
package server

import (
	"context"

	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CurrencyProfileCurrency answers "what is this user's profile currency code"
// for the hide-currency guard, over the SAME user-repo dependency
// BudgetUserLookup.CurrencyCode already uses (see glue_budget.go) rather than
// through the user feature's public Service, keeping this a thin repo read.
type CurrencyProfileCurrency struct {
	users budgetUserRepo
}

var _ appcurrency.ProfileCurrency = (*CurrencyProfileCurrency)(nil)

// NewCurrencyProfileCurrency wraps a user repository.
func NewCurrencyProfileCurrency(users budgetUserRepo) *CurrencyProfileCurrency {
	return &CurrencyProfileCurrency{users: users}
}

// CurrencyCode returns the user's profile currency option, defaulting to the
// domain default when the option is unset.
func (p *CurrencyProfileCurrency) CurrencyCode(ctx context.Context, userID string) (string, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return "", err
	}
	u, err := p.users.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if o := u.Option(model.OptionCurrency); o != nil && o.Value != nil {
		return *o.Value, nil
	}
	return model.DefaultCurrency, nil
}
