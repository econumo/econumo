// Package transaction is the transaction aggregate's domain layer: the
// Transaction entity, its Type value object, and the repository interface.
//
// A transaction is an expense (type 0), income (1), or transfer (2). Transfers
// carry a recipient account + recipient amount and no category/payee/tag;
// non-transfers carry an optional category/payee/tag and no recipient. The
// entity enforces those type-dependent field rules in its mutators.
package transaction

import (
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
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

// Transaction is the transaction aggregate root. Optional references are pointer
// value objects (nil = absent). amountRecipient is a normalized decimal string
// (nil unless a transfer with a distinct recipient amount).
type Transaction struct {
	id              vo.Id
	userID          vo.Id
	typ             Type
	accountID       vo.Id
	accountRecipID  *vo.Id
	amount          string
	amountRecipient *string
	categoryID      *vo.Id
	payeeID         *vo.Id
	tagID           *vo.Id
	description     string
	spentAt         time.Time
	createdAt       time.Time
	updatedAt       time.Time
}

// NewState bundles the fields for constructing/reconstructing a Transaction.
// Optional ids are nil when absent; amountRecipient is nil unless set.
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

// New constructs a freshly-created transaction (createdAt == updatedAt). The
// service has already applied the type-dependent field rules when building the
// state.
func New(s NewState) *Transaction {
	return &Transaction{
		id: s.ID, userID: s.UserID, typ: s.Type, accountID: s.AccountID,
		accountRecipID: s.AccountRecipID, amount: s.Amount, amountRecipient: s.AmountRecipient,
		categoryID: s.CategoryID, payeeID: s.PayeeID, tagID: s.TagID,
		description: s.Description, spentAt: s.SpentAt, createdAt: s.CreatedAt, updatedAt: s.CreatedAt,
	}
}

func FromState(s NewState) *Transaction {
	return &Transaction{
		id: s.ID, userID: s.UserID, typ: s.Type, accountID: s.AccountID,
		accountRecipID: s.AccountRecipID, amount: s.Amount, amountRecipient: s.AmountRecipient,
		categoryID: s.CategoryID, payeeID: s.PayeeID, tagID: s.TagID,
		description: s.Description, spentAt: s.SpentAt, createdAt: s.CreatedAt, updatedAt: s.UpdatedAt,
	}
}

func (t *Transaction) Id() vo.Id                  { return t.id }
func (t *Transaction) UserId() vo.Id              { return t.userID }
func (t *Transaction) Type() Type                 { return t.typ }
func (t *Transaction) AccountId() vo.Id           { return t.accountID }
func (t *Transaction) AccountRecipientId() *vo.Id { return t.accountRecipID }
func (t *Transaction) Amount() string             { return t.amount }
func (t *Transaction) AmountRecipient() *string   { return t.amountRecipient }
func (t *Transaction) CategoryId() *vo.Id         { return t.categoryID }
func (t *Transaction) PayeeId() *vo.Id            { return t.payeeID }
func (t *Transaction) TagId() *vo.Id              { return t.tagID }
func (t *Transaction) Description() string        { return t.description }
func (t *Transaction) SpentAt() time.Time         { return t.spentAt }
func (t *Transaction) CreatedAt() time.Time       { return t.createdAt }
func (t *Transaction) UpdatedAt() time.Time       { return t.updatedAt }

// Update applies a full update from the given state, enforcing the
// type-dependent field rules: a transfer clears category/payee/tag and keeps
// recipient account+amount; a non-transfer clears recipient and keeps
// category/payee/tag. Always stamps updatedAt = now, since a full update
// replaces the mutable state.
func (t *Transaction) Update(s NewState, now time.Time) {
	t.typ = s.Type
	t.accountID = s.AccountID
	t.amount = s.Amount
	t.description = s.Description
	t.spentAt = s.SpentAt
	if s.Type.IsTransfer() {
		t.categoryID = nil
		t.payeeID = nil
		t.tagID = nil
		t.accountRecipID = s.AccountRecipID
		t.amountRecipient = s.AmountRecipient
	} else {
		t.accountRecipID = nil
		t.amountRecipient = nil
		t.categoryID = s.CategoryID
		t.payeeID = s.PayeeID
		t.tagID = s.TagID
	}
	t.updatedAt = now
}
