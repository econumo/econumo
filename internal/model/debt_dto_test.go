package model

import (
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func validDebtRequest() CreateDebtRequest {
	return CreateDebtRequest{
		BudgetId: "0197c8b2-1111-7000-8000-0000000000b1", ElementId: "0197c8b2-1111-7000-8000-000000000001",
		StartDate: ptrTo("2026-03-01"), Payment: vo.NewFlexString("1000"), Schedule: "monthly",
	}
}

func validUpdateRequest() UpdateDebtRequest {
	return UpdateDebtRequest{
		Id: "0197c8b2-1111-7000-8000-000000000002", AmountBorrowed: vo.NewFlexString("12000"), StartDate: ptrTo("2026-03-01"),
		Payment: vo.NewFlexString("1000"), Schedule: "monthly", LastPaymentDate: "2027-03-01",
	}
}

func debtFieldCodes(t *testing.T, err error) map[string]string {
	t.Helper()
	var v *errs.ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("err = %v, want a validation error", err)
	}
	out := map[string]string{}
	for _, f := range v.Fields {
		out[f.Key] = f.Code
	}
	return out
}

func TestCreateDebtRequest_Validate(t *testing.T) {
	if err := validDebtRequest().Validate(); err != nil {
		t.Fatalf("valid request: %v", err)
	}
	noStart := validDebtRequest()
	noStart.StartDate = nil
	if err := noStart.Validate(); err != nil {
		t.Fatalf("the start date is optional: %v", err)
	}

	bad := validDebtRequest()
	bad.BudgetId, bad.ElementId, bad.StartDate = "", "nope", ptrTo("2026-13-01")
	bad.Payment, bad.Schedule = vo.NewFlexString("0"), "daily"
	got := debtFieldCodes(t, bad.Validate())
	want := map[string]string{
		"budgetId": errs.CodeIsBlank, "elementId": errs.CodeInvalidUUID, "startDate": errs.CodeInvalidFormat,
		"payment": errs.CodeOutOfRange, "schedule": errs.CodeInvalidChoice,
	}
	for k, code := range want {
		if got[k] != code {
			t.Errorf("%s: code %q, want %q (all: %v)", k, got[k], code, got)
		}
	}

	blank := CreateDebtRequest{}
	got = debtFieldCodes(t, blank.Validate())
	for _, k := range []string{"budgetId", "elementId", "payment", "schedule"} {
		if got[k] != errs.CodeIsBlank {
			t.Errorf("%s: code %q, want blank (%v)", k, got[k], got)
		}
	}
}

func TestDebtRequests_ValidateIds(t *testing.T) {
	if err := (DeleteDebtRequest{}).Validate(); err == nil {
		t.Error("delete without an id should fail")
	}
	if err := (MarkDebtPaidOffRequest{Id: "x"}).Validate(); err == nil {
		t.Error("mark paid off with a bad id should fail")
	}
	if err := (GetDebtRequest{Id: "0197c8b2-1111-7000-8000-000000000001"}).Validate(); err != nil {
		t.Errorf("get with a valid id: %v", err)
	}
	up := validUpdateRequest()
	up.Id = ""
	if got := debtFieldCodes(t, up.Validate()); got["id"] != errs.CodeIsBlank {
		t.Errorf("update without an id: %v", got)
	}
	if got := debtFieldCodes(t, (MarkDebtPaidOffRequest{Id: "0197c8b2-1111-7000-8000-000000000001", Date: "soon"}).Validate()); got["date"] != errs.CodeInvalidFormat {
		t.Errorf("mark paid off with a bad date: %v", got)
	}
	if err := (MarkDebtPaidOffRequest{Id: "0197c8b2-1111-7000-8000-000000000001"}).Validate(); err != nil {
		t.Errorf("no date is today: %v", err)
	}
	if err := (ReopenDebtRequest{Id: "x"}).Validate(); err == nil {
		t.Error("reopen with a bad id should fail")
	}
}

func ptrTo(s string) *string { return &s }

// The amount is stored as a decimal: forms ParseFloat takes but the decimal
// type cannot read would be stored as 0.
func TestUpdateDebtRequest_RejectsNonDecimalAmounts(t *testing.T) {
	for _, v := range []string{"1_000", "0x1p3", "1e3", "Inf"} {
		req := validUpdateRequest()
		req.AmountBorrowed, req.Balance = vo.NewFlexString(v), vo.NewFlexString(v)
		got := debtFieldCodes(t, req.Validate())
		if got["amountBorrowed"] != errs.CodeInvalidFormat || got["balance"] != errs.CodeInvalidFormat {
			t.Errorf("%q: codes %v, want invalid format on both", v, got)
		}
	}
	for _, v := range []string{"1000", "1000.5", ".5"} {
		req := validUpdateRequest()
		req.AmountBorrowed, req.Balance = vo.NewFlexString(v), vo.NewFlexString(v)
		if err := req.Validate(); err != nil {
			t.Errorf("%q: %v", v, err)
		}
	}
}

func TestUpdateDebtRequest_SetupTrio(t *testing.T) {
	up := UpdateDebtRequest{Id: "0197c8b2-1111-7000-8000-000000000002",
		Payment: vo.NewFlexString("1"), Schedule: "weekly"}
	if err := up.Validate(); err != nil {
		t.Errorf("update without terms is valid: %v", err)
	}
	// a set-up debt takes a balance now on its own
	owed := up
	owed.Balance = vo.NewFlexString("10")
	if err := owed.Validate(); err != nil {
		t.Errorf("a balance now alone is valid on an update: %v", err)
	}
	half := up
	half.AmountBorrowed = vo.NewFlexString("10")
	if got := debtFieldCodes(t, half.Validate()); got["lastPaymentDate"] != errs.CodeIsBlank {
		t.Errorf("amount borrowed without the last payment date: %v", got)
	} else if _, ok := got["startDate"]; ok {
		t.Errorf("an update may leave the start date out: %v", got)
	}
	// a set-up debt's corrected facts without its unchanged start date
	facts := up
	facts.AmountBorrowed, facts.LastPaymentDate = vo.NewFlexString("10"), "2030-01-01"
	if err := facts.Validate(); err != nil {
		t.Errorf("facts without the start date are valid on an update: %v", err)
	}
}

// An update may leave out the payment and the schedule (kept as stored), even
// alongside the loan's facts.
func TestUpdateDebtRequest_PaymentAndScheduleOptional(t *testing.T) {
	up := UpdateDebtRequest{Id: "0197c8b2-1111-7000-8000-000000000002"}
	if err := up.Validate(); err != nil {
		t.Errorf("update without payment, schedule or terms is valid: %v", err)
	}
	bad := up
	bad.Payment, bad.Schedule = vo.NewFlexString("0"), "daily"
	if got := debtFieldCodes(t, bad.Validate()); got["payment"] != errs.CodeOutOfRange || got["schedule"] != errs.CodeInvalidChoice {
		t.Errorf("a payment or schedule sent is still checked: %v", got)
	}
	terms := up
	terms.AmountBorrowed, terms.StartDate, terms.LastPaymentDate = vo.NewFlexString("10"), ptrTo("2026-01-01"), "2027-03-01"
	if err := terms.Validate(); err != nil {
		t.Errorf("terms without payment and schedule keep the stored ones: %v", err)
	}
}

func TestDebtRequests_BalanceDate(t *testing.T) {
	create := validUpdateRequest()
	create.Balance, create.BalanceDate = vo.NewFlexString("100"), "2026-02-01"
	if err := create.Validate(); err != nil {
		t.Fatalf("a dated balance: %v", err)
	}
	create.BalanceDate = "1 Feb"
	if got := debtFieldCodes(t, create.Validate()); got["balanceDate"] != errs.CodeInvalidFormat {
		t.Errorf("bad balance date: %v", got)
	}
	create.Balance, create.BalanceDate = vo.FlexString{}, "2026-02-01"
	if got := debtFieldCodes(t, create.Validate()); got["balance"] != errs.CodeIsBlank {
		t.Errorf("a date without a balance: %v", got)
	}
}

func TestReconcileDebtRequest_Validate(t *testing.T) {
	ok := ReconcileDebtRequest{Id: "0197c8b2-1111-7000-8000-000000000001", Balance: vo.NewFlexString("0")}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid: %v", err)
	}
	bad := ReconcileDebtRequest{Id: "x", Balance: vo.NewFlexString("-1"), BalanceDate: "soon"}
	got := debtFieldCodes(t, bad.Validate())
	if got["id"] != errs.CodeInvalidUUID || got["balance"] != errs.CodeOutOfRange || got["balanceDate"] != errs.CodeInvalidFormat {
		t.Errorf("codes = %v", got)
	}
	if got := debtFieldCodes(t, (ReconcileDebtRequest{Id: ok.Id}).Validate()); got["balance"] != errs.CodeIsBlank {
		t.Errorf("no balance: %v", got)
	}
	del := DeleteDebtReconciliationRequest{Id: ok.Id}
	if got := debtFieldCodes(t, del.Validate()); got["reconciliationId"] != errs.CodeIsBlank {
		t.Errorf("delete without a reconciliation: %v", got)
	}
}

func TestAddDebtRateChangeRequest_Validate(t *testing.T) {
	id := "0197c8b2-1111-7000-8000-000000000001"
	ok := AddDebtRateChangeRequest{Id: id, Date: "2026-01-05", AnnualRate: vo.NewFlexString("5.5"), Payment: vo.NewFlexString("100")}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid: %v", err)
	}
	for _, bad := range []string{"NaN", "Inf", "1e1", "0x10", "-1", "101"} {
		r := ok
		r.AnnualRate = vo.NewFlexString(bad)
		if got := debtFieldCodes(t, r.Validate()); got["annualRate"] == "" {
			t.Errorf("annualRate %q accepted", bad)
		}
	}
	for _, bad := range []string{"NaN", "Inf", "1e1", "0x10", "0", "-1"} {
		r := ok
		r.Payment = vo.NewFlexString(bad)
		if got := debtFieldCodes(t, r.Validate()); got["payment"] == "" {
			t.Errorf("payment %q accepted", bad)
		}
	}
	if got := debtFieldCodes(t, (AddDebtRateChangeRequest{Id: id}).Validate()); got["date"] != errs.CodeIsBlank || got["annualRate"] != errs.CodeIsBlank || got["payment"] != errs.CodeIsBlank {
		t.Errorf("blank: %v", got)
	}
	del := DeleteDebtRateChangeRequest{Id: id}
	if got := debtFieldCodes(t, del.Validate()); got["rateChangeId"] != errs.CodeIsBlank {
		t.Errorf("delete without id: %v", got)
	}
}
