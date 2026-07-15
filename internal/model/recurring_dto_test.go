package model_test

import (
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func fieldKeys(err error) []string {
	v, ok := errs.AsValidation(err)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(v.Fields))
	for _, f := range v.Fields {
		keys = append(keys, f.Key)
	}
	return keys
}

func TestCreateRecurringTransactionRequest_Validate(t *testing.T) {
	err := model.CreateRecurringTransactionRequest{}.Validate()
	keys := strings.Join(fieldKeys(err), ",")
	for _, want := range []string{"id", "type", "amount", "accountId", "schedule", "nextPaymentAt"} {
		if !strings.Contains(keys, want) {
			t.Fatalf("missing field error %q in %q", want, keys)
		}
	}

	ok := model.CreateRecurringTransactionRequest{
		Id: "0197b7e0-0000-7000-8000-000000000001", Type: "expense", Amount: "50",
		AccountId: "0197b7e0-0000-7000-8000-000000000002",
		Schedule:  "monthly", NextPaymentAt: "2026-08-01 00:00:00",
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	bad := ok
	bad.NextPaymentAt = "2026-08-01"
	badErr := bad.Validate()
	if badErr == nil {
		t.Fatal("date without time must be rejected")
	}
	v, isValidation := errs.AsValidation(badErr)
	if !isValidation {
		t.Fatalf("expected ValidationError, got %T", badErr)
	}
	found := false
	for _, f := range v.Fields {
		if f.Key == "nextPaymentAt" {
			found = true
			if f.Message != "This value is not a valid datetime." {
				t.Fatalf("nextPaymentAt message = %q, want %q", f.Message, "This value is not a valid datetime.")
			}
		}
	}
	if !found {
		t.Fatal("missing nextPaymentAt field error for bad datetime")
	}
}

func TestPostRecurringTransactionRequest_Validate(t *testing.T) {
	err := model.PostRecurringTransactionRequest{}.Validate()
	keys := strings.Join(fieldKeys(err), ",")
	for _, want := range []string{"recurringId", "id", "type", "amount", "accountId", "date"} {
		if !strings.Contains(keys, want) {
			t.Fatalf("missing field error %q in %q", want, keys)
		}
	}
}
