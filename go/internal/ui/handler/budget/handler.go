// Package budget wires the budget module's HTTP edge: the /api/v1/budget/*
// endpoints, all JWT-protected. Each handler is a thin adapter (decode + tier-1
// Validate, pull userID, call the service, emit the frozen envelope).
package budget

import (
	"net/http"

	appbudget "github.com/econumo/econumo/internal/app/budget"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

var _ = apidoc.JsonResponseOk{}

// Handlers holds the budget service and the dev flag.
type Handlers struct {
	svc *appbudget.Service
	dev bool
}

// NewHandlers constructs the handler set.
func NewHandlers(svc *appbudget.Service, dev bool) *Handlers {
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
