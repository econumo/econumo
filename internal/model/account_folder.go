// Folder aggregate (folders group accounts). A folder belongs to a user, has a
// position and a visibility flag; its account membership is loaded/persisted by
// the repo rather than held on the entity.
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Folder is the account-folder aggregate root. Fields are exported for direct
// read access; all writes after construction go through the mutators.
type Folder struct {
	ID     vo.Id
	UserID vo.Id
	Name   string
	// SortKey is the fractional index key that decides this row's slot in its
	// list. It never leaves the server: responses carry a dense 0-based index.
	SortKey   sortkey.Key
	IsVisible bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewFolder constructs a freshly-created folder. The sort key is assigned by the
// service via SetSortKey before the first save.
func NewFolder(id, userID vo.Id, name string, now time.Time) *Folder {
	return &Folder{
		ID: id, UserID: userID, Name: name, IsVisible: true, CreatedAt: now, UpdatedAt: now,
	}
}

func (f *Folder) UpdateName(name string, now time.Time) {
	if f.Name != name {
		f.Name = name
		f.UpdatedAt = now
	}
}

func (f *Folder) MakeVisible(now time.Time) {
	if !f.IsVisible {
		f.IsVisible = true
		f.UpdatedAt = now
	}
}

func (f *Folder) MakeInvisible(now time.Time) {
	if f.IsVisible {
		f.IsVisible = false
		f.UpdatedAt = now
	}
}

// SetSortKey sets the initial sort key at creation. It does not bump UpdatedAt,
// because it is part of construction.
func (f *Folder) SetSortKey(k sortkey.Key) { f.SortKey = k }

// UpdateSortKey moves the row, bumping updated_at only on a real change.
func (f *Folder) UpdateSortKey(k sortkey.Key, now time.Time) {
	if f.SortKey != k {
		f.SortKey = k
		f.UpdatedAt = now
	}
}
