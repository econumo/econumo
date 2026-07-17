package budget

import (
	"context"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ExcludeAccount excludes an account the user owns from the budget; returns meta.
func (s *Service) ExcludeAccount(ctx context.Context, userID vo.Id, req model.ExcludeAccountRequest) (*model.ExcludeAccountResult, error) {
	meta, err := s.toggleAccount(ctx, userID, req.BudgetId, req.AccountId, true)
	if err != nil {
		return nil, err
	}
	return &model.ExcludeAccountResult{Item: meta}, nil
}

// IncludeAccount re-includes a previously excluded account; returns meta.
func (s *Service) IncludeAccount(ctx context.Context, userID vo.Id, req model.IncludeAccountRequest) (*model.IncludeAccountResult, error) {
	meta, err := s.toggleAccount(ctx, userID, req.BudgetId, req.AccountId, false)
	if err != nil {
		return nil, err
	}
	return &model.IncludeAccountResult{Item: meta}, nil
}

// toggleAccount excludes (exclude=true) or includes an account; the account must
// be owned by the requester (access denied otherwise).
func (s *Service) toggleAccount(ctx context.Context, userID vo.Id, rawBudget, rawAccount string, exclude bool) (model.MetaResult, error) {
	budgetID, err := vo.ParseId(rawBudget)
	if err != nil {
		return model.MetaResult{}, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	accountID, err := vo.ParseId(rawAccount)
	if err != nil {
		return model.MetaResult{}, model.ValidateBlank(map[string]string{"accountId": ""})
	}
	owner, err := s.accounts.AccountOwner(ctx, accountID)
	if err != nil {
		return model.MetaResult{}, err
	}
	if !owner.Equal(userID) {
		return model.MetaResult{}, accessDenied()
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if exclude {
			return s.budgets.ExcludeAccount(txCtx, budgetID, accountID)
		}
		return s.budgets.IncludeAccount(txCtx, budgetID, accountID)
	}); err != nil {
		return model.MetaResult{}, err
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return model.MetaResult{}, err
	}
	return s.buildMeta(ctx, b)
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
// create-budget and maintained by restoreElementsOrder, which runs only on
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
	if rerr := s.restoreElementsOrder(ctx, budgetID, now); rerr != nil {
		return nil, rerr
	}
	return s.elements.GetElementByExternal(ctx, budgetID, externalID)
}
