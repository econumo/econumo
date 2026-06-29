package category

import (
	"net/http"

	appcategory "github.com/econumo/econumo/internal/app/category"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// Forces the apidoc import so swag annotations can resolve its envelope schemas.
var _ = apidoc.JsonResponseError{}

// OrderCategoryList handles POST /api/v1/category/order-category-list (auth).
//
// @Summary     Reorder the category list
// @Description Applies position changes to the user's categories and returns the full ordered list.
// @Tags        Category
// @Accept      json
// @Produce     json
// @Param       request body     appcategory.OrderCategoryListRequest true "Order category list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appcategory.OrderCategoryListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/order-category-list [post]
func (h *Handlers) OrderCategoryList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appcategory.OrderCategoryListRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.OrderCategoryList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// GetCategoryList handles GET /api/v1/category/get-category-list (auth). The
// request has no body, so there is nothing to decode; the handler returns the
// user's categories ordered by position.
//
// @Summary     Get the category list
// @Description Returns all the authenticated user's categories (archived and not) ordered by position.
// @Tags        Category
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=appcategory.GetCategoryListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/get-category-list [get]
func (h *Handlers) GetCategoryList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	res, err := h.read.GetCategoryList(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
