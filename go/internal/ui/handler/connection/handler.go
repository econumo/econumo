// Package connection wires the connection module's HTTP edge: 7 endpoints under
// /api/v1/connection/, all JWT-protected. In the self-hosted product four are
// 501 stubs ("Not supported in Econumo One"): generate-invite, delete-invite,
// accept-invite, delete-connection. The three live endpoints are
// set-account-access, revoke-account-access, and get-connection-list. Each live
// handler is a thin adapter (decode + tier-1 Validate, pull userID, call the
// service, emit the frozen envelope).
package connection

import (
	"net/http"

	appconnection "github.com/econumo/econumo/internal/app/connection"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

// _ references the apidoc envelope schemas so the swag annotations resolve the
// apidoc import alias. No runtime effect.
var _ = apidoc.JsonResponseOk{}

// notImplementedMessage matches the self-hosted PHP stub bodies.
const notImplementedMessage = "Not supported in Econumo One"

// Handlers holds the connection service and the dev flag.
type Handlers struct {
	svc *appconnection.Service
	dev bool
}

// NewHandlers constructs the handler set.
func NewHandlers(svc *appconnection.Service, dev bool) *Handlers {
	return &Handlers{svc: svc, dev: dev}
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (vo.Id, bool) {
	id, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		httpx.WriteError(w, errs.NewUnauthorized("JWT Token not found"), h.dev)
		return vo.Id{}, false
	}
	return id, true
}
