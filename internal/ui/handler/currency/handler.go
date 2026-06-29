// Package currency wires the currency module's HTTP edge.
package currency

import (
	"net/http"

	appcurrency "github.com/econumo/econumo/internal/app/currency"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	read *appcurrency.ReadService
	dev  bool
}

func NewHandlers(read *appcurrency.ReadService, dev bool) *Handlers {
	return &Handlers{read: read, dev: dev}
}

// requireUser extracts the authenticated user id placed by the JWT middleware.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (vo.Id, bool) {
	id, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		httpx.WriteError(w, errs.NewUnauthorized("JWT Token not found"), h.dev)
		return vo.Id{}, false
	}
	return id, true
}
