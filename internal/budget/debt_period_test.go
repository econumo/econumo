package budget_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// newShiftedDebtHarness is the attribution harness on a budget whose periods
// start on the 15th (key June = 15 Jun - 14 Jul; today, 2026-06-15, opens
// it), with a 13000 zero-interest loan taken on 2026-06-01, paid 1000 on the
// 10th of each month through 2027-06-10, and one payment already made on
// 2026-06-12, which is in period May (12000 owed now).
func newShiftedDebtHarness(t *testing.T) (*attrHarness, string) {
	t.Helper()
	h := newAttrHarness(t)
	if _, err := h.tdb.Raw.Exec(h.tdb.Rebind(`UPDATE budgets SET period_start_day = 15 WHERE id = ?`), h.budgetID); err != nil {
		t.Fatal(err)
	}
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC))
	start := "2026-06-01"
	res, err := h.debts.CreateDebt(h.ctx, h.owner, model.CreateDebtRequest{
		BudgetId: h.budgetID, Name: "Car loan", CurrencyId: fixture.USD, Rules: rules(h.loan),
		StartDate: &start, AmountBorrowed: vo.NewFlexString("13000"), Balance: vo.NewFlexString("12000"), Payment: vo.NewFlexString("1000"),
		Schedule: "monthly", LastPaymentDate: "2027-06-10",
	})
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	return h, res.Id
}

func TestShiftedPeriods_DebtPaidCountsInItsPeriod(t *testing.T) {
	h, debtID := newShiftedDebtHarness(t)

	may := debtRow(h.budgetAt(t, h.owner, "2026-05-01"), debtID)
	if may == nil || may.Spent != "1000" || may.IsPlannable != 1 {
		t.Fatalf("May (15 May - 14 Jun) row = %+v, want the 12 Jun payment and plannable", may)
	}
	june := debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID)
	if june == nil || june.Spent != "0" {
		t.Fatalf("June (15 Jun - 14 Jul) row = %+v, want nothing paid", june)
	}
	if june.Available != "-1000" {
		t.Errorf("June available = %s, want -1000 (May's unplanned payment carries)", june.Available)
	}
}

func TestShiftedPeriods_DebtScheduleFollowsPeriods(t *testing.T) {
	h, debtID := newShiftedDebtHarness(t)

	june := debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID)
	if june == nil {
		t.Fatal("no debt row in June")
	}
	// The 10 Jul due lies in period June; a calendar June would hold none of
	// the dues still to pay.
	if june.ScheduledTotal != "1000" || june.Upcoming != "1000" || june.ClosingBalance != "11000" || june.PaymentsLeft != 11 {
		t.Errorf("June = scheduled %s, still due %s, closing %s, %d left; want 1000, 1000, 11000, 11",
			june.ScheduledTotal, june.Upcoming, june.ClosingBalance, june.PaymentsLeft)
	}

	h.setLimit(t, debtID, "2026-06-01", "1500")
	june = debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID)
	if june.ClosingBalance != "10500" {
		t.Errorf("June closing with a 1500 plan = %s, want 10500 (500 extra on top of the 10 Jul due)", june.ClosingBalance)
	}
}

func TestShiftedPeriods_DebtPlanBoundsArePeriods(t *testing.T) {
	h, debtID := newShiftedDebtHarness(t)
	set := func(period string) error {
		a := vo.NewFlexString("100")
		_, err := h.svc.SetLimit(h.ctx, h.owner, model.SetLimitRequest{BudgetId: h.budgetID, ElementId: debtID, Period: period, Amount: &a})
		return err
	}
	// The first payment (12 Jun) is in period May, the last due (10 Jun 2027)
	// in period May 2027.
	for _, ok := range []string{"2026-05-01", "2027-05-01"} {
		if err := set(ok); err != nil {
			t.Errorf("SetLimit %s: %v, want accepted", ok, err)
		}
	}
	for _, out := range []string{"2026-04-01", "2027-06-01"} {
		err := set(out)
		ve, ok := errs.AsValidation(err)
		if !ok || len(ve.Fields) == 0 || ve.Fields[0].Code != errs.CodeBudgetDebtPeriodOutOfRange {
			t.Errorf("SetLimit %s = %v, want %s", out, err, errs.CodeBudgetDebtPeriodOutOfRange)
		}
	}
}

func TestShiftedPeriods_DebtPlanCellsAlign(t *testing.T) {
	h, debtID := newShiftedDebtHarness(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC))

	res := h.plan(t, "2026-04-01", "4")
	row := planDebtRow(res, debtID)
	if row == nil || len(row.Cells) != 4 {
		t.Fatalf("plan debt row = %+v", row)
	}
	type cell struct {
		actual, scheduled, upcoming string
		plannable                   int
	}
	// April is before the debt's life; the 9 Jul payment is in period June
	// (future-dated rows are upcoming, never actual), with the 10 Jul due.
	want := []cell{{"0", "0", "0", 0}, {"1000", "1000", "0", 1}, {"0", "1000", "1000", 1}, {"0", "1000", "1000", 1}}
	for i, c := range row.Cells {
		got := cell{c.Actual, c.ScheduledTotal, c.Upcoming, c.IsPlannable}
		if got != want[i] {
			t.Errorf("cell %s = %+v, want %+v", res.Item.Months[i], got, want[i])
		}
	}
	for _, el := range res.Item.Structure.Elements {
		if el.Id != h.loan {
			continue
		}
		for i, c := range el.Cells {
			if c.Actual != "0" {
				t.Errorf("loan category cell %d actual = %s, want 0 (the debt row has it)", i, c.Actual)
			}
		}
	}
}
