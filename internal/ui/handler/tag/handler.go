// Package tag wires the tag module's HTTP edge.
package tag

import (
	"net/http"

	apptag "github.com/econumo/econumo/internal/app/tag"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc  *apptag.Service
	read *apptag.ReadService
	dev  bool
}

func NewHandlers(svc *apptag.Service, read *apptag.ReadService, dev bool) *Handlers {
	return &Handlers{svc: svc, read: read, dev: dev}
}

// requireUser extracts the authenticated user id placed by the JWT middleware;
// absence fails closed as unauthorized.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (vo.Id, bool) {
	id, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		httpx.WriteError(w, errs.NewUnauthorized("JWT Token not found"), h.dev)
		return vo.Id{}, false
	}
	return id, true
}
