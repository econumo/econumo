// Package user wires the user module's HTTP edge.
package user

import (
	"net/http"
	"time"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

// Clock supplies issuance time for login token minting. A seam so handler tests
// can pin time; production passes a real clock. It is re-declared here (rather
// than reusing app/user.Clock) to keep the handler package's dependency surface
// explicit.
type Clock interface {
	Now() time.Time
}

type Handlers struct {
	svc  *appuser.Service
	read *appuser.ReadService
	dev  bool
	now  Clock
}

func NewHandlers(svc *appuser.Service, read *appuser.ReadService, dev bool, now Clock) *Handlers {
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
