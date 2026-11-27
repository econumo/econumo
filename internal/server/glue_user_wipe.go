package server

import (
	"context"
	"slices"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

// ErrWipeSetChanged: retry the wipe; nothing was changed.
var ErrWipeSetChanged = errs.NewSentinel("server.ErrWipeSetChanged", "wipe: the set of budgets changed while locking")

type wipeDebts interface {
	BudgetIDsOfCategories(ctx context.Context, ids ...vo.Id) ([]vo.Id, error)
	LockBudget(ctx context.Context, budgetID vo.Id) error
	HoldsBudgetLock(ctx context.Context, budgetID vo.Id) bool
	ReleaseCategory(ctx context.Context, id vo.Id) error
}

type wipeCategories interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Category, error)
}

type wipeBudgets interface {
	WipeBudgetIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
	UnwindUser(ctx context.Context, userID vo.Id) error
	DeleteOwned(ctx context.Context, userID vo.Id) error
}

type wipeAccounts interface {
	LockOwnedForWipe(ctx context.Context, userID vo.Id) error
	UnwindGrantsOf(ctx context.Context, userID vo.Id) error
}

type wipeTransactions interface {
	RewriteForDeletedUser(ctx context.Context, userID vo.Id) error
}

type wipeCurrencies interface {
	MoveSharedCustomCurrencies(ctx context.Context, userID vo.Id) error
}

type wipeApps interface {
	DeleteAppsOwnedBy(ctx context.Context, userID vo.Id) error
}

type wipeImports interface {
	ClearAppliedRefsToUser(ctx context.Context, userID vo.Id) error
}

type wipeIdempotency interface {
	DeleteByUser(ctx context.Context, userID vo.Id) error
}

type wipeOAuthStates interface {
	DeleteByLinkUser(ctx context.Context, userID vo.Id) (int64, error)
}

// UserDataWiper sequences every feature's part of hard-deleting a user.
type UserDataWiper struct {
	debts        wipeDebts
	categories   wipeCategories
	budgets      wipeBudgets
	accounts     wipeAccounts
	transactions wipeTransactions
	currencies   wipeCurrencies
	apps         wipeApps
	imports      wipeImports
	idempotency  wipeIdempotency
	oauthStates  wipeOAuthStates
}

var _ appuser.DataWiper = (*UserDataWiper)(nil)

func NewUserDataWiper(
	debts wipeDebts, categories wipeCategories, budgets wipeBudgets, accounts wipeAccounts,
	transactions wipeTransactions, currencies wipeCurrencies, apps wipeApps, imports wipeImports,
	idempotency wipeIdempotency, oauthStates wipeOAuthStates,
) *UserDataWiper {
	return &UserDataWiper{
		debts: debts, categories: categories, budgets: budgets, accounts: accounts,
		transactions: transactions, currencies: currencies, apps: apps, imports: imports,
		idempotency: idempotency, oauthStates: oauthStates,
	}
}

// budgetSet is the budgets the wipe touches (the user's own and shared ones,
// plus those holding a debt on one of the user's categories), sorted and
// distinct, together with the user's categories.
func (w *UserDataWiper) budgetSet(ctx context.Context, userID vo.Id) ([]vo.Id, []vo.Id, error) {
	budgets, err := w.budgets.WipeBudgetIDs(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	cats, err := w.categories.ListByOwner(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	catIDs := make([]vo.Id, len(cats))
	for i, c := range cats {
		catIDs[i] = c.ID
	}
	debtBudgets, err := w.debts.BudgetIDsOfCategories(ctx, catIDs...)
	if err != nil {
		return nil, nil, err
	}
	all := slices.Concat(budgets, debtBudgets)
	slices.SortFunc(all, func(a, b vo.Id) int { return strings.Compare(a.String(), b.String()) })
	all = slices.CompactFunc(all, func(a, b vo.Id) bool { return a.String() == b.String() })
	return all, catIDs, nil
}

func (w *UserDataWiper) LockBudgets(ctx context.Context, userID vo.Id) error {
	budgets, _, err := w.budgetSet(ctx, userID)
	if err != nil {
		return err
	}
	for _, id := range budgets {
		if err := w.debts.LockBudget(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (w *UserDataWiper) Erase(ctx context.Context, userID vo.Id) error {
	if err := w.accounts.LockOwnedForWipe(ctx, userID); err != nil {
		return err
	}
	// A budget cloned or shared between LockBudgets reading the set and taking
	// the locks is not locked; going on would take its lock after account rows,
	// inverting the order every other writer uses.
	budgets, catIDs, err := w.budgetSet(ctx, userID)
	if err != nil {
		return err
	}
	for _, id := range budgets {
		if !w.debts.HoldsBudgetLock(ctx, id) {
			return ErrWipeSetChanged
		}
	}
	if err := w.budgets.UnwindUser(ctx, userID); err != nil {
		return err
	}
	if err := w.accounts.UnwindGrantsOf(ctx, userID); err != nil {
		return err
	}
	for _, id := range catIDs {
		if err := w.debts.ReleaseCategory(ctx, id); err != nil {
			return err
		}
	}
	steps := []func(context.Context, vo.Id) error{
		w.transactions.RewriteForDeletedUser,
		w.currencies.MoveSharedCustomCurrencies,
		w.apps.DeleteAppsOwnedBy,
		w.imports.ClearAppliedRefsToUser,
		w.idempotency.DeleteByUser,
		func(ctx context.Context, userID vo.Id) error {
			_, err := w.oauthStates.DeleteByLinkUser(ctx, userID)
			return err
		},
		w.budgets.DeleteOwned,
	}
	for _, step := range steps {
		if err := step(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}
