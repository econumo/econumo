// Package account is the account aggregate's domain layer: the Account entity,
// its value objects (Type), and the repository interface. It is pure — no
// framework, persistence, or JSON imports.
//
// The account's balance and per-user position are NOT part of this entity: the
// balance is computed from the transactions table and the position lives in the
// accounts_options table (per-user), both resolved in the app/infra layers. The
// entity carries only the account's own columns. Folder membership likewise
// lives in the accounts_folders join, not on the entity.
package account

import (
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// Type is the account type value object. The DB stores it as a SMALLINT and the
// wire contract uses the same int (NOT an alias string, unlike category): CASH=1,
// CREDIT_CARD=2. New accounts are always created as CREDIT_CARD (matching the PHP
// AccountService::create, which hardcodes the type). See CLAUDE.md.
type Type int16

const (
	// TypeCash is the cash account type (db/wire value 1).
	TypeCash Type = 1
	// TypeCreditCard is the credit-card account type (db/wire value 2); the
	// default for newly-created accounts.
	TypeCreditCard Type = 2
)

// Int16 returns the persisted/wire SMALLINT value.
func (t Type) Int16() int16 { return int16(t) }

// Valid reports whether t is a known account type.
func (t Type) Valid() bool { return t == TypeCash || t == TypeCreditCard }

// Account is the account aggregate root. Strings are validated on the way in via
// the value-object constructors in the application layer; the entity holds
// already-valid state and exposes intent-revealing mutators. Soft delete: an
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

// NewAccount constructs a freshly-created account (always CREDIT_CARD, matching
// the PHP factory). The initial balance is applied separately by the service as
// a correction transaction; position lives in accounts_options.
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

// FromState rebuilds an Account from persisted row data (repo reconstruction).
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

// Id returns the account id.
func (a *Account) Id() vo.Id { return a.id }

// UserId returns the owner user id.
func (a *Account) UserId() vo.Id { return a.userID }

// CurrencyId returns the account's currency id.
func (a *Account) CurrencyId() vo.Id { return a.currencyID }

// Name returns the display name.
func (a *Account) Name() string { return a.name }

// Type returns the account type.
func (a *Account) Type() Type { return a.typ }

// Icon returns the icon identifier.
func (a *Account) Icon() string { return a.icon }

// IsDeleted reports whether the account is soft-deleted.
func (a *Account) IsDeleted() bool { return a.isDeleted }

// CreatedAt returns the creation time.
func (a *Account) CreatedAt() time.Time { return a.createdAt }

// UpdatedAt returns the last-modification time.
func (a *Account) UpdatedAt() time.Time { return a.updatedAt }

// UpdateName changes the name, bumping updatedAt only on a real change.
func (a *Account) UpdateName(name string, now time.Time) {
	if a.name != name {
		a.name = name
		a.updatedAt = now
	}
}

// UpdateIcon changes the icon, bumping updatedAt only on a real change.
func (a *Account) UpdateIcon(icon string, now time.Time) {
	if a.icon != icon {
		a.icon = icon
		a.updatedAt = now
	}
}

// UpdateCurrency changes the currency, bumping updatedAt only on a real change.
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
