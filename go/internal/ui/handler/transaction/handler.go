// Package transaction wires the transaction module's HTTP edge. Ported now:
// create/update/delete/get-list (5 of 6; import is deferred — it needs the
// account+folder create paths via findOrCreate; export is a later CSV add). All
// JWT-protected under /api/v1/transaction/.
package transaction

import (
	"net/http"

	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

var _ = apidoc.JsonResponseOk{}

// Handlers holds the transaction service and the dev flag.
type Handlers struct {
	svc *apptransaction.Service
	dev bool
}

// NewHandlers constructs the handler set.
func NewHandlers(svc *apptransaction.Service, dev bool) *Handlers {
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
