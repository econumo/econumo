// Package payee is the payee aggregate's domain layer: the Payee entity, its
// value object (Name), and the repository interface. It is pure — no framework,
// persistence, or JSON imports. The application service invokes the
// intent-revealing mutators below and persists the whole aggregate.
//
// A payee has the same shape as a tag: no type and no icon (the PayeeResult DTO
// carries only id/name/position/isArchived/timestamps). See COMPATIBILITY.md.
package payee

import (
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// Payee is the payee aggregate root. The name is validated on the way in via the
// value-object constructor in the application layer; the entity holds
// already-valid state and exposes intent-revealing mutators
// (UpdateName/UpdatePosition/Archive/Unarchive), each bumping updatedAt only on
// a real change.
type Payee struct {
	id         vo.Id
	userID     vo.Id
	name       string
	position   int16
	isArchived bool
	createdAt  time.Time
	updatedAt  time.Time
}

// NewPayee constructs a freshly-created payee. position defaults to 0 and is set
// by the service to count(existing payees) via SetPosition before the first
// save.
func NewPayee(id, userID vo.Id, name string, now time.Time) *Payee {
	return &Payee{
		id:         id,
		userID:     userID,
		name:       name,
		position:   0,
		isArchived: false,
		createdAt:  now,
		updatedAt:  now,
	}
}

// FromState rebuilds a Payee from persisted row data (repo reconstruction).
func FromState(id, userID vo.Id, name string, position int16, isArchived bool, createdAt, updatedAt time.Time) *Payee {
	return &Payee{
		id:         id,
		userID:     userID,
		name:       name,
		position:   position,
		isArchived: isArchived,
		createdAt:  createdAt,
		updatedAt:  updatedAt,
	}
}

// Id returns the payee id.
func (p *Payee) Id() vo.Id { return p.id }

// UserId returns the owner user id.
func (p *Payee) UserId() vo.Id { return p.userID }

// Name returns the display name.
func (p *Payee) Name() string { return p.name }

// Position returns the sort position.
func (p *Payee) Position() int16 { return p.position }

// IsArchived reports whether the payee is archived.
func (p *Payee) IsArchived() bool { return p.isArchived }

// CreatedAt returns the creation time.
func (p *Payee) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt returns the last-modification time.
func (p *Payee) UpdatedAt() time.Time { return p.updatedAt }

// SetPosition sets the initial position at creation (the service passes
// count(existing payees)). It does not bump updatedAt — it is part of
// construction.
func (p *Payee) SetPosition(position int16) { p.position = position }

// UpdateName changes the name, bumping updatedAt only on a real change.
func (p *Payee) UpdateName(name string, now time.Time) {
	if p.name != name {
		p.name = name
		p.updatedAt = now
	}
}

// UpdatePosition changes the position, bumping updatedAt only on a real change.
func (p *Payee) UpdatePosition(position int16, now time.Time) {
	if p.position != position {
		p.position = position
		p.updatedAt = now
	}
}

// Archive marks the payee archived, bumping updatedAt only on a real change.
func (p *Payee) Archive(now time.Time) {
	if !p.isArchived {
		p.isArchived = true
		p.updatedAt = now
	}
}

// Unarchive clears the archived flag, bumping updatedAt only on a real change.
func (p *Payee) Unarchive(now time.Time) {
	if p.isArchived {
		p.isArchived = false
		p.updatedAt = now
	}
}
