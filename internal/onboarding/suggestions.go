package onboarding

import (
	"context"
	"strconv"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Plan suggestions look at the complete periods before the current one; a
// category needs transactions in at least suggestMinMonths of them.
const (
	suggestMonths    = 3
	suggestMinMonths = 2
)

// GetPlanSuggestions proposes a plan for the current period of the budget
// (budgetId, which the caller must be able to update; else the user's default
// budget when they may plan it, otherwise none): for each of the user's own active categories, income and expense, the
// average monthly amount over the last three complete periods, rounded to the
// element currency's fraction digits. The history is every account the caller
// owns (see budget.CategoryHistory), not only the budget's members. A category with transactions in fewer
// than two of those periods, or with no positive average, gets no suggestion.
func (s *Service) GetPlanSuggestions(ctx context.Context, userID vo.Id, req model.OnboardingGetPlanSuggestionsRequest) (*model.OnboardingGetPlanSuggestionsResult, error) {
	out := &model.OnboardingGetPlanSuggestionsResult{BudgetId: strings.ToLower(strings.TrimSpace(req.BudgetId)), Items: []model.OnboardingPlanSuggestion{}}
	if out.BudgetId == "" {
		user, err := s.steps.Users.OnboardingUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		id, writable, found, err := s.steps.Budgets.DefaultBudget(ctx, userID, preferredBudget(user))
		if err != nil {
			return nil, err
		}
		// Without a budget to plan, there is nothing to suggest; a named
		// budget the caller may not plan is a 403 instead.
		if !found || !writable {
			return out, nil
		}
		out.BudgetId = id.String()
	}
	history, err := s.steps.Budgets.CategoryHistory(ctx, userID, vo.MustParseId(out.BudgetId), suggestMonths)
	if err != nil {
		return nil, err
	}
	byID := map[string]model.BudgetCategoryHistory{}
	for _, h := range history {
		byID[strings.ToLower(h.CategoryID)] = h
	}
	own, err := s.steps.Categories.OwnCategories(ctx, userID)
	if err != nil {
		return nil, err
	}
	digits := map[string]int{}
	divisor := vo.NewDecimal(strconv.Itoa(suggestMonths))
	for _, c := range own {
		h, ok := byID[strings.ToLower(c.Id)]
		if c.IsArchived == 1 || !ok || h.IsArchived {
			continue
		}
		total, active := vo.NewDecimal("0"), 0
		for _, a := range h.Amounts {
			if a != "" {
				total = total.Add(vo.NewDecimal(a))
				active++
			}
		}
		if active < suggestMinMonths {
			continue
		}
		fd, ok := digits[h.CurrencyID]
		if !ok {
			if fd, err = s.steps.Currencies.FractionDigits(ctx, h.CurrencyID); err != nil {
				return nil, err
			}
			digits[h.CurrencyID] = fd
		}
		avg := total.Div(divisor).Round(fd)
		if avg.IsZero() || avg.IsNegative() {
			continue
		}
		out.Items = append(out.Items, model.OnboardingPlanSuggestion{CategoryId: c.Id, Amount: avg.String()})
	}
	return out, nil
}
