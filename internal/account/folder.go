// Folder aggregate (folders group accounts). A folder belongs to a user, has a
// position and a visibility flag; its account membership is loaded/persisted by
// the repo rather than held on the entity.
package account

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Folder is the account-folder aggregate root. Fields are exported for direct
// read access; all writes after construction go through the mutators.
type Folder struct {
	ID        vo.Id
	UserID    vo.Id
	Name      string
	Position  int16
	IsVisible bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewFolder constructs a freshly-created folder. Position is assigned by the
// service (last position + 1).
func NewFolder(id, userID vo.Id, name string, now time.Time) *Folder {
	return &Folder{
		ID: id, UserID: userID, Name: name, IsVisible: true, CreatedAt: now, UpdatedAt: now,
	}
}

func FolderFromState(id, userID vo.Id, name string, position int16, isVisible bool, createdAt, updatedAt time.Time) *Folder {
	return &Folder{
		ID: id, UserID: userID, Name: name, Position: position, IsVisible: isVisible,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}

// SetPosition sets the position at creation. It does not bump UpdatedAt — it is
// part of construction.
func (f *Folder) SetPosition(position int16) { f.Position = position }

func (f *Folder) UpdateName(name string, now time.Time) {
	if f.Name != name {
		f.Name = name
		f.UpdatedAt = now
	}
}

func (f *Folder) UpdatePosition(position int16, now time.Time) {
	if f.Position != position {
		f.Position = position
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
