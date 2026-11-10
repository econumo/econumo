// Onboarding glue: adapters for the ports internal/onboarding declares. The
// budget port is satisfied by the budget service itself.
package server

import (
	"context"
	"errors"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/model"
	apponboarding "github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

var _ apponboarding.BudgetWriter = (*appbudget.Service)(nil)

type onboardingAccountRepo interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Account, error)
}

type onboardingCurrencyLookup interface {
	GetByID(ctx context.Context, id string) (model.CurrencyView, error)
}

// OnboardingAccountReader adapts the account repository plus the currency
// lookup to onboarding.AccountReader.
type OnboardingAccountReader struct {
	accounts onboardingAccountRepo
	currency onboardingCurrencyLookup
}

var _ apponboarding.AccountReader = (*OnboardingAccountReader)(nil)

func NewOnboardingAccountReader(accounts onboardingAccountRepo, currency onboardingCurrencyLookup) *OnboardingAccountReader {
	return &OnboardingAccountReader{accounts: accounts, currency: currency}
}

func (r *OnboardingAccountReader) AccountsByIDs(ctx context.Context, ids []vo.Id) ([]model.OnboardingAccount, error) {
	out := make([]model.OnboardingAccount, 0, len(ids))
	for _, id := range ids {
		a, err := r.accounts.GetByID(ctx, id)
		if err != nil {
			var nf *errs.NotFoundError
			if errors.As(err, &nf) {
				continue
			}
			return nil, err
		}
		code := ""
		if cur, cerr := r.currency.GetByID(ctx, a.CurrencyID.String()); cerr == nil {
			code = cur.Code
		}
		out = append(out, model.OnboardingAccount{
			ID: a.ID.String(), OwnerID: a.UserID.String(), Name: a.Name, CurrencyCode: code, IsDeleted: a.IsDeleted,
		})
	}
	return out, nil
}

type onboardingTimezoneRepo interface {
	GetTimezone(ctx context.Context, id vo.Id) (string, error)
}

// OnboardingTimezones adapts the user repository to onboarding.TimezoneReader.
type OnboardingTimezones struct {
	users onboardingTimezoneRepo
}

func NewOnboardingTimezones(users onboardingTimezoneRepo) *OnboardingTimezones {
	return &OnboardingTimezones{users: users}
}

func (t *OnboardingTimezones) StoredTimezone(ctx context.Context, userID vo.Id) (string, error) {
	return t.users.GetTimezone(ctx, userID)
}
