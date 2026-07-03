// Package api wires the user module's HTTP edge.
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
	appuser "github.com/econumo/econumo/internal/user"
)

type Handlers struct {
	svc  *appuser.Service
	read *appuser.ReadService
	dev  bool
	now  port.Clock
}

func NewHandlers(svc *appuser.Service, read *appuser.ReadService, dev bool, now port.Clock) *Handlers {
	return &Handlers{svc: svc, read: read, dev: dev, now: now}
}

// requireUser extracts the authenticated user id placed by the JWT middleware;
// absence fails closed as unauthorized rather than panicking on a wiring slip.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (vo.Id, bool) {
	id, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		httpx.WriteError(w, errs.NewUnauthorized("JWT Token not found"), h.dev)
		return vo.Id{}, false
	}
	return id, true
}
