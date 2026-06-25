// Adapters that satisfy the account service's CurrencyLookup and UserLookup
// ports by delegating to the existing currency + user repositories. They live
// here (infra) so the app layer depends only on its own small interfaces, not on
// the currency/user repo packages.
package accountrepo

import (
	"context"

	appaccount "github.com/econumo/econumo/internal/app/account"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domuser "github.com/econumo/econumo/internal/domain/user"
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

// userByID is the minimal user-repo surface this adapter needs.
type userByID interface {
	GetHeaderByID(ctx context.Context, id vo.Id) (domuser.Header, error)
}

// UserLookup adapts the user repository to app/account.UserLookup (owner embed).
type UserLookup struct {
	users userByID
}

var _ appaccount.UserLookup = (*UserLookup)(nil)

// NewUserLookup wraps a user repository (anything exposing GetByID).
func NewUserLookup(users userByID) *UserLookup {
	return &UserLookup{users: users}
}

// GetOwner resolves the owner (id, name, avatar) for the account-result embed.
func (l *UserLookup) GetOwner(ctx context.Context, userID string) (appaccount.OwnerView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return appaccount.OwnerView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return appaccount.OwnerView{}, err
	}
	return appaccount.OwnerView{ID: h.ID, Name: h.Name, Avatar: h.AvatarURL}, nil
}
