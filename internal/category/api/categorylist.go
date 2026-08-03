package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// Forces the apidoc/model imports so swag annotations can resolve their
// schemas (this file's handler bodies no longer reference these types
// directly, since they delegate to method values).
var _ = apidoc.JsonResponseError{}
var _ = model.GetCategoryListResult{}

// MoveCategory handles POST /api/v1/category/move-category (auth).
//
// @Summary     Move a category
// @Description Places one category immediately after another (afterId null = first) and returns the full ordered list.
// @Tags        Category
// @Accept      json
// @Produce     json
// @Param       request body     model.MoveCategoryRequest true "Move category request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.MoveCategoryResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/move-category [post]
func (h *Handlers) MoveCategory(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.MoveCategory)
}

// GetCategoryList handles GET /api/v1/category/get-category-list (auth). The
// request has no body, so there is nothing to decode; the handler returns the
// user's categories ordered by position.
//
// @Summary     Get the category list
// @Description Returns all the authenticated user's categories (archived and not) ordered by position.
// @Tags        Category
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetCategoryListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/get-category-list [get]
func (h *Handlers) GetCategoryList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.read.GetCategoryList)
}

// SortCategoryList handles POST /api/v1/category/sort-category-list (auth).
//
// @Summary     Sort the category list
// @Description Applies an explicit order (a full list of ids) to the caller's own categorys and returns the full ordered list.
// @Tags        Category
// @Accept      json
// @Produce     json
// @Param       request body     model.SortCategoryListRequest true "Sort category list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.SortCategoryListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/sort-category-list [post]
func (h *Handlers) SortCategoryList(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SortCategoryList)
}
