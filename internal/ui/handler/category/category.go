package category

import (
	"net/http"

	appcategory "github.com/econumo/econumo/internal/app/category"
	"github.com/econumo/econumo/internal/app/reqctx"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// _ keeps the apidoc import alias visible to swag's per-file annotation parser
// (the @Success {object} apidoc.* references below). No runtime effect.
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
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appcategory.CreateCategoryRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	reqctx.AddLogAttr(r.Context(), "category_id", req.Id)
	res, err := h.svc.CreateCategory(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
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
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appcategory.UpdateCategoryRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	reqctx.AddLogAttr(r.Context(), "category_id", req.Id)
	res, err := h.svc.UpdateCategory(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
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
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appcategory.ArchiveCategoryRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.ArchiveCategory(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
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
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appcategory.UnarchiveCategoryRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.UnarchiveCategory(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
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
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appcategory.DeleteCategoryRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	reqctx.AddLogAttr(r.Context(), "category_id", req.Id)
	res, err := h.svc.DeleteCategory(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
