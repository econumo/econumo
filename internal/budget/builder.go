// The BudgetBuilder: the heaviest read in the module. It assembles the full
// get-budget model.BudgetResult from the budget aggregate + the financial reports +
// per-element limits & spending, converting multi-currency amounts through the
// budget/element currency via the currency convertor. The work splits into six
// sub-builders: meta, filters, financial summary, element limits, element
// spending, and structure.
package budget

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// filters is the internal filter set the builder derives.
type filters struct {
	periodStart, periodEnd time.Time
	userIDs                []vo.Id
	// accountFilters are the REQUESTER's own member accounts, with their
	// removability; includedAccountIDs is every participant's member account.
	accountFilters     []model.BudgetAccountFilter
	includedAccountIDs []vo.Id
	currencyIDs        []vo.Id
	categories         map[string]model.CategoryMeta // expense-only, keyed by id
	incomeCategories   map[string]model.CategoryMeta // income-only; read by the plan builder ONLY
	tags               map[string]model.TagMeta
	labels             map[string]model.LabelMeta
}

// BuildBudget assembles the full model.BudgetResult for a budget as of periodStart
// (which the caller has already snapped to first-of-month). now is the clock
// time (controls nullable balance fields).
func (s *Service) BuildBudget(ctx context.Context, userID vo.Id, b *budgetAggregate, periodStart, now time.Time) (model.BudgetResult, error) {
	periodEnd := periodStart.AddDate(0, 1, 0)

	meta, err := s.buildMeta(ctx, b)
	if err != nil {
		return model.BudgetResult{}, err
	}
	f, err := s.buildFilters(ctx, userID, b, periodStart, periodEnd)
	if err != nil {
		return model.BudgetResult{}, err
	}
	balances, rates, err := s.buildFinancialSummary(ctx, b.budget.CurrencyID, f, now)
	if err != nil {
		return model.BudgetResult{}, err
	}
	limits, err := s.buildElementsLimits(ctx, b, f)
	if err != nil {
		return model.BudgetResult{}, err
	}
	spending, err := s.buildElementsSpending(ctx, b, f)
	if err != nil {
		return model.BudgetResult{}, err
	}
	structure, err := s.buildStructure(ctx, b, f, limits, spending)
	if err != nil {
		return model.BudgetResult{}, err
	}

	return model.BudgetResult{
		Meta: meta,
		Filters: model.FiltersResult{
			PeriodStart: f.periodStart.Format(datetime.Layout),
			PeriodEnd:   f.periodEnd.Format(datetime.Layout),
			Accounts:    f.accountFilters,
		},
		Balances:      balances,
		CurrencyRates: rates,
		Structure:     structure,
	}, nil
}

// buildMeta builds the access list plus a synthetic owner entry.
func (s *Service) buildMeta(ctx context.Context, b *budgetAggregate) (model.MetaResult, error) {
	access := make([]model.AccessResult, 0, len(b.access)+1)
	for _, a := range b.access {
		owner, err := s.users.GetOwner(ctx, a.UserID.String())
		if err != nil {
			return model.MetaResult{}, err
		}
		access = append(access, model.AccessResult{
			User:       model.UserResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name},
			Role:       a.Role.Alias(),
			IsAccepted: boolToInt(a.IsAccepted),
		})
	}
	owner, err := s.users.GetOwner(ctx, b.budget.UserID.String())
	if err != nil {
		return model.MetaResult{}, err
	}
	access = append(access, model.AccessResult{
		User:       model.UserResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name},
		Role:       "owner",
		IsAccepted: 1,
	})
	endedAt := ""
	if b.budget.EndedAt != nil {
		endedAt = b.budget.EndedAt.Format(datetime.Layout)
	}
	return model.MetaResult{
		Id:          b.budget.ID.String(),
		OwnerUserId: b.budget.UserID.String(),
		Name:        b.budget.Name,
		StartedAt:   b.budget.StartedAt.Format(datetime.Layout),
		EndedAt:     endedAt,
		CurrencyId:  b.budget.CurrencyID.String(),
		IsArchived:  boolToInt(b.budget.IsArchived),
		Access:      access,
	}, nil
}

func (s *Service) buildFilters(ctx context.Context, userID vo.Id, b *budgetAggregate, periodStart, periodEnd time.Time) (filters, error) {
	// userIds = owner + accepted non-reader access users (reader == guest).
	userIDs := []vo.Id{b.budget.UserID}
	for _, a := range b.access {
		if a.IsAccepted && a.Role != roleGuest() {
			userIDs = append(userIDs, a.UserID)
		}
	}

	// The budget counts every member account of every participant, deleted or
	// not; the filters block reports only the requester's own, with the
	// removability the membership rule allows.
	memberIDs := make([]vo.Id, 0, len(b.accounts))
	for _, m := range b.accounts {
		memberIDs = append(memberIDs, m.AccountID)
	}
	views, err := s.accounts.AccountsByIDs(ctx, memberIDs)
	if err != nil {
		return filters{}, err
	}
	included := memberIDs
	currencySet := map[string]vo.Id{}
	var currencyIDs []vo.Id
	var ownIDs []vo.Id
	for i, v := range views {
		if v.OwnerID == userID.String() {
			ownIDs = append(ownIDs, memberIDs[i])
		}
		if _, seen := currencySet[v.CurrencyID]; !seen {
			cid, cerr := vo.ParseId(v.CurrencyID)
			if cerr != nil {
				return filters{}, cerr
			}
			currencySet[v.CurrencyID] = cid
			currencyIDs = append(currencyIDs, cid)
		}
	}
	removable, err := s.removableAccounts(ctx, b, ownIDs, s.clock.Now())
	if err != nil {
		return filters{}, err
	}
	accountFilters := make([]model.BudgetAccountFilter, 0, len(ownIDs))
	for _, id := range ownIDs {
		accountFilters = append(accountFilters, model.BudgetAccountFilter{Id: id.String(), Removable: removable[id.String()]})
	}

	cats, err := s.metadata.CategoriesByOwners(ctx, userIDs)
	if err != nil {
		return filters{}, err
	}
	catMap := map[string]model.CategoryMeta{}
	incomeCatMap := map[string]model.CategoryMeta{}
	for _, c := range cats {
		if c.IsIncome {
			incomeCatMap[c.ID] = c
		} else {
			catMap[c.ID] = c
		}
	}
	tags, err := s.metadata.TagsByOwners(ctx, userIDs)
	if err != nil {
		return filters{}, err
	}
	tagMap := map[string]model.TagMeta{}
	for _, t := range tags {
		tagMap[t.ID] = t
	}

	// Labels resolve over the same owner set as tags, so a shared account's
	// spend aggregates under the ACCOUNT OWNER's labels exactly like it does
	// for tags.
	labels, err := s.read.LabelsForUsers(ctx, userIDs)
	if err != nil {
		return filters{}, err
	}

	return filters{
		periodStart: periodStart, periodEnd: periodEnd,
		userIDs: userIDs, accountFilters: accountFilters,
		includedAccountIDs: included, currencyIDs: currencyIDs,
		categories: catMap, incomeCategories: incomeCatMap, tags: tagMap, labels: labels,
	}, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func sortByPosition[T any](items []T, pos func(T) int) {
	sort.SliceStable(items, func(i, j int) bool { return pos(items[i]) < pos(items[j]) })
}

// sortByPositionThenID orders by position ascending, breaking ties by id
// ascending. Elements accumulate from Go map iteration (randomized order) and
// many share a position, so a position-only sort leaves ties in random order
// and the response varies run-to-run. The id tiebreak makes it deterministic;
// the frontend reorders when it needs a different presentation order.
func sortByPositionThenID[T any](items []T, pos func(T) int, id func(T) string) {
	sort.Slice(items, func(i, j int) bool {
		if pi, pj := pos(items[i]), pos(items[j]); pi != pj {
			return pi < pj
		}
		return id(items[i]) < id(items[j])
	})
}

// sortBySortKeyThenID is sortByPositionThenID for a list ordered by fractional
// sort key rather than a dense index. The id tiebreak matters more here: rows
// backfilled from the old integer positions can share a key.
func sortBySortKeyThenID[T any](items []T, key func(T) string, id func(T) string) {
	sort.Slice(items, func(i, j int) bool {
		if ki, kj := key(items[i]), key(items[j]); ki != kj {
			return ki < kj
		}
		return id(items[i]) < id(items[j])
	})
}
