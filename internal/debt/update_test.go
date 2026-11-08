package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestUpdateDebt_SnapshotOnlyWhenValuesChange(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)

	req := e.updateReq(res)
	req.Name = "Renamed"
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	if n := len(e.snapshots(t, res.Id)); n != 1 {
		t.Fatalf("a rename wrote a snapshot: %d", n)
	}

	e.clock.now = e.clock.now.AddDate(0, 0, 5)
	req.Balance = vo.NewFlexString("9000")
	got, err := e.svc.UpdateDebt(ctx, e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 2 || snaps[1].Balance.String() != "9000" || !snaps[1].AsOf.Equal(ymd(2026, 3, 15)) {
		t.Fatalf("snapshots = %+v", snaps)
	}
	if got.Name != "Renamed" || got.Balance != "9000" {
		t.Fatalf("result = %+v", got)
	}

	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	if n := len(e.snapshots(t, res.Id)); n != 2 {
		t.Fatalf("an unchanged save wrote a snapshot: %d", n)
	}
}

func TestUpdateDebt_ValidatesChangedTermsOnly(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)

	req := e.updateReq(res)
	req.Balance = vo.NewFlexString("30000")
	_, err := e.svc.UpdateDebt(ctx, e.uid(), req)
	requireCode(t, err, errs.CodeDebtPaymentsDontCover)

	// Past the last payment date a rename must still work.
	e.clock.now = e.clock.now.AddDate(2, 0, 0)
	rename := e.updateReq(res)
	rename.Name = "Old loan"
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), rename); err != nil {
		t.Fatalf("rename of an old loan: %v", err)
	}
	rename.Balance = vo.NewFlexString("500")
	_, err = e.svc.UpdateDebt(ctx, e.uid(), rename)
	requireCode(t, err, errs.CodeDebtLastPaymentInPast)
}

func TestUpdateDebt_KeyChecksRunOnEdit(t *testing.T) {
	e := newEnv(t)
	payee := e.newPayee()
	e.create(t, nil)
	narrow := e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-02-01") })
	req := e.updateReq(narrow)
	req.StartDate = nil
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtStartDateTaken)

	req = e.updateReq(narrow)
	req.PayeeId = &payee
	_, err = e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtKeyConflict)
}

type currentTerms struct {
	asOf, last, balance, payment, schedule string
}

func termsOf(asOf, last interface{ Format(string) string }, balance, payment vo.DecimalNumber, schedule model.RecurringSchedule) currentTerms {
	return currentTerms{asOf.Format("2006-01-02"), last.Format("2006-01-02"), balance.String(), payment.String(), string(schedule)}
}

func TestDebtCurrentMatchesLatestSnapshot(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	id := vo.MustParseId(res.Id)

	check := func(step string) {
		t.Helper()
		d, err := e.repo.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		snaps := e.snapshots(t, res.Id)
		last := snaps[len(snaps)-1]
		got := termsOf(d.Current.AsOf, d.Current.LastPaymentDate, d.Current.Balance, d.Current.Payment, d.Current.Schedule)
		want := termsOf(last.AsOf, last.LastPaymentDate, last.Balance, last.Payment, last.Schedule)
		if got != want {
			t.Errorf("%s: Current = %+v, latest snapshot = %+v", step, got, want)
		}
	}
	check("create")

	e.clock.now = e.clock.now.AddDate(0, 0, 3)
	req := e.updateReq(res)
	req.Balance, req.Payment, req.Schedule = vo.NewFlexString("8000"), vo.NewFlexString("500"), "biweekly"
	req.LastPaymentDate = "2027-06-04"
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	check("update")

	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	check("no-op update")

	e.clock.now = e.clock.now.AddDate(0, 0, 3)
	if _, err := e.svc.MarkDebtPaidOff(ctx, e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id}); err != nil {
		t.Fatal(err)
	}
	check("mark paid off")
	if n := len(e.snapshots(t, res.Id)); n != 3 {
		t.Fatalf("snapshots = %d, want 3", n)
	}
}

func TestMarkPaidOff_ZeroSnapshot(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.clock.now = e.clock.now.AddDate(0, 0, 20)

	got, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsPaidOff != 1 || got.CurrentBalance != "0" || got.Balance != "0" || got.PaymentsLeft != 0 || got.PayoffDate != "" {
		t.Fatalf("result = %+v", got)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snaps))
	}
	z := snaps[1]
	if !z.AsOf.Equal(ymd(2026, 3, 30)) || !z.Balance.IsZero() || z.Payment.String() != "1000" ||
		z.Schedule != model.RecurringScheduleMonthly || !z.LastPaymentDate.Equal(ymd(2027, 3, 1)) {
		t.Fatalf("zero snapshot = %+v", z)
	}

	again, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id})
	if err != nil || again.IsPaidOff != 1 || len(e.snapshots(t, res.Id)) != 2 {
		t.Fatalf("second mark: %+v, %v, %d snapshots", again, err, len(e.snapshots(t, res.Id)))
	}
}

func TestUpdateDebt_RematchKeepsStoredSplits(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p1, p2 := e.newPayee(), e.newPayee()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.PayeeId = &p1 })
	e.clock.now = ymd(2026, 7, 1)
	a := e.expense(t, e.account, ymd(2026, 4, 1), "1000", p1)
	b := e.expense(t, e.account, ymd(2026, 5, 1), "1000", p2)
	e.sync(t)

	first := e.splits(t, res.Id)
	if len(first) != 1 || first[a] == "" || first[a] == "0" {
		t.Fatalf("splits = %v, want only a with interest", first)
	}

	// Widening to the whole category brings b in as a late entry; a keeps
	// its interest even though the balance was corrected.
	req := e.updateReq(res)
	req.PayeeId = nil
	req.Balance = vo.NewFlexString("5000")
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	second := e.splits(t, res.Id)
	if len(second) != 2 || second[a] != first[a] || second[b] == "" {
		t.Fatalf("after widening: %v (first %v)", second, first)
	}

	// Narrowing again drops b's split and keeps a's.
	req.PayeeId = &p1
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	third := e.splits(t, res.Id)
	if len(third) != 1 || third[a] != first[a] {
		t.Fatalf("after narrowing: %v", third)
	}
}

func TestUpdateDebt_PaymentRoundingToZeroRefused(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	req := e.updateReq(res)
	req.Payment = vo.NewFlexString("0.001")
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeOutOfRange)

	create := e.createReq()
	create.Payment = vo.NewFlexString("0.004")
	_, err = e.svc.CreateDebt(context.Background(), e.uid(), create)
	requireCode(t, err, errs.CodeOutOfRange)
}

// A payment after a zero snapshot reopens the debt; marking it paid off again
// must then write a new zero snapshot rather than do nothing.
func TestMarkPaidOff_ReopenedDebtCanBeMarkedAgain(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	if _, err := e.svc.MarkDebtPaidOff(ctx, e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id}); err != nil {
		t.Fatal(err)
	}
	e.expense(t, e.account, march10.AddDate(0, 0, 2), "50", "")
	e.clock.now = e.clock.now.AddDate(0, 0, 5)
	reopened, err := e.svc.GetDebt(ctx, e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil || reopened.Item.IsPaidOff != 0 {
		t.Fatalf("after a later payment: %+v, %v; want reopened", reopened, err)
	}
	got, err := e.svc.MarkDebtPaidOff(ctx, e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsPaidOff != 1 || len(e.snapshots(t, res.Id)) != 3 {
		t.Fatalf("second mark: %+v, %d snapshots; want paid off with a new zero snapshot", got, len(e.snapshots(t, res.Id)))
	}
}

// An edit that makes a debt cover its whole category takes the category's
// plans, as creating it that way would; narrowing it back keeps them.
func TestUpdateDebt_BecomingWholeCategoryTakesItsPlans(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	payee := e.newPayee()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.PayeeId = &payee })
	e.budget.transfers = nil

	req := e.updateReq(res)
	req.PayeeId = nil
	whole, err := e.svc.UpdateDebt(ctx, e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	want := transferCall{from: e.category, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt, currency: fixture.USD,
		sum: true, first: "2026-03-10", last: "2027-03-01"}
	if len(e.budget.transfers) != 1 || e.budget.transfers[0] != want {
		t.Fatalf("transfers = %+v, want [%+v]", e.budget.transfers, want)
	}

	e.budget.transfers = nil
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), e.updateReq(whole)); err != nil {
		t.Fatal(err)
	}
	narrowed := e.updateReq(whole)
	narrowed.PayeeId = &payee
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), narrowed); err != nil {
		t.Fatal(err)
	}
	if len(e.budget.transfers) != 0 {
		t.Fatalf("an edit that keeps or narrows the key moved data: %+v", e.budget.transfers)
	}
}

func (e *env) eur() string {
	return e.f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
}

// A set-up debt's currency changes without its terms: the stored numbers are
// read in the new currency, no snapshot is written, the budget rows follow,
// and the payments are split again from their amounts in the new currency.
func TestUpdateDebt_CurrencyChangeResplitsPayments(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	eur := e.eur()
	res := e.create(t, nil)
	e.clock.now = ymd(2026, 4, 10)
	// below the period's interest, so the split takes all of it
	tx := e.expense(t, e.account, ymd(2026, 4, 1), "50", "")
	e.sync(t)
	if got := e.splits(t, res.Id)[tx]; got != vo.NewDecimal("50").String() {
		t.Fatalf("interest before = %q, want 50", got)
	}

	e.rates.multiplier[fixture.USD] = 2
	req := e.updateReq(res)
	req.CurrencyId = eur
	req.Balance, req.Payment, req.Schedule, req.LastPaymentDate = vo.FlexString{}, vo.FlexString{}, "", ""
	got, err := e.svc.UpdateDebt(ctx, e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrencyId != eur || got.Balance != "10000" || got.Payment != "1000" {
		t.Fatalf("result = %+v", got)
	}
	if n := len(e.snapshots(t, res.Id)); n != 1 {
		t.Errorf("a currency change wrote a snapshot: %d", n)
	}
	if len(e.budget.currencies) != 1 || e.budget.currencies[0] != eur {
		t.Errorf("budget rows moved to %v, want [%s]", e.budget.currencies, eur)
	}
	details, err := e.svc.GetDebt(ctx, e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Payments) != 1 {
		t.Fatalf("payments = %+v", details.Payments)
	}
	p := details.Payments[0]
	if p.Amount != "100" || p.Interest != "100" {
		t.Errorf("payment = %+v, want 100 split again as 100 interest", p)
	}
}

func TestUpdateDebt_UnchangedCurrencyTouchesNothing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	tx := e.expense(t, e.account, ymd(2026, 3, 1), "50", "")
	e.sync(t)
	before := e.splits(t, res.Id)[tx]
	// a currency no longer usable stays on the debt that holds it
	e.currency.unusable[fixture.USD] = true

	for _, cur := range []string{fixture.USD, ""} {
		req := e.updateReq(res)
		req.CurrencyId = cur
		got, err := e.svc.UpdateDebt(ctx, e.uid(), req)
		if err != nil {
			t.Fatalf("currency %q: %v", cur, err)
		}
		if got.CurrencyId != fixture.USD {
			t.Errorf("currency %q: result currency %s", cur, got.CurrencyId)
		}
	}
	if len(e.budget.currencies) != 0 {
		t.Errorf("budget rows moved: %v", e.budget.currencies)
	}
	if got := e.splits(t, res.Id)[tx]; got != before {
		t.Errorf("split %q -> %q", before, got)
	}
}

func TestUpdateDebt_RefusesAnUnusableCurrency(t *testing.T) {
	e := newEnv(t)
	eur := e.eur()
	res := e.create(t, nil)
	e.currency.unusable[eur] = true
	req := e.updateReq(res)
	req.CurrencyId = eur
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeCurrencyNotAvailable)
}

// The payment of a debt not set up is in the currency it moves to, rounded to
// that currency's digits.
func TestUpdateDebt_CurrencyChangeOfADebtNotSetUp(t *testing.T) {
	e := newEnv(t)
	jpy := e.f.Currency(fixture.Currency{Code: "JPY", Symbol: "¥", Name: "Yen"})
	e.rates.digitsOf = map[string]int{jpy: 0}
	res := e.createWithoutTerms(t, nil)
	req := e.updateReq(res)
	req.CurrencyId = jpy
	req.Payment = vo.NewFlexString("1500.6")
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrencyId != jpy || got.Payment != "1501" || got.IsSetUp != 0 {
		t.Fatalf("result = %+v", got)
	}
	if len(e.budget.currencies) != 1 || e.budget.currencies[0] != jpy {
		t.Errorf("budget rows moved to %v", e.budget.currencies)
	}
}

// Moving to a currency with fewer digits, the stored terms sent back (or a
// payment that only differs by that rounding) are no change of terms: no
// snapshot as of today undoes the payments made since the stored one.
func TestUpdateDebt_CurrencyChangeRoundingIsNoChangeOfTerms(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	jpy := e.f.Currency(fixture.Currency{Code: "JPY", Symbol: "¥", Name: "Yen"})
	e.rates.digitsOf = map[string]int{jpy: 0}
	res := e.create(t, func(r *model.CreateDebtRequest) {
		r.Balance, r.Payment = vo.NewFlexString("10000.50"), vo.NewFlexString("1000.25")
	})
	e.clock.now = ymd(2026, 4, 10)

	req := e.updateReq(res)
	req.CurrencyId = jpy
	got, err := e.svc.UpdateDebt(ctx, e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(e.snapshots(t, res.Id)); n != 1 {
		t.Fatalf("the stored terms in the new currency wrote a snapshot: %d", n)
	}
	if got.CurrencyId != jpy || got.Balance != "10000.5" || got.Payment != "1000.25" {
		t.Fatalf("result = %+v", got)
	}

	back := e.updateReq(res)
	back.CurrencyId = fixture.USD
	back.Balance, back.LastPaymentDate, back.Schedule = vo.FlexString{}, "", ""
	back.Payment = vo.NewFlexString("1000.25")
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), back); err != nil {
		t.Fatalf("a payment equal to the stored one: %v", err)
	}
	if n := len(e.snapshots(t, res.Id)); n != 1 {
		t.Fatalf("snapshots = %d", n)
	}
}

func (b *fakeBudget) indexOf(call string) int {
	for i, c := range b.calls {
		if c == call {
			return i
		}
	}
	return -1
}

// Finishing setup of a whole-category debt in the same save as a currency
// change hands the months after its end back in the currency the rows are
// still in, before they switch.
func TestUpdateDebt_CurrencyChangeAtSetupMovesTheTailInTheOldCurrency(t *testing.T) {
	e := newEnv(t)
	eur := e.eur()
	res := e.createWithoutTerms(t, nil)
	e.budget.calls, e.budget.transfers = nil, nil

	req := e.updateReq(res)
	req.CurrencyId = eur
	req.Balance, req.Payment, req.LastPaymentDate = vo.NewFlexString("9000"), vo.NewFlexString("900"), "2027-04-01"
	if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatal(err)
	}
	if len(e.budget.transfers) != 1 {
		t.Fatalf("transfers = %+v", e.budget.transfers)
	}
	tr := e.budget.transfers[0]
	if tr.fromType != model.ElementDebt || tr.to != e.category || tr.currency != fixture.USD || tr.after == "" {
		t.Errorf("tail transfer = %+v, want debt -> category in USD", tr)
	}
	if ti, ci := e.budget.indexOf("transfer"), e.budget.indexOf("currency"); ti < 0 || ci < 0 || ti > ci {
		t.Errorf("calls = %v, want the transfer before the rows' currency", e.budget.calls)
	}
}

func TestUpdateDebt_CurrencyChangeWithALaterStartMovesTheHeadInTheOldCurrency(t *testing.T) {
	e := newEnv(t)
	eur := e.eur()
	res := e.create(t, nil)
	e.budget.calls, e.budget.transfers = nil, nil

	req := e.updateReq(res)
	req.CurrencyId = eur
	req.StartDate = ptr("2026-06-01")
	if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatal(err)
	}
	if len(e.budget.transfers) != 1 {
		t.Fatalf("transfers = %+v", e.budget.transfers)
	}
	tr := e.budget.transfers[0]
	if tr.fromType != model.ElementDebt || tr.to != e.category || tr.currency != fixture.USD || tr.before == "" {
		t.Errorf("head transfer = %+v, want debt -> category in USD", tr)
	}
	if ti, ci := e.budget.indexOf("transfer"), e.budget.indexOf("currency"); ti < 0 || ci < 0 || ti > ci {
		t.Errorf("calls = %v, want the transfer before the rows' currency", e.budget.calls)
	}
}
