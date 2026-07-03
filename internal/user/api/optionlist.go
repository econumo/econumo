package api

import (
	"net/http"

	appuser "github.com/econumo/econumo/internal/user"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc and appuser import aliases visible to swag's annotation parser.
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
	endpoint.HandleNoBody(w, r, h.dev, h.read.GetOptionList)
}
