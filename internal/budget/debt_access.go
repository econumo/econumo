package budget

import (
	"context"
	"sort"

	"github.com/econumo/econumo/internal/model"
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
	currencies := []vo.Id{b.budget.CurrencyID}
	for _, el := range b.elements {
		if el.CurrencyID != nil && !containsID(currencies, *el.CurrencyID) {
			currencies = append(currencies, *el.CurrencyID)
		}
	}
	return model.DebtBudget{
		ID: b.budget.ID, CurrencyID: b.budget.CurrencyID, Name: b.budget.Name,
		PeriodStartDay: b.budget.PeriodStartDay, CurrencyIDs: currencies,
		IsArchived: b.budget.IsArchived, CanPlan: s.canUpdate(b, userID),
		ParticipantIDs: b.participantIDs(), AccountIDs: b.debtScope().AccountIDs,
	}
}

func containsID(ids []vo.Id, id vo.Id) bool {
	for _, x := range ids {
		if x.Equal(id) {
			return true
		}
	}
	return false
}

// DebtScope is the budget's member accounts, soft-deleted ones included, read
// with no access check: the debt sync of a budget whose rules a
// classification change touched runs for every reader alike.
func (s *Service) DebtScope(ctx context.Context, budgetID vo.Id) (model.DebtScope, error) {
	members, err := s.budgets.MemberAccounts(ctx, budgetID)
	if err != nil {
		return model.DebtScope{}, err
	}
	ids := make([]vo.Id, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.AccountID)
	}
	return model.DebtScope{BudgetID: budgetID, AccountIDs: ids}, nil
}

// DebtRuleOptions lists the budget's participants, the viewer first and the
// rest by name, each with what a debt rule can name of theirs: active expense
// categories, payees, tags and labels. The debt service has checked the
// viewer's right to plan in the budget.
func (s *Service) DebtRuleOptions(ctx context.Context, viewerID, budgetID vo.Id) (*model.DebtRuleOptionsResult, error) {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	participants := make([]model.DebtRuleParticipant, 0, len(b.access)+1)
	for _, id := range b.participantIDs() {
		p, err := s.debtRuleParticipant(ctx, id)
		if err != nil {
			return nil, err
		}
		participants = append(participants, p)
	}
	viewer := viewerID.String()
	sort.SliceStable(participants, func(i, j int) bool {
		pi, pj := participants[i], participants[j]
		if (pi.UserId == viewer) != (pj.UserId == viewer) {
			return pi.UserId == viewer
		}
		if pi.Name != pj.Name {
			return pi.Name < pj.Name
		}
		return pi.UserId < pj.UserId
	})
	return &model.DebtRuleOptionsResult{Participants: participants}, nil
}

func (s *Service) debtRuleParticipant(ctx context.Context, userID vo.Id) (model.DebtRuleParticipant, error) {
	owner, err := s.users.GetOwner(ctx, userID.String())
	if err != nil {
		return model.DebtRuleParticipant{}, err
	}
	p := model.DebtRuleParticipant{
		UserId: userID.String(), Name: owner.Name,
		Categories: []model.DebtRuleOption{}, Payees: []model.DebtRuleOption{},
		Tags: []model.DebtRuleOption{}, Labels: []model.DebtRuleOption{},
	}
	owners := []vo.Id{userID}
	cats, err := s.metadata.CategoriesByOwners(ctx, owners)
	if err != nil {
		return p, err
	}
	for _, c := range cats {
		if !c.IsIncome && !c.IsArchived {
			p.Categories = append(p.Categories, model.DebtRuleOption{Id: c.ID, Name: c.Name, Icon: c.Icon})
		}
	}
	payees, err := s.metadata.PayeesByOwners(ctx, owners)
	if err != nil {
		return p, err
	}
	for _, py := range payees {
		if !py.IsArchived {
			p.Payees = append(p.Payees, model.DebtRuleOption{Id: py.ID, Name: py.Name})
		}
	}
	tags, err := s.metadata.TagsByOwners(ctx, owners)
	if err != nil {
		return p, err
	}
	for _, t := range tags {
		if !t.IsArchived {
			p.Tags = append(p.Tags, model.DebtRuleOption{Id: t.ID, Name: t.Name, Icon: t.Icon})
		}
	}
	labels, err := s.read.LabelsForUsers(ctx, owners)
	if err != nil {
		return p, err
	}
	active := make([]model.LabelMeta, 0, len(labels))
	for _, l := range labels {
		if !l.IsArchived {
			active = append(active, l)
		}
	}
	sortBySortKeyThenID(active, func(l model.LabelMeta) string { return l.SortKey }, func(l model.LabelMeta) string { return l.ID })
	for _, l := range active {
		p.Labels = append(p.Labels, model.DebtRuleOption{Id: l.ID, Name: l.Name, Icon: l.Icon})
	}
	return p, nil
}
