// The Payee entity: the Payee aggregate root. The repository interface, the
// write-side Service, and the read-side ReadService stay in internal/payee.
//
// A payee has the same shape as a tag: no type and no icon (the PayeeResult DTO
// carries only id/name/position/isArchived/timestamps).
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Payee is the payee aggregate root. The name is validated on the way in by the
// application layer; the entity holds already-valid state and its mutators each
// bump UpdatedAt only on a real change. Fields are exported for direct read
// access; all writes after construction go through the mutators.
type Payee struct {
	ID     vo.Id
	UserID vo.Id
	Name   string
	// SortKey is the fractional index key that decides this row's slot in its
	// list. It never leaves the server: responses carry a dense 0-based index.
	SortKey    sortkey.Key
	IsArchived bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewPayee constructs a freshly-created payee. The sort key is assigned by the
// service via SetSortKey before the first save.
func NewPayee(id, userID vo.Id, name string, now time.Time) *Payee {
	return &Payee{ID: id, UserID: userID, Name: name, CreatedAt: now, UpdatedAt: now}
}

func (p *Payee) UpdateName(name string, now time.Time) {
	if p.Name != name {
		p.Name = name
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

// SetSortKey sets the initial sort key at creation. It does not bump UpdatedAt,
// because it is part of construction.
func (p *Payee) SetSortKey(k sortkey.Key) { p.SortKey = k }

// UpdateSortKey moves the row, bumping updated_at only on a real change.
func (p *Payee) UpdateSortKey(k sortkey.Key, now time.Time) {
	if p.SortKey != k {
		p.SortKey = k
		p.UpdatedAt = now
	}
}
