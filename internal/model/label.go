package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DefaultLabelIcon is the icon every label is created with. Like a tag's, the
// value is persisted per row rather than re-derived at render time, so a
// user-selectable icon later needs no schema or rendering change.
const DefaultLabelIcon = "label"

// Label is the reporting-classification aggregate root. It deliberately has no
// budget role: a label never becomes a budgets_elements row, which is what lets
// several labels on one transaction each count the full amount without
// corrupting envelope math.
type Label struct {
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

func NewLabel(id, userID vo.Id, name string, now time.Time) *Label {
	return &Label{ID: id, UserID: userID, Name: name, Icon: DefaultLabelIcon, CreatedAt: now, UpdatedAt: now}
}

func (l *Label) UpdateName(name string, now time.Time) {
	if l.Name != name {
		l.Name = name
		l.UpdatedAt = now
	}
}

func (l *Label) Archive(now time.Time) {
	if !l.IsArchived {
		l.IsArchived = true
		l.UpdatedAt = now
	}
}

func (l *Label) Unarchive(now time.Time) {
	if l.IsArchived {
		l.IsArchived = false
		l.UpdatedAt = now
	}
}

// SetSortKey sets the initial sort key at creation. It does not bump UpdatedAt,
// because it is part of construction.
func (l *Label) SetSortKey(k sortkey.Key) { l.SortKey = k }

// UpdateSortKey moves the row, bumping updated_at only on a real change.
func (l *Label) UpdateSortKey(k sortkey.Key, now time.Time) {
	if l.SortKey != k {
		l.SortKey = k
		l.UpdatedAt = now
	}
}
