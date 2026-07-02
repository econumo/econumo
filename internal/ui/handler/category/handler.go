package category

import (
	"net/http"

	appcategory "github.com/econumo/econumo/internal/app/category"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

// Forces the apidoc import so swag annotations can resolve its envelope schemas.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc  *appcategory.Service
	read *appcategory.ReadService
	dev  bool
}

func NewHandlers(svc *appcategory.Service, read *appcategory.ReadService, dev bool) *Handlers {
	return &Handlers{svc: svc, read: read, dev: dev}
}

// requireUser pulls the user id placed by the JWT middleware; absence is treated
// as unauthorized (defense in depth — every route is already behind that middleware).
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (vo.Id, bool) {
	id, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		httpx.WriteError(w, errs.NewUnauthorized("JWT Token not found"), h.dev)
		return vo.Id{}, false
	}
	return id, true
}
