// Package category wires the category module's HTTP edge: one thin handler per
// endpoint plus the route registration func that mounts all 7 /api/v1/category/
// routes (all JWT-protected) onto the API mux.
//
// Each handler is a thin adapter: decode + tier-1 Validate via httpx, pull the
// authenticated user id from the request context (middleware.UserIDFromCtx),
// call the application service, and emit the frozen success/error envelope via
// httpx.OK / httpx.WriteError. There is no business logic here — the service
// owns it.
package category

import (
	"net/http"

	appcategory "github.com/econumo/econumo/internal/app/category"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

// _ references the apidoc envelope schemas so the swaggo "@Success ... {object}
// apidoc.JsonResponseOk{...}" annotations on the handlers below resolve the
// `apidoc` import alias to its package during `swag init`. It has no runtime
// effect. (swag matches the leading identifier of a type reference against the
// import aliases in the same file, so the package must be imported here.)
var _ = apidoc.JsonResponseOk{}

// Handlers holds the collaborators every category endpoint needs: the write-side
// application service (svc), the read-side query service (read), and the dev
// flag (for the 500 envelope's stack trace).
type Handlers struct {
	svc  *appcategory.Service
	read *appcategory.ReadService
	dev  bool
}

// NewHandlers constructs the handler set.
func NewHandlers(svc *appcategory.Service, read *appcategory.ReadService, dev bool) *Handlers {
	return &Handlers{svc: svc, read: read, dev: dev}
}

// requireUser extracts the authenticated user id placed by the JWT middleware.
// Absence is treated as unauthorized (defense in depth: every category route is
// mounted behind the JWT middleware, so this should never fire in practice).
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (vo.Id, bool) {
	id, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		httpx.WriteError(w, errs.NewUnauthorized("JWT Token not found"), h.dev)
		return vo.Id{}, false
	}
	return id, true
}
