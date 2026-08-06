package budget

// Covers the labels block's second level: a reporting label's period spend
// broken down by the categories it landed in. A label's children must sum to
// the label's own total, even though totals ACROSS labels deliberately overlap.

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// labelSpendingStub serves a fixed row set from CountSpendingByLabel; the rest
// of ReadModel is unused by buildStructure in these tests.
type labelSpendingStub struct {
	spendingStub
	labelRows []model.LabelSpendingRow
}

func (s *labelSpendingStub) CountSpendingByLabel(context.Context, []vo.Id, time.Time, time.Time) ([]model.LabelSpendingRow, error) {
	return s.labelRows, nil
}

// sumConvertor stands in for the currency convertor: every key's items are
// summed as-is (the tests use a single currency, so no rate is involved). This
// keeps the assertions about WHICH keys the builder registers, which is the
// part the rollup owns.
type sumConvertor struct{}

func (sumConvertor) BulkConvert(_ context.Context, _, _ time.Time, items map[string][]model.ConvertItem) (map[string]vo.DecimalNumber, error) {
	out := map[string]vo.DecimalNumber{}
	for key, list := range items {
		total := vo.NewDecimal("0")
		for _, it := range list {
			total = total.Add(it.Amount)
		}
		out[key] = total
	}
	return out, nil
}

// noEnvelopes satisfies the EnvelopeStore calls buildStructure makes; the label
// tests declare no envelopes, so only the unused methods need to exist.
type noEnvelopes struct{ EnvelopeStore }

func (noEnvelopes) EnvelopeCategoryIDs(context.Context, vo.Id) ([]vo.Id, error) { return nil, nil }

const (
	lblA     = "eeee0000-0000-7000-8000-000000000001"
	lblB     = "eeee0000-0000-7000-8000-000000000002"
	lblCatX  = "cccc0000-0000-7000-8000-000000000001"
	lblCatY  = "cccc0000-0000-7000-8000-000000000002"
	lblCurr  = "1a000000-0000-0000-0000-0000000000e1"
	lblOwner = "0000ffff-0000-7000-8000-000000000001"
)

// labelFixture builds the buildStructure inputs shared by the tests below: two
// labels and two categories, with the caller supplying the label spending rows.
func labelFixture(t *testing.T, rows []model.LabelSpendingRow) ([]model.LabelSpendResult, error) {
	t.Helper()
	s := &Service{
		read:      &labelSpendingStub{labelRows: rows},
		convertor: sumConvertor{},
		envelopes: noEnvelopes{},
	}
	periodStart := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	currencyID := vo.MustParseId(lblCurr)
	b := &budgetAggregate{budget: &model.Budget{StartedAt: periodStart, CurrencyID: currencyID}}
	f := filters{
		periodStart: periodStart,
		periodEnd:   periodStart.AddDate(0, 1, 0),
		categories: map[string]model.CategoryMeta{
			lblCatX: {ID: lblCatX, OwnerID: lblOwner, Name: "Groceries", Icon: "cart"},
			lblCatY: {ID: lblCatY, OwnerID: lblOwner, Name: "Toys", Icon: "toy", IsArchived: true},
		},
		labels: map[string]model.LabelMeta{
			lblA: {ID: lblA, OwnerID: lblOwner, Name: "Kid A", Icon: "face", SortKey: "a"},
			lblB: {ID: lblB, OwnerID: lblOwner, Name: "Kid B", Icon: "face", SortKey: "b"},
		},
	}
	res, err := s.buildStructure(context.Background(), b, f, map[string]budgetedAmount{}, map[string]*elementSpending{})
	if err != nil {
		return nil, err
	}
	return res.Labels, nil
}

func findLabelResult(labels []model.LabelSpendResult, id string) (model.LabelSpendResult, bool) {
	for _, l := range labels {
		if l.Id == id {
			return l, true
		}
	}
	return model.LabelSpendResult{}, false
}

// TestLabelChildrenSumToTheLabelTotal is the headline invariant: a label's
// category children partition ITS OWN total exactly, while the same expense
// counted under a second label leaves that second label's total untouched.
func TestLabelChildrenSumToTheLabelTotal(t *testing.T) {
	labels, err := labelFixture(t, []model.LabelSpendingRow{
		{LabelID: lblA, CategoryID: strptr(lblCatX), CurrencyID: lblCurr, Amount: "30.00"},
		{LabelID: lblA, CategoryID: strptr(lblCatY), CurrencyID: lblCurr, Amount: "12.00"},
		// Same spend also carried by label B: overlap across labels is the feature.
		{LabelID: lblB, CategoryID: strptr(lblCatX), CurrencyID: lblCurr, Amount: "30.00"},
	})
	if err != nil {
		t.Fatalf("buildStructure: %v", err)
	}

	a, ok := findLabelResult(labels, lblA)
	if !ok {
		t.Fatalf("label A missing from labels block: %+v", labels)
	}
	if a.Spent != "42" {
		t.Errorf("label A spent = %q, want 42", a.Spent)
	}
	if len(a.Children) != 2 {
		t.Fatalf("label A children = %d, want 2: %+v", len(a.Children), a.Children)
	}
	// Children come out id-ordered, so lblCatX (…001) precedes lblCatY (…002).
	if a.Children[0].Id != lblCatX || a.Children[1].Id != lblCatY {
		t.Fatalf("label A children ids = %q,%q want %q,%q", a.Children[0].Id, a.Children[1].Id, lblCatX, lblCatY)
	}
	sum := vo.NewDecimal("0")
	for _, ch := range a.Children {
		sum = sum.Add(vo.NewDecimal(ch.Spent))
	}
	if sum.String() != a.Spent {
		t.Errorf("label A children sum to %s, want the label total %s", sum.String(), a.Spent)
	}
	if got := a.Children[0]; got.Spent != "30" || got.Name != "Groceries" || got.Icon != "cart" ||
		got.OwnerUserId != lblOwner || got.IsArchived != 0 || got.Type != int(model.ElementCategory.Int16()) {
		t.Errorf("label A first child = %+v, want the Groceries category metadata with spent 30", got)
	}
	// An archived category still renders as a child when it carries spend.
	if got := a.Children[1]; got.Spent != "12" || got.IsArchived != 1 {
		t.Errorf("label A second child = %+v, want archived Toys with spent 12", got)
	}
	if got := a.Children[0].BudgetSpent; got != "30" {
		t.Errorf("label A first child budgetSpent = %q, want 30", got)
	}

	bLbl, ok := findLabelResult(labels, lblB)
	if !ok {
		t.Fatalf("label B missing from labels block: %+v", labels)
	}
	if bLbl.Spent != "30" {
		t.Errorf("label B spent = %q, want 30 (unaffected by label A's rows)", bLbl.Spent)
	}
	if len(bLbl.Children) != 1 || bLbl.Children[0].Id != lblCatX {
		t.Fatalf("label B children = %+v, want one child for %s", bLbl.Children, lblCatX)
	}
}

// TestLabelUncategorizedChild: a nil CategoryID becomes ONE child carrying the
// shared uncategorized id/name/icon, not a dropped row.
func TestLabelUncategorizedChild(t *testing.T) {
	labels, err := labelFixture(t, []model.LabelSpendingRow{
		{LabelID: lblA, CategoryID: nil, CurrencyID: lblCurr, Amount: "5.00"},
		{LabelID: lblA, CategoryID: nil, CurrencyID: lblCurr, Amount: "3.00"},
	})
	if err != nil {
		t.Fatalf("buildStructure: %v", err)
	}
	a, ok := findLabelResult(labels, lblA)
	if !ok {
		t.Fatalf("label A missing from labels block: %+v", labels)
	}
	if a.Spent != "8" {
		t.Errorf("label A spent = %q, want 8", a.Spent)
	}
	if len(a.Children) != 1 {
		t.Fatalf("uncategorized rows must collapse into one child, got %+v", a.Children)
	}
	ch := a.Children[0]
	if ch.Id != model.UncategorizedID || ch.Name != model.UncategorizedName || ch.Icon != model.UncategorizedIcon {
		t.Errorf("uncategorized child = %+v, want the shared uncategorized id/name/icon", ch)
	}
	if ch.Spent != "8" {
		t.Errorf("uncategorized child spent = %q, want 8 (both rows)", ch.Spent)
	}
}

// TestLabelZeroSpendHidesLabelAndZeroChild: a label whose rows net to zero is
// absent entirely (it has no limit that could keep it visible), and inside a
// visible label a category whose own spend nets to zero is omitted.
func TestLabelZeroSpendHidesLabelAndZeroChild(t *testing.T) {
	labels, err := labelFixture(t, []model.LabelSpendingRow{
		// Label A: catX nets to zero, catY carries the whole total.
		{LabelID: lblA, CategoryID: strptr(lblCatX), CurrencyID: lblCurr, Amount: "10.00"},
		{LabelID: lblA, CategoryID: strptr(lblCatX), CurrencyID: lblCurr, Amount: "-10.00"},
		{LabelID: lblA, CategoryID: strptr(lblCatY), CurrencyID: lblCurr, Amount: "7.00"},
		// Label B nets to zero overall.
		{LabelID: lblB, CategoryID: strptr(lblCatX), CurrencyID: lblCurr, Amount: "4.00"},
		{LabelID: lblB, CategoryID: strptr(lblCatX), CurrencyID: lblCurr, Amount: "-4.00"},
	})
	if err != nil {
		t.Fatalf("buildStructure: %v", err)
	}
	if _, ok := findLabelResult(labels, lblB); ok {
		t.Errorf("a label with zero net spend must be absent: %+v", labels)
	}
	a, ok := findLabelResult(labels, lblA)
	if !ok {
		t.Fatalf("label A missing from labels block: %+v", labels)
	}
	if len(a.Children) != 1 || a.Children[0].Id != lblCatY {
		t.Fatalf("label A children = %+v, want only %s (the zero-spend category is omitted)", a.Children, lblCatY)
	}
}

// TestLabelChildrenNeverNil: the block's children field is a list on the wire
// even when a label's every category child was filtered out, so a nil slice
// (which marshals to null) must never reach the response.
func TestLabelChildrenNeverNil(t *testing.T) {
	labels, err := labelFixture(t, []model.LabelSpendingRow{
		// Non-zero label total whose only category nets to zero, leaving no
		// children behind: the one shape where a nil slice could slip through.
		{LabelID: lblA, CategoryID: strptr(lblCatX), CurrencyID: lblCurr, Amount: "10.00"},
		{LabelID: lblA, CategoryID: strptr(lblCatX), CurrencyID: lblCurr, Amount: "-10.00"},
		{LabelID: lblA, CategoryID: nil, CurrencyID: lblCurr, Amount: "0.00"},
		{LabelID: lblA, CategoryID: strptr(lblCatY), CurrencyID: lblCurr, Amount: "0.00"},
	})
	if err != nil {
		t.Fatalf("buildStructure: %v", err)
	}
	if _, ok := findLabelResult(labels, lblA); ok {
		t.Fatalf("precondition: this fixture nets to zero so label A should be hidden: %+v", labels)
	}

	labels, err = labelFixture(t, []model.LabelSpendingRow{
		{LabelID: lblA, CategoryID: strptr(lblCatX), CurrencyID: lblCurr, Amount: "9.00"},
	})
	if err != nil {
		t.Fatalf("buildStructure: %v", err)
	}
	a, ok := findLabelResult(labels, lblA)
	if !ok {
		t.Fatalf("label A missing: %+v", labels)
	}
	if a.Children == nil {
		t.Fatal("label children must be a non-nil slice so the wire carries [] not null")
	}
}

// TestLabelChildForUnknownCategoryIsDropped: spend can reference a category the
// filters do not resolve (e.g. an income category, or one belonging to a
// since-revoked collaborator). Such a child cannot be rendered, so it is
// dropped — but the label's own total still includes it, matching the total the
// repo reports.
func TestLabelChildForUnknownCategoryIsDropped(t *testing.T) {
	const unknownCat = "cccc0000-0000-7000-8000-00000000dead"
	labels, err := labelFixture(t, []model.LabelSpendingRow{
		{LabelID: lblA, CategoryID: strptr(lblCatX), CurrencyID: lblCurr, Amount: "6.00"},
		{LabelID: lblA, CategoryID: strptr(unknownCat), CurrencyID: lblCurr, Amount: "4.00"},
	})
	if err != nil {
		t.Fatalf("buildStructure: %v", err)
	}
	a, ok := findLabelResult(labels, lblA)
	if !ok {
		t.Fatalf("label A missing: %+v", labels)
	}
	if a.Spent != "10" {
		t.Errorf("label A spent = %q, want 10 (the unresolvable category still counts toward the total)", a.Spent)
	}
	if len(a.Children) != 1 || a.Children[0].Id != lblCatX {
		t.Fatalf("label A children = %+v, want only the resolvable category %s", a.Children, lblCatX)
	}
}
