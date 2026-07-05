// Budget is the budget feature's aggregate root and its related entities
// (BudgetAccess, BudgetFolder, BudgetEnvelope, BudgetElement,
// BudgetElementLimit). The write-side Service (use-case orchestrator),
// repository interface, and read-model port stay in internal/budget; only the
// entity/value-object/DTO shapes live here.
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// PositionUnset is BudgetElement::POSITION_UNSET.
const PositionUnset = 0

// Budget is the aggregate root. Its excluded accounts, access grants, folders,
// envelopes, and elements are loaded/persisted alongside it by the repository,
// but modeled here as the root plus separate entities the repo manages. Fields
// are exported for direct read access; all writes after construction go
// through the mutators, which bump UpdatedAt only on a real change.
type Budget struct {
	ID         vo.Id
	UserID     vo.Id
	Name       string
	CurrencyID vo.Id
	StartedAt  time.Time // always the first of a month, 00:00:00
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewBudget creates a budget. startDate is snapped to the first of its month.
// The caller persists excluded accounts/access separately.
func NewBudget(id, userID vo.Id, name string, currencyID vo.Id, startDate, now time.Time) *Budget {
	return &Budget{
		ID:         id,
		UserID:     userID,
		Name:       name,
		CurrencyID: currencyID,
		StartedAt:  FirstOfMonth(startDate),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// UpdateName changes the name, bumping updated_at only on change.
func (b *Budget) UpdateName(name string, now time.Time) {
	if b.Name != name {
		b.Name = name
		b.UpdatedAt = now
	}
}

// UpdateCurrency changes the currency, bumping updated_at only on change.
func (b *Budget) UpdateCurrency(currencyID vo.Id, now time.Time) {
	if !b.CurrencyID.Equal(currencyID) {
		b.CurrencyID = currencyID
		b.UpdatedAt = now
	}
}

// StartFrom resets the budget's start month (snapped to first-of-month).
func (b *Budget) StartFrom(startedAt, now time.Time) {
	b.StartedAt = FirstOfMonth(startedAt)
	b.UpdatedAt = now
}

// BudgetAccess is a participant's grant on a budget (role + accepted flag).
type BudgetAccess struct {
	ID         vo.Id
	BudgetID   vo.Id
	UserID     vo.Id
	Role       BudgetRole
	IsAccepted bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewBudgetAccess creates a pending (not accepted) grant.
func NewBudgetAccess(id, budgetID, userID vo.Id, role BudgetRole, now time.Time) *BudgetAccess {
	return &BudgetAccess{ID: id, BudgetID: budgetID, UserID: userID, Role: role, IsAccepted: false, CreatedAt: now, UpdatedAt: now}
}

// UpdateRole changes the role, bumping updated_at only on change.
func (a *BudgetAccess) UpdateRole(role BudgetRole, now time.Time) {
	if a.Role != role {
		a.Role = role
		a.UpdatedAt = now
	}
}

// Accept marks the grant accepted.
func (a *BudgetAccess) Accept(now time.Time) {
	if !a.IsAccepted {
		a.IsAccepted = true
		a.UpdatedAt = now
	}
}

// BudgetFolder groups budget elements (distinct from account folders).
type BudgetFolder struct {
	ID        vo.Id
	BudgetID  vo.Id
	Name      string
	Position  int16
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewBudgetFolder creates a folder.
func NewBudgetFolder(id, budgetID vo.Id, name string, position int16, now time.Time) *BudgetFolder {
	return &BudgetFolder{ID: id, BudgetID: budgetID, Name: name, Position: position, CreatedAt: now, UpdatedAt: now}
}

// UpdateName changes the name, bumping updated_at only on change.
func (f *BudgetFolder) UpdateName(name string, now time.Time) {
	if f.Name != name {
		f.Name = name
		f.UpdatedAt = now
	}
}

// UpdatePosition changes the position, bumping updated_at only on change.
func (f *BudgetFolder) UpdatePosition(position int16, now time.Time) {
	if f.Position != position {
		f.Position = position
		f.UpdatedAt = now
	}
}

// BudgetEnvelope groups categories under a named, icon-bearing bucket.
type BudgetEnvelope struct {
	ID         vo.Id
	BudgetID   vo.Id
	Name       string
	Icon       string
	IsArchived bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewBudgetEnvelope creates an envelope.
func NewBudgetEnvelope(id, budgetID vo.Id, name, icon string, now time.Time) *BudgetEnvelope {
	return &BudgetEnvelope{ID: id, BudgetID: budgetID, Name: name, Icon: icon, IsArchived: false, CreatedAt: now, UpdatedAt: now}
}

// UpdateName changes the name, bumping updated_at only on change.
func (e *BudgetEnvelope) UpdateName(name string, now time.Time) {
	if e.Name != name {
		e.Name = name
		e.UpdatedAt = now
	}
}

// UpdateIcon changes the icon, bumping updated_at only on change.
func (e *BudgetEnvelope) UpdateIcon(icon string, now time.Time) {
	if e.Icon != icon {
		e.Icon = icon
		e.UpdatedAt = now
	}
}

// SetArchived sets the archived flag, bumping updated_at only on change.
func (e *BudgetEnvelope) SetArchived(v bool, now time.Time) {
	if e.IsArchived != v {
		e.IsArchived = v
		e.UpdatedAt = now
	}
}

// BudgetElement is a positioned member of a budget: an envelope, category, or
// tag (ExternalID), optionally in a folder, optionally with a display currency.
type BudgetElement struct {
	ID         vo.Id
	BudgetID   vo.Id
	ExternalID vo.Id
	Type       ElementType
	CurrencyID *vo.Id
	FolderID   *vo.Id
	Position   int16
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewBudgetElement creates an element.
func NewBudgetElement(id, budgetID, externalID vo.Id, typ ElementType, currencyID, folderID *vo.Id, position int16, now time.Time) *BudgetElement {
	return &BudgetElement{ID: id, BudgetID: budgetID, ExternalID: externalID, Type: typ, CurrencyID: currencyID, FolderID: folderID, Position: position, CreatedAt: now, UpdatedAt: now}
}

// IsPositionUnset reports whether the element still has its unset (zero)
// position — a comparison against PositionUnset, not a bare field return.
func (e *BudgetElement) IsPositionUnset() bool { return e.Position == PositionUnset }

func (e *BudgetElement) UpdatePosition(position int16, now time.Time) {
	if e.Position != position {
		e.Position = position
		e.UpdatedAt = now
	}
}

// UpdateCurrency changes the display currency (nil clears it).
func (e *BudgetElement) UpdateCurrency(currencyID *vo.Id, now time.Time) {
	if !idPtrEqual(e.CurrencyID, currencyID) {
		e.CurrencyID = currencyID
		e.UpdatedAt = now
	}
}

// UpdateFolder moves the element to a folder (nil removes it from folders).
func (e *BudgetElement) UpdateFolder(folderID *vo.Id, now time.Time) {
	if !idPtrEqual(e.FolderID, folderID) {
		e.FolderID = folderID
		e.UpdatedAt = now
	}
}

// BudgetElementLimit is a per-period (month) spending limit on an element.
type BudgetElementLimit struct {
	ID        vo.Id
	ElementID vo.Id
	Amount    vo.DecimalNumber
	Period    time.Time // first of the month
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewBudgetElementLimit creates a limit for an element in a period (snapped to
// first-of-month).
func NewBudgetElementLimit(id, elementID vo.Id, amount vo.DecimalNumber, period, now time.Time) *BudgetElementLimit {
	return &BudgetElementLimit{ID: id, ElementID: elementID, Amount: amount, Period: FirstOfMonth(period), CreatedAt: now, UpdatedAt: now}
}

func (l *BudgetElementLimit) UpdateAmount(amount vo.DecimalNumber, now time.Time) {
	if !l.Amount.Equals(amount) {
		l.Amount = amount
		l.UpdatedAt = now
	}
}

// FirstOfMonth returns the first day of t's month at 00:00:00 in t's location.
func FirstOfMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}

// idPtrEqual compares two optional ids for equality (both nil == equal).
func idPtrEqual(a, b *vo.Id) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
