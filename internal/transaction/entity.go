// Package transaction is the transaction feature: the Transaction entity, its
// Type value object, and the repository interface (domain), plus the
// request/result DTOs (with their tier-1 Validate() methods) and the
// write-side Service (create/update/delete, export, and CSV import), which
// owns the tx boundary and builds the response-shaped *Result directly.
//
// A transaction is an expense (type 0), income (1), or transfer (2). Transfers
// carry a recipient account + recipient amount and no category/payee/tag;
// non-transfers carry an optional category/payee/tag and no recipient. The
// entity enforces those type-dependent field rules in its mutators.
//
// JSON field names are frozen to the existing API wire contract; see
// CLAUDE.md.
package transaction

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Type is the transaction type value object. DB SMALLINT; wire uses the alias
// string (expense/income/transfer).
type Type int16

const (
	// TypeExpense is an expense (db 0, alias "expense").
	TypeExpense Type = 0
	// TypeIncome is income (db 1, alias "income").
	TypeIncome Type = 1
	// TypeTransfer is a transfer between accounts (db 2, alias "transfer").
	TypeTransfer Type = 2
)

func (t Type) Int16() int16 { return int16(t) }

// Alias returns the wire alias.
func (t Type) Alias() string {
	switch t {
	case TypeIncome:
		return "income"
	case TypeTransfer:
		return "transfer"
	default:
		return "expense"
	}
}

// IsExpense / IsIncome / IsTransfer report the type.
func (t Type) IsExpense() bool  { return t == TypeExpense }
func (t Type) IsIncome() bool   { return t == TypeIncome }
func (t Type) IsTransfer() bool { return t == TypeTransfer }

// Transaction is the transaction aggregate root. Fields are exported for
// direct read access; all writes after construction go through New/FromState
// (reconstruction) and Update (full mutation). Optional references are
// pointer value objects (nil = absent). AmountRecipient is a normalized
// decimal string (nil unless a transfer with a distinct recipient amount).
type Transaction struct {
	ID              vo.Id
	UserID          vo.Id
	Type            Type
	AccountID       vo.Id
	AccountRecipID  *vo.Id
	Amount          string
	AmountRecipient *string
	CategoryID      *vo.Id
	PayeeID         *vo.Id
	TagID           *vo.Id
	Description     string
	SpentAt         time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewState bundles the fields for constructing/reconstructing a Transaction.
// Optional ids are nil when absent; AmountRecipient is nil unless set.
type NewState struct {
	ID              vo.Id
	UserID          vo.Id
	Type            Type
	AccountID       vo.Id
	AccountRecipID  *vo.Id
	Amount          string
	AmountRecipient *string
	CategoryID      *vo.Id
	PayeeID         *vo.Id
	TagID           *vo.Id
	Description     string
	SpentAt         time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// New constructs a freshly-created transaction (CreatedAt == UpdatedAt). The
// service has already applied the type-dependent field rules when building the
// state.
func New(s NewState) *Transaction {
	return &Transaction{
		ID: s.ID, UserID: s.UserID, Type: s.Type, AccountID: s.AccountID,
		AccountRecipID: s.AccountRecipID, Amount: s.Amount, AmountRecipient: s.AmountRecipient,
		CategoryID: s.CategoryID, PayeeID: s.PayeeID, TagID: s.TagID,
		Description: s.Description, SpentAt: s.SpentAt, CreatedAt: s.CreatedAt, UpdatedAt: s.CreatedAt,
	}
}

func FromState(s NewState) *Transaction {
	return &Transaction{
		ID: s.ID, UserID: s.UserID, Type: s.Type, AccountID: s.AccountID,
		AccountRecipID: s.AccountRecipID, Amount: s.Amount, AmountRecipient: s.AmountRecipient,
		CategoryID: s.CategoryID, PayeeID: s.PayeeID, TagID: s.TagID,
		Description: s.Description, SpentAt: s.SpentAt, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

// Update applies a full update from the given state, enforcing the
// type-dependent field rules: a transfer clears category/payee/tag and keeps
// recipient account+amount; a non-transfer clears recipient and keeps
// category/payee/tag. Always stamps UpdatedAt = now, since a full update
// replaces the mutable state.
func (t *Transaction) Update(s NewState, now time.Time) {
	t.Type = s.Type
	t.AccountID = s.AccountID
	t.Amount = s.Amount
	t.Description = s.Description
	t.SpentAt = s.SpentAt
	if s.Type.IsTransfer() {
		t.CategoryID = nil
		t.PayeeID = nil
		t.TagID = nil
		t.AccountRecipID = s.AccountRecipID
		t.AmountRecipient = s.AmountRecipient
	} else {
		t.AccountRecipID = nil
		t.AmountRecipient = nil
		t.CategoryID = s.CategoryID
		t.PayeeID = s.PayeeID
		t.TagID = s.TagID
	}
	t.UpdatedAt = now
}
