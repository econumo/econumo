package user

import (
	"net/http"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// _ keeps the apidoc and appuser import aliases visible to swag's per-file
// annotation parser. No runtime effect.
var (
	_ = apidoc.JsonResponseError{}
	_ = appuser.GetOptionListResult{}
)

// GetOptionList handles GET /api/v1/user/get-option-list (auth). No body/query;
// returns the user's raw persisted options ({items: [...]}), without the
// synthetic currency_id that CurrentUserResult carries.
//
// @Summary     Get option list
// @Description Returns the authenticated user's raw persisted options ({items: [...]}).
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=appuser.GetOptionListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/get-option-list [get]
func (h *Handlers) GetOptionList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	res, err := h.read.GetOptionList(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
