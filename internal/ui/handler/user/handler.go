// Package user wires the user module's HTTP edge: one handler per endpoint
// (grouped by resource) plus the route registration func that mounts all 13
// /api/v1/user/ routes onto the API mux.
//
// Each handler is a thin adapter: decode + tier-1 Validate via httpx, pull the
// authenticated user id from the request context (middleware.UserIDFromCtx) for
// the auth-required endpoints, call the application service, and emit the frozen
// success/error envelope via httpx.OK / httpx.WriteError. There is no business
// logic here — the service owns it.
package user

import (
	"net/http"
	"time"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
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

// Handlers holds the collaborators every user endpoint needs: the write-side
// application service (svc), the read-side query service (read, used by the read
// endpoints per the CQRS split), the dev flag (for the 500 envelope's stack
// trace), and the clock (login issuance time).
type Handlers struct {
	svc  *appuser.Service
	read *appuser.ReadService
	dev  bool
	now  Clock
}

// NewHandlers constructs the handler set.
func NewHandlers(svc *appuser.Service, read *appuser.ReadService, dev bool, now Clock) *Handlers {
	return &Handlers{svc: svc, read: read, dev: dev, now: now}
}

// requireUser extracts the authenticated user id placed by the JWT middleware.
// Absence is treated as unauthorized — defense in depth: the route wiring only
// mounts auth handlers behind the JWT middleware, so this should never fire in
// practice, but a wiring slip should fail closed rather than panic.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (vo.Id, bool) {
	id, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		httpx.WriteError(w, errs.NewUnauthorized("JWT Token not found"), h.dev)
		return vo.Id{}, false
	}
	return id, true
}
