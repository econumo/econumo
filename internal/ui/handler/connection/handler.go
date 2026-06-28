// Package connection wires the connection module's HTTP edge: 7 endpoints under
// /api/v1/connection/, all JWT-protected and fully live. Three serve account
// sharing (get-connection-list, set-account-access, revoke-account-access); the
// other four — generate-invite, delete-invite, accept-invite, delete-connection
// — were ported from EconumoCloudBundle (this build has no CE/cloud split). Each
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
