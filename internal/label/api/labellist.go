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
var _ = model.GetLabelListResult{}

// MoveLabel handles POST /api/v1/label/move-label (auth).
//
// @Summary     Move a label
// @Description Places one label immediately after another (afterId null = first) and returns the full available list (own + shared).
// @Tags        Label
// @Accept      json
// @Produce     json
// @Param       request body     model.MoveLabelRequest true "Move label request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.MoveLabelResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/label/move-label [post]
func (h *Handlers) MoveLabel(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.MoveLabel)
}

// SortLabelList handles POST /api/v1/label/sort-label-list (auth).
//
// @Summary     Sort the label list
// @Description Applies an explicit order (a full list of ids) to the caller's own labels and returns the full available list (own + shared).
// @Tags        Label
// @Accept      json
// @Produce     json
// @Param       request body     model.SortLabelListRequest true "Sort label list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.SortLabelListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/label/sort-label-list [post]
func (h *Handlers) SortLabelList(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SortLabelList)
}

// GetLabelList handles GET /api/v1/label/get-label-list (auth). The request
// has no body, so there is nothing to decode; the handler returns the user's
// available labels (own + shared via account access) in list order.
//
// @Summary     Get the label list
// @Description Returns all the authenticated user's available labels (own + shared via account access, archived and not) in list order.
// @Tags        Label
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetLabelListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/label/get-label-list [get]
func (h *Handlers) GetLabelList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.read.GetLabelList)
}
