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
// bump UpdatedAt only on a real change. Fields are exported for direct read
// access; all writes after construction go through the mutators.
type Payee struct {
	ID         vo.Id
	UserID     vo.Id
	Name       string
	Position   int16
	IsArchived bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewPayee constructs a freshly-created payee. Position defaults to 0 and is set
// by the service via SetPosition before the first save.
func NewPayee(id, userID vo.Id, name string, now time.Time) *Payee {
	return &Payee{ID: id, UserID: userID, Name: name, CreatedAt: now, UpdatedAt: now}
}

// SetPosition sets the initial position at creation. It does not bump UpdatedAt
// — it is part of construction.
func (p *Payee) SetPosition(position int16) { p.Position = position }

func (p *Payee) UpdateName(name string, now time.Time) {
	if p.Name != name {
		p.Name = name
		p.UpdatedAt = now
	}
}

func (p *Payee) UpdatePosition(position int16, now time.Time) {
	if p.Position != position {
		p.Position = position
		p.UpdatedAt = now
	}
}

func (p *Payee) Archive(now time.Time) {
	if !p.IsArchived {
		p.IsArchived = true
		p.UpdatedAt = now
	}
}

func (p *Payee) Unarchive(now time.Time) {
	if p.IsArchived {
		p.IsArchived = false
		p.UpdatedAt = now
	}
}
