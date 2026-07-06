package model

import (
	"testing"
)

func TestType_AliasUnknownFallsBackToExpense(t *testing.T) {
	// Any unmapped numeric type defaults to "expense" (the switch default).
	if TransactionType(99).Alias() != "expense" {
		t.Errorf("TransactionType(99).Alias()=%q want expense", TransactionType(99).Alias())
	}
}

func TestFromState_RoundTrip_PreservesUpdatedAt(t *testing.T) {
	s := baseState(t)
	s.UpdatedAt = tc1 // FromState (unlike New) must keep the distinct UpdatedAt.
	tx := FromState(s)
	if !tx.CreatedAt.Equal(tc0) {
		t.Errorf("createdAt=%v want %v", tx.CreatedAt, tc0)
	}
	if !tx.UpdatedAt.Equal(tc1) {
		t.Errorf("updatedAt=%v want %v (FromState must preserve it)", tx.UpdatedAt, tc1)
	}
}

func TestFromState_AllAccessors(t *testing.T) {
	s := baseState(t)
	s.Type = TransactionTypeTransfer
	s.AccountRecipID = ptrID(t, "77777777-7777-7777-7777-777777777777")
	s.AmountRecipient = strPtr("99.99")
	tx := FromState(s)

	if !tx.ID.Equal(s.ID) || !tx.UserID.Equal(s.UserID) {
		t.Error("id/userId did not round-trip")
	}
	if tx.Type != TransactionTypeTransfer {
		t.Errorf("type=%d want transfer", tx.Type)
	}
	if !tx.AccountID.Equal(s.AccountID) {
		t.Error("accountId did not round-trip")
	}
	if tx.AccountRecipID == nil || !tx.AccountRecipID.Equal(*s.AccountRecipID) {
		t.Error("recipient account did not round-trip")
	}
	if tx.Amount != "42.50" {
		t.Errorf("amount=%q", tx.Amount)
	}
	if tx.AmountRecipient == nil || *tx.AmountRecipient != "99.99" {
		t.Errorf("amountRecipient=%v", tx.AmountRecipient)
	}
	if tx.Description != "groceries" {
		t.Errorf("description=%q", tx.Description)
	}
	if !tx.SpentAt.Equal(tc0) {
		t.Errorf("spentAt=%v", tx.SpentAt)
	}
	// FromState keeps the optional metadata verbatim (no type rules applied).
	if tx.CategoryID == nil || tx.PayeeID == nil || tx.TagID == nil {
		t.Error("FromState should preserve category/payee/tag as stored")
	}
}

// Updating an existing transfer back into a non-transfer must clear the
// recipient fields and adopt the supplied metadata.
func TestUpdate_TransferToNonTransfer(t *testing.T) {
	s := baseState(t)
	s.Type = TransactionTypeTransfer
	s.CategoryID, s.PayeeID, s.TagID = nil, nil, nil
	s.AccountRecipID = ptrID(t, "77777777-7777-7777-7777-777777777777")
	s.AmountRecipient = strPtr("5")
	tx := New(s)
	if tx.AccountRecipID == nil {
		t.Fatal("precondition: transfer should have a recipient")
	}

	// Now switch to an expense carrying metadata.
	upd := baseState(t)
	upd.Type = TransactionTypeExpense
	tx.Update(upd, tc1)

	if tx.AccountRecipID != nil || tx.AmountRecipient != nil {
		t.Error("switching to non-transfer must clear recipient fields")
	}
	if tx.CategoryID == nil || tx.PayeeID == nil || tx.TagID == nil {
		t.Error("non-transfer update must adopt category/payee/tag")
	}
	if !tx.UpdatedAt.Equal(tc1) {
		t.Errorf("updatedAt=%v want %v", tx.UpdatedAt, tc1)
	}
}

// Update always stamps updatedAt = now, even when the values are identical
// (a full-replace update, per the entity contract).
func TestUpdate_AlwaysStampsUpdatedAt(t *testing.T) {
	tx := New(baseState(t))
	tx.Update(baseState(t), tc1) // identical content
	if !tx.UpdatedAt.Equal(tc1) {
		t.Errorf("updatedAt=%v want %v (Update always stamps now)", tx.UpdatedAt, tc1)
	}
}
