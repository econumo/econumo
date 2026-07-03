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
// bump updatedAt only on a real change.
type Tag struct {
	id         vo.Id
	userID     vo.Id
	name       string
	position   int16
	isArchived bool
	createdAt  time.Time
	updatedAt  time.Time
}

// NewTag constructs a freshly-created tag. position defaults to 0 and is set by
// the service via SetPosition before the first save.
func NewTag(id, userID vo.Id, name string, now time.Time) *Tag {
	return &Tag{
		id:         id,
		userID:     userID,
		name:       name,
		position:   0,
		isArchived: false,
		createdAt:  now,
		updatedAt:  now,
	}
}

func FromState(id, userID vo.Id, name string, position int16, isArchived bool, createdAt, updatedAt time.Time) *Tag {
	return &Tag{
		id:         id,
		userID:     userID,
		name:       name,
		position:   position,
		isArchived: isArchived,
		createdAt:  createdAt,
		updatedAt:  updatedAt,
	}
}

func (t *Tag) Id() vo.Id { return t.id }

func (t *Tag) UserId() vo.Id { return t.userID }

func (t *Tag) Name() string { return t.name }

func (t *Tag) Position() int16 { return t.position }

func (t *Tag) IsArchived() bool { return t.isArchived }

func (t *Tag) CreatedAt() time.Time { return t.createdAt }

func (t *Tag) UpdatedAt() time.Time { return t.updatedAt }

// SetPosition sets the initial position at creation. It does not bump updatedAt
// — it is part of construction.
func (t *Tag) SetPosition(position int16) { t.position = position }

func (t *Tag) UpdateName(name string, now time.Time) {
	if t.name != name {
		t.name = name
		t.updatedAt = now
	}
}

func (t *Tag) UpdatePosition(position int16, now time.Time) {
	if t.position != position {
		t.position = position
		t.updatedAt = now
	}
}

func (t *Tag) Archive(now time.Time) {
	if !t.isArchived {
		t.isArchived = true
		t.updatedAt = now
	}
}

func (t *Tag) Unarchive(now time.Time) {
	if t.isArchived {
		t.isArchived = false
		t.updatedAt = now
	}
}
