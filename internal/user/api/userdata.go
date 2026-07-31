package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc and model import aliases visible to swag's annotation parser.
var (
	_ = apidoc.JsonResponseError{}
	_ = model.GetUserDataResult{}
)

// GetUserData handles GET /api/v1/user/get-user-data (auth). The request has no
// body/query, so there is nothing to decode or validate; the handler pulls the
// authenticated user id from the context and returns {user: CurrentUserResult}.
//
// @Summary     Get user data
// @Description Returns the authenticated user's profile ({user: CurrentUserResult}).
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetUserDataResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/get-user-data [get]
func (h *Handlers) GetUserData(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.read.GetUserData)
}
