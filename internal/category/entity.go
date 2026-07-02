// Package category is the category feature: the Category entity, the Type
// value object, and the repository interface (domain), plus the request/result
// DTOs (with their tier-1 Validate() methods), the write-side Service (which
// owns the tx boundary and builds the response-shaped *Result directly), and
// the read-side ReadService for the pure get-category-list read.
//
// JSON field names are frozen to the existing API wire contract; see
// CLAUDE.md.
package category

import (
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Type is the category type value object. The DB stores it as a SMALLINT
// (expense=0, income=1); the wire contract uses the alias string ("expense" /
// "income").
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

// Category is the category aggregate root. Fields are validated on the way in by
// the application layer; the entity holds already-valid state and its mutators
// each bump updatedAt only on a real change.
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
// is set by the service via SetPosition before the first save.
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

func (c *Category) Id() vo.Id { return c.id }

func (c *Category) UserId() vo.Id { return c.userID }

func (c *Category) Name() string { return c.name }

func (c *Category) Position() int16 { return c.position }

func (c *Category) Type() Type { return c.typ }

func (c *Category) Icon() string { return c.icon }

func (c *Category) IsArchived() bool { return c.isArchived }

func (c *Category) CreatedAt() time.Time { return c.createdAt }

func (c *Category) UpdatedAt() time.Time { return c.updatedAt }

// SetPosition sets the initial position at creation. It does not bump updatedAt
// — it is part of construction.
func (c *Category) SetPosition(position int16) { c.position = position }

func (c *Category) UpdateName(name string, now time.Time) {
	if c.name != name {
		c.name = name
		c.updatedAt = now
	}
}

func (c *Category) UpdateIcon(icon string, now time.Time) {
	if c.icon != icon {
		c.icon = icon
		c.updatedAt = now
	}
}

func (c *Category) UpdatePosition(position int16, now time.Time) {
	if c.position != position {
		c.position = position
		c.updatedAt = now
	}
}

func (c *Category) Archive(now time.Time) {
	if !c.isArchived {
		c.isArchived = true
		c.updatedAt = now
	}
}

func (c *Category) Unarchive(now time.Time) {
	if c.isArchived {
		c.isArchived = false
		c.updatedAt = now
	}
}
