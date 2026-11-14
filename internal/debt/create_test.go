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

	if res.ElementId != e.category || res.ElementType != "category" || res.Status != "active" || res.EndedAt != "" || res.CurrencyId != fixture.USD {
		t.Errorf("identity = %+v", res)
	}
	if res.AmountBorrowed != "10000" || res.Balance != "10000" || res.Payment != "1000" || res.Schedule != "monthly" ||
		res.LastPaymentDate != "2027-03-01" || res.StartDate != "2026-03-10" {
		t.Errorf("terms = %+v", res)
	}
	if res.CurrentBalance != "10000" || res.PaymentsLeft != 12 || res.IsPaidOff != 0 {
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
	_, err := e.tryCreate(req)
	requireCode(t, err, errs.CodeDebtLastPaymentBeforeStart)
}

// A debt created without the loan's facts is not set up: no
// snapshot, and every figure that needs the terms is empty.
func TestCreateDebt_WithoutTerms(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	withoutTerms(&req)
	res, err := e.tryCreate(req)
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
}

func TestCreateDebt_LoanFactsRequiredTogether(t *testing.T) {
	e := newEnv(t)
	for key, mod := range map[string]func(*createSpec){
		"lastPaymentDate": func(r *createSpec) { r.LastPaymentDate = "" },
		"amountBorrowed":  func(r *createSpec) { r.AmountBorrowed = vo.FlexString{} },
		"startDate":       func(r *createSpec) { r.StartDate = nil },
	} {
		req := e.createReq()
		mod(&req)
		_, err := e.tryCreate(req)
		requireField(t, err, key, errs.CodeIsBlank)
	}
}

func TestCreateDebt_WithoutTermsNeedsAPositivePayment(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	withoutTerms(&req)
	req.Payment = vo.NewFlexString("0.001")
	_, err := e.tryCreate(req)
	requireField(t, err, "payment", errs.CodeOutOfRange)
}

func TestCreateDebt_RefusesPaymentsDontCover(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.AmountBorrowed = vo.NewFlexString("20000")
	_, err := e.tryCreate(req)
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
	_, err := e.tryCreate(req)
	requireCode(t, err, errs.CodeDebtPaymentsDontCover)

	req.LastPaymentDate = "2027-01-01"
	if _, err := e.tryCreate(req); err != nil {
		t.Fatalf("ten payments of 0.001 cover 0.01: %v", err)
	}
}

// A blank start date passes the request's validation of a debt without terms,
// so the service must read it as none rather than fail to parse it.
func TestCreateDebt_BlankStartDateIsNone(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, func(r *createSpec) { withoutTerms(r); r.StartDate = ptr(" ") })
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
	_, err := e.tryCreate(req)
	requireCode(t, err, errs.CodeOutOfRange)
	req.LastPaymentDate = "2126-03-01"
	req.AmountBorrowed = vo.NewFlexString("1200000")
	if _, err := e.tryCreate(req); err != nil {
		t.Fatalf("a 100-year term is allowed: %v", err)
	}
}

func TestCreateDebt_GuestForbidden(t *testing.T) {
	e := newEnv(t)
	guest := e.partner(t, "guest", roleGuest)
	_, err := e.svc.CreateDebt(context.Background(), vo.MustParseId(guest), e.createDebtReq())
	if _, ok := errs.AsAccessDenied(err); !ok {
		t.Fatalf("err = %v, want access denied", err)
	}
}

func TestCreateDebt_NoAccessNotFound(t *testing.T) {
	e := newEnv(t)
	other := e.f.User(fixture.User{Name: "other", Email: "other@example.test"})
	invited := e.partner(t, "invited", rolePending)
	for _, u := range []string{other, invited} {
		_, err := e.svc.CreateDebt(context.Background(), vo.MustParseId(u), e.createDebtReq())
		requireNotFound(t, err)
	}
}

func TestCreateDebt_ArchivedBudget(t *testing.T) {
	e := newEnv(t)
	e.fakeBudget(e.budget).archived = true
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), e.createDebtReq())
	requireArchived(t, err)
}

// The creation day is the creating user's, from their stored timezone.
func TestCreateDebt_CreatedDayInTheCreatorsTimezone(t *testing.T) {
	e := newEnv(t)
	sam := e.partner(t, "Sam", roleUser)
	e.users.zones[sam] = "Pacific/Kiritimati"
	e.clock.now = time.Date(2026, 3, 10, 20, 0, 0, 0, time.UTC)
	res, err := e.svc.CreateDebt(context.Background(), vo.MustParseId(sam), e.createDebtReq())
	if err != nil {
		t.Fatal(err)
	}
	if d := e.debtOf(t, res.Id); !d.CreatedDay.Equal(ymd(2026, 3, 11)) {
		t.Fatalf("created day = %v, want March 11 in Kiritimati", d.CreatedDay)
	}
}

// A debt attaches to a top-level expense category or an envelope, and takes
// its name, icon and currency from it.
func TestCreateDebt_OnACategoryAndAnEnvelope(t *testing.T) {
	e := newEnv(t)
	eur := e.eur()
	el := e.elements.items[e.budget][e.category]
	el.CurrencyID = vo.MustParseId(eur)
	e.elements.items[e.budget][e.category] = el

	onCategory := e.create(t, withoutTerms)
	if onCategory.ElementId != e.category || onCategory.ElementType != "category" || onCategory.Name != el.Name ||
		onCategory.Icon != "i" || onCategory.CurrencyId != eur {
		t.Fatalf("category debt = %+v", onCategory)
	}

	child := e.newCategory(e.user, true)
	env := e.newEnvelope("Housing", child)
	onEnvelope := e.create(t, func(r *createSpec) { withoutTerms(r); r.ElementId = env })
	if onEnvelope.ElementId != env || onEnvelope.ElementType != "envelope" || onEnvelope.Name != "Housing" ||
		onEnvelope.Icon != "env" || onEnvelope.CurrencyId != fixture.USD {
		t.Fatalf("envelope debt = %+v", onEnvelope)
	}
}

func TestCreateDebt_Refusals(t *testing.T) {
	e := newEnv(t)
	child := e.newCategory(e.user, true)
	e.newEnvelope("Housing", child)
	for name, element := range map[string]string{
		"a category inside an envelope": child,
		"an income category":            e.newCategory(e.user, false),
		"a tag":                         e.newTag(),
	} {
		req := e.createDebtReq()
		req.ElementId = element
		_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
		if err == nil {
			t.Fatalf("%s: created", name)
		}
		requireField(t, err, "elementId", errs.CodeDebtElementNotEligible)
	}

	first := e.create(t, func(r *createSpec) { r.StartDate = ptr("2026-01-01") })
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), e.createDebtReq())
	requireField(t, err, "elementId", errs.CodeDebtElementHasDebt)

	e.markPaidOff(t, first.Id, "2026-02-01")
	for _, start := range []string{"2026-01-15", "2026-02-01"} {
		req := e.createDebtReq()
		req.StartDate = ptr(start)
		_, err = e.svc.CreateDebt(context.Background(), e.uid(), req)
		requireField(t, err, "startDate", errs.CodeDebtStartBeforePreviousEnd)
	}
	req := e.createDebtReq()
	req.StartDate = ptr("2026-02-02")
	if _, err := e.svc.CreateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatalf("a start after the previous end: %v", err)
	}

	guest := e.partner(t, "guest", roleGuest)
	_, err = e.svc.CreateDebt(context.Background(), vo.MustParseId(guest), e.createDebtReq())
	requireForbidden(t, err)
}
