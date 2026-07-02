// The BudgetBuilder: the heaviest read in the module. It assembles the full
// get-budget BudgetResult from the budget aggregate + the financial reports +
// per-element limits & spending, converting multi-currency amounts through the
// budget/element currency via the currency convertor. The work splits into six
// sub-builders: meta, filters, financial summary, element limits, element
// spending, and structure.
package budget

import (
	"context"
	"sort"
	"time"

	domcurrency "github.com/econumo/econumo/internal/domain/currency"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CategoryMeta is a category's display metadata for the structure.
type CategoryMeta struct {
	ID         string
	OwnerID    string
	Name       string
	Icon       string
	IsIncome   bool
	IsArchived bool
}

// TagMeta is a tag's display metadata.
type TagMeta struct {
	ID         string
	OwnerID    string
	Name       string
	IsArchived bool
}

// PayeeMeta is a payee's display metadata (for the transaction list).
type PayeeMeta struct {
	ID   string
	Name string
}

// MetadataLookup resolves the categories + tags (+ payees) owned by a set of
// users (the budget participants), for the structure builder and the
// transaction list. Categories include both income and expense; the builder
// filters.
type MetadataLookup interface {
	CategoriesByOwners(ctx context.Context, userIDs []vo.Id) ([]CategoryMeta, error)
	TagsByOwners(ctx context.Context, userIDs []vo.Id) ([]TagMeta, error)
	PayeesByOwners(ctx context.Context, userIDs []vo.Id) ([]PayeeMeta, error)
}

// Convertor is the currency convertor port (domain currency.Convertor satisfies it).
type Convertor interface {
	BulkConvert(ctx context.Context, periodStart, periodEnd time.Time, items map[string][]domcurrency.ConvertItem) (map[string]vo.DecimalNumber, error)
}

// AverageRateLookup resolves period-average rates for the financial summary's
// currencyRates block (domain currency.RateProvider satisfies AverageRates).
type AverageRateLookup interface {
	AverageRates(ctx context.Context, start, end time.Time) ([]domcurrency.FullRate, error)
	// SnappedRatePeriod returns the [start,end) AverageRates actually used (the
	// latest-rate month <= end, or the requested period when no rate exists).
	// The currencyRates block reports THIS period.
	SnappedRatePeriod(ctx context.Context, start, end time.Time) (time.Time, time.Time, error)
	BaseCurrencyID(ctx context.Context) (vo.Id, error)
}

// filters is the internal filter set the builder derives.
type filters struct {
	periodStart, periodEnd time.Time
	userIDs                []vo.Id
	excludedAccountIDs     []vo.Id
	includedAccountIDs     []vo.Id
	currencyIDs            []vo.Id
	categories             map[string]CategoryMeta // expense-only, keyed by id
	tags                   map[string]TagMeta
}

// BuildBudget assembles the full BudgetResult for a budget as of periodStart
// (which the caller has already snapped to first-of-month). now is the clock
// time (controls nullable balance fields).
func (s *Service) BuildBudget(ctx context.Context, userID vo.Id, b *budgetAggregate, periodStart, now time.Time) (BudgetResult, error) {
	periodEnd := periodStart.AddDate(0, 1, 0)

	meta, err := s.buildMeta(ctx, b)
	if err != nil {
		return BudgetResult{}, err
	}
	f, err := s.buildFilters(ctx, userID, b, periodStart, periodEnd)
	if err != nil {
		return BudgetResult{}, err
	}
	balances, rates, err := s.buildFinancialSummary(ctx, b.budget.CurrencyId(), f, now)
	if err != nil {
		return BudgetResult{}, err
	}
	limits, err := s.buildElementsLimits(ctx, b, f)
	if err != nil {
		return BudgetResult{}, err
	}
	spending, err := s.buildElementsSpending(ctx, b, f)
	if err != nil {
		return BudgetResult{}, err
	}
	structure, err := s.buildStructure(ctx, b, f, limits, spending)
	if err != nil {
		return BudgetResult{}, err
	}

	return BudgetResult{
		Meta: meta,
		Filters: FiltersResult{
			PeriodStart:         f.periodStart.Format(datetime.Layout),
			PeriodEnd:           f.periodEnd.Format(datetime.Layout),
			ExcludedAccountsIds: idStrings(f.excludedAccountIDs),
		},
		Balances:      balances,
		CurrencyRates: rates,
		Structure:     structure,
	}, nil
}

// buildMeta builds the access list plus a synthetic owner entry.
func (s *Service) buildMeta(ctx context.Context, b *budgetAggregate) (MetaResult, error) {
	access := make([]AccessResult, 0, len(b.access)+1)
	for _, a := range b.access {
		owner, err := s.users.GetOwner(ctx, a.UserId().String())
		if err != nil {
			return MetaResult{}, err
		}
		access = append(access, AccessResult{
			User:       UserResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name},
			Role:       a.Role().Alias(),
			IsAccepted: boolToInt(a.IsAccepted()),
		})
	}
	owner, err := s.users.GetOwner(ctx, b.budget.UserId().String())
	if err != nil {
		return MetaResult{}, err
	}
	access = append(access, AccessResult{
		User:       UserResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name},
		Role:       "owner",
		IsAccepted: 1,
	})
	return MetaResult{
		Id:          b.budget.Id().String(),
		OwnerUserId: b.budget.UserId().String(),
		Name:        b.budget.Name(),
		StartedAt:   b.budget.StartedAt().Format(datetime.Layout),
		CurrencyId:  b.budget.CurrencyId().String(),
		Access:      access,
	}, nil
}

func (s *Service) buildFilters(ctx context.Context, userID vo.Id, b *budgetAggregate, periodStart, periodEnd time.Time) (filters, error) {
	// userIds = owner + accepted non-reader access users (reader == guest).
	userIDs := []vo.Id{b.budget.UserId()}
	for _, a := range b.access {
		if a.IsAccepted() && a.Role() != roleGuest() {
			userIDs = append(userIDs, a.UserId())
		}
	}

	// excludedAccountIds for THIS user (meta filter shows the requester's set).
	excludedForUser := make([]vo.Id, 0)
	for _, aid := range b.excludedAccountIDs {
		ownerID, ok := s.accountOwners[aid.String()]
		if !ok {
			o, err := s.accounts.AccountOwner(ctx, aid)
			if err == nil {
				ownerID = o.String()
				s.accountOwners[aid.String()] = ownerID
			}
		}
		if ownerID == userID.String() {
			excludedForUser = append(excludedForUser, aid)
		}
	}

	// included = all participant accounts minus ALL excluded.
	excludedSet := map[string]bool{}
	for _, aid := range b.excludedAccountIDs {
		excludedSet[aid.String()] = true
	}
	var included []vo.Id
	currencySet := map[string]vo.Id{}
	var currencyIDs []vo.Id
	accounts, err := s.accounts.AccountsForOwners(ctx, userIDs)
	if err != nil {
		return filters{}, err
	}
	for _, a := range accounts {
		if excludedSet[a.ID] {
			continue
		}
		aid, perr := vo.ParseId(a.ID)
		if perr != nil {
			return filters{}, perr
		}
		included = append(included, aid)
		if _, seen := currencySet[a.CurrencyID]; !seen {
			cid, cerr := vo.ParseId(a.CurrencyID)
			if cerr != nil {
				return filters{}, cerr
			}
			currencySet[a.CurrencyID] = cid
			currencyIDs = append(currencyIDs, cid)
		}
	}

	cats, err := s.metadata.CategoriesByOwners(ctx, userIDs)
	if err != nil {
		return filters{}, err
	}
	catMap := map[string]CategoryMeta{}
	for _, c := range cats {
		if !c.IsIncome { // expense categories only
			catMap[c.ID] = c
		}
	}
	tags, err := s.metadata.TagsByOwners(ctx, userIDs)
	if err != nil {
		return filters{}, err
	}
	tagMap := map[string]TagMeta{}
	for _, t := range tags {
		tagMap[t.ID] = t
	}

	return filters{
		periodStart: periodStart, periodEnd: periodEnd,
		userIDs: userIDs, excludedAccountIDs: excludedForUser,
		includedAccountIDs: included, currencyIDs: currencyIDs,
		categories: catMap, tags: tagMap,
	}, nil
}

func idStrings(ids []vo.Id) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
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
