// Onboarding glue: adapters for the ports internal/onboarding declares. The
// budget port is satisfied by the budget service itself.
package server

import (
	"context"
	"errors"

	appaccount "github.com/econumo/econumo/internal/account"
	appbudget "github.com/econumo/econumo/internal/budget"
	appcategory "github.com/econumo/econumo/internal/category"
	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/model"
	apponboarding "github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
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

var (
	_ apponboarding.UserSteps   = (*appuser.Service)(nil)
	_ apponboarding.BudgetSteps = (*appbudget.Service)(nil)
)

// OnboardingCategories adapts the category services to onboarding.CategorySteps.
type OnboardingCategories struct {
	*appcategory.Service
	read *appcategory.ReadService
}

func NewOnboardingCategories(svc *appcategory.Service, read *appcategory.ReadService) *OnboardingCategories {
	return &OnboardingCategories{Service: svc, read: read}
}

// OwnCategories is the caller's own slice of get-category-list, in its order.
func (c *OnboardingCategories) OwnCategories(ctx context.Context, userID vo.Id) ([]model.CategoryResult, error) {
	res, err := c.read.GetCategoryList(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.CategoryResult, 0, len(res.Items))
	for _, item := range res.Items {
		if item.OwnerUserId == userID.String() {
			out = append(out, item)
		}
	}
	return out, nil
}

type onboardingAccountList interface {
	ListAvailable(ctx context.Context, userID vo.Id) ([]*model.Account, error)
}

// OnboardingAccounts adapts the account service and repository to
// onboarding.AccountSteps.
type OnboardingAccounts struct {
	*appaccount.Service
	accounts onboardingAccountList
}

func NewOnboardingAccounts(svc *appaccount.Service, accounts onboardingAccountList) *OnboardingAccounts {
	return &OnboardingAccounts{Service: svc, accounts: accounts}
}

func (a *OnboardingAccounts) OwnLiveAccounts(ctx context.Context, userID vo.Id) ([]model.OnboardingAccount, error) {
	list, err := a.accounts.ListAvailable(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.OnboardingAccount, 0, len(list))
	for _, acc := range list {
		if acc.UserID.Equal(userID) && !acc.IsDeleted {
			out = append(out, model.OnboardingAccount{ID: acc.ID.String(), OwnerID: acc.UserID.String(), Name: acc.Name})
		}
	}
	return out, nil
}

func (a *OnboardingAccounts) FirstFolderID(ctx context.Context, userID vo.Id) (string, error) {
	res, err := a.GetFolderList(ctx, userID)
	if err != nil || len(res.Items) == 0 {
		return "", err
	}
	return res.Items[0].Id, nil
}

// OnboardingCurrencies adapts the currency manage service and lookup to
// onboarding.CurrencySteps.
type OnboardingCurrencies struct {
	*appcurrency.ManageService
	lookup onboardingCurrencyLookup
}

func NewOnboardingCurrencies(svc *appcurrency.ManageService, lookup onboardingCurrencyLookup) *OnboardingCurrencies {
	return &OnboardingCurrencies{ManageService: svc, lookup: lookup}
}

func (c *OnboardingCurrencies) CurrencyCode(ctx context.Context, currencyID string) (string, error) {
	cur, err := c.lookup.GetByID(ctx, currencyID)
	if err != nil {
		return "", err
	}
	return cur.Code, nil
}
