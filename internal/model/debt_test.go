package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func TestDebtMatches(t *testing.T) {
	str := func(s string) *string { return &s }
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	from, until := day(time.March, 1), day(time.June, 1)
	whole := DebtView{CategoryID: "c"}
	withPayee := DebtView{CategoryID: "c", PayeeID: str("p")}
	withTag := DebtView{CategoryID: "c", TagID: str("t")}
	ranged := DebtView{CategoryID: "c", From: &from, Until: &until}

	tests := []struct {
		name            string
		view            DebtView
		cat, payee, tag *string
		at              time.Time
		want            bool
	}{
		{"whole category matches any payee and tag", whole, str("c"), str("x"), str("y"), day(time.April, 1), true},
		{"whole category matches nil payee and tag", whole, str("c"), nil, nil, day(time.April, 1), true},
		{"other category", whole, str("z"), nil, nil, day(time.April, 1), false},
		{"nil category", whole, nil, nil, nil, day(time.April, 1), false},
		{"payee set and equal", withPayee, str("c"), str("p"), nil, day(time.April, 1), true},
		{"payee set and different", withPayee, str("c"), str("q"), nil, day(time.April, 1), false},
		{"payee set and nil", withPayee, str("c"), nil, nil, day(time.April, 1), false},
		{"tag set and equal", withTag, str("c"), nil, str("t"), day(time.April, 1), true},
		{"tag set and different", withTag, str("c"), nil, str("u"), day(time.April, 1), false},
		{"tag set and nil", withTag, str("c"), nil, nil, day(time.April, 1), false},
		{"day before From", ranged, str("c"), nil, nil, from.AddDate(0, 0, -1), false},
		{"From is inclusive", ranged, str("c"), nil, nil, from, true},
		{"day before Until", ranged, str("c"), nil, nil, until.AddDate(0, 0, -1), true},
		{"Until is exclusive", ranged, str("c"), nil, nil, until, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DebtMatches(tc.view, tc.cat, tc.payee, tc.tag, nil, tc.at); got != tc.want {
				t.Fatalf("DebtMatches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDebtView_PlanningBounds(t *testing.T) {
	v := DebtView{FirstDay: ymd(2026, time.June, 12), LastDay: ymd(2027, time.June, 10),
		StartMonth: ymd(2026, time.June, 1), EndMonth: ymd(2027, time.June, 1)}
	if f, l := v.PlanningBounds(1); !f.Equal(v.StartMonth) || !l.Equal(v.EndMonth) {
		t.Errorf("day 1 = %v .. %v, want the calendar months", f, l)
	}
	if f, l := v.PlanningBounds(15); !f.Equal(ymd(2026, time.May, 1)) || !l.Equal(ymd(2027, time.May, 1)) {
		t.Errorf("day 15 = %v .. %v, want the periods keyed May 2026 .. May 2027", f, l)
	}
	months := DebtView{StartMonth: ymd(2026, time.March, 1), EndMonth: ymd(2026, time.July, 1)}
	if f, l := months.PlanningBounds(15); !f.Equal(months.StartMonth) || !l.Equal(months.EndMonth) {
		t.Errorf("no days = %v .. %v, want the months as given", f, l)
	}
}

func TestDebtMatches_Label(t *testing.T) {
	str := func(s string) *string { return &s }
	at := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	labelled := DebtView{CategoryID: "c", LabelID: str("car")}
	if !DebtMatches(labelled, str("c"), nil, nil, []string{"x", "car"}, at) {
		t.Error("a transaction carrying the label should match")
	}
	if DebtMatches(labelled, str("c"), nil, nil, []string{"student"}, at) {
		t.Error("a transaction without the label should not match")
	}
	if DebtMatches(labelled, str("c"), nil, nil, nil, at) {
		t.Error("a transaction with no labels should not match a labelled debt")
	}
	if !DebtMatches(DebtView{CategoryID: "c"}, str("c"), nil, nil, []string{"car"}, at) {
		t.Error("a debt without a label ignores labels")
	}
}

func TestAssignDebt_OldestWins(t *testing.T) {
	str := func(s string) *string { return &s }
	at := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	older, newer := at.AddDate(0, -2, 0), at.AddDate(0, -1, 0)
	car := DebtView{ID: "b", CategoryID: "c", LabelID: str("car"), CreatedAt: newer}
	student := DebtView{ID: "a", CategoryID: "c", LabelID: str("student"), CreatedAt: older}
	both := []string{"car", "student"}

	got, ok := AssignDebt([]DebtView{car, student}, str("c"), nil, nil, both, at)
	if !ok || got.ID != "a" {
		t.Fatalf("got %q, %v; want the older debt a", got.ID, ok)
	}
	got, ok = AssignDebt([]DebtView{car, student}, str("c"), nil, nil, []string{"car"}, at)
	if !ok || got.ID != "b" {
		t.Fatalf("got %q, %v; want the only matching debt b", got.ID, ok)
	}

	car.CreatedAt = older
	car.ID, student.ID = "a", "b"
	got, ok = AssignDebt([]DebtView{student, car}, str("c"), nil, nil, both, at)
	if !ok || got.ID != "a" {
		t.Fatalf("equal CreatedAt: got %q, %v; want the lower id a", got.ID, ok)
	}
}

func TestAssignDebt_NoMatch(t *testing.T) {
	str := func(s string) *string { return &s }
	at := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	views := []DebtView{{ID: "a", CategoryID: "c", LabelID: str("car")}}
	if _, ok := AssignDebt(views, str("c"), nil, nil, []string{"student"}, at); ok {
		t.Error("no debt matches, want false")
	}
	if _, ok := AssignDebt(nil, str("c"), nil, nil, nil, at); ok {
		t.Error("no debts, want false")
	}
}

func TestDebt_IsSetUp(t *testing.T) {
	d := &Debt{}
	if d.IsSetUp() {
		t.Error("a debt without terms is not set up")
	}
	d.SetTerms(DebtTerms{Payment: vo.NewDecimal("100"), Schedule: RecurringScheduleWeekly})
	if !d.IsSetUp() || !d.Payment.Equals(vo.NewDecimal("100")) || d.Schedule != RecurringScheduleWeekly {
		t.Errorf("SetTerms should set up the debt and copy payment and schedule: %+v", d)
	}
}
