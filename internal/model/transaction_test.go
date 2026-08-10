package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

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
		typ                      TransactionType
		alias                    string
		num                      int16
		isExp, isInc, isTransfer bool
	}{
		{TransactionTypeExpense, "expense", 0, true, false, false},
		{TransactionTypeIncome, "income", 1, false, true, false},
		{TransactionTypeTransfer, "transfer", 2, false, false, true},
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
		Type:   TransactionTypeExpense, AccountID: mustID(t, "33333333-3333-3333-3333-333333333333"),
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
	if !tx.CreatedAt.Equal(tc0) || !tx.UpdatedAt.Equal(tc0) {
		t.Fatalf("New: created=%v updated=%v want both %v", tx.CreatedAt, tx.UpdatedAt, tc0)
	}
}

func TestUpdate_NonTransfer_KeepsMetadata_ClearsRecipient(t *testing.T) {
	tx := New(baseState(t))
	// Update to an income with a recipient set in the state -- non-transfer must
	// DROP recipient + keep category/payee/tag.
	s := baseState(t)
	s.Type = TransactionTypeIncome
	s.AccountRecipID = ptrID(t, "77777777-7777-7777-7777-777777777777")
	s.AmountRecipient = strPtr("10")
	tx.Update(s, tc1)

	if tx.Type != TransactionTypeIncome {
		t.Fatalf("type=%d want income", tx.Type)
	}
	if tx.AccountRecipID != nil || tx.AmountRecipient != nil {
		t.Fatal("non-transfer must clear recipient account + amount")
	}
	if tx.CategoryID == nil || tx.PayeeID == nil || tx.TagID == nil {
		t.Fatal("non-transfer must keep category/payee/tag")
	}
	if !tx.UpdatedAt.Equal(tc1) {
		t.Fatalf("updatedAt=%v want %v", tx.UpdatedAt, tc1)
	}
}

func TestUpdate_Transfer_KeepsRecipient_ClearsMetadata(t *testing.T) {
	tx := New(baseState(t))
	s := baseState(t)
	s.Type = TransactionTypeTransfer
	s.AccountRecipID = ptrID(t, "77777777-7777-7777-7777-777777777777")
	s.AmountRecipient = strPtr("40")
	// category/payee/tag still set in state -- transfer must DROP them.
	tx.Update(s, tc1)

	if !tx.Type.IsTransfer() {
		t.Fatalf("type=%d want transfer", tx.Type)
	}
	if tx.AccountRecipID == nil || tx.AmountRecipient == nil {
		t.Fatal("transfer must keep recipient account + amount")
	}
	if *tx.AmountRecipient != "40" {
		t.Fatalf("amountRecipient=%q want 40", *tx.AmountRecipient)
	}
	if tx.CategoryID != nil || tx.PayeeID != nil || tx.TagID != nil {
		t.Fatal("transfer must clear category/payee/tag")
	}
}

func TestUpdateClearsLabelsOnTransfer(t *testing.T) {
	now := time.Now()
	labels := []vo.Id{vo.NewId(), vo.NewId()}
	tx := New(NewState{
		ID: vo.NewId(), UserID: vo.NewId(), Type: TransactionTypeExpense,
		AccountID: vo.NewId(), Amount: "10", LabelIDs: labels,
		SpentAt: now, CreatedAt: now,
	})
	if len(tx.LabelIDs) != 2 {
		t.Fatalf("expense should keep labels, got %d", len(tx.LabelIDs))
	}

	recip := vo.NewId()
	tx.Update(NewState{
		Type: TransactionTypeTransfer, AccountID: tx.AccountID,
		AccountRecipID: &recip, Amount: "10", LabelIDs: labels, SpentAt: now,
	}, now)
	if len(tx.LabelIDs) != 0 {
		t.Fatalf("transfer must clear labels, got %d", len(tx.LabelIDs))
	}
}

// TestUpdate_NonTransfer_SetsLabelsFromState pins the non-transfer branch's
// "t.LabelIDs = s.LabelIDs" assignment: the transaction starts with NO
// labels, so if that line were ever deleted, tx.LabelIDs would stay nil
// after Update and this test would go red (unlike a test that starts with
// the SAME labels already set, where a deleted assignment is a silent no-op).
func TestUpdate_NonTransfer_SetsLabelsFromState(t *testing.T) {
	s := baseState(t)
	s.LabelIDs = nil
	tx := New(s)
	if len(tx.LabelIDs) != 0 {
		t.Fatalf("precondition: want no labels, got %d", len(tx.LabelIDs))
	}

	labels := []vo.Id{vo.NewId(), vo.NewId()}
	upd := baseState(t)
	upd.LabelIDs = labels
	tx.Update(upd, tc1)

	if len(tx.LabelIDs) != 2 || tx.LabelIDs[0] != labels[0] || tx.LabelIDs[1] != labels[1] {
		t.Fatalf("non-transfer Update must set labels from state: got %v, want %v", tx.LabelIDs, labels)
	}
}

// TestFromState_CarriesLabelIDs pins the "LabelIDs: s.LabelIDs" field in
// FromState's struct literal: unlike New (exercised by
// TestUpdateClearsLabelsOnTransfer), nothing else calls FromState with
// labels set, so a deleted assignment there would stay green everywhere else.
func TestFromState_CarriesLabelIDs(t *testing.T) {
	labels := []vo.Id{vo.NewId(), vo.NewId()}
	s := baseState(t)
	s.LabelIDs = labels
	tx := FromState(s)
	if len(tx.LabelIDs) != 2 || tx.LabelIDs[0] != labels[0] || tx.LabelIDs[1] != labels[1] {
		t.Fatalf("FromState must carry LabelIDs: got %v, want %v", tx.LabelIDs, labels)
	}
}
