// Package payee is the payee aggregate's domain layer: the Payee entity and the
// repository interface.
//
// A payee has the same shape as a tag: no type and no icon (the PayeeResult DTO
// carries only id/name/position/isArchived/timestamps).
package payee

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Payee is the payee aggregate root. The name is validated on the way in by the
// application layer; the entity holds already-valid state and its mutators each
// bump updatedAt only on a real change.
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
// by the service via SetPosition before the first save.
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

func (p *Payee) Id() vo.Id { return p.id }

func (p *Payee) UserId() vo.Id { return p.userID }

func (p *Payee) Name() string { return p.name }

func (p *Payee) Position() int16 { return p.position }

func (p *Payee) IsArchived() bool { return p.isArchived }

func (p *Payee) CreatedAt() time.Time { return p.createdAt }

func (p *Payee) UpdatedAt() time.Time { return p.updatedAt }

// SetPosition sets the initial position at creation. It does not bump updatedAt
// — it is part of construction.
func (p *Payee) SetPosition(position int16) { p.position = position }

func (p *Payee) UpdateName(name string, now time.Time) {
	if p.name != name {
		p.name = name
		p.updatedAt = now
	}
}

func (p *Payee) UpdatePosition(position int16, now time.Time) {
	if p.position != position {
		p.position = position
		p.updatedAt = now
	}
}

func (p *Payee) Archive(now time.Time) {
	if !p.isArchived {
		p.isArchived = true
		p.updatedAt = now
	}
}

func (p *Payee) Unarchive(now time.Time) {
	if p.isArchived {
		p.isArchived = false
		p.updatedAt = now
	}
}
