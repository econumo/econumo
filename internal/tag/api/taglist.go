package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc/model import aliases visible to swag's annotation
// parser (this file's handler bodies no longer reference these types
// directly, since they delegate to method values).
var _ = apidoc.JsonResponseError{}
var _ = model.GetTagListResult{}

// OrderTagList handles POST /api/v1/tag/order-tag-list (auth).
//
// @Summary     Reorder the tag list
// @Description Applies position changes to the user's tags and returns the full ordered list.
// @Tags        Tag
// @Accept      json
// @Produce     json
// @Param       request body     model.OrderTagListRequest true "Order tag list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.OrderTagListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/tag/order-tag-list [post]
func (h *Handlers) OrderTagList(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.OrderTagList)
}

// GetTagList handles GET /api/v1/tag/get-tag-list (auth). The request has no
// body, so there is nothing to decode; the handler returns the user's tags
// ordered by position.
//
// @Summary     Get the tag list
// @Description Returns all the authenticated user's tags (archived and not) ordered by position.
// @Tags        Tag
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetTagListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/tag/get-tag-list [get]
func (h *Handlers) GetTagList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.read.GetTagList)
}
