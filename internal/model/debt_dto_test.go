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
		AmountBorrowed: vo.NewFlexString("12000"), StartDate: ptrTo("2026-03-01"),
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
	owedNow := validDebtRequest()
	owedNow.Balance = vo.NewFlexString("0")
	if err := owedNow.Validate(); err != nil {
		t.Fatalf("a zero balance now is allowed: %v", err)
	}

	bad := validDebtRequest()
	bad.Name, bad.CurrencyId, bad.CategoryId = " ", "", "nope"
	bad.PayeeId, bad.StartDate = ptrTo("x"), ptrTo("2026-13-01")
	bad.AmountBorrowed, bad.Balance, bad.Payment = vo.NewFlexString("0"), vo.NewFlexString("-1"), vo.NewFlexString("0")
	bad.Schedule, bad.LastPaymentDate = "daily", "03/01/2027"
	got := debtFieldCodes(t, bad.Validate())
	want := map[string]string{
		"name": errs.CodeIsBlank, "currencyId": errs.CodeIsBlank, "categoryId": errs.CodeInvalidUUID, "payeeId": errs.CodeInvalidUUID,
		"startDate": errs.CodeInvalidFormat, "amountBorrowed": errs.CodeOutOfRange, "balance": errs.CodeOutOfRange,
		"payment": errs.CodeOutOfRange, "schedule": errs.CodeInvalidChoice, "lastPaymentDate": errs.CodeInvalidFormat,
	}
	for k, code := range want {
		if got[k] != code {
			t.Errorf("%s: code %q, want %q (all: %v)", k, got[k], code, got)
		}
	}

	blank := validDebtRequest()
	blank.AmountBorrowed, blank.StartDate, blank.Payment, blank.Schedule, blank.LastPaymentDate = vo.FlexString{}, nil, vo.FlexString{}, "", ""
	got = debtFieldCodes(t, blank.Validate())
	for _, k := range []string{"payment", "schedule"} {
		if got[k] != errs.CodeIsBlank {
			t.Errorf("%s: code %q, want blank", k, got[k])
		}
	}
	for _, k := range []string{"amountBorrowed", "startDate", "lastPaymentDate", "balance"} {
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
	up := UpdateDebtRequest{Id: "", Name: "n", CategoryId: "0197c8b2-1111-7000-8000-000000000001", AmountBorrowed: vo.NewFlexString("1"),
		StartDate: ptrTo("2026-01-01"), Payment: vo.NewFlexString("1"), Schedule: "weekly", LastPaymentDate: "2027-01-01"}
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
		req.AmountBorrowed, req.Balance = vo.NewFlexString(v), vo.NewFlexString(v)
		got := debtFieldCodes(t, req.Validate())
		if got["amountBorrowed"] != errs.CodeInvalidFormat || got["balance"] != errs.CodeInvalidFormat {
			t.Errorf("%q: codes %v, want invalid format on both", v, got)
		}
	}
	for _, v := range []string{"1000", "1000.5", ".5"} {
		req := validDebtRequest()
		req.AmountBorrowed, req.Balance = vo.NewFlexString(v), vo.NewFlexString(v)
		if err := req.Validate(); err != nil {
			t.Errorf("%q: %v", v, err)
		}
	}
}

// The loan's facts (amount borrowed, start date, last payment date) come
// together; the balance now is optional on top of them.
func TestCreateDebtRequest_SetupTrio(t *testing.T) {
	for name, tc := range map[string]struct {
		mod   func(*CreateDebtRequest)
		blank []string
	}{
		"no amount borrowed": {func(r *CreateDebtRequest) { r.AmountBorrowed = vo.FlexString{} }, []string{"amountBorrowed"}},
		"no last payment":    {func(r *CreateDebtRequest) { r.LastPaymentDate = "" }, []string{"lastPaymentDate"}},
		"no start date":      {func(r *CreateDebtRequest) { r.StartDate = nil }, []string{"startDate"}},
		"blank start date":   {func(r *CreateDebtRequest) { r.StartDate = ptrTo(" ") }, []string{"startDate"}},
		"balance alone": {func(r *CreateDebtRequest) {
			r.AmountBorrowed, r.StartDate, r.LastPaymentDate, r.Balance = vo.FlexString{}, nil, "", vo.NewFlexString("5")
		}, []string{"amountBorrowed", "startDate", "lastPaymentDate"}},
	} {
		req := validDebtRequest()
		tc.mod(&req)
		got := debtFieldCodes(t, req.Validate())
		if len(got) != len(tc.blank) {
			t.Errorf("%s: %v, want only %v blank", name, got, tc.blank)
		}
		for _, k := range tc.blank {
			if got[k] != errs.CodeIsBlank {
				t.Errorf("%s: %s code %q, want blank (%v)", name, k, got[k], got)
			}
		}
	}
	// a start date alone only bounds which payments count
	startOnly := validDebtRequest()
	startOnly.AmountBorrowed, startOnly.LastPaymentDate = vo.FlexString{}, ""
	if err := startOnly.Validate(); err != nil {
		t.Errorf("a start date without terms is valid: %v", err)
	}
	neither := startOnly
	neither.StartDate = nil
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

func TestUpdateDebtRequest_SetupTrio(t *testing.T) {
	up := UpdateDebtRequest{Id: "0197c8b2-1111-7000-8000-000000000002", Name: "n", CategoryId: "0197c8b2-1111-7000-8000-000000000001",
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
	terms.AmountBorrowed, terms.StartDate, terms.LastPaymentDate = vo.NewFlexString("10"), ptrTo("2026-01-01"), "2027-03-01"
	if err := terms.Validate(); err != nil {
		t.Errorf("terms without payment and schedule keep the stored ones: %v", err)
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

func TestDebtRequests_BalanceDate(t *testing.T) {
	create := validDebtRequest()
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
