// Package currency wires the currency module's HTTP edge: the two read endpoints
// (get-currency-list, get-currency-rate-list) plus the route registration func.
// Both are JWT-protected GET reads with no request body.
//
// Each handler is a thin adapter: pull the authenticated user id from the
// request context (middleware.UserIDFromCtx), call the read service, and emit
// the frozen envelope via httpx.OK / httpx.WriteError. No business logic here.
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

// _ references the apidoc envelope schemas so the swaggo annotations resolve the
// apidoc import alias during swag init. No runtime effect.
var _ = apidoc.JsonResponseOk{}

// Handlers holds the read-side service and the dev flag (for the 500 envelope's
// stack trace). The currency module is read-only, so there is no write service.
type Handlers struct {
	read *appcurrency.ReadService
	dev  bool
}

// NewHandlers constructs the handler set.
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
