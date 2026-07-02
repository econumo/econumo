package budget

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// PositionUnset is BudgetElement::POSITION_UNSET.
const PositionUnset = 0

// Budget is the aggregate root. Its excluded accounts, access grants, folders,
// envelopes, and elements are loaded/persisted alongside it by the repository,
// but modeled here as the root plus separate entities the repo manages.
type Budget struct {
	id         vo.Id
	userID     vo.Id
	name       string
	currencyID vo.Id
	startedAt  time.Time // always the first of a month, 00:00:00
	createdAt  time.Time
	updatedAt  time.Time
}

// NewBudget creates a budget. startDate is snapped to the first of its month.
// The caller persists excluded accounts/access separately.
func NewBudget(id, userID vo.Id, name string, currencyID vo.Id, startDate, now time.Time) *Budget {
	return &Budget{
		id:         id,
		userID:     userID,
		name:       name,
		currencyID: currencyID,
		startedAt:  firstOfMonth(startDate),
		createdAt:  now,
		updatedAt:  now,
	}
}

// FromState rehydrates a Budget from storage.
func FromState(id, userID vo.Id, name string, currencyID vo.Id, startedAt, createdAt, updatedAt time.Time) *Budget {
	return &Budget{id: id, userID: userID, name: name, currencyID: currencyID, startedAt: startedAt, createdAt: createdAt, updatedAt: updatedAt}
}

func (b *Budget) Id() vo.Id            { return b.id }
func (b *Budget) UserId() vo.Id        { return b.userID }
func (b *Budget) Name() string         { return b.name }
func (b *Budget) CurrencyId() vo.Id    { return b.currencyID }
func (b *Budget) StartedAt() time.Time { return b.startedAt }
func (b *Budget) CreatedAt() time.Time { return b.createdAt }
func (b *Budget) UpdatedAt() time.Time { return b.updatedAt }

// UpdateName changes the name, bumping updated_at only on change.
func (b *Budget) UpdateName(name string, now time.Time) {
	if b.name != name {
		b.name = name
		b.updatedAt = now
	}
}

// UpdateCurrency changes the currency, bumping updated_at only on change.
func (b *Budget) UpdateCurrency(currencyID vo.Id, now time.Time) {
	if !b.currencyID.Equal(currencyID) {
		b.currencyID = currencyID
		b.updatedAt = now
	}
}

// StartFrom resets the budget's start month (snapped to first-of-month).
func (b *Budget) StartFrom(startedAt, now time.Time) {
	b.startedAt = firstOfMonth(startedAt)
	b.updatedAt = now
}

// BudgetAccess is a participant's grant on a budget (role + accepted flag).
type BudgetAccess struct {
	id         vo.Id
	budgetID   vo.Id
	userID     vo.Id
	role       UserRole
	isAccepted bool
	createdAt  time.Time
	updatedAt  time.Time
}

// NewBudgetAccess creates a pending (not accepted) grant.
func NewBudgetAccess(id, budgetID, userID vo.Id, role UserRole, now time.Time) *BudgetAccess {
	return &BudgetAccess{id: id, budgetID: budgetID, userID: userID, role: role, isAccepted: false, createdAt: now, updatedAt: now}
}

// AccessFromState rehydrates a BudgetAccess from storage.
func AccessFromState(id, budgetID, userID vo.Id, role UserRole, isAccepted bool, createdAt, updatedAt time.Time) *BudgetAccess {
	return &BudgetAccess{id: id, budgetID: budgetID, userID: userID, role: role, isAccepted: isAccepted, createdAt: createdAt, updatedAt: updatedAt}
}

func (a *BudgetAccess) Id() vo.Id            { return a.id }
func (a *BudgetAccess) BudgetId() vo.Id      { return a.budgetID }
func (a *BudgetAccess) UserId() vo.Id        { return a.userID }
func (a *BudgetAccess) Role() UserRole       { return a.role }
func (a *BudgetAccess) IsAccepted() bool     { return a.isAccepted }
func (a *BudgetAccess) CreatedAt() time.Time { return a.createdAt }
func (a *BudgetAccess) UpdatedAt() time.Time { return a.updatedAt }

// UpdateRole changes the role, bumping updated_at only on change.
func (a *BudgetAccess) UpdateRole(role UserRole, now time.Time) {
	if a.role != role {
		a.role = role
		a.updatedAt = now
	}
}

// Accept marks the grant accepted.
func (a *BudgetAccess) Accept(now time.Time) {
	if !a.isAccepted {
		a.isAccepted = true
		a.updatedAt = now
	}
}

// BudgetFolder groups budget elements (distinct from account folders).
type BudgetFolder struct {
	id        vo.Id
	budgetID  vo.Id
	name      string
	position  int16
	createdAt time.Time
	updatedAt time.Time
}

// NewBudgetFolder creates a folder.
func NewBudgetFolder(id, budgetID vo.Id, name string, position int16, now time.Time) *BudgetFolder {
	return &BudgetFolder{id: id, budgetID: budgetID, name: name, position: position, createdAt: now, updatedAt: now}
}

// FolderFromState rehydrates a BudgetFolder.
func FolderFromState(id, budgetID vo.Id, name string, position int16, createdAt, updatedAt time.Time) *BudgetFolder {
	return &BudgetFolder{id: id, budgetID: budgetID, name: name, position: position, createdAt: createdAt, updatedAt: updatedAt}
}

func (f *BudgetFolder) Id() vo.Id            { return f.id }
func (f *BudgetFolder) BudgetId() vo.Id      { return f.budgetID }
func (f *BudgetFolder) Name() string         { return f.name }
func (f *BudgetFolder) Position() int16      { return f.position }
func (f *BudgetFolder) CreatedAt() time.Time { return f.createdAt }
func (f *BudgetFolder) UpdatedAt() time.Time { return f.updatedAt }

// UpdateName changes the name, bumping updated_at only on change.
func (f *BudgetFolder) UpdateName(name string, now time.Time) {
	if f.name != name {
		f.name = name
		f.updatedAt = now
	}
}

// UpdatePosition changes the position, bumping updated_at only on change.
func (f *BudgetFolder) UpdatePosition(position int16, now time.Time) {
	if f.position != position {
		f.position = position
		f.updatedAt = now
	}
}

// BudgetEnvelope groups categories under a named, icon-bearing bucket.
type BudgetEnvelope struct {
	id         vo.Id
	budgetID   vo.Id
	name       string
	icon       string
	isArchived bool
	createdAt  time.Time
	updatedAt  time.Time
}

// NewBudgetEnvelope creates an envelope.
func NewBudgetEnvelope(id, budgetID vo.Id, name, icon string, now time.Time) *BudgetEnvelope {
	return &BudgetEnvelope{id: id, budgetID: budgetID, name: name, icon: icon, isArchived: false, createdAt: now, updatedAt: now}
}

// EnvelopeFromState rehydrates a BudgetEnvelope.
func EnvelopeFromState(id, budgetID vo.Id, name, icon string, isArchived bool, createdAt, updatedAt time.Time) *BudgetEnvelope {
	return &BudgetEnvelope{id: id, budgetID: budgetID, name: name, icon: icon, isArchived: isArchived, createdAt: createdAt, updatedAt: updatedAt}
}

func (e *BudgetEnvelope) Id() vo.Id            { return e.id }
func (e *BudgetEnvelope) BudgetId() vo.Id      { return e.budgetID }
func (e *BudgetEnvelope) Name() string         { return e.name }
func (e *BudgetEnvelope) Icon() string         { return e.icon }
func (e *BudgetEnvelope) IsArchived() bool     { return e.isArchived }
func (e *BudgetEnvelope) CreatedAt() time.Time { return e.createdAt }
func (e *BudgetEnvelope) UpdatedAt() time.Time { return e.updatedAt }

// UpdateName changes the name, bumping updated_at only on change.
func (e *BudgetEnvelope) UpdateName(name string, now time.Time) {
	if e.name != name {
		e.name = name
		e.updatedAt = now
	}
}

// UpdateIcon changes the icon, bumping updated_at only on change.
func (e *BudgetEnvelope) UpdateIcon(icon string, now time.Time) {
	if e.icon != icon {
		e.icon = icon
		e.updatedAt = now
	}
}

// SetArchived sets the archived flag, bumping updated_at only on change.
func (e *BudgetEnvelope) SetArchived(v bool, now time.Time) {
	if e.isArchived != v {
		e.isArchived = v
		e.updatedAt = now
	}
}

// BudgetElement is a positioned member of a budget: an envelope, category, or
// tag (externalID), optionally in a folder, optionally with a display currency.
type BudgetElement struct {
	id         vo.Id
	budgetID   vo.Id
	externalID vo.Id
	typ        ElementType
	currencyID *vo.Id
	folderID   *vo.Id
	position   int16
	createdAt  time.Time
	updatedAt  time.Time
}

// NewBudgetElement creates an element.
func NewBudgetElement(id, budgetID, externalID vo.Id, typ ElementType, currencyID, folderID *vo.Id, position int16, now time.Time) *BudgetElement {
	return &BudgetElement{id: id, budgetID: budgetID, externalID: externalID, typ: typ, currencyID: currencyID, folderID: folderID, position: position, createdAt: now, updatedAt: now}
}

// ElementFromState rehydrates a BudgetElement.
func ElementFromState(id, budgetID, externalID vo.Id, typ ElementType, currencyID, folderID *vo.Id, position int16, createdAt, updatedAt time.Time) *BudgetElement {
	return &BudgetElement{id: id, budgetID: budgetID, externalID: externalID, typ: typ, currencyID: currencyID, folderID: folderID, position: position, createdAt: createdAt, updatedAt: updatedAt}
}

func (e *BudgetElement) Id() vo.Id             { return e.id }
func (e *BudgetElement) BudgetId() vo.Id       { return e.budgetID }
func (e *BudgetElement) ExternalId() vo.Id     { return e.externalID }
func (e *BudgetElement) Type() ElementType     { return e.typ }
func (e *BudgetElement) CurrencyId() *vo.Id    { return e.currencyID }
func (e *BudgetElement) FolderId() *vo.Id      { return e.folderID }
func (e *BudgetElement) Position() int16       { return e.position }
func (e *BudgetElement) IsPositionUnset() bool { return e.position == PositionUnset }
func (e *BudgetElement) CreatedAt() time.Time  { return e.createdAt }
func (e *BudgetElement) UpdatedAt() time.Time  { return e.updatedAt }

func (e *BudgetElement) UpdatePosition(position int16, now time.Time) {
	if e.position != position {
		e.position = position
		e.updatedAt = now
	}
}

// UpdateCurrency changes the display currency (nil clears it).
func (e *BudgetElement) UpdateCurrency(currencyID *vo.Id, now time.Time) {
	if !idPtrEqual(e.currencyID, currencyID) {
		e.currencyID = currencyID
		e.updatedAt = now
	}
}

// UpdateFolder moves the element to a folder (nil removes it from folders).
func (e *BudgetElement) UpdateFolder(folderID *vo.Id, now time.Time) {
	if !idPtrEqual(e.folderID, folderID) {
		e.folderID = folderID
		e.updatedAt = now
	}
}

// BudgetElementLimit is a per-period (month) spending limit on an element.
type BudgetElementLimit struct {
	id        vo.Id
	elementID vo.Id
	amount    vo.DecimalNumber
	period    time.Time // first of the month
	createdAt time.Time
	updatedAt time.Time
}

// NewBudgetElementLimit creates a limit for an element in a period (snapped to
// first-of-month).
func NewBudgetElementLimit(id, elementID vo.Id, amount vo.DecimalNumber, period, now time.Time) *BudgetElementLimit {
	return &BudgetElementLimit{id: id, elementID: elementID, amount: amount, period: firstOfMonth(period), createdAt: now, updatedAt: now}
}

func LimitFromState(id, elementID vo.Id, amount vo.DecimalNumber, period, createdAt, updatedAt time.Time) *BudgetElementLimit {
	return &BudgetElementLimit{id: id, elementID: elementID, amount: amount, period: period, createdAt: createdAt, updatedAt: updatedAt}
}

func (l *BudgetElementLimit) Id() vo.Id                { return l.id }
func (l *BudgetElementLimit) ElementId() vo.Id         { return l.elementID }
func (l *BudgetElementLimit) Amount() vo.DecimalNumber { return l.amount }
func (l *BudgetElementLimit) Period() time.Time        { return l.period }
func (l *BudgetElementLimit) CreatedAt() time.Time     { return l.createdAt }
func (l *BudgetElementLimit) UpdatedAt() time.Time     { return l.updatedAt }

func (l *BudgetElementLimit) UpdateAmount(amount vo.DecimalNumber, now time.Time) {
	if !l.amount.Equals(amount) {
		l.amount = amount
		l.updatedAt = now
	}
}

// firstOfMonth returns the first day of t's month at 00:00:00 in t's location.
func firstOfMonth(t time.Time) time.Time {
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
