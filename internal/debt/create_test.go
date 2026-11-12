package debt_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestCreateDebt_ValidatesAndSnapshots(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)

	if res.Name != "Car loan" || res.Icon != "i" || res.CurrencyId != fixture.USD || len(res.Rules) != 1 || res.Rules[0].CategoryId != e.category {
		t.Errorf("identity = %+v", res)
	}
	if res.AmountBorrowed != "10000" || res.Balance != "10000" || res.Payment != "1000" || res.Schedule != "monthly" ||
		res.LastPaymentDate != "2027-03-01" || res.StartDate != "2026-03-10" {
		t.Errorf("terms = %+v", res)
	}
	if res.CurrentBalance != "10000" || res.PaymentsLeft != 12 || res.IsPaidOff != 0 || res.Rules[0].PayeeId != nil || res.Rules[0].TagId != nil {
		t.Errorf("figures = %+v", res)
	}
	if res.PeriodRate == "" || res.PeriodRate == "0.0000000000" || res.NextDueDate != "2026-04-01" || res.PayoffDate != "2027-03-01" {
		t.Errorf("rate/dates = %q %q %q", res.PeriodRate, res.NextDueDate, res.PayoffDate)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	if sn := snaps[0]; !sn.AsOf.Equal(ymd(2026, 3, 10)) || sn.Balance.String() != "10000" || sn.Payment.String() != "1000" ||
		sn.Schedule != model.RecurringScheduleMonthly || !sn.LastPaymentDate.Equal(ymd(2027, 3, 1)) {
		t.Errorf("snapshot = %+v", sn)
	}
}

func TestCreateDebt_RefusesLastPaymentBeforeStart(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.LastPaymentDate = "2026-03-10"
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtLastPaymentBeforeStart)
}

// A debt created without the loan's facts is not set up: no
// snapshot, and every figure that needs the terms is empty.
func TestCreateDebt_WithoutTerms(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	withoutTerms(&req)
	if err := req.Validate(); err != nil {
		t.Fatalf("the request without terms is valid: %v", err)
	}
	res, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsSetUp != 0 || res.Payment != "1000" || res.Schedule != "monthly" || res.IsPaidOff != 0 {
		t.Errorf("result = %+v", res)
	}
	for name, v := range map[string]string{
		"amountBorrowed": res.AmountBorrowed, "balance": res.Balance, "lastPaymentDate": res.LastPaymentDate, "currentBalance": res.CurrentBalance,
		"moneyLeft": res.MoneyLeft, "overpaymentLeft": res.OverpaymentLeft, "overpaidSoFar": res.OverpaidSoFar,
		"payoffDate": res.PayoffDate, "nextDueDate": res.NextDueDate, "periodRate": res.PeriodRate,
	} {
		if v != "" {
			t.Errorf("%s = %q, want empty", name, v)
		}
	}
	if res.PaymentsLeft != 0 || res.PeriodsPaid != 0 {
		t.Errorf("counts = %d %d, want 0", res.PaymentsLeft, res.PeriodsPaid)
	}
	if snaps := e.snapshots(t, res.Id); len(snaps) != 0 {
		t.Fatalf("snapshots = %d, want none", len(snaps))
	}
	// a whole-category debt without terms takes the category's plans from
	// its creation day on, with no end
	want := transferCall{budget: e.budget, from: e.category, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt,
		currency: fixture.USD, sum: true, first: "2026-03-10", last: ""}
	if len(e.elements.transfers) != 1 || e.elements.transfers[0] != want {
		t.Fatalf("transfers = %+v, want %+v", e.elements.transfers, want)
	}
}

func TestCreateDebt_LoanFactsRequiredTogether(t *testing.T) {
	e := newEnv(t)
	for key, mod := range map[string]func(*model.CreateDebtRequest){
		"lastPaymentDate": func(r *model.CreateDebtRequest) { r.LastPaymentDate = "" },
		"amountBorrowed":  func(r *model.CreateDebtRequest) { r.AmountBorrowed = vo.FlexString{} },
		"startDate":       func(r *model.CreateDebtRequest) { r.StartDate = nil },
	} {
		req := e.createReq()
		mod(&req)
		_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
		requireField(t, err, key, errs.CodeIsBlank)
	}
}

func TestCreateDebt_WithoutTermsNeedsAPositivePayment(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	withoutTerms(&req)
	req.Payment = vo.NewFlexString("0.001")
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "payment", errs.CodeOutOfRange)
}

func TestCreateDebt_LabelMustBeTheCategoryOwners(t *testing.T) {
	e := newEnv(t)
	partner := e.partner(t, "sam", roleUser)
	foreign := e.newLabelOf(partner)
	req := e.createReq()
	req.Rules[0].LabelId = &foreign
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireNotFound(t, err)

	own := e.newLabel()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].LabelId = &own })
	if r := res.Rules[0]; r.LabelId == nil || *r.LabelId != own || r.LabelName == "" {
		t.Fatalf("rule = %+v, want label %s", r, own)
	}
	// a labelled debt is not whole-category: the category keeps its plans
	if len(e.elements.transfers) != 0 {
		t.Fatalf("transfers = %+v, want none", e.elements.transfers)
	}
}

func TestCheckKeys_LabelsTellDebtsApart(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	car, student := e.newLabel(), e.newLabel()
	e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].LabelId = &car })
	e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].LabelId = &student })

	req := e.createReq()
	_, err := e.svc.CreateDebt(ctx, e.uid(), req)
	requireCode(t, err, errs.CodeDebtKeyConflict)

	req = e.createReq()
	req.Rules[0].LabelId = &car
	_, err = e.svc.CreateDebt(ctx, e.uid(), req)
	requireCode(t, err, errs.CodeDebtStartDateTaken)

	req.StartDate = ptr("2026-03-01")
	if _, err := e.svc.CreateDebt(ctx, e.uid(), req); err != nil {
		t.Fatalf("same label with a distinct start date: %v", err)
	}
}

func TestCreateDebt_ReportsSetUpAndLabel(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	if res.IsSetUp != 1 || res.Rules[0].LabelId != nil || res.PeriodsPaid != 0 {
		t.Errorf("isSetUp/labelId/periodsPaid = %d %v %d", res.IsSetUp, res.Rules[0].LabelId, res.PeriodsPaid)
	}
}

func TestCreateDebt_RefusesIncomeCategory(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.Rules[0].CategoryId = e.newCategory(e.user, false)
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "rules", errs.CodeDebtCategoryNotExpense)
}

func TestCreateDebt_CategoryOfANonParticipantNotFound(t *testing.T) {
	e := newEnv(t)
	other := e.f.User(fixture.User{Name: "other", Email: "other@example.test"})
	guest := e.partner(t, "guest", roleGuest)
	invited := e.partner(t, "invited", rolePending)
	for _, owner := range []string{other, guest, invited} {
		req := e.createReq()
		req.Rules[0].CategoryId = e.newCategory(owner, true)
		_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
		requireNotFound(t, err)
	}
}

func TestCreateDebt_RefusesUnusableCurrency(t *testing.T) {
	e := newEnv(t)
	eur := e.eur()
	e.currency.unusable[eur] = true
	req := e.createReq()
	req.CurrencyId = eur
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeCurrencyNotAvailable)
	if got, err := e.repo.ListByBudget(context.Background(), vo.MustParseId(e.budget)); err != nil || len(got) != 0 {
		t.Errorf("debts after a refused create = %v, %v", got, err)
	}
}

// A currency the budget already uses is usable for its debts whoever asks:
// it may be the owner's custom currency, which a partner could not choose.
func TestCreateDebt_BudgetCurrencySkipsEnsureUsable(t *testing.T) {
	e := newEnv(t)
	eur := e.eur()
	e.currency.unusable[fixture.USD], e.currency.unusable[eur] = true, true
	e.fakeBudget(e.budget).currencies = []string{eur}
	for _, cur := range []string{fixture.USD, eur} {
		res := e.create(t, func(r *model.CreateDebtRequest) {
			r.CurrencyId, r.Rules[0].CategoryId = cur, e.newCategory(e.user, true)
		})
		if res.CurrencyId != cur {
			t.Fatalf("currency = %s, want %s", res.CurrencyId, cur)
		}
	}
}

func TestCreateDebt_CurrencyDefaultsToTheBudgets(t *testing.T) {
	e := newEnv(t)
	eur := e.eur()
	e.fakeBudget(e.budget).currency = eur
	res := e.create(t, func(r *model.CreateDebtRequest) { r.CurrencyId = "" })
	if res.CurrencyId != eur {
		t.Fatalf("currency = %s, want the budget's %s", res.CurrencyId, eur)
	}
}

func TestCreateDebt_RefusesPaymentsDontCover(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.AmountBorrowed = vo.NewFlexString("20000")
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtPaymentsDontCover)
}

// In an 8-digit currency nine payments of 0.001 are short of 0.01: the
// tolerance is half that currency's smallest unit, not half a cent.
func TestCreateDebt_RefusesPaymentsDontCoverInAPreciseCurrency(t *testing.T) {
	e := newEnv(t)
	e.rates.digits = 8
	req := e.createReq()
	// nine monthly dues after Mar 10, 2026: Apr 1 to Dec 1
	req.AmountBorrowed, req.Payment, req.LastPaymentDate = vo.NewFlexString("0.01"), vo.NewFlexString("0.001"), "2026-12-01"
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtPaymentsDontCover)

	req.LastPaymentDate = "2027-01-01"
	if _, err := e.svc.CreateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatalf("ten payments of 0.001 cover 0.01: %v", err)
	}
}

func TestCreateDebt_KeyConflict(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	payee := e.newPayee()
	e.create(t, nil)
	req := e.createReq()
	req.Rules[0].PayeeId = &payee
	_, err := e.svc.CreateDebt(ctx, e.uid(), req)
	requireField(t, err, "rules", errs.CodeDebtKeyConflict)

	// A payee-only debt blocks a category-only one, and a tag-only debt
	// overlaps a payee-only one.
	cat2 := e.newCategory(e.user, true)
	payee2, tag2 := e.newPayee(), e.newTag()
	e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].CategoryId = cat2; r.Rules[0].PayeeId = &payee2 })
	plain := e.createReq()
	plain.Rules[0].CategoryId = cat2
	_, err = e.svc.CreateDebt(ctx, e.uid(), plain)
	requireCode(t, err, errs.CodeDebtKeyConflict)
	tagReq := e.createReq()
	tagReq.Rules[0].CategoryId, tagReq.Rules[0].TagId = cat2, &tag2
	_, err = e.svc.CreateDebt(ctx, e.uid(), tagReq)
	requireCode(t, err, errs.CodeDebtKeyConflict)

	// Disjoint payees coexist.
	other := e.newPayee()
	e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].CategoryId = cat2; r.Rules[0].PayeeId = &other })
}

func TestCreateDebt_SameKeyNeedsDistinctStartDates(t *testing.T) {
	e := newEnv(t)
	e.create(t, nil)
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), e.createReq())
	requireCode(t, err, errs.CodeDebtStartDateTaken)

	e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-01-01") })
	dup := e.createReq()
	dup.StartDate = ptr("2026-01-01")
	_, err = e.svc.CreateDebt(context.Background(), e.uid(), dup)
	requireCode(t, err, errs.CodeDebtStartDateTaken)

	e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-02-01") })
}

func TestCreateDebt_WholeCategoryTransfersPlans(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	// only the debt's life moves: from its first snapshot to its last payment
	want := transferCall{budget: e.budget, from: e.category, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt, currency: fixture.USD,
		sum: true, first: "2026-03-10", last: "2027-03-01"}
	if len(e.elements.transfers) != 1 || e.elements.transfers[0] != want {
		t.Fatalf("transfers = %+v, want [%+v]", e.elements.transfers, want)
	}

	payee := e.newPayee()
	e.elements.transfers = nil
	e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules[0].CategoryId = e.newCategory(e.user, true)
		r.Rules[0].PayeeId = &payee
	})
	if len(e.elements.transfers) != 0 {
		t.Fatalf("a payee-narrowed debt must not move the category's plans: %+v", e.elements.transfers)
	}
}

func TestCreateDebt_WorksWithoutBudgetPeer(t *testing.T) {
	e := newEnv(t)
	e.svc.SetBudgetElements(nil)
	e.create(t, nil)
}

// A blank start date passes the request's validation of a debt without terms,
// so the service must read it as none rather than fail to parse it.
func TestCreateDebt_BlankStartDateIsNone(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, func(r *model.CreateDebtRequest) { withoutTerms(r); r.StartDate = ptr(" ") })
	if res.StartDate != "" {
		t.Fatalf("start date = %q, want none", res.StartDate)
	}
}

// The projection covers 100 years; a longer term (a mistyped year) is refused
// rather than projected partially.
func TestCreateDebt_TermBeyondTheProjectionIsRefused(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.LastPaymentDate = "2126-04-01"
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeOutOfRange)
	req.LastPaymentDate = "2126-03-01"
	req.AmountBorrowed = vo.NewFlexString("1200000")
	if _, err := e.svc.CreateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatalf("a 100-year term is allowed: %v", err)
	}
}

// A shared mortgage: he pays from his category, she from hers. One debt
// counts both, its rules naming each category with its owner.
func TestCreateDebt_RulesAcrossParticipants(t *testing.T) {
	e := newEnv(t)
	sam := e.partner(t, "Sam", roleUser)
	hers := e.newCategory(sam, true)
	res := e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules = []model.DebtRuleRequest{rule(e.category), rule(hers)}
	})
	if res.BudgetId != e.budget || res.BudgetName != "Family" || res.CanEdit != 1 {
		t.Fatalf("budget = %q %q, canEdit %d", res.BudgetId, res.BudgetName, res.CanEdit)
	}
	if len(res.Rules) != 2 || res.Rules[0].CategoryId != e.category || res.Rules[1].CategoryId != hers {
		t.Fatalf("rules = %+v, want in the order given", res.Rules)
	}
	if r := res.Rules[0]; r.OwnerUserId != e.user || r.OwnerName != "Alex" || r.CategoryName == "" || r.CategoryIcon != "i" || r.Id == "" {
		t.Errorf("his rule = %+v", r)
	}
	if r := res.Rules[1]; r.OwnerUserId != sam || r.OwnerName != "Sam" {
		t.Errorf("her rule = %+v", r)
	}
	// a participant who plans creates one too
	req := e.createReq()
	req.Rules = []model.DebtRuleRequest{rule(e.newCategory(e.user, true))}
	if _, err := e.svc.CreateDebt(context.Background(), vo.MustParseId(sam), req); err != nil {
		t.Fatalf("a participant creates a debt: %v", err)
	}
}

func TestCreateDebt_PayeeMustBeTheCategoryOwners(t *testing.T) {
	e := newEnv(t)
	sam := e.partner(t, "Sam", roleUser)
	hers := e.newCategory(sam, true)
	mine, herPayee := e.newPayee(), e.newPayeeOf(sam)
	req := e.createReq()
	req.Rules = []model.DebtRuleRequest{{CategoryId: hers, PayeeId: &mine}}
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireNotFound(t, err)

	res := e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules = []model.DebtRuleRequest{{CategoryId: hers, PayeeId: &herPayee}}
	})
	if r := res.Rules[0]; r.PayeeId == nil || *r.PayeeId != herPayee || r.PayeeName == "" {
		t.Fatalf("rule = %+v", r)
	}
}

func TestCreateDebt_GuestForbidden(t *testing.T) {
	e := newEnv(t)
	guest := e.partner(t, "guest", roleGuest)
	_, err := e.svc.CreateDebt(context.Background(), vo.MustParseId(guest), e.createReq())
	if _, ok := errs.AsAccessDenied(err); !ok {
		t.Fatalf("err = %v, want access denied", err)
	}
}

func TestCreateDebt_NoAccessNotFound(t *testing.T) {
	e := newEnv(t)
	other := e.f.User(fixture.User{Name: "other", Email: "other@example.test"})
	invited := e.partner(t, "invited", rolePending)
	for _, u := range []string{other, invited} {
		_, err := e.svc.CreateDebt(context.Background(), vo.MustParseId(u), e.createReq())
		requireNotFound(t, err)
	}
}

func TestCreateDebt_ArchivedBudget(t *testing.T) {
	e := newEnv(t)
	e.fakeBudget(e.budget).archived = true
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), e.createReq())
	requireArchived(t, err)
}

// The creation day is the creating user's, from their stored timezone.
func TestCreateDebt_CreatedDayInTheCreatorsTimezone(t *testing.T) {
	e := newEnv(t)
	sam := e.partner(t, "Sam", roleUser)
	e.users.zones[sam] = "Pacific/Kiritimati"
	e.clock.now = time.Date(2026, 3, 10, 20, 0, 0, 0, time.UTC)
	req := e.createReq()
	withoutTerms(&req)
	res, err := e.svc.CreateDebt(context.Background(), vo.MustParseId(sam), req)
	if err != nil {
		t.Fatal(err)
	}
	if d := e.debtOf(t, res.Id); !d.CreatedDay.Equal(ymd(2026, 3, 11)) {
		t.Fatalf("created day = %v, want March 11 in Kiritimati", d.CreatedDay)
	}
}

// Each whole-category rule takes its category row's plans in the debt's
// budget; a narrowed rule takes none.
func TestCreateDebt_MovesPlansOfEachWholeCategoryRule(t *testing.T) {
	e := newEnv(t)
	sam := e.partner(t, "Sam", roleUser)
	hers, third := e.newCategory(sam, true), e.newCategory(e.user, true)
	payee := e.newPayee()
	res := e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules = []model.DebtRuleRequest{rule(e.category), rule(hers), {CategoryId: third, PayeeId: &payee}}
	})
	if len(e.elements.transfers) != 2 {
		t.Fatalf("transfers = %+v, want one per whole rule", e.elements.transfers)
	}
	for i, cat := range []string{e.category, hers} {
		want := transferCall{budget: e.budget, from: cat, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt,
			currency: fixture.USD, sum: true, first: "2026-03-10", last: "2027-03-01"}
		if e.elements.transfers[i] != want {
			t.Errorf("transfer %d = %+v, want %+v", i, e.elements.transfers[i], want)
		}
	}
}

func TestCreateDebt_TwoRulesOfTheDebtMayNotOverlap(t *testing.T) {
	e := newEnv(t)
	payee := e.newPayee()
	l1, l2 := e.newLabel(), e.newLabel()
	req := e.createReq()
	req.Rules = []model.DebtRuleRequest{rule(e.category), {CategoryId: e.category, PayeeId: &payee}}
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "rules", errs.CodeDebtKeyConflict)
	// two labels of one category tell the rules apart
	e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules = []model.DebtRuleRequest{{CategoryId: e.category, LabelId: &l1}, {CategoryId: e.category, LabelId: &l2}}
	})
}
