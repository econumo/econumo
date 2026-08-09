// The Tag entity: the Tag aggregate root. The repository interface, the
// write-side Service, and the read-side ReadService stay in internal/tag.
//
// Unlike a category, a tag has no type column, but it does have a persisted
// icon.
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DefaultTagIcon is the icon every tag is created with. The value is persisted
// per row (not re-derived at render time) so a user-selectable icon later needs
// no schema or rendering change.
const DefaultTagIcon = "tag"

// Tag is the tag aggregate root. The name is validated on the way in by the
// application layer; the entity holds already-valid state and its mutators each
// bump UpdatedAt only on a real change. Fields are exported for direct read
// access; all writes after construction go through the mutators.
type Tag struct {
	ID     vo.Id
	UserID vo.Id
	Name   string
	Icon   string
	// SortKey is the fractional index key that decides this row's slot in its
	// list. It never leaves the server: responses carry a dense 0-based index.
	SortKey    sortkey.Key
	IsArchived bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewTag constructs a freshly-created tag. The sort key is assigned by
// the service via SetSortKey before the first save.
func NewTag(id, userID vo.Id, name string, now time.Time) *Tag {
	return &Tag{ID: id, UserID: userID, Name: name, Icon: DefaultTagIcon, CreatedAt: now, UpdatedAt: now}
}

func (t *Tag) UpdateName(name string, now time.Time) {
	if t.Name != name {
		t.Name = name
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

// SetSortKey sets the initial sort key at creation. It does not bump UpdatedAt,
// because it is part of construction.
func (t *Tag) SetSortKey(k sortkey.Key) { t.SortKey = k }

// UpdateSortKey moves the row, bumping updated_at only on a real change.
func (t *Tag) UpdateSortKey(k sortkey.Key, now time.Time) {
	if t.SortKey != k {
		t.SortKey = k
		t.UpdatedAt = now
	}
}
