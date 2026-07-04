// The Category entity: the Category aggregate root and the CategoryType value
// object. The repository interface, the write-side Service, and the read-side
// ReadService stay in internal/category.
//
// JSON field names are frozen to the existing API wire contract; see
// CLAUDE.md.
package model

import (
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// CategoryType is the category type value object. The DB stores it as a
// SMALLINT (expense=0, income=1); the wire contract uses the alias string
// ("expense" / "income").
type CategoryType int16

const (
	// TypeExpense is the default category type (db value 0, alias "expense").
	TypeExpense CategoryType = 0
	// TypeIncome is the income category type (db value 1, alias "income").
	TypeIncome CategoryType = 1

	aliasExpense = "expense"
	aliasIncome  = "income"
)

// DefaultIcon is the create-time fallback icon used when none is provided.
const DefaultIcon = "local_offer"

func (t CategoryType) Int16() int16 { return int16(t) }

// Alias returns the wire alias ("expense" / "income").
func (t CategoryType) Alias() string {
	if t == TypeIncome {
		return aliasIncome
	}
	return aliasExpense
}

// TypeFromAlias parses a wire alias ("expense"/"income", case-insensitive and
// space-trimmed) into a CategoryType. ok is false for any other value.
func TypeFromAlias(alias string) (CategoryType, bool) {
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
// each bump UpdatedAt only on a real change. Fields are exported for direct read
// access; all writes after construction go through the mutators. The Type field
// shares its name with the CategoryType value object (legal in Go: a struct
// field and a package-level type occupy separate namespaces) — its wire alias
// logic lives on the CategoryType value object above (Alias/Int16/
// TypeFromAlias), not on Category.
type Category struct {
	ID         vo.Id
	UserID     vo.Id
	Name       string
	Position   int16
	Type       CategoryType
	Icon       string
	IsArchived bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewCategory constructs a freshly-created category. Position defaults to 0 and
// is set by the service via SetPosition before the first save.
func NewCategory(id, userID vo.Id, name string, typ CategoryType, icon string, now time.Time) *Category {
	return &Category{ID: id, UserID: userID, Name: name, Type: typ, Icon: icon, CreatedAt: now, UpdatedAt: now}
}

// SetPosition sets the initial position at creation. It does not bump UpdatedAt
// — it is part of construction.
func (c *Category) SetPosition(position int16) { c.Position = position }

func (c *Category) UpdateName(name string, now time.Time) {
	if c.Name != name {
		c.Name = name
		c.UpdatedAt = now
	}
}

func (c *Category) UpdateIcon(icon string, now time.Time) {
	if c.Icon != icon {
		c.Icon = icon
		c.UpdatedAt = now
	}
}

func (c *Category) UpdatePosition(position int16, now time.Time) {
	if c.Position != position {
		c.Position = position
		c.UpdatedAt = now
	}
}

func (c *Category) Archive(now time.Time) {
	if !c.IsArchived {
		c.IsArchived = true
		c.UpdatedAt = now
	}
}

func (c *Category) Unarchive(now time.Time) {
	if c.IsArchived {
		c.IsArchived = false
		c.UpdatedAt = now
	}
}
