package transaction

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

func ptrID(t *testing.T, s string) *vo.Id {
	v := mustID(t, s)
	return &v
}

var (
	tc0 = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	tc1 = time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
)

func TestType_AliasAndPredicates(t *testing.T) {
	cases := []struct {
		typ                      Type
		alias                    string
		num                      int16
		isExp, isInc, isTransfer bool
	}{
		{TypeExpense, "expense", 0, true, false, false},
		{TypeIncome, "income", 1, false, true, false},
		{TypeTransfer, "transfer", 2, false, false, true},
	}
	for _, c := range cases {
		if c.typ.Alias() != c.alias {
			t.Errorf("%d.Alias()=%q want %q", c.typ, c.typ.Alias(), c.alias)
		}
		if c.typ.Int16() != c.num {
			t.Errorf("%s.Int16()=%d want %d", c.alias, c.typ.Int16(), c.num)
		}
		if c.typ.IsExpense() != c.isExp || c.typ.IsIncome() != c.isInc || c.typ.IsTransfer() != c.isTransfer {
			t.Errorf("%s predicates wrong: exp=%v inc=%v transfer=%v", c.alias, c.typ.IsExpense(), c.typ.IsIncome(), c.typ.IsTransfer())
		}
	}
}

func baseState(t *testing.T) NewState {
	return NewState{
		ID:     mustID(t, "11111111-1111-1111-1111-111111111111"),
		UserID: mustID(t, "22222222-2222-2222-2222-222222222222"),
		Type:   TypeExpense, AccountID: mustID(t, "33333333-3333-3333-3333-333333333333"),
		Amount: "42.50", CategoryID: ptrID(t, "44444444-4444-4444-4444-444444444444"),
		PayeeID:     ptrID(t, "55555555-5555-5555-5555-555555555555"),
		TagID:       ptrID(t, "66666666-6666-6666-6666-666666666666"),
		Description: "groceries", SpentAt: tc0, CreatedAt: tc0, UpdatedAt: tc0,
	}
}

func TestNew_CreatedEqualsUpdated(t *testing.T) {
	s := baseState(t)
	s.UpdatedAt = tc1 // New ignores UpdatedAt, uses CreatedAt for both.
	tx := New(s)
	if !tx.CreatedAt().Equal(tc0) || !tx.UpdatedAt().Equal(tc0) {
		t.Fatalf("New: created=%v updated=%v want both %v", tx.CreatedAt(), tx.UpdatedAt(), tc0)
	}
}

func TestUpdate_NonTransfer_KeepsMetadata_ClearsRecipient(t *testing.T) {
	tx := New(baseState(t))
	// Update to an income with a recipient set in the state -- non-transfer must
	// DROP recipient + keep category/payee/tag.
	s := baseState(t)
	s.Type = TypeIncome
	s.AccountRecipID = ptrID(t, "77777777-7777-7777-7777-777777777777")
	s.AmountRecipient = strptr("10")
	tx.Update(s, tc1)

	if tx.Type() != TypeIncome {
		t.Fatalf("type=%d want income", tx.Type())
	}
	if tx.AccountRecipientId() != nil || tx.AmountRecipient() != nil {
		t.Fatal("non-transfer must clear recipient account + amount")
	}
	if tx.CategoryId() == nil || tx.PayeeId() == nil || tx.TagId() == nil {
		t.Fatal("non-transfer must keep category/payee/tag")
	}
	if !tx.UpdatedAt().Equal(tc1) {
		t.Fatalf("updatedAt=%v want %v", tx.UpdatedAt(), tc1)
	}
}

func TestUpdate_Transfer_KeepsRecipient_ClearsMetadata(t *testing.T) {
	tx := New(baseState(t))
	s := baseState(t)
	s.Type = TypeTransfer
	s.AccountRecipID = ptrID(t, "77777777-7777-7777-7777-777777777777")
	s.AmountRecipient = strptr("40")
	// category/payee/tag still set in state -- transfer must DROP them.
	tx.Update(s, tc1)

	if !tx.Type().IsTransfer() {
		t.Fatalf("type=%d want transfer", tx.Type())
	}
	if tx.AccountRecipientId() == nil || tx.AmountRecipient() == nil {
		t.Fatal("transfer must keep recipient account + amount")
	}
	if *tx.AmountRecipient() != "40" {
		t.Fatalf("amountRecipient=%q want 40", *tx.AmountRecipient())
	}
	if tx.CategoryId() != nil || tx.PayeeId() != nil || tx.TagId() != nil {
		t.Fatal("transfer must clear category/payee/tag")
	}
}

func strptr(s string) *string { return &s }
