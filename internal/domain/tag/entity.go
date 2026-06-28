// Package tag is the tag aggregate's domain layer: the Tag entity, its value
// object (Name), and the repository interface. It is pure — no framework,
// persistence, or JSON imports. The application service invokes the
// intent-revealing mutators below and persists the whole aggregate.
//
// Unlike a category, a tag has no type and no persisted icon: its icon is a
// fixed "tag" and is not stored or returned on the wire (the TagResult DTO has
// no icon field). See CLAUDE.md.
package tag

import (
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// Tag is the tag aggregate root. The name is validated on the way in via the
// value-object constructor in the application layer; the entity holds
// already-valid state and exposes intent-revealing mutators
// (UpdateName/UpdatePosition/Archive/Unarchive), each bumping updatedAt only on
// a real change.
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
// the service to count(existing tags) via SetPosition before the first save.
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

// FromState rebuilds a Tag from persisted row data (repo reconstruction).
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

// Id returns the tag id.
func (t *Tag) Id() vo.Id { return t.id }

// UserId returns the owner user id.
func (t *Tag) UserId() vo.Id { return t.userID }

// Name returns the display name.
func (t *Tag) Name() string { return t.name }

// Position returns the sort position.
func (t *Tag) Position() int16 { return t.position }

// IsArchived reports whether the tag is archived.
func (t *Tag) IsArchived() bool { return t.isArchived }

// CreatedAt returns the creation time.
func (t *Tag) CreatedAt() time.Time { return t.createdAt }

// UpdatedAt returns the last-modification time.
func (t *Tag) UpdatedAt() time.Time { return t.updatedAt }

// SetPosition sets the initial position at creation (the service passes
// count(existing tags)). It does not bump updatedAt — it is part of
// construction.
func (t *Tag) SetPosition(position int16) { t.position = position }

// UpdateName changes the name, bumping updatedAt only on a real change.
func (t *Tag) UpdateName(name string, now time.Time) {
	if t.name != name {
		t.name = name
		t.updatedAt = now
	}
}

// UpdatePosition changes the position, bumping updatedAt only on a real change.
func (t *Tag) UpdatePosition(position int16, now time.Time) {
	if t.position != position {
		t.position = position
		t.updatedAt = now
	}
}

// Archive marks the tag archived, bumping updatedAt only on a real change.
func (t *Tag) Archive(now time.Time) {
	if !t.isArchived {
		t.isArchived = true
		t.updatedAt = now
	}
}

// Unarchive clears the archived flag, bumping updatedAt only on a real change.
func (t *Tag) Unarchive(now time.Time) {
	if t.isArchived {
		t.isArchived = false
		t.updatedAt = now
	}
}
