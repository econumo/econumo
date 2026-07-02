// Package account is the account aggregate's domain layer: the Account entity,
// the Type value object, and the repository interface.
//
// The account's balance and per-user position are NOT part of this entity: the
// balance is computed from the transactions table and the position lives in a
// per-user options table, both resolved in the app/infra layers. The entity
// carries only the account's own columns. Folder membership likewise lives in a
// join table, not on the entity.
package account

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Type is the account type value object. The DB stores it as a SMALLINT and the
// wire contract uses the same int (NOT an alias string, unlike category): CASH=1,
// CREDIT_CARD=2. New accounts are always created as CREDIT_CARD.
type Type int16

const (
	// TypeCash is the cash account type (db/wire value 1).
	TypeCash Type = 1
	// TypeCreditCard is the credit-card account type (db/wire value 2); the
	// default for newly-created accounts.
	TypeCreditCard Type = 2
)

func (t Type) Int16() int16 { return int16(t) }

// Valid reports whether t is a known account type.
func (t Type) Valid() bool { return t == TypeCash || t == TypeCreditCard }

// Account is the account aggregate root. Fields are validated on the way in by
// the application layer; the entity holds already-valid state. Soft delete: an
// account is marked is_deleted rather than removed.
type Account struct {
	id         vo.Id
	userID     vo.Id
	currencyID vo.Id
	name       string
	typ        Type
	icon       string
	isDeleted  bool
	createdAt  time.Time
	updatedAt  time.Time
}

// NewAccount constructs a freshly-created account (always CREDIT_CARD). The
// initial balance is applied separately by the service as a correction
// transaction; position is stored per-user.
func NewAccount(id, userID, currencyID vo.Id, name, icon string, now time.Time) *Account {
	return &Account{
		id:         id,
		userID:     userID,
		currencyID: currencyID,
		name:       name,
		typ:        TypeCreditCard,
		icon:       icon,
		isDeleted:  false,
		createdAt:  now,
		updatedAt:  now,
	}
}

func FromState(id, userID, currencyID vo.Id, name string, typ Type, icon string, isDeleted bool, createdAt, updatedAt time.Time) *Account {
	return &Account{
		id:         id,
		userID:     userID,
		currencyID: currencyID,
		name:       name,
		typ:        typ,
		icon:       icon,
		isDeleted:  isDeleted,
		createdAt:  createdAt,
		updatedAt:  updatedAt,
	}
}

func (a *Account) Id() vo.Id { return a.id }

func (a *Account) UserId() vo.Id { return a.userID }

func (a *Account) CurrencyId() vo.Id { return a.currencyID }

func (a *Account) Name() string { return a.name }

func (a *Account) Type() Type { return a.typ }

func (a *Account) Icon() string { return a.icon }

// IsDeleted reports whether the account is soft-deleted.
func (a *Account) IsDeleted() bool { return a.isDeleted }

func (a *Account) CreatedAt() time.Time { return a.createdAt }

func (a *Account) UpdatedAt() time.Time { return a.updatedAt }

func (a *Account) UpdateName(name string, now time.Time) {
	if a.name != name {
		a.name = name
		a.updatedAt = now
	}
}

func (a *Account) UpdateIcon(icon string, now time.Time) {
	if a.icon != icon {
		a.icon = icon
		a.updatedAt = now
	}
}

func (a *Account) UpdateCurrency(currencyID vo.Id, now time.Time) {
	if !a.currencyID.Equal(currencyID) {
		a.currencyID = currencyID
		a.updatedAt = now
	}
}

// Delete soft-deletes the account, bumping updatedAt only on a real change.
func (a *Account) Delete(now time.Time) {
	if !a.isDeleted {
		a.isDeleted = true
		a.updatedAt = now
	}
}
