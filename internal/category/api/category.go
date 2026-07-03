package api

import (
	"context"
	"net/http"

	appcategory "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// Forces the apidoc import so swag annotations can resolve its envelope schemas.
var _ = apidoc.JsonResponseError{}

// CreateCategory handles POST /api/v1/category/create-category (auth).
//
// @Summary     Create a category
// @Description Creates a category for the authenticated user. Idempotent on the request id.
// @Tags        Category
// @Accept      json
// @Produce     json
// @Param       request body     appcategory.CreateCategoryRequest true "Create category request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appcategory.CreateCategoryResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/create-category [post]
func (h *Handlers) CreateCategory(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req appcategory.CreateCategoryRequest) (*appcategory.CreateCategoryResult, error) {
		reqctx.AddLogAttr(ctx, "category_id", req.Id)
		return h.svc.CreateCategory(ctx, userID, req)
	})
}

// UpdateCategory handles POST /api/v1/category/update-category (auth).
//
// @Summary     Update a category
// @Description Updates a category's name and icon. Requires ownership.
// @Tags        Category
// @Accept      json
// @Produce     json
// @Param       request body     appcategory.UpdateCategoryRequest true "Update category request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appcategory.UpdateCategoryResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/update-category [post]
func (h *Handlers) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req appcategory.UpdateCategoryRequest) (*appcategory.UpdateCategoryResult, error) {
		reqctx.AddLogAttr(ctx, "category_id", req.Id)
		return h.svc.UpdateCategory(ctx, userID, req)
	})
}

// ArchiveCategory handles POST /api/v1/category/archive-category (auth).
//
// @Summary     Archive a category
// @Description Marks a category archived. Requires ownership.
// @Tags        Category
// @Accept      json
// @Produce     json
// @Param       request body     appcategory.ArchiveCategoryRequest true "Archive category request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appcategory.ArchiveCategoryResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/archive-category [post]
func (h *Handlers) ArchiveCategory(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.ArchiveCategory)
}

// UnarchiveCategory handles POST /api/v1/category/unarchive-category (auth).
//
// @Summary     Unarchive a category
// @Description Clears a category's archived flag. Requires ownership.
// @Tags        Category
// @Accept      json
// @Produce     json
// @Param       request body     appcategory.UnarchiveCategoryRequest true "Unarchive category request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appcategory.UnarchiveCategoryResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/unarchive-category [post]
func (h *Handlers) UnarchiveCategory(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UnarchiveCategory)
}

// DeleteCategory handles POST /api/v1/category/delete-category (auth).
//
// @Summary     Delete a category
// @Description Deletes a category (mode=delete) or reassigns its transactions to a replacement then deletes it (mode=replace). Requires ownership.
// @Tags        Category
// @Accept      json
// @Produce     json
// @Param       request body     appcategory.DeleteCategoryRequest true "Delete category request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appcategory.DeleteCategoryResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/delete-category [post]
func (h *Handlers) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req appcategory.DeleteCategoryRequest) (*appcategory.DeleteCategoryResult, error) {
		reqctx.AddLogAttr(ctx, "category_id", req.Id)
		return h.svc.DeleteCategory(ctx, userID, req)
	})
}
