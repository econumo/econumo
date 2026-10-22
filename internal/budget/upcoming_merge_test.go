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
			{Id: "only-up", Type: 3, Name: "Rent", Spent: "99", BudgetSpent: "98", Available: "-99", Budgeted: "5",
				Children: []model.ChildElementResult{{Id: "c9", Spent: "99", BudgetSpent: "98"}}},
		},
		Labels: []model.LabelSpendResult{
			{Id: "l1", Spent: "3", Children: []model.ChildElementResult{{Id: "c0", Spent: "1", BudgetSpent: "1"}, {Id: "c1", Spent: "2", BudgetSpent: "2"}}},
			{Id: "l3", Name: "Kid", Spent: "4"},
		},
		Savings: []model.SavingsElementResult{{Id: "s1", Spent: "100", ClosingBalance: "999"}},
	}}
	if !mergeBudgetUpcoming(&real, up) {
		t.Error("rows only the projection has must report as added")
	}

	a := real.Structure.Elements[0]
	if a.Spent != "5" || a.Upcoming != "20" || a.BudgetUpcoming != "40" || a.UpcomingBefore != "30" {
		t.Errorf("element a = %+v", a)
	}
	if c := a.Children; c[0].Upcoming != "0" || c[0].BudgetUpcoming != "0" || c[1].Upcoming != "20" || c[1].BudgetUpcoming != "40" {
		t.Errorf("children = %+v", c)
	}
	for _, e := range real.Structure.Elements[1:3] {
		if e.Upcoming != "0" || e.BudgetUpcoming != "0" || e.UpcomingBefore != "0" {
			t.Errorf("element %s/%d absent from the projection = %+v, want zeros", e.Id, e.Type, e)
		}
	}
	if len(real.Structure.Elements) != 4 {
		t.Fatalf("the projection-only element must be added: %+v", real.Structure.Elements)
	}
	if n := real.Structure.Elements[3]; n.Id != "only-up" || n.Name != "Rent" || n.Budgeted != "0" || n.Spent != "0" ||
		n.BudgetSpent != "0" || n.Available != "0" || n.Upcoming != "99" || n.BudgetUpcoming != "98" || n.UpcomingBefore != "0" ||
		len(n.Children) != 1 || n.Children[0].Spent != "0" || n.Children[0].Upcoming != "99" {
		t.Errorf("added element = %+v", n)
	}
	l := real.Structure.Labels
	if len(l) != 3 || l[0].Upcoming != "3" || l[1].Upcoming != "0" || l[2].Id != "l3" || l[2].Spent != "0" || l[2].Upcoming != "4" {
		t.Fatalf("labels = %+v", l)
	}
	if c := l[0].Children; len(c) != 2 || c[0].Id != "c0" || c[0].Spent != "0" || c[0].Upcoming != "1" || c[1].Upcoming != "2" {
		t.Errorf("label children = %+v, want the projection-only child added in id order", c)
	}
	s := real.Structure.Savings
	if s[0].Upcoming != "100" || s[0].ClosingBalance != "" || s[1].Upcoming != "0" {
		t.Errorf("savings = %+v", s)
	}
}
