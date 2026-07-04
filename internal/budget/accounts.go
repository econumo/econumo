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
		el, gerr := s.elements.GetElementByExternal(txCtx, budgetID, elementID)
		if gerr != nil {
			return gerr
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

	// elementId on the wire is the EXTERNAL id; resolve to the budget element.
	element, err := s.elements.GetElementByExternal(ctx, budgetID, externalID)
	if err != nil {
		return nil, err
	}
	elementID := element.ID

	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
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
