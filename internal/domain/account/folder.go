// Folder aggregate (lives in the account package: folders are an account-module
// concept — they group accounts). A folder belongs to a user, has a position and
// a visibility flag, and contains accounts via the accounts_folders join (the
// membership is loaded/persisted by the repo, not held on the entity here —
// kept as an explicit id set the service manipulates).
package account

import (
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
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

// FolderFromState rebuilds a Folder from persisted row data.
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

// Id returns the folder id.
func (f *Folder) Id() vo.Id { return f.id }

// UserId returns the owner user id.
func (f *Folder) UserId() vo.Id { return f.userID }

// Name returns the folder name.
func (f *Folder) Name() string { return f.name }

// Position returns the sort position.
func (f *Folder) Position() int16 { return f.position }

// IsVisible reports whether the folder (and its accounts) is visible.
func (f *Folder) IsVisible() bool { return f.isVisible }

// CreatedAt returns the creation time.
func (f *Folder) CreatedAt() time.Time { return f.createdAt }

// UpdatedAt returns the last-modification time.
func (f *Folder) UpdatedAt() time.Time { return f.updatedAt }

// SetPosition sets the position at creation (service passes lastPosition+1). It
// does not bump updatedAt — it is part of construction.
func (f *Folder) SetPosition(position int16) { f.position = position }

// UpdateName changes the name, bumping updatedAt only on a real change.
func (f *Folder) UpdateName(name string, now time.Time) {
	if f.name != name {
		f.name = name
		f.updatedAt = now
	}
}

// UpdatePosition changes the position, bumping updatedAt only on a real change.
func (f *Folder) UpdatePosition(position int16, now time.Time) {
	if f.position != position {
		f.position = position
		f.updatedAt = now
	}
}

// MakeVisible clears the hidden flag, bumping updatedAt only on a real change.
func (f *Folder) MakeVisible(now time.Time) {
	if !f.isVisible {
		f.isVisible = true
		f.updatedAt = now
	}
}

// MakeInvisible sets the hidden flag, bumping updatedAt only on a real change.
func (f *Folder) MakeInvisible(now time.Time) {
	if f.isVisible {
		f.isVisible = false
		f.updatedAt = now
	}
}
