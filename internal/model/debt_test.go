package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestDebtMatches(t *testing.T) {
	str := func(s string) *string { return &s }
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	from, until := day(time.March, 1), day(time.June, 1)
	whole := DebtView{Rules: []DebtRuleView{{CategoryID: "c"}}}
	withPayee := DebtView{Rules: []DebtRuleView{{CategoryID: "c", PayeeID: str("p")}}}
	withTag := DebtView{Rules: []DebtRuleView{{CategoryID: "c", TagID: str("t")}}}
	ranged := DebtView{Rules: []DebtRuleView{{CategoryID: "c"}}, From: &from, Until: &until}

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
		{"no rules matches nothing", DebtView{}, str("c"), nil, nil, day(time.April, 1), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DebtMatches(tc.view, tc.cat, tc.payee, tc.tag, nil, tc.at); got != tc.want {
				t.Fatalf("DebtMatches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDebtMatches_AnyRule(t *testing.T) {
	str := func(s string) *string { return &s }
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	from, until := day(time.March, 1), day(time.June, 1)
	v := DebtView{Rules: []DebtRuleView{{CategoryID: "x"}, {CategoryID: "y", PayeeID: str("p")}}, From: &from, Until: &until}
	at := day(time.April, 1)
	if !DebtMatches(v, str("y"), str("p"), nil, nil, at) {
		t.Error("category Y with payee P should match the second rule")
	}
	if DebtMatches(v, str("y"), str("q"), nil, nil, at) {
		t.Error("category Y with payee Q matches no rule")
	}
	if !DebtMatches(v, str("x"), str("q"), str("t"), []string{"l"}, at) {
		t.Error("category X matches the whole-category rule whatever else it carries")
	}
	if DebtMatches(v, str("x"), nil, nil, nil, from.AddDate(0, 0, -1)) || DebtMatches(v, str("x"), nil, nil, nil, until) {
		t.Error("a matching rule outside the debt's range does not match")
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
	labelled := DebtView{Rules: []DebtRuleView{{CategoryID: "c", LabelID: str("car")}}}
	if !DebtMatches(labelled, str("c"), nil, nil, []string{"x", "car"}, at) {
		t.Error("a transaction carrying the label should match")
	}
	if DebtMatches(labelled, str("c"), nil, nil, []string{"student"}, at) {
		t.Error("a transaction without the label should not match")
	}
	if DebtMatches(labelled, str("c"), nil, nil, nil, at) {
		t.Error("a transaction with no labels should not match a labelled debt")
	}
	if !DebtMatches(DebtView{Rules: []DebtRuleView{{CategoryID: "c"}}}, str("c"), nil, nil, []string{"car"}, at) {
		t.Error("a debt without a label ignores labels")
	}
}

func TestAssignDebt_OldestWins(t *testing.T) {
	str := func(s string) *string { return &s }
	at := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	older, newer := at.AddDate(0, -2, 0), at.AddDate(0, -1, 0)
	car := DebtView{ID: "b", Rules: []DebtRuleView{{CategoryID: "c", LabelID: str("car")}}, CreatedAt: newer}
	student := DebtView{ID: "a", Rules: []DebtRuleView{{CategoryID: "z"}, {CategoryID: "c", LabelID: str("student")}}, CreatedAt: older}
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
	views := []DebtView{{ID: "a", Rules: []DebtRuleView{{CategoryID: "c", LabelID: str("car")}}}}
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

func testRuleID(n int) vo.Id {
	return vo.MustParseId(fmt.Sprintf("00000000-0000-7000-8000-%012d", n))
}

var debtKeys = struct{ X, Y, Z, P, P1, P2, T, L1, L2 vo.Id }{
	X: testRuleID(1), Y: testRuleID(2), Z: testRuleID(3),
	P: testRuleID(11), P1: testRuleID(12), P2: testRuleID(13),
	T: testRuleID(21), L1: testRuleID(31), L2: testRuleID(32),
}

var ruleEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// ruleAt builds rule n (created n minutes after the epoch) on cat, narrowed by
// payee, tag and label when given.
func ruleAt(n int, cat vo.Id, payee, tag, label *vo.Id) DebtRule {
	return DebtRule{ID: testRuleID(1000 + n), CategoryID: cat, PayeeID: payee, TagID: tag, LabelID: label,
		CreatedAt: ruleEpoch.Add(time.Duration(n) * time.Minute)}
}

// ruledDebt builds debt n, created n hours after the epoch.
func ruledDebt(n int, start *time.Time, rules ...DebtRule) *Debt {
	return &Debt{ID: testRuleID(100 + n), StartDate: start, Rules: rules,
		CreatedAt: ruleEpoch.Add(time.Duration(n) * time.Hour)}
}

func idp(id vo.Id) *vo.Id { return &id }

func startOn(m time.Month) *time.Time {
	t := time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)
	return &t
}

func ruleIDs(rules []DebtRule) []vo.Id {
	out := make([]vo.Id, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.ID)
	}
	return out
}

func TestDebtRule_Overlaps(t *testing.T) {
	k := debtKeys
	tests := []struct {
		name string
		a, b DebtRule
		want bool
	}{
		{"category alone vs category and payee", ruleAt(1, k.X, nil, nil, nil), ruleAt(2, k.X, idp(k.P), nil, nil), true},
		{"different payees", ruleAt(1, k.X, idp(k.P1), nil, nil), ruleAt(2, k.X, idp(k.P2), nil, nil), false},
		{"different labels", ruleAt(1, k.X, nil, nil, idp(k.L1)), ruleAt(2, k.X, nil, nil, idp(k.L2)), false},
		{"labelled vs unlabelled", ruleAt(1, k.X, nil, nil, idp(k.L1)), ruleAt(2, k.X, nil, nil, nil), true},
		{"payee vs tag", ruleAt(1, k.X, idp(k.P), nil, nil), ruleAt(2, k.X, nil, idp(k.T), nil), true},
		{"different categories", ruleAt(1, k.X, nil, nil, nil), ruleAt(2, k.Y, nil, nil, nil), false},
		{"identical", ruleAt(1, k.X, idp(k.P), nil, nil), ruleAt(2, k.X, idp(k.P), nil, nil), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Overlaps(tc.b); got != tc.want {
				t.Errorf("a.Overlaps(b) = %v, want %v", got, tc.want)
			}
			if got := tc.b.Overlaps(tc.a); got != tc.want {
				t.Errorf("b.Overlaps(a) = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDebtRule_Covers(t *testing.T) {
	k := debtKeys
	x := ruleAt(1, k.X, nil, nil, nil)
	xp := ruleAt(2, k.X, idp(k.P), nil, nil)
	xpt := ruleAt(3, k.X, idp(k.P), idp(k.T), nil)
	if !x.Covers(xp) {
		t.Error("X should cover X+P")
	}
	if xp.Covers(x) {
		t.Error("X+P should not cover X")
	}
	if !xp.Covers(xpt) {
		t.Error("X+P should cover X+P+T")
	}
	if x.Covers(ruleAt(4, k.Y, idp(k.P), nil, nil)) {
		t.Error("a rule never covers another category")
	}
}

func TestDebtRule_SameKeyAndIsWhole(t *testing.T) {
	k := debtKeys
	a, b := ruleAt(1, k.X, idp(k.P), nil, nil), ruleAt(2, k.X, idp(k.P), nil, nil)
	if !a.SameKey(b) {
		t.Error("rules with the same category and payee have the same key, whatever their ids")
	}
	if a.SameKey(ruleAt(3, k.X, idp(k.P), nil, idp(k.L1))) {
		t.Error("a label makes a different key")
	}
	if a.IsWhole() || !ruleAt(4, k.X, nil, nil, nil).IsWhole() {
		t.Error("only a rule with no payee, tag or label is whole")
	}
}

func TestCheckDebtRules_WithinOneDebt(t *testing.T) {
	k := debtKeys
	d := ruledDebt(1, startOn(time.January), ruleAt(1, k.X, nil, nil, nil), ruleAt(2, k.X, nil, nil, nil))
	if got := debtFieldCodes(t, CheckDebtRules(d, nil)); got["rules"] != errs.CodeDebtKeyConflict {
		t.Errorf("{X, X}: codes = %v, want key_conflict on rules", got)
	}
	d.Rules = []DebtRule{ruleAt(1, k.X, nil, nil, nil), ruleAt(2, k.X, idp(k.P), nil, nil)}
	if got := debtFieldCodes(t, CheckDebtRules(d, []*Debt{d})); got["rules"] != errs.CodeDebtKeyConflict {
		t.Errorf("{X, X+P}: codes = %v, want key_conflict on rules", got)
	}
	d.Rules = []DebtRule{ruleAt(1, k.X, nil, nil, idp(k.L1)), ruleAt(2, k.X, nil, nil, idp(k.L2))}
	if err := CheckDebtRules(d, []*Debt{d}); err != nil {
		t.Errorf("{X+L1, X+L2}: %v, want nil", err)
	}
}

func TestCheckDebtRules_AcrossDebts(t *testing.T) {
	k := debtKeys
	other := ruledDebt(1, startOn(time.January), ruleAt(1, k.X, nil, nil, nil))
	d := ruledDebt(2, startOn(time.January), ruleAt(2, k.Y, nil, nil, nil), ruleAt(3, k.X, nil, nil, nil))
	if got := debtFieldCodes(t, CheckDebtRules(d, []*Debt{other, d})); got["startDate"] != errs.CodeDebtStartDateTaken {
		t.Errorf("identical rule, same start: codes = %v, want start_date_taken on startDate", got)
	}
	d.StartDate = startOn(time.February)
	if err := CheckDebtRules(d, []*Debt{other, d}); err != nil {
		t.Errorf("identical rule, distinct starts: %v, want nil", err)
	}
	d.Rules = []DebtRule{ruleAt(3, k.X, idp(k.P), nil, nil)}
	if got := debtFieldCodes(t, CheckDebtRules(d, []*Debt{other, d})); got["rules"] != errs.CodeDebtKeyConflict {
		t.Errorf("X vs X+P: codes = %v, want key_conflict on rules", got)
	}
	other.Rules = []DebtRule{ruleAt(1, k.X, nil, nil, idp(k.L1))}
	d.Rules = []DebtRule{ruleAt(3, k.X, nil, nil, idp(k.L2))}
	if err := CheckDebtRules(d, []*Debt{other, d}); err != nil {
		t.Errorf("different labels: %v, want nil", err)
	}

	noStart := ruledDebt(3, nil, ruleAt(4, k.Z, nil, nil, nil))
	alsoNoStart := ruledDebt(4, nil, ruleAt(5, k.Z, nil, nil, nil))
	if got := debtFieldCodes(t, CheckDebtRules(alsoNoStart, []*Debt{noStart})); got["startDate"] != errs.CodeDebtStartDateTaken {
		t.Errorf("identical rule, both without a start: codes = %v, want start_date_taken", got)
	}
}

func TestDebtUntil_LaterIdenticalRuleEndsEveryRule(t *testing.T) {
	k := debtKeys
	d := ruledDebt(1, startOn(time.January), ruleAt(1, k.X, nil, nil, nil), ruleAt(2, k.Y, nil, nil, nil))
	e := ruledDebt(2, startOn(time.June), ruleAt(3, k.Y, nil, nil, nil))
	f := ruledDebt(3, startOn(time.March), ruleAt(4, k.Z, nil, nil, nil))
	g := ruledDebt(4, startOn(time.April), ruleAt(5, k.X, nil, nil, idp(k.L1)))
	h := ruledDebt(5, startOn(time.September), ruleAt(6, k.X, nil, nil, nil))
	earlier := ruledDebt(6, nil, ruleAt(7, k.X, nil, nil, nil))
	budget := []*Debt{h, g, f, e, d, earlier}

	if got := DebtUntil(d, budget); got == nil || !got.Equal(*startOn(time.June)) {
		t.Errorf("Until = %v, want June (the earliest later debt sharing a rule)", got)
	}
	d.StartDate = nil
	earlier.StartDate = startOn(time.February)
	if got := DebtUntil(d, budget); got == nil || !got.Equal(*startOn(time.February)) {
		t.Errorf("no start: Until = %v, want February", got)
	}
	d.StartDate = startOn(time.January)
	if got := DebtUntil(d, []*Debt{d, f, g}); got != nil {
		t.Errorf("nothing later shares a rule: Until = %v, want nil", got)
	}
	if got := DebtUntil(h, budget); got != nil {
		t.Errorf("the latest debt: Until = %v, want nil", got)
	}
}

func TestDebtView_CarriesKey(t *testing.T) {
	str := func(s string) *string { return &s }
	tagged := DebtView{Rules: []DebtRuleView{{CategoryID: "x", TagID: str("t")}}}
	if !tagged.CarriesKey(str("x"), str("t")) {
		t.Error("{X, tag T} should carry (X, T)")
	}
	if tagged.CarriesKey(str("x"), nil) || tagged.CarriesKey(str("x"), str("u")) {
		t.Error("{X, tag T} should not carry (X, no tag) or (X, another tag)")
	}
	whole := DebtView{Rules: []DebtRuleView{{CategoryID: "y"}, {CategoryID: "x", PayeeID: str("p")}}}
	if !whole.CarriesKey(str("x"), nil) || !whole.CarriesKey(str("x"), str("t")) {
		t.Error("a rule without a tag carries its category with any tag")
	}
	if whole.CarriesKey(str("z"), nil) || whole.CarriesKey(nil, nil) {
		t.Error("no rule on the category carries nothing")
	}
}

func TestSoleWholeCategory(t *testing.T) {
	k := debtKeys
	d := ruledDebt(1, startOn(time.January), ruleAt(1, k.X, nil, nil, nil))
	if cat, ok := SoleWholeCategory(d, []*Debt{d}); !ok || !cat.Equal(k.X) {
		t.Errorf("{X} alone = %v, %v; want X, true", cat, ok)
	}
	partial := ruledDebt(3, startOn(time.March), ruleAt(4, k.X, idp(k.P), nil, nil))
	if _, ok := SoleWholeCategory(d, []*Debt{d, partial}); !ok {
		t.Error("another debt's partial rule on X does not stop {X} being whole")
	}
	two := ruledDebt(2, startOn(time.January), ruleAt(2, k.X, nil, nil, nil), ruleAt(3, k.Y, nil, nil, nil))
	if _, ok := SoleWholeCategory(two, []*Debt{two}); ok {
		t.Error("{X, Y} has two rules")
	}
	if _, ok := SoleWholeCategory(partial, []*Debt{partial}); ok {
		t.Error("{X+P} is not whole-category")
	}
	later := ruledDebt(4, startOn(time.June), ruleAt(5, k.X, nil, nil, nil))
	if _, ok := SoleWholeCategory(d, []*Debt{d, later}); ok {
		t.Error("another debt with a whole rule on X (distinct start) shares the category")
	}
	if IsWholeCategoryRule(d, d.Rules[0], []*Debt{d, later}) {
		t.Error("IsWholeCategoryRule: another debt's whole rule on X shares the category")
	}
}

func TestNormalizeBudgetRules_DuplicateInOneDebtCollapses(t *testing.T) {
	k := debtKeys
	older, newer := ruleAt(1, k.X, nil, nil, nil), ruleAt(2, k.X, nil, nil, nil)
	d := ruledDebt(1, startOn(time.January), newer, older)
	changed := NormalizeBudgetRules([]*Debt{d})
	if len(d.Rules) != 1 || !d.Rules[0].ID.Equal(older.ID) {
		t.Errorf("rules = %v, want only the older %v", ruleIDs(d.Rules), older.ID)
	}
	if len(changed) != 1 || changed[0] != d {
		t.Errorf("changed = %v, want [d]", changed)
	}
}

func TestNormalizeBudgetRules_NarrowerRuleOfTheSameDebtDropped(t *testing.T) {
	k := debtKeys
	narrow, whole := ruleAt(1, k.X, idp(k.P2), nil, nil), ruleAt(2, k.X, nil, nil, nil)
	d := ruledDebt(1, startOn(time.January), narrow, whole)
	NormalizeBudgetRules([]*Debt{d})
	if len(d.Rules) != 1 || !d.Rules[0].ID.Equal(whole.ID) {
		t.Errorf("rules = %v, want only the whole-category rule %v", ruleIDs(d.Rules), whole.ID)
	}
}

func TestNormalizeBudgetRules_OverlapAcrossDebtsNewerLoses(t *testing.T) {
	k := debtKeys
	a := ruledDebt(1, startOn(time.January), ruleAt(1, k.X, nil, nil, nil))
	y := ruleAt(3, k.Y, nil, nil, nil)
	b := ruledDebt(2, startOn(time.March), ruleAt(2, k.X, idp(k.P), nil, nil), y)
	changed := NormalizeBudgetRules([]*Debt{b, a})
	if len(b.Rules) != 1 || !b.Rules[0].ID.Equal(y.ID) {
		t.Errorf("b rules = %v, want only Y", ruleIDs(b.Rules))
	}
	if len(a.Rules) != 1 {
		t.Errorf("a rules = %v, want unchanged", ruleIDs(a.Rules))
	}
	if len(changed) != 1 || changed[0] != b {
		t.Errorf("changed = %v, want [b]", changed)
	}
}

func TestNormalizeBudgetRules_IdenticalAcrossDebtsSameStartNewerLoses(t *testing.T) {
	k := debtKeys
	a := ruledDebt(1, startOn(time.January), ruleAt(1, k.X, nil, nil, nil))
	y := ruleAt(3, k.Y, nil, nil, nil)
	b := ruledDebt(2, startOn(time.January), ruleAt(2, k.X, nil, nil, nil), y)
	changed := NormalizeBudgetRules([]*Debt{a, b})
	if len(b.Rules) != 1 || !b.Rules[0].ID.Equal(y.ID) {
		t.Errorf("b rules = %v, want only Y", ruleIDs(b.Rules))
	}
	if len(a.Rules) != 1 || len(changed) != 1 || changed[0] != b {
		t.Errorf("a rules = %v, changed = %v; want a unchanged and [b]", ruleIDs(a.Rules), changed)
	}
}

func TestNormalizeBudgetRules_IdenticalAcrossDebtsDistinctStartsKept(t *testing.T) {
	k := debtKeys
	a := ruledDebt(1, startOn(time.January), ruleAt(1, k.X, nil, nil, nil))
	b := ruledDebt(2, startOn(time.June), ruleAt(2, k.X, nil, nil, nil))
	if changed := NormalizeBudgetRules([]*Debt{a, b}); changed != nil || len(a.Rules) != 1 || len(b.Rules) != 1 {
		t.Errorf("changed = %v, a = %v, b = %v; want both kept, partitioned by start", changed, ruleIDs(a.Rules), ruleIDs(b.Rules))
	}
}

func TestNormalizeBudgetRules_DifferentLabelsKept(t *testing.T) {
	k := debtKeys
	a := ruledDebt(1, startOn(time.January), ruleAt(1, k.X, nil, nil, idp(k.L1)))
	b := ruledDebt(2, startOn(time.January), ruleAt(2, k.X, nil, nil, idp(k.L2)))
	if changed := NormalizeBudgetRules([]*Debt{a, b}); changed != nil || len(a.Rules) != 1 || len(b.Rules) != 1 {
		t.Errorf("changed = %v; want nil and both rules kept", changed)
	}
}

func TestNormalizeBudgetRules_OlderDebtByIDOnEqualCreatedAt(t *testing.T) {
	k := debtKeys
	a := ruledDebt(1, startOn(time.January), ruleAt(1, k.X, nil, nil, nil))
	b := ruledDebt(2, startOn(time.January), ruleAt(2, k.X, idp(k.P), nil, nil))
	b.CreatedAt = a.CreatedAt
	NormalizeBudgetRules([]*Debt{b, a})
	if len(a.Rules) != 1 || len(b.Rules) != 0 {
		t.Errorf("a = %v, b = %v; want the lower id a to keep its rule", ruleIDs(a.Rules), ruleIDs(b.Rules))
	}
}

func TestNormalizeBudgetRules_NarrowerRuleSurvivesWhenItsCoverConflicts(t *testing.T) {
	k := debtKeys
	a := ruledDebt(1, startOn(time.January), ruleAt(1, k.X, idp(k.P2), nil, nil))
	narrow := ruleAt(3, k.X, idp(k.P1), nil, nil)
	b := ruledDebt(2, startOn(time.March), ruleAt(2, k.X, nil, nil, nil), narrow)
	NormalizeBudgetRules([]*Debt{a, b})
	if len(b.Rules) != 1 || !b.Rules[0].ID.Equal(narrow.ID) {
		t.Errorf("b rules = %v, want X+P1 kept once X is lost to a", ruleIDs(b.Rules))
	}
}
