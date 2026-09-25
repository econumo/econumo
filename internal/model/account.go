// The Account entity: the Account aggregate root and the AccountType value
// object. The repository interfaces, use-case services, and their
// request/result DTOs stay in internal/account.
//
// The account's balance and per-user position are NOT part of the Account
// entity: the balance is computed from the transactions table and the
// position lives in a per-user options table, both resolved in the repo/api
// layers. The entity carries only the account's own columns. Folder
// membership likewise lives in a join table, not on the entity.
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountType is the account type value object. The DB stores it as a
// SMALLINT and the wire contract uses the same int (NOT an alias string,
// unlike category): CASH=1, CREDIT_CARD=2. New accounts are always created
// as CREDIT_CARD.
type AccountType int16

const (
	// TypeCash is the cash account type (db/wire value 1).
	TypeCash AccountType = 1
	// TypeCreditCard is the credit-card account type (db/wire value 2); the
	// default for newly-created accounts.
	TypeCreditCard AccountType = 2
)

func (t AccountType) Int16() int16 { return int16(t) }

// Valid reports whether t is a known account type.
func (t AccountType) Valid() bool { return t == TypeCash || t == TypeCreditCard }

// Account is the account aggregate root. Fields are validated on the way in by
// the application layer; the entity holds already-valid state. Soft delete: an
// account is marked IsDeleted rather than removed. Fields are exported for
// direct read access; all writes after construction go through the mutators.
type Account struct {
	ID         vo.Id
	UserID     vo.Id
	CurrencyID vo.Id
	Name       string
	Type       AccountType
	Icon       string
	IsDeleted  bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewAccount constructs a freshly-created account (always CREDIT_CARD). The
// initial balance is applied separately by the service as a correction
// transaction; position is stored per-user.
func NewAccount(id, userID, currencyID vo.Id, name, icon string, now time.Time) *Account {
	return &Account{
		ID: id, UserID: userID, CurrencyID: currencyID, Name: name,
		Type: TypeCreditCard, Icon: icon, CreatedAt: now, UpdatedAt: now,
	}
}

func (a *Account) UpdateName(name string, now time.Time) {
	if a.Name != name {
		a.Name = name
		a.UpdatedAt = now
	}
}

func (a *Account) UpdateIcon(icon string, now time.Time) {
	if a.Icon != icon {
		a.Icon = icon
		a.UpdatedAt = now
	}
}

func (a *Account) UpdateCurrency(currencyID vo.Id, now time.Time) {
	if !a.CurrencyID.Equal(currencyID) {
		a.CurrencyID = currencyID
		a.UpdatedAt = now
	}
}

// Delete soft-deletes the account, bumping UpdatedAt only on a real change.
func (a *Account) Delete(now time.Time) {
	if !a.IsDeleted {
		a.IsDeleted = true
		a.UpdatedAt = now
	}
}

// AccountCorrection is a balance-correction transaction to insert (account
// create with non-zero balance, or update changing the balance).
type AccountCorrection struct {
	ID          vo.Id
	UserID      vo.Id
	AccountID   vo.Id
	Description string
	Type        int16 // 0 expense, 1 income
	Amount      string
	SpentAt     time.Time
	CreatedAt   time.Time
}
