package user

import (
	"net/http"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// _ keeps the apidoc and appuser import aliases visible to swag's per-file
// annotation parser (the @Success {object} apidoc.* / {data=appuser.*}
// references below). No runtime effect.
var (
	_ = apidoc.JsonResponseError{}
	_ = appuser.GetUserDataResult{}
)

// GetUserData handles GET /api/v1/user/get-user-data (auth). The request has no
// body/query, so there is nothing to decode or validate; the handler pulls the
// authenticated user id from the context and returns {user: CurrentUserResult}.
//
// @Summary     Get user data
// @Description Returns the authenticated user's profile ({user: CurrentUserResult}).
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=appuser.GetUserDataResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/get-user-data [get]
func (h *Handlers) GetUserData(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	res, err := h.read.GetUserData(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
