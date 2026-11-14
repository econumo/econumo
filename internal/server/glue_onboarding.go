// Onboarding glue: adapters for the ports internal/onboarding declares. The
// budget port is satisfied by the budget service itself.
package server

import (
	"context"
	"errors"
	"strings"

	appaccount "github.com/econumo/econumo/internal/account"
	appbudget "github.com/econumo/econumo/internal/budget"
	appcategory "github.com/econumo/econumo/internal/category"
	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/model"
	apponboarding "github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

var _ apponboarding.BudgetWriter = (*appbudget.Service)(nil)

type onboardingAccountRepo interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Account, error)
}

type onboardingCurrencyLookup interface {
	GetByID(ctx context.Context, id string) (model.CurrencyView, error)
	EnsureUsable(ctx context.Context, userID, currencyID string) error
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

// OnboardingAccounts adapts the account service and repository plus the
// currency lookup to onboarding.AccountSteps.
type OnboardingAccounts struct {
	*appaccount.Service
	accounts onboardingAccountList
	currency onboardingCurrencyLookup
	clock    port.Clock
}

func NewOnboardingAccounts(svc *appaccount.Service, accounts onboardingAccountList, currency onboardingCurrencyLookup, clock port.Clock) *OnboardingAccounts {
	return &OnboardingAccounts{Service: svc, accounts: accounts, currency: currency, clock: clock}
}

// ReviseAccounts runs update-account for each change with the account's own
// name and icon as get-account-list shows them, and its shown balance unless
// the change sets one: an unchanged balance equals the computed one, so no
// correction is written. A correction is dated now in the caller's timezone,
// the day the "balance as of today" it fixes is computed for.
func (a *OnboardingAccounts) ReviseAccounts(ctx context.Context, userID vo.Id, changes []model.OnboardingAccountRevision) error {
	list, err := a.AccountListForUser(ctx, userID)
	if err != nil {
		return err
	}
	byID := make(map[string]model.AccountResult, len(list))
	for _, item := range list {
		byID[strings.ToLower(item.Id)] = item
	}
	updatedAt := a.clock.Now().In(reqctx.Location(ctx)).Format(datetime.Layout)
	for _, c := range changes {
		item, ok := byID[strings.ToLower(c.AccountId)]
		if !ok {
			return errs.NewNotFound("Account not found")
		}
		req := model.UpdateAccountRequest{
			Id: item.Id, Name: item.Name, Balance: vo.NewFlexString(item.Balance), Icon: item.Icon, UpdatedAt: updatedAt,
		}
		if c.Balance != "" {
			req.Balance = vo.NewFlexString(c.Balance)
		}
		if c.CurrencyId != "" {
			currencyID := c.CurrencyId
			req.CurrencyId = &currencyID
		}
		if _, err := a.UpdateAccount(ctx, userID, req); err != nil {
			return err
		}
	}
	return nil
}

func (a *OnboardingAccounts) OwnLiveAccounts(ctx context.Context, userID vo.Id) ([]model.OnboardingAccount, error) {
	list, err := a.accounts.ListAvailable(ctx, userID)
	if err != nil {
		return nil, err
	}
	visible, err := a.VisibleAccountIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	shown := make(map[string]bool, len(visible))
	for _, id := range visible {
		shown[id.String()] = true
	}
	codes := map[string]string{}
	out := make([]model.OnboardingAccount, 0, len(list))
	for _, acc := range list {
		if !acc.UserID.Equal(userID) || acc.IsDeleted {
			continue
		}
		cid := acc.CurrencyID.String()
		code, seen := codes[cid]
		if !seen {
			if cur, cerr := a.currency.GetByID(ctx, cid); cerr == nil {
				code = cur.Code
			}
			codes[cid] = code
		}
		out = append(out, model.OnboardingAccount{
			ID: acc.ID.String(), OwnerID: acc.UserID.String(), Name: acc.Name, CurrencyID: cid, CurrencyCode: code,
			Icon: acc.Icon, Hidden: !shown[acc.ID.String()], CreatedAt: acc.CreatedAt,
		})
	}
	return out, nil
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

func (c *OnboardingCurrencies) FractionDigits(ctx context.Context, currencyID string) (int, error) {
	cur, err := c.lookup.GetByID(ctx, currencyID)
	if err != nil {
		return 0, err
	}
	return int(cur.FractionDigits), nil
}

func (c *OnboardingCurrencies) UsableCurrencyCode(ctx context.Context, userID vo.Id, currencyID string) (string, error) {
	if err := c.lookup.EnsureUsable(ctx, userID.String(), currencyID); err != nil {
		return "", err
	}
	cur, err := c.lookup.GetByID(ctx, currencyID)
	if err != nil {
		return "", err
	}
	return cur.Code, nil
}

type onboardingDebtService interface {
	CreateDebt(ctx context.Context, userID vo.Id, req model.CreateDebtRequest) (*model.DebtResult, error)
}

// OnboardingDebts adapts the debt service to onboarding.DebtSteps: a debt
// detection found attaches a debt not set up to the category's element in the
// budget, through create-debt's own validation, and a refusal (the category
// already carries an active debt or cannot carry one, the caller cannot plan
// the budget) creates nothing.
type OnboardingDebts struct {
	onboardingDebtService
}

var _ apponboarding.DebtSteps = OnboardingDebts{}

func NewOnboardingDebts(debts onboardingDebtService) OnboardingDebts {
	return OnboardingDebts{onboardingDebtService: debts}
}

func (d OnboardingDebts) CreateDebt(ctx context.Context, userID, budgetID, categoryID vo.Id, payment vo.DecimalNumber, schedule model.RecurringSchedule) (bool, error) {
	req := model.CreateDebtRequest{
		BudgetId: budgetID.String(), ElementId: categoryID.String(),
		Payment: vo.NewFlexString(payment.String()), Schedule: string(schedule),
	}
	err := req.Validate()
	if err == nil {
		_, err = d.onboardingDebtService.CreateDebt(ctx, userID, req)
	}
	if err == nil {
		return true, nil
	}
	var ve *errs.ValidationError
	_, notFound := errs.AsNotFound(err)
	_, denied := errs.AsAccessDenied(err)
	if notFound || denied || errors.As(err, &ve) {
		return false, nil
	}
	return false, err
}
