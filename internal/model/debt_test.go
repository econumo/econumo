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
	open := DebtView{CategoryIDs: []string{"c", "d"}}
	ranged := DebtView{CategoryIDs: []string{"c"}, From: &from, Until: &until}

	tests := []struct {
		name     string
		view     DebtView
		cat, tag *string
		at       time.Time
		want     bool
	}{
		{"a category of the element", open, str("c"), nil, day(time.April, 1), true},
		{"another child of the element", open, str("d"), nil, day(time.April, 1), true},
		{"an empty tag is no tag", open, str("c"), str(""), day(time.April, 1), true},
		{"a tagged expense counts on its tag", open, str("c"), str("t"), day(time.April, 1), false},
		{"other category", open, str("z"), nil, day(time.April, 1), false},
		{"nil category", open, nil, nil, day(time.April, 1), false},
		{"day before From", ranged, str("c"), nil, from.AddDate(0, 0, -1), false},
		{"From is inclusive", ranged, str("c"), nil, from, true},
		{"Until is inclusive", ranged, str("c"), nil, until.Add(20 * time.Hour), true},
		{"day after Until", ranged, str("c"), nil, until.AddDate(0, 0, 1), false},
		{"no categories match nothing", DebtView{}, str("c"), nil, day(time.April, 1), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DebtMatches(tc.view, tc.cat, tc.tag, tc.at); got != tc.want {
				t.Fatalf("DebtMatches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAssignDebt(t *testing.T) {
	str := func(s string) *string { return &s }
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	end, start := day(time.February, 1), day(time.February, 2)
	old := DebtView{ID: "old", CategoryIDs: []string{"c"}, Until: &end}
	next := DebtView{ID: "next", CategoryIDs: []string{"c"}, From: &start}
	views := []DebtView{old, next}
	if got, ok := AssignDebt(views, str("c"), nil, day(time.February, 1)); !ok || got.ID != "old" {
		t.Fatalf("end day = %q, %v; want the old debt", got.ID, ok)
	}
	if got, ok := AssignDebt(views, str("c"), nil, day(time.February, 2)); !ok || got.ID != "next" {
		t.Fatalf("next day = %q, %v; want the next debt", got.ID, ok)
	}
	if _, ok := AssignDebt(views, str("x"), nil, day(time.March, 1)); ok {
		t.Fatal("another category matched")
	}
	if _, ok := AssignDebt(nil, str("c"), nil, day(time.March, 1)); ok {
		t.Fatal("no debts, want false")
	}
}

// A payment inside a range of the category's own debt is that debt's, even
// when the envelope's debt covering it is older and listed first.
func TestAssignDebt_TheCategorysOwnDebtWins(t *testing.T) {
	str := func(s string) *string { return &s }
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	from, until := day(time.March, 1), day(time.May, 31)
	envelope := DebtView{ID: "envelope", ElementType: DebtElementEnvelope, CategoryIDs: []string{"c", "d"}}
	own := DebtView{ID: "own", ElementType: DebtElementCategory, ElementID: "c", CategoryIDs: []string{"c"}, From: &from, Until: &until}
	views := []DebtView{envelope, own}
	tests := []struct {
		name string
		cat  string
		at   time.Time
		want string
	}{
		{"before the own debt", "c", day(time.February, 28), "envelope"},
		{"its first day", "c", from, "own"},
		{"its last day", "c", until, "own"},
		{"after its end", "c", day(time.June, 1), "envelope"},
		{"another child of the envelope", "d", day(time.April, 1), "envelope"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := AssignDebt(views, str(tc.cat), nil, tc.at)
			if !ok || got.ID != tc.want {
				t.Fatalf("AssignDebt = %q, %v; want %q", got.ID, ok, tc.want)
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

func TestDebt_IsSetUpAndPaidOff(t *testing.T) {
	d := &Debt{Status: DebtActive}
	if d.IsSetUp() || d.IsPaidOff() {
		t.Error("a new debt is neither set up nor paid off")
	}
	d.SetTerms(DebtTerms{Payment: vo.NewDecimal("100"), Schedule: RecurringScheduleWeekly})
	if !d.IsSetUp() || !d.Payment.Equals(vo.NewDecimal("100")) || d.Schedule != RecurringScheduleWeekly {
		t.Errorf("SetTerms should set up the debt and copy payment and schedule: %+v", d)
	}
	d.Status = DebtPaidOff
	if !d.IsPaidOff() {
		t.Error("a paid-off status reads paid off")
	}
	if !DebtElementCategory.Valid() || !DebtElementEnvelope.Valid() || DebtElementType("tag").Valid() {
		t.Error("only categories and envelopes carry debts")
	}
}
