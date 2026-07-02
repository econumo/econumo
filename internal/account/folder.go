// Folder aggregate (folders group accounts). A folder belongs to a user, has a
// position and a visibility flag; its account membership is loaded/persisted by
// the repo rather than held on the entity.
package account

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Folder is the account-folder aggregate root.
type Folder struct {
	id        vo.Id
	userID    vo.Id
	name      string
	position  int16
	isVisible bool
	createdAt time.Time
	updatedAt time.Time
}

// NewFolder constructs a freshly-created folder. position is assigned by the
// service (last position + 1).
func NewFolder(id, userID vo.Id, name string, now time.Time) *Folder {
	return &Folder{
		id:        id,
		userID:    userID,
		name:      name,
		position:  0,
		isVisible: true,
		createdAt: now,
		updatedAt: now,
	}
}

func FolderFromState(id, userID vo.Id, name string, position int16, isVisible bool, createdAt, updatedAt time.Time) *Folder {
	return &Folder{
		id:        id,
		userID:    userID,
		name:      name,
		position:  position,
		isVisible: isVisible,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}
}

func (f *Folder) Id() vo.Id { return f.id }

func (f *Folder) UserId() vo.Id { return f.userID }

func (f *Folder) Name() string { return f.name }

func (f *Folder) Position() int16 { return f.position }

// IsVisible reports whether the folder (and its accounts) is visible.
func (f *Folder) IsVisible() bool { return f.isVisible }

func (f *Folder) CreatedAt() time.Time { return f.createdAt }

func (f *Folder) UpdatedAt() time.Time { return f.updatedAt }

// SetPosition sets the position at creation. It does not bump updatedAt — it is
// part of construction.
func (f *Folder) SetPosition(position int16) { f.position = position }

func (f *Folder) UpdateName(name string, now time.Time) {
	if f.name != name {
		f.name = name
		f.updatedAt = now
	}
}

func (f *Folder) UpdatePosition(position int16, now time.Time) {
	if f.position != position {
		f.position = position
		f.updatedAt = now
	}
}

func (f *Folder) MakeVisible(now time.Time) {
	if !f.isVisible {
		f.isVisible = true
		f.updatedAt = now
	}
}

func (f *Folder) MakeInvisible(now time.Time) {
	if f.isVisible {
		f.isVisible = false
		f.updatedAt = now
	}
}
