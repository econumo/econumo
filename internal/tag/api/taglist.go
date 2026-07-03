package api

import (
	"net/http"

	apptag "github.com/econumo/econumo/internal/tag"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}

// OrderTagList handles POST /api/v1/tag/order-tag-list (auth).
//
// @Summary     Reorder the tag list
// @Description Applies position changes to the user's tags and returns the full ordered list.
// @Tags        Tag
// @Accept      json
// @Produce     json
// @Param       request body     apptag.OrderTagListRequest true "Order tag list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apptag.OrderTagListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/tag/order-tag-list [post]
func (h *Handlers) OrderTagList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	var req apptag.OrderTagListRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.OrderTagList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// GetTagList handles GET /api/v1/tag/get-tag-list (auth). The request has no
// body, so there is nothing to decode; the handler returns the user's tags
// ordered by position.
//
// @Summary     Get the tag list
// @Description Returns all the authenticated user's tags (archived and not) ordered by position.
// @Tags        Tag
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=apptag.GetTagListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/tag/get-tag-list [get]
func (h *Handlers) GetTagList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, err := h.read.GetTagList(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
