// Package account wires the account+folder module's HTTP edge: 12 endpoints
// (5 account + 7 folder) under /api/v1/account/, all JWT-protected. Each handler
// is a thin adapter (decode + tier-1 Validate, pull userID, call the service,
// emit the frozen envelope); the service owns all logic.
package account

import (
	"net/http"

	appaccount "github.com/econumo/econumo/internal/app/account"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

// _ references the apidoc envelope schemas so the swag annotations resolve the
// apidoc import alias. No runtime effect.
var _ = apidoc.JsonResponseOk{}

// Handlers holds the account+folder service and the dev flag.
type Handlers struct {
	svc *appaccount.Service
	dev bool
}

// NewHandlers constructs the handler set.
func NewHandlers(svc *appaccount.Service, dev bool) *Handlers {
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
