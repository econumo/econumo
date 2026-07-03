package account

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func mustID(t *testing.T, s string) vo.Id {
	t.Helper()
	v, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("parse id %q: %v", s, err)
	}
	return v
}

var (
	ta0 = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	ta1 = time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
)

func TestType_Values(t *testing.T) {
	if TypeCash != 1 || TypeCreditCard != 2 {
		t.Fatalf("type values drifted: cash=%d cc=%d", TypeCash, TypeCreditCard)
	}
	if TypeCash.Int16() != 1 || TypeCreditCard.Int16() != 2 {
		t.Fatal("Int16 mismatch")
	}
	if !TypeCash.Valid() || !TypeCreditCard.Valid() {
		t.Fatal("cash/credit-card must be valid")
	}
	if Type(0).Valid() || Type(3).Valid() {
		t.Fatal("0 and 3 must be invalid types")
	}
}

func newAcct(t *testing.T) *Account {
	return NewAccount(
		mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"),
		mustID(t, "33333333-3333-3333-3333-333333333333"),
		"Cash", "wallet", ta0)
}

func TestNewAccount_AlwaysCreditCard(t *testing.T) {
	a := newAcct(t)
	if a.Type != TypeCreditCard {
		t.Fatalf("new account type=%d want CREDIT_CARD(2)", a.Type)
	}
	if a.IsDeleted {
		t.Fatal("new account must not be deleted")
	}
}

func TestAccount_Delete_OnlyBumpsOnChange(t *testing.T) {
	a := newAcct(t)
	a.Delete(ta1)
	if !a.IsDeleted || !a.UpdatedAt.Equal(ta1) {
		t.Fatalf("delete: deleted=%v updatedAt=%v", a.IsDeleted, a.UpdatedAt)
	}
	// second delete is a no-op.
	a.Delete(time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC))
	if !a.UpdatedAt.Equal(ta1) {
		t.Fatalf("re-delete bumped updatedAt to %v", a.UpdatedAt)
	}
}

func TestAccount_UpdateCurrency_OnlyBumpsOnChange(t *testing.T) {
	a := newAcct(t)
	same := mustID(t, "33333333-3333-3333-3333-333333333333")
	other := mustID(t, "44444444-4444-4444-4444-444444444444")
	a.UpdateCurrency(same, ta1)
	if !a.UpdatedAt.Equal(ta0) {
		t.Fatal("same-currency update bumped updatedAt")
	}
	a.UpdateCurrency(other, ta1)
	if !a.CurrencyID.Equal(other) || !a.UpdatedAt.Equal(ta1) {
		t.Fatalf("currency change: %v / %v", a.CurrencyID, a.UpdatedAt)
	}
}

func TestAccount_UpdateIcon_OnlyBumpsOnChange(t *testing.T) {
	a := newAcct(t)
	a.UpdateIcon("wallet", ta1)
	if !a.UpdatedAt.Equal(ta0) {
		t.Fatal("same-icon update bumped updatedAt")
	}
	a.UpdateIcon("bank", ta1)
	if a.Icon != "bank" || !a.UpdatedAt.Equal(ta1) {
		t.Fatalf("icon change: %q / %v", a.Icon, a.UpdatedAt)
	}
}
