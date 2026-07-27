// Package admin serves the private listener the payment portal talks to. It
// never imports another feature: the user and connection capabilities it needs
// are declared here and wired by internal/server.
package admin

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UserRecord is the neutral shape this package works in — raw columns, not yet
// collapsed against a clock, so the effective level is derived in exactly one
// place (Service.view).
type UserRecord struct {
	ID          string
	Name        string
	Email       string
	Avatar      string
	AccessLevel model.AccessLevel
	AccessUntil *time.Time
}

type UserLookup interface {
	GetUser(ctx context.Context, id vo.Id) (UserRecord, error)
	// SetAccess returns the record as written, so the caller's response cannot
	// diverge from the write under a concurrent update.
	SetAccess(ctx context.Context, id vo.Id, level model.AccessLevel, until *time.Time) (UserRecord, error)
}

type ConnectionLookup interface {
	ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}
