package budget

import (
	"context"
	"strconv"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// A debt belongs to a budget; these are what the debt feature reads of one
// (wrapped into debt.Budgets in internal/server).

// DebtBudget is the budget as userID sees it. No accepted access (a pending
// invite included) reads as not found, as for any budget the user cannot see.
func (s *Service) DebtBudget(ctx context.Context, userID, budgetID vo.Id) (model.DebtBudget, error) {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return model.DebtBudget{}, err
	}
	if !s.canRead(b, userID) {
		return model.DebtBudget{}, errs.NewNotFound("Budget not found")
	}
	return s.debtBudgetOf(b, userID), nil
}

// DebtBudgets lists the budgets userID can read, in the budget list's order.
func (s *Service) DebtBudgets(ctx context.Context, userID vo.Id) ([]model.DebtBudget, error) {
	budgets, err := s.budgets.ListForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.DebtBudget, 0, len(budgets))
	for _, bud := range budgets {
		b, err := s.loadAggregate(ctx, bud.ID)
		if err != nil {
			return nil, err
		}
		if s.canRead(b, userID) {
			out = append(out, s.debtBudgetOf(b, userID))
		}
	}
	return out, nil
}

func (s *Service) debtBudgetOf(b *budgetAggregate, userID vo.Id) model.DebtBudget {
	return model.DebtBudget{
		ID: b.budget.ID, CurrencyID: b.budget.CurrencyID, Name: b.budget.Name,
		PeriodStartDay: b.budget.PeriodStartDay, IsArchived: b.budget.IsArchived,
		CanPlan: s.canUpdate(b, userID), AccountIDs: b.debtScope().AccountIDs,
	}
}

// DebtScope is the budget's everyday member accounts, soft-deleted ones
// included, read with no access check: the debt sync of a budget whose
// categories a classification change touched runs for every reader alike.
func (s *Service) DebtScope(ctx context.Context, budgetID vo.Id) (model.DebtScope, error) {
	members, err := s.budgets.MemberAccounts(ctx, budgetID)
	if err != nil {
		return model.DebtScope{}, err
	}
	return model.DebtScope{BudgetID: budgetID, AccountIDs: everydayIDs(members)}, nil
}

func everydayIDs(members []model.BudgetAccount) []vo.Id {
	ids := make([]vo.Id, 0, len(members))
	for _, m := range members {
		if !m.IsSavings {
			ids = append(ids, m.AccountID)
		}
	}
	return ids
}

// DebtElement describes the budget's element of that external id for the
// debt feature: an expense envelope, or a category a participant owns. It
// reads only, so the debt service can call it inside a budget read.
func (s *Service) DebtElement(ctx context.Context, budgetID, elementID vo.Id) (model.DebtElementInfo, error) {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return model.DebtElementInfo{}, err
	}
	info := model.DebtElementInfo{ID: elementID, CurrencyID: b.budget.CurrencyID}
	var row *model.BudgetElement
	for _, el := range b.elements {
		if el.ExternalID.Equal(elementID) {
			row = el
			break
		}
	}
	if row != nil && row.CurrencyID != nil {
		info.CurrencyID = *row.CurrencyID
	}
	for _, env := range b.envelopes {
		if !env.ID.Equal(elementID) {
			continue
		}
		children, err := s.envelopes.EnvelopeCategoryIDs(ctx, env.ID)
		if err != nil {
			return info, err
		}
		info.Type, info.Name, info.Icon, info.CategoryIDs = model.DebtElementEnvelope, env.Name, env.Icon, children
		info.Eligible = row != nil && row.Type == model.ElementEnvelope
		return info, nil
	}
	cats, err := s.metadata.CategoriesByOwners(ctx, b.participantIDs())
	if err != nil {
		return info, err
	}
	for _, c := range cats {
		if c.ID != elementID.String() {
			continue
		}
		info.Type, info.Name, info.Icon, info.CategoryIDs = model.DebtElementCategory, c.Name, c.Icon, []vo.Id{elementID}
		hidden, err := s.inEnvelopeID(ctx, b, elementID)
		if err != nil {
			return info, err
		}
		info.Eligible = !c.IsIncome && !hidden
		return info, nil
	}
	return info, nil
}

func (s *Service) inEnvelopeID(ctx context.Context, b *budgetAggregate, categoryID vo.Id) (bool, error) {
	children, err := s.envelopeCategories(ctx, b)
	if err != nil {
		return false, err
	}
	for _, ids := range children {
		for _, id := range ids {
			if id == categoryID.String() {
				return true, nil
			}
		}
	}
	return false, nil
}

// DebtElementPlans are the plans of the budget's row of the external id, by
// period key, in the row's currency; none when the budget has no such row.
func (s *Service) DebtElementPlans(ctx context.Context, budgetID, elementID vo.Id) (map[string]float64, error) {
	el, err := s.elements.GetElementByExternal(ctx, budgetID, elementID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return map[string]float64{}, nil
		}
		return nil, err
	}
	limits, err := s.limits.ListLimitsByElement(ctx, el.ID)
	if err != nil {
		return nil, err
	}
	plans := make(map[string]float64, len(limits))
	for _, l := range limits {
		if f, err := strconv.ParseFloat(l.Amount.String(), 64); err == nil {
			plans[l.Period.Format(datetime.DateLayout)] = f
		}
	}
	return plans, nil
}
