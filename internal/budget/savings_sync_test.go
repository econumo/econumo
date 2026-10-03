package budget

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// The no-folder area holds two ordering groups, savings and everything else, so
// a keyless row is appended after its OWN group's tail: a new category must not
// be pushed past the savings rows, nor a new savings row land among categories.
func TestAssignMissingKeys_SavingsHaveTheirOwnTail(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	catKey := sortkey.Seed(sortkey.GrowsDown)
	mid, err := sortkey.Between(catKey, "")
	if err != nil {
		t.Fatal(err)
	}
	savKey, err := sortkey.Between(mid, "")
	if err != nil {
		t.Fatal(err)
	}
	budgetID := vo.NewId()
	el := func(typ model.ElementType, key sortkey.Key) *model.BudgetElement {
		e := model.NewBudgetElement(vo.NewId(), budgetID, vo.NewId(), typ, nil, nil, now)
		e.SetSortKey(key)
		return e
	}
	oldCat, oldSav := el(model.ElementCategory, catKey), el(model.ElementSavings, savKey)
	newCat, newSav := el(model.ElementCategory, ""), el(model.ElementSavings, "")
	byKey := map[string]*model.BudgetElement{}
	live := map[string]bool{}
	for _, e := range []*model.BudgetElement{oldCat, oldSav, newCat, newSav} {
		k := elementKey(e.ExternalID.String(), e.Type)
		byKey[k] = e
		live[k] = true
	}

	if err := (&Service{}).assignMissingKeys(byKey, live, func(*model.BudgetElement) {}, now); err != nil {
		t.Fatal(err)
	}
	if !(newCat.SortKey > catKey && newCat.SortKey < savKey) {
		t.Errorf("new category key %q, want it right after the category tail %q (before savings %q)", newCat.SortKey, catKey, savKey)
	}
	if newSav.SortKey <= savKey {
		t.Errorf("new savings key %q, want it after the savings tail %q", newSav.SortKey, savKey)
	}
}
