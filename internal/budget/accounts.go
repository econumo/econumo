package budget

import (
	"context"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// removableAccounts reports, per account, whether it may still leave the
// budget: only while it has no transactions in a CLOSED month of the budget
// (started_at <= spent_at < first of the caller's current month). Removing such
// an account changes only the month being viewed; anything older would shrink
// spentBefore under limits that stay — the drift this design exists to end.
func (s *Service) removableAccounts(ctx context.Context, b *budgetAggregate, ids []vo.Id, now time.Time) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id.String()] = true
	}
	if len(ids) == 0 {
		return out, nil
	}
	end := localMonth(now, reqctx.Location(ctx))
	active, err := s.read.AccountsWithTransactions(ctx, ids, model.FirstOfMonth(b.budget.StartedAt), end)
	if err != nil {
		return nil, err
	}
	for _, id := range active {
		out[id.String()] = false
	}
	return out, nil
}

func accountNotRemovable() error {
	return errs.NewValidation("Validation failed", errs.FieldError{
		Key:     "accountId",
		Message: "This account has transactions in past months and can no longer be removed from the budget",
		Code:    errs.CodeBudgetAccountNotRemovable,
	})
}

// AddAccount makes an account the caller owns a member of the budget. Adding an
// account that is already a member is a no-op — deleted members stay listed in
// the filters block (they keep counting), so a client may well name one again;
// only a NEW member has to be a live account.
func (s *Service) AddAccount(ctx context.Context, userID vo.Id, req model.AddAccountRequest) (*model.AddAccountResult, error) {
	budgetID, accountID, b, err := s.membershipPrelude(ctx, userID, req.BudgetId, req.AccountId)
	if err != nil {
		return nil, err
	}
	if !b.hasAccount(accountID) {
		views, verr := s.accounts.AccountsByIDs(ctx, []vo.Id{accountID})
		if verr != nil {
			return nil, verr
		}
		if views[0].IsDeleted {
			return nil, model.ValidateBlank(map[string]string{"accountId": ""})
		}
		if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
			return s.budgets.AddAccount(txCtx, budgetID, accountID, s.clock.Now())
		}); err != nil {
			return nil, err
		}
	}
	meta, err := s.reloadMeta(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	return &model.AddAccountResult{Item: meta}, nil
}

// RemoveAccount drops a member account the caller owns, unless its history in
// closed months has locked it in place.
func (s *Service) RemoveAccount(ctx context.Context, userID vo.Id, req model.RemoveAccountRequest) (*model.RemoveAccountResult, error) {
	budgetID, accountID, b, err := s.membershipPrelude(ctx, userID, req.BudgetId, req.AccountId)
	if err != nil {
		return nil, err
	}
	if b.hasAccount(accountID) {
		removable, rerr := s.removableAccounts(ctx, b, []vo.Id{accountID}, s.clock.Now())
		if rerr != nil {
			return nil, rerr
		}
		if !removable[accountID.String()] {
			return nil, accountNotRemovable()
		}
		if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
			return s.budgets.RemoveAccount(txCtx, budgetID, accountID)
		}); err != nil {
			return nil, err
		}
	}
	meta, err := s.reloadMeta(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	return &model.RemoveAccountResult{Item: meta}, nil
}

// membershipPrelude parses ids and enforces the two authorization rules every
// membership write shares: the caller owns the account and can update the budget.
func (s *Service) membershipPrelude(ctx context.Context, userID vo.Id, rawBudget, rawAccount string) (vo.Id, vo.Id, *budgetAggregate, error) {
	budgetID, err := vo.ParseId(rawBudget)
	if err != nil {
		return vo.Id{}, vo.Id{}, nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	accountID, err := vo.ParseId(rawAccount)
	if err != nil {
		return vo.Id{}, vo.Id{}, nil, model.ValidateBlank(map[string]string{"accountId": ""})
	}
	owner, err := s.accounts.AccountOwner(ctx, accountID)
	if err != nil {
		return vo.Id{}, vo.Id{}, nil, err
	}
	if !owner.Equal(userID) {
		return vo.Id{}, vo.Id{}, nil, accessDenied()
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return vo.Id{}, vo.Id{}, nil, err
	}
	if !s.canUpdate(b, userID) {
		return vo.Id{}, vo.Id{}, nil, accessDenied()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return vo.Id{}, vo.Id{}, nil, aerr
	}
	return budgetID, accountID, b, nil
}

func (s *Service) reloadMeta(ctx context.Context, budgetID vo.Id) (model.MetaResult, error) {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return model.MetaResult{}, err
	}
	return s.buildMeta(ctx, b)
}

// ownsAccount reports whether accountID belongs to userID. The lookup is an
// indexed primary-key read and is deliberately NOT memoized on the Service:
// one Service instance serves every caller in parallel, so a shared map here
// would be a `fatal error: concurrent map writes` (unrecoverable — the process
// dies, the recover middleware never sees it). A missing account is "not
// owned": callers skip such ids rather than failing the whole update.
func (s *Service) ownsAccount(ctx context.Context, userID, accountID vo.Id) (bool, error) {
	owner, err := s.accounts.AccountOwner(ctx, accountID)
	if err != nil {
		var nf *errs.NotFoundError
		if errors.As(err, &nf) {
			return false, nil
		}
		return false, err
	}
	return owner.Equal(userID), nil
}

// ChangeElementCurrency sets a budget element's display currency (canUpdate).
func (s *Service) ChangeElementCurrency(ctx context.Context, userID vo.Id, req model.ChangeElementCurrencyRequest) (*model.ChangeElementCurrencyResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	elementID, err := vo.ParseId(req.ElementId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"elementId": ""})
	}
	curID, err := vo.ParseId(req.CurrencyId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"currencyId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// elementId on the wire is the element's EXTERNAL id (category/tag/envelope).
		el, gerr := s.getElementSelfHeal(txCtx, budgetID, elementID, now)
		if gerr != nil {
			return gerr
		}
		if el.CurrencyID == nil || !el.CurrencyID.Equal(curID) {
			if eerr := s.currency.EnsureUsable(txCtx, userID.String(), curID.String()); eerr != nil {
				return eerr
			}
		}
		el.UpdateCurrency(&curID, now)
		return s.elements.SaveElement(txCtx, el)
	})
	if err != nil {
		return nil, err
	}
	return &model.ChangeElementCurrencyResult{}, nil
}

// SetLimit sets or clears an element's period limit (canUpdate). amount nil ->
// delete the limit; period must be >= budget.startedAt.
func (s *Service) SetLimit(ctx context.Context, userID vo.Id, req model.SetLimitRequest) (*model.SetLimitResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	externalID, err := vo.ParseId(req.ElementId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"elementId": ""})
	}
	period, err := time.Parse(datetime.DateLayout, req.Period)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"period": ""})
	}
	period = model.FirstOfMonth(period)

	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}
	if period.Before(model.FirstOfMonth(b.budget.StartedAt)) {
		return nil, model.ValidateBlank(map[string]string{"period": ""}) // invalid-date guard
	}

	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// elementId on the wire is the EXTERNAL id; resolve to the budget element.
		element, gerr := s.getElementSelfHeal(txCtx, budgetID, externalID, now)
		if gerr != nil {
			return gerr
		}
		elementID := element.ID

		existing, gerr := s.limits.GetLimit(txCtx, elementID, period)
		hasExisting := gerr == nil
		if gerr != nil {
			var nf *errs.NotFoundError
			if !errors.As(gerr, &nf) {
				return gerr
			}
		}
		if req.Amount == nil {
			if hasExisting {
				return s.limits.DeleteLimit(txCtx, existing.ID)
			}
			return nil
		}
		amount := vo.NewDecimal(req.Amount.String())
		if hasExisting {
			existing.UpdateAmount(amount, now)
			return s.limits.SaveLimit(txCtx, existing)
		}
		limit := model.NewBudgetElementLimit(s.limits.NextIdentity(), elementID, amount, period, now)
		return s.limits.SaveLimit(txCtx, limit)
	})
	if err != nil {
		return nil, err
	}
	return &model.SetLimitResult{}, nil
}

// getElementSelfHeal resolves a wire (external) element id to its budget
// element, backfilling a missing budget_elements row. Rows are seeded at
// create-budget and maintained by syncElements, which runs only on
// structure mutations — so a tag/category created after the budget has no row
// yet, even though get-budget already shows it (visibility is computed from
// spending/limits, not element rows). On a miss, restore the element order
// (which creates rows for every participant entity) and retry once; an id that
// is no participant entity at all still resolves to "BudgetElement not found".
func (s *Service) getElementSelfHeal(ctx context.Context, budgetID, externalID vo.Id, now time.Time) (*model.BudgetElement, error) {
	el, err := s.elements.GetElementByExternal(ctx, budgetID, externalID)
	var nf *errs.NotFoundError
	if err == nil || !errors.As(err, &nf) {
		return el, err
	}
	if rerr := s.syncElements(ctx, budgetID, now); rerr != nil {
		return nil, rerr
	}
	return s.elements.GetElementByExternal(ctx, budgetID, externalID)
}
