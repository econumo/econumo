// Package category is the category aggregate's domain layer: the Category
// entity, its value objects (Name, Type), and the repository interface. It is
// pure — no framework, persistence, or JSON imports. The application service
// invokes the intent-revealing mutators below and persists the whole aggregate.
package category

import (
	"strings"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// Type is the category type value object. The DB stores it as a SMALLINT
// (expense=0, income=1); the wire contract uses the alias string ("expense" /
// "income"). See CLAUDE.md.
type Type int16

const (
	// TypeExpense is the default category type (db value 0, alias "expense").
	TypeExpense Type = 0
	// TypeIncome is the income category type (db value 1, alias "income").
	TypeIncome Type = 1

	aliasExpense = "expense"
	aliasIncome  = "income"
)

// DefaultIcon is the create-time fallback icon used when none is provided.
const DefaultIcon = "local_offer"

// Int16 returns the persisted SMALLINT value.
func (t Type) Int16() int16 { return int16(t) }

// Alias returns the wire alias ("expense" / "income").
func (t Type) Alias() string {
	if t == TypeIncome {
		return aliasIncome
	}
	return aliasExpense
}

// TypeFromAlias parses a wire alias ("expense"/"income", case-insensitive and
// space-trimmed) into a Type. ok is false for any other value.
func TypeFromAlias(alias string) (Type, bool) {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case aliasExpense:
		return TypeExpense, true
	case aliasIncome:
		return TypeIncome, true
	default:
		return 0, false
	}
}

// Category is the category aggregate root. Strings/ints are validated on the way
// in via the value-object constructors in the application layer; the entity
// holds already-valid state and exposes intent-revealing mutators
// (UpdateName/UpdateIcon/UpdatePosition/Archive/Unarchive), each bumping
// updatedAt only on a real change.
type Category struct {
	id         vo.Id
	userID     vo.Id
	name       string
	position   int16
	typ        Type
	icon       string
	isArchived bool
	createdAt  time.Time
	updatedAt  time.Time
}

// NewCategory constructs a freshly-created category. position defaults to 0 and
// is set by the service to count(existing categories) via SetPosition before the
// first save.
func NewCategory(id, userID vo.Id, name string, typ Type, icon string, now time.Time) *Category {
	return &Category{
		id:         id,
		userID:     userID,
		name:       name,
		position:   0,
		typ:        typ,
		icon:       icon,
		isArchived: false,
		createdAt:  now,
		updatedAt:  now,
	}
}

// FromState rebuilds a Category from persisted row data (repo reconstruction).
func FromState(id, userID vo.Id, name string, position int16, typ Type, icon string, isArchived bool, createdAt, updatedAt time.Time) *Category {
	return &Category{
		id:         id,
		userID:     userID,
		name:       name,
		position:   position,
		typ:        typ,
		icon:       icon,
		isArchived: isArchived,
		createdAt:  createdAt,
		updatedAt:  updatedAt,
	}
}

// Id returns the category id.
func (c *Category) Id() vo.Id { return c.id }

// UserId returns the owner user id.
func (c *Category) UserId() vo.Id { return c.userID }

// Name returns the display name.
func (c *Category) Name() string { return c.name }

// Position returns the sort position.
func (c *Category) Position() int16 { return c.position }

// Type returns the category type.
func (c *Category) Type() Type { return c.typ }

// Icon returns the icon identifier.
func (c *Category) Icon() string { return c.icon }

// IsArchived reports whether the category is archived.
func (c *Category) IsArchived() bool { return c.isArchived }

// CreatedAt returns the creation time.
func (c *Category) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt returns the last-modification time.
func (c *Category) UpdatedAt() time.Time { return c.updatedAt }

// SetPosition sets the initial position at creation (the service passes
// count(existing categories)). It does not bump updatedAt — it is part of
// construction.
func (c *Category) SetPosition(position int16) { c.position = position }

// UpdateName changes the name, bumping updatedAt only on a real change.
func (c *Category) UpdateName(name string, now time.Time) {
	if c.name != name {
		c.name = name
		c.updatedAt = now
	}
}

// UpdateIcon changes the icon, bumping updatedAt only on a real change.
func (c *Category) UpdateIcon(icon string, now time.Time) {
	if c.icon != icon {
		c.icon = icon
		c.updatedAt = now
	}
}

// UpdatePosition changes the position, bumping updatedAt only on a real change.
func (c *Category) UpdatePosition(position int16, now time.Time) {
	if c.position != position {
		c.position = position
		c.updatedAt = now
	}
}

// Archive marks the category archived, bumping updatedAt only on a real change.
func (c *Category) Archive(now time.Time) {
	if !c.isArchived {
		c.isArchived = true
		c.updatedAt = now
	}
}

// Unarchive clears the archived flag, bumping updatedAt only on a real change.
func (c *Category) Unarchive(now time.Time) {
	if c.isArchived {
		c.isArchived = false
		c.updatedAt = now
	}
}
