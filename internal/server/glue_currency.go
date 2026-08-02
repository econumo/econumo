// Currency glue: adapters for ports the currency feature declares.
package server

import (
	"context"
	"fmt"

	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CurrencyProfileCurrency answers "what is this user's profile currency id"
// for the hide/archive guards, over the SAME user-repo dependency
// BudgetUserLookup uses (see glue_budget.go) rather than through the user
// feature's public Service, keeping this a thin repo read.
type CurrencyProfileCurrency struct {
	users budgetUserRepo
}

var _ appcurrency.ProfileCurrency = (*CurrencyProfileCurrency)(nil)

// NewCurrencyProfileCurrency wraps a user repository.
func NewCurrencyProfileCurrency(users budgetUserRepo) *CurrencyProfileCurrency {
	return &CurrencyProfileCurrency{users: users}
}

// CurrencyID returns the user's stored profile currency id. The migration
// guarantees every user holds one, so an absent option is an error.
func (p *CurrencyProfileCurrency) CurrencyID(ctx context.Context, userID string) (string, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return "", err
	}
	u, err := p.users.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if o := u.Option(model.OptionCurrency); o != nil && o.Value != nil && *o.Value != "" {
		return *o.Value, nil
	}
	return "", fmt.Errorf("user %s: currency option missing", userID)
}
