package onboarding

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateBudget creates the Day 0 budget through the budget feature's own
// create path, answers with its result, then classifies the accounts the flow
// created (newAccountIds) in the background when an AI provider is configured.
func (s *Service) CreateBudget(ctx context.Context, userID vo.Id, req model.OnboardingCreateBudgetRequest) (*model.CreateBudgetResult, error) {
	res, err := s.budgets.CreateBudget(ctx, userID, model.CreateBudgetRequest{
		Id: req.Id, Name: req.Name, StartDate: req.StartDate, CurrencyId: req.CurrencyId, AccountIds: req.AccountIds,
	})
	if err != nil {
		return nil, err
	}
	if budgetID, perr := vo.ParseId(req.Id); perr == nil {
		s.schedule(ctx, userID, budgetID, parseIDs(req.NewAccountIds))
	}
	return res, nil
}

// AddBudgetAccounts adds the accounts to the budget as everyday members (an
// existing member keeps its flag), then classifies the ones that were not
// members before in the background: a member the user already set stays as is.
func (s *Service) AddBudgetAccounts(ctx context.Context, userID vo.Id, req model.OnboardingAddBudgetAccountsRequest) (*model.OnboardingAddBudgetAccountsResult, error) {
	ids := parseIDs(req.AccountIds)
	budgetID, perr := vo.ParseId(req.BudgetId)
	var joined []vo.Id
	if perr == nil {
		members, err := s.budgets.MemberAccountIDs(ctx, userID, budgetID)
		if err != nil {
			return nil, err
		}
		was := map[string]bool{}
		for _, m := range members {
			was[m.String()] = true
		}
		for _, id := range ids {
			if !was[id.String()] {
				joined = append(joined, id)
			}
		}
	}
	out := &model.OnboardingAddBudgetAccountsResult{}
	for _, id := range ids {
		res, err := s.budgets.AddAccount(ctx, userID, model.AddAccountRequest{BudgetId: req.BudgetId, AccountId: id.String()})
		if err != nil {
			return nil, err
		}
		out.Item = res.Item
	}
	if perr == nil {
		s.schedule(ctx, userID, budgetID, joined)
	}
	return out, nil
}

// parseIDs keeps the parseable ids, deduplicated, in request order.
func parseIDs(raw []string) []vo.Id {
	seen := map[string]bool{}
	var out []vo.Id
	for _, r := range raw {
		id, err := vo.ParseId(strings.TrimSpace(r))
		if err != nil || seen[id.String()] {
			continue
		}
		seen[id.String()] = true
		out = append(out, id)
	}
	return out
}
