// Package api wires the user module's HTTP edge.
package api

import (
	"github.com/econumo/econumo/internal/shared/port"
	appuser "github.com/econumo/econumo/internal/user"
)

type Handlers struct {
	svc  *appuser.Service
	read *appuser.ReadService
	now  port.Clock
}

func NewHandlers(svc *appuser.Service, read *appuser.ReadService, now port.Clock) *Handlers {
	return &Handlers{svc: svc, read: read, now: now}
}
