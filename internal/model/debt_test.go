package model

import (
	"testing"
	"time"
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
			if got := DebtMatches(tc.view, tc.cat, tc.payee, tc.tag, tc.at); got != tc.want {
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
