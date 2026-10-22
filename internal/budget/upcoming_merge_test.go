package budget

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
)

func TestMergeBudgetUpcoming(t *testing.T) {
	real := model.BudgetResult{Structure: model.StructureResult{
		Elements: []model.ParentElementResult{
			{Id: "a", Type: 1, Spent: "5", Children: []model.ChildElementResult{{Id: "c1"}, {Id: "c2"}}},
			{Id: "a", Type: 2, Spent: "7"},
			{Id: "b", Type: 1},
		},
		Labels:  []model.LabelSpendResult{{Id: "l1", Children: []model.ChildElementResult{{Id: "c1"}}}, {Id: "l2"}},
		Savings: []model.SavingsElementResult{{Id: "s1"}, {Id: "s2"}},
	}}
	up := model.BudgetResult{Structure: model.StructureResult{
		Elements: []model.ParentElementResult{
			// Available = -(spentBefore 30 + spent 20)
			{Id: "a", Type: 1, Spent: "20", BudgetSpent: "40", Available: "-50",
				Children: []model.ChildElementResult{{Id: "c2", Spent: "20", BudgetSpent: "40"}}},
			{Id: "only-up", Type: 1, Spent: "99"},
		},
		Labels:  []model.LabelSpendResult{{Id: "l1", Spent: "3", Children: []model.ChildElementResult{{Id: "c1", Spent: "3", BudgetSpent: "3"}}}},
		Savings: []model.SavingsElementResult{{Id: "s1", Spent: "100", ClosingBalance: "999"}},
	}}
	mergeBudgetUpcoming(&real, up)

	a := real.Structure.Elements[0]
	if a.Spent != "5" || a.Upcoming != "20" || a.BudgetUpcoming != "40" || a.UpcomingBefore != "30" {
		t.Errorf("element a = %+v", a)
	}
	if c := a.Children; c[0].Upcoming != "0" || c[0].BudgetUpcoming != "0" || c[1].Upcoming != "20" || c[1].BudgetUpcoming != "40" {
		t.Errorf("children = %+v", c)
	}
	for _, e := range real.Structure.Elements[1:] {
		if e.Upcoming != "0" || e.BudgetUpcoming != "0" || e.UpcomingBefore != "0" {
			t.Errorf("element %s/%d absent from the projection = %+v, want zeros", e.Id, e.Type, e)
		}
	}
	if len(real.Structure.Elements) != 3 {
		t.Errorf("an element only the projection has must not be added: %+v", real.Structure.Elements)
	}
	l := real.Structure.Labels
	if l[0].Upcoming != "3" || l[0].Children[0].Upcoming != "3" || l[1].Upcoming != "0" {
		t.Errorf("labels = %+v", l)
	}
	s := real.Structure.Savings
	if s[0].Upcoming != "100" || s[0].ClosingBalance != "" || s[1].Upcoming != "0" {
		t.Errorf("savings = %+v", s)
	}
}
