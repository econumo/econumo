package model

import (
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func validDebtRequest() CreateDebtRequest {
	return CreateDebtRequest{
		Name: "Car loan", CurrencyId: "dffc2a06-6f29-4704-8575-31709adee926", CategoryId: "0197c8b2-1111-7000-8000-000000000001",
		Balance: vo.NewFlexString("10000"), Payment: vo.NewFlexString("1000"), Schedule: "monthly", LastPaymentDate: "2027-03-01",
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
	zeroBalance := validDebtRequest()
	zeroBalance.Balance = vo.NewFlexString("0")
	if err := zeroBalance.Validate(); err != nil {
		t.Fatalf("a zero balance is allowed: %v", err)
	}

	bad := validDebtRequest()
	bad.Name, bad.CurrencyId, bad.CategoryId = " ", "", "nope"
	bad.PayeeId, bad.StartDate = ptrTo("x"), ptrTo("2026-13-01")
	bad.Balance, bad.Payment = vo.NewFlexString("-1"), vo.NewFlexString("0")
	bad.Schedule, bad.LastPaymentDate = "daily", "03/01/2027"
	got := debtFieldCodes(t, bad.Validate())
	want := map[string]string{
		"name": errs.CodeIsBlank, "currencyId": errs.CodeIsBlank, "categoryId": errs.CodeInvalidUUID, "payeeId": errs.CodeInvalidUUID,
		"startDate": errs.CodeInvalidFormat, "balance": errs.CodeOutOfRange, "payment": errs.CodeOutOfRange,
		"schedule": errs.CodeInvalidChoice, "lastPaymentDate": errs.CodeInvalidFormat,
	}
	for k, code := range want {
		if got[k] != code {
			t.Errorf("%s: code %q, want %q (all: %v)", k, got[k], code, got)
		}
	}

	blank := validDebtRequest()
	blank.Balance, blank.Payment, blank.Schedule, blank.LastPaymentDate = vo.FlexString{}, vo.FlexString{}, "", ""
	got = debtFieldCodes(t, blank.Validate())
	for _, k := range []string{"payment", "schedule"} {
		if got[k] != errs.CodeIsBlank {
			t.Errorf("%s: code %q, want blank", k, got[k])
		}
	}
	for _, k := range []string{"balance", "lastPaymentDate"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s: the terms are optional together, got %v", k, got)
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
	up := UpdateDebtRequest{Id: "", Name: "n", CategoryId: "0197c8b2-1111-7000-8000-000000000001",
		Balance: vo.NewFlexString("1"), Payment: vo.NewFlexString("1"), Schedule: "weekly", LastPaymentDate: "2027-01-01"}
	if got := debtFieldCodes(t, up.Validate()); got["id"] != errs.CodeIsBlank {
		t.Errorf("update without an id: %v", got)
	}
	up.Id = "0197c8b2-1111-7000-8000-000000000002"
	if err := up.Validate(); err != nil {
		t.Errorf("update needs no currency: %v", err)
	}
}

func ptrTo(s string) *string { return &s }

// The amount is stored as a decimal: forms ParseFloat takes but the decimal
// type cannot read would be stored as 0.
func TestCreateDebtRequest_RejectsNonDecimalAmounts(t *testing.T) {
	for _, v := range []string{"1_000", "0x1p3", "1e3", "Inf"} {
		req := validDebtRequest()
		req.Balance = vo.NewFlexString(v)
		if got := debtFieldCodes(t, req.Validate()); got["balance"] != errs.CodeInvalidFormat {
			t.Errorf("balance %q: code %q, want invalid format", v, got["balance"])
		}
	}
	for _, v := range []string{"1000", "1000.5", ".5", "0"} {
		req := validDebtRequest()
		req.Balance = vo.NewFlexString(v)
		if err := req.Validate(); err != nil {
			t.Errorf("balance %q: %v", v, err)
		}
	}
}

func TestCreateDebtRequest_TermsPair(t *testing.T) {
	balanceOnly := validDebtRequest()
	balanceOnly.LastPaymentDate = ""
	if got := debtFieldCodes(t, balanceOnly.Validate()); got["lastPaymentDate"] != errs.CodeIsBlank || len(got) != 1 {
		t.Errorf("balance without lastPaymentDate: %v, want only lastPaymentDate blank", got)
	}
	dateOnly := validDebtRequest()
	dateOnly.Balance = vo.FlexString{}
	if got := debtFieldCodes(t, dateOnly.Validate()); got["balance"] != errs.CodeIsBlank || len(got) != 1 {
		t.Errorf("lastPaymentDate without balance: %v, want only balance blank", got)
	}
	neither := validDebtRequest()
	neither.Balance, neither.LastPaymentDate = vo.FlexString{}, ""
	if err := neither.Validate(); err != nil {
		t.Errorf("no terms is valid: %v", err)
	}
	noPayment := neither
	noPayment.Payment = vo.FlexString{}
	if got := debtFieldCodes(t, noPayment.Validate()); got["payment"] != errs.CodeIsBlank {
		t.Errorf("payment missing: %v", got)
	}
	noSchedule := neither
	noSchedule.Schedule = ""
	if got := debtFieldCodes(t, noSchedule.Validate()); got["schedule"] != errs.CodeIsBlank {
		t.Errorf("schedule missing: %v", got)
	}
	badLabel := validDebtRequest()
	badLabel.LabelId = ptrTo("x")
	if got := debtFieldCodes(t, badLabel.Validate()); got["labelId"] != errs.CodeInvalidUUID {
		t.Errorf("bad labelId: %v", got)
	}
}

func TestUpdateDebtRequest_TermsPair(t *testing.T) {
	up := UpdateDebtRequest{Id: "0197c8b2-1111-7000-8000-000000000002", Name: "n", CategoryId: "0197c8b2-1111-7000-8000-000000000001",
		Payment: vo.NewFlexString("1"), Schedule: "weekly"}
	if err := up.Validate(); err != nil {
		t.Errorf("update without terms is valid: %v", err)
	}
	up.Balance = vo.NewFlexString("10")
	if got := debtFieldCodes(t, up.Validate()); got["lastPaymentDate"] != errs.CodeIsBlank {
		t.Errorf("balance without lastPaymentDate: %v", got)
	}
}

// An update may leave out the payment and the schedule (kept as stored), but
// not alongside a balance and a last payment date.
func TestUpdateDebtRequest_PaymentAndScheduleOptional(t *testing.T) {
	up := UpdateDebtRequest{Id: "0197c8b2-1111-7000-8000-000000000002", Name: "n", CategoryId: "0197c8b2-1111-7000-8000-000000000001"}
	if err := up.Validate(); err != nil {
		t.Errorf("update without payment, schedule or terms is valid: %v", err)
	}
	bad := up
	bad.Payment, bad.Schedule = vo.NewFlexString("0"), "daily"
	if got := debtFieldCodes(t, bad.Validate()); got["payment"] != errs.CodeOutOfRange || got["schedule"] != errs.CodeInvalidChoice {
		t.Errorf("a payment or schedule sent is still checked: %v", got)
	}
	terms := up
	terms.Balance, terms.LastPaymentDate = vo.NewFlexString("10"), "2027-03-01"
	if got := debtFieldCodes(t, terms.Validate()); got["payment"] != errs.CodeIsBlank || got["schedule"] != errs.CodeIsBlank || len(got) != 2 {
		t.Errorf("terms without payment and schedule: %v, want both blank", got)
	}
}

// A debt names a tag or a reporting tag, never both.
func TestDebtRequests_RefuseATagAndALabelTogether(t *testing.T) {
	tag, label := "0197c8b2-1111-7000-8000-000000000002", "0197c8b2-1111-7000-8000-000000000003"
	create := validDebtRequest()
	create.TagId, create.LabelId = &tag, &label
	if got := debtFieldCodes(t, create.Validate()); got["labelId"] != errs.CodeInvalidChoice {
		t.Errorf("create: %v, want labelId %s", got, errs.CodeInvalidChoice)
	}
	update := UpdateDebtRequest{Id: "0197c8b2-1111-7000-8000-000000000009", Name: "Car loan", CategoryId: create.CategoryId,
		TagId: &tag, LabelId: &label}
	if got := debtFieldCodes(t, update.Validate()); got["labelId"] != errs.CodeInvalidChoice {
		t.Errorf("update: %v, want labelId %s", got, errs.CodeInvalidChoice)
	}
	empty := ""
	create.LabelId = &empty
	if err := create.Validate(); err != nil {
		t.Errorf("an empty label is none: %v", err)
	}
}

func TestUpdateDebtRequest_ValidatesAChangedCurrency(t *testing.T) {
	update := UpdateDebtRequest{Id: "0197c8b2-1111-7000-8000-000000000009", Name: "Car loan",
		CategoryId: "0197c8b2-1111-7000-8000-000000000001", CurrencyId: "nope"}
	if got := debtFieldCodes(t, update.Validate()); got["currencyId"] != errs.CodeInvalidUUID {
		t.Errorf("%v, want currencyId %s", got, errs.CodeInvalidUUID)
	}
	update.CurrencyId = ""
	if err := update.Validate(); err != nil {
		t.Errorf("no currency keeps the stored one: %v", err)
	}
}
