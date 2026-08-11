package budget

// Covers the bucket routing in buildElementsSpending: which top-level element a
// spending row lands under, and which category it is filed under inside that
// element. The three buckets must stay a disjoint partition, and a tag must win
// over the category (and over having none), so a tagged-but-uncategorized
// expense stays under its tag instead of falling into the top-level
// uncategorized row.

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// spendingStub serves a fixed row set from CountSpending; the rest of ReadModel
// is unused by buildElementsSpending.
type spendingStub struct{ rows []model.SpendingRow }

func (s *spendingStub) CountSpending(context.Context, []vo.Id, []vo.Id, time.Time, time.Time) ([]model.SpendingRow, error) {
	return s.rows, nil
}

func (s *spendingStub) AccountsBalancesOnDate(context.Context, []vo.Id, time.Time) ([]model.AccountBalanceRow, error) {
	return nil, nil
}

func (s *spendingStub) AccountsBalancesBeforeDate(context.Context, []vo.Id, time.Time) ([]model.AccountBalanceRow, error) {
	return nil, nil
}

func (s *spendingStub) AccountsReport(context.Context, []vo.Id, time.Time, time.Time) ([]model.AccountReportRow, error) {
	return nil, nil
}

func (s *spendingStub) HoldingsReport(context.Context, []vo.Id, time.Time, time.Time) ([]model.HoldingsRow, error) {
	return nil, nil
}

func (s *spendingStub) SummarizedLimits(context.Context, vo.Id, time.Time, time.Time) ([]model.SummarizedLimitRow, error) {
	return nil, nil
}

func (s *spendingStub) SpendingByMonth(context.Context, []vo.Id, []vo.Id, time.Time, time.Time) ([]model.MonthlySpendingRow, error) {
	return nil, nil
}

func (s *spendingStub) IncomeByMonth(context.Context, []vo.Id, time.Time, time.Time) ([]model.MonthlyIncomeRow, error) {
	return nil, nil
}

func (s *spendingStub) LimitsByMonth(context.Context, vo.Id, time.Time, time.Time) ([]model.MonthlyLimitRow, error) {
	return nil, nil
}

func (s *spendingStub) BudgetTransactionsByCategories(context.Context, []vo.Id, []vo.Id, time.Time, time.Time) ([]model.BudgetTransactionRow, error) {
	return nil, nil
}

func (s *spendingStub) BudgetTransactionsByTag(context.Context, vo.Id, *vo.Id, bool, []vo.Id, time.Time, time.Time) ([]model.BudgetTransactionRow, error) {
	return nil, nil
}

func (s *spendingStub) BudgetTransactionsUncategorized(context.Context, []vo.Id, time.Time, time.Time) ([]model.BudgetTransactionRow, error) {
	return nil, nil
}

func (s *spendingStub) BudgetTransactionsByLabel(context.Context, vo.Id, []vo.Id, time.Time, time.Time) ([]model.BudgetTransactionRow, error) {
	return nil, nil
}

func (s *spendingStub) BudgetTransactionsByLabelAndCategory(context.Context, vo.Id, vo.Id, []vo.Id, time.Time, time.Time) ([]model.BudgetTransactionRow, error) {
	return nil, nil
}

func (s *spendingStub) BudgetTransactionsByLabelUncategorized(context.Context, vo.Id, []vo.Id, time.Time, time.Time) ([]model.BudgetTransactionRow, error) {
	return nil, nil
}

func (s *spendingStub) CountSpendingByLabel(context.Context, []vo.Id, time.Time, time.Time) ([]model.LabelSpendingRow, error) {
	return nil, nil
}

func (s *spendingStub) LabelsForUsers(context.Context, []vo.Id) (map[string]model.LabelMeta, error) {
	return nil, nil
}

func strptr(s string) *string { return &s }

func TestBuildElementsSpending_RoutesTheThreeBuckets(t *testing.T) {
	const (
		catID  = "c0000000-0000-0000-0000-0000000000c1"
		tagID  = "7a000000-0000-0000-0000-0000000000d1"
		currID = "1a000000-0000-0000-0000-0000000000e1"
		acctID = "a0000000-0000-0000-0000-0000000000f1"
	)
	// One row of every shape the widened CountSpending can now return.
	stub := &spendingStub{rows: []model.SpendingRow{
		{CategoryID: strptr(catID), TagID: strptr(tagID), CurrencyID: currID, Amount: "1.00"},
		{CategoryID: nil, TagID: strptr(tagID), CurrencyID: currID, Amount: "2.00"},
		{CategoryID: strptr(catID), TagID: nil, CurrencyID: currID, Amount: "4.00"},
		{CategoryID: nil, TagID: nil, CurrencyID: currID, Amount: "8.00"},
	}}
	s := &Service{read: stub}

	periodStart := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	// StartedAt == periodStart, so the month-by-month "before" walk is empty and
	// every row below comes from the single current-period call.
	b := &budgetAggregate{budget: &model.Budget{StartedAt: periodStart}}
	f := filters{
		periodStart:        periodStart,
		periodEnd:          periodStart.AddDate(0, 1, 0),
		includedAccountIDs: []vo.Id{vo.MustParseId(acctID)},
		categories:         map[string]model.CategoryMeta{catID: {ID: catID}},
	}

	data, err := s.buildElementsSpending(context.Background(), b, f)
	if err != nil {
		t.Fatalf("buildElementsSpending: %v", err)
	}

	// element key -> category sub-key -> amount. The two tagged rows share the
	// tag bucket and split by category inside it: that split is what later
	// renders as an "Uncategorized" child under the tag.
	want := map[string]map[string]string{
		tagID + "-tag": {
			catID:                 "1.00",
			model.UncategorizedID: "2.00",
		},
		catID + "-category": {
			catID: "4.00",
		},
		model.UncategorizedID + "-category": {
			model.UncategorizedID: "8.00",
		},
	}

	if got := keysOf(data); !equalKeys(got, keysOfWant(want)) {
		t.Fatalf("top-level element keys = %v, want %v", got, keysOfWant(want))
	}
	for key, wantCats := range want {
		es := data[key]
		if es == nil {
			t.Fatalf("missing element bucket %q; got %v", key, keysOf(data))
		}
		var gotCats []string
		for catKey := range es.spendingInCategories {
			gotCats = append(gotCats, catKey)
		}
		if !equalKeys(sorted(gotCats), sorted(mapKeys(wantCats))) {
			t.Errorf("element %q spendingInCategories keys = %v, want %v", key, sorted(gotCats), sorted(mapKeys(wantCats)))
			continue
		}
		for catKey, wantAmount := range wantCats {
			cs := es.spendingInCategories[catKey]
			if cs.categoryID != catKey {
				t.Errorf("element %q sub-key %q has categoryID %q, want %q", key, catKey, cs.categoryID, catKey)
			}
			if len(cs.currenciesSpent) != 1 {
				t.Fatalf("element %q sub-key %q: want 1 current amount, got %d", key, catKey, len(cs.currenciesSpent))
			}
			if got := cs.currenciesSpent[0].amount.String(); got != vo.NewDecimal(wantAmount).String() {
				t.Errorf("element %q sub-key %q amount = %s, want %s", key, catKey, got, wantAmount)
			}
			if len(cs.currenciesSpentBefore) != 0 {
				t.Errorf("element %q sub-key %q: want no before-amounts, got %d", key, catKey, len(cs.currenciesSpentBefore))
			}
		}
	}
}

func keysOf(data map[string]*elementSpending) []string {
	out := make([]string, 0, len(data))
	for k := range data {
		out = append(out, k)
	}
	return sorted(out)
}

func keysOfWant(want map[string]map[string]string) []string {
	out := make([]string, 0, len(want))
	for k := range want {
		out = append(out, k)
	}
	return sorted(out)
}

func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func sorted(in []string) []string {
	sort.Strings(in)
	return in
}

func equalKeys(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }
