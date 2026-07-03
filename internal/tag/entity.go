// Package tag is the tag aggregate's domain layer: the Tag entity and the
// repository interface.
//
// Unlike a category, a tag has no type and no persisted icon: its icon is a
// fixed "tag" and is not stored or returned on the wire (the TagResult DTO has
// no icon field).
package tag

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Tag is the tag aggregate root. The name is validated on the way in by the
// application layer; the entity holds already-valid state and its mutators each
// bump UpdatedAt only on a real change. Fields are exported for direct read
// access; all writes after construction go through the mutators.
type Tag struct {
	ID         vo.Id
	UserID     vo.Id
	Name       string
	Position   int16
	IsArchived bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewTag constructs a freshly-created tag. Position defaults to 0 and is set by
// the service via SetPosition before the first save.
func NewTag(id, userID vo.Id, name string, now time.Time) *Tag {
	return &Tag{ID: id, UserID: userID, Name: name, CreatedAt: now, UpdatedAt: now}
}

// SetPosition sets the initial position at creation. It does not bump UpdatedAt
// — it is part of construction.
func (t *Tag) SetPosition(position int16) { t.Position = position }

func (t *Tag) UpdateName(name string, now time.Time) {
	if t.Name != name {
		t.Name = name
		t.UpdatedAt = now
	}
}

func (t *Tag) UpdatePosition(position int16, now time.Time) {
	if t.Position != position {
		t.Position = position
		t.UpdatedAt = now
	}
}

func (t *Tag) Archive(now time.Time) {
	if !t.IsArchived {
		t.IsArchived = true
		t.UpdatedAt = now
	}
}

func (t *Tag) Unarchive(now time.Time) {
	if t.IsArchived {
		t.IsArchived = false
		t.UpdatedAt = now
	}
}
