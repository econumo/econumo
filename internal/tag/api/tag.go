package api

import (
	"net/http"

	apptag "github.com/econumo/econumo/internal/tag"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}

// CreateTag handles POST /api/v1/tag/create-tag (auth).
//
// @Summary     Create a tag
// @Description Creates a tag for the authenticated user. Idempotent on the request id; the name must be unique among the user's tags.
// @Tags        Tag
// @Accept      json
// @Produce     json
// @Param       request body     apptag.CreateTagRequest true "Create tag request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apptag.CreateTagResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/tag/create-tag [post]
func (h *Handlers) CreateTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req apptag.CreateTagRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.CreateTag(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// UpdateTag handles POST /api/v1/tag/update-tag (auth).
//
// @Summary     Update a tag
// @Description Updates a tag's name. Requires ownership; the new name must be unique among the user's tags.
// @Tags        Tag
// @Accept      json
// @Produce     json
// @Param       request body     apptag.UpdateTagRequest true "Update tag request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apptag.UpdateTagResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/tag/update-tag [post]
func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req apptag.UpdateTagRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.UpdateTag(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// ArchiveTag handles POST /api/v1/tag/archive-tag (auth).
//
// @Summary     Archive a tag
// @Description Marks a tag archived. Requires ownership.
// @Tags        Tag
// @Accept      json
// @Produce     json
// @Param       request body     apptag.ArchiveTagRequest true "Archive tag request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apptag.ArchiveTagResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/tag/archive-tag [post]
func (h *Handlers) ArchiveTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req apptag.ArchiveTagRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.ArchiveTag(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// UnarchiveTag handles POST /api/v1/tag/unarchive-tag (auth).
//
// @Summary     Unarchive a tag
// @Description Clears a tag's archived flag. Requires ownership.
// @Tags        Tag
// @Accept      json
// @Produce     json
// @Param       request body     apptag.UnarchiveTagRequest true "Unarchive tag request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apptag.UnarchiveTagResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/tag/unarchive-tag [post]
func (h *Handlers) UnarchiveTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req apptag.UnarchiveTagRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.UnarchiveTag(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// DeleteTag handles POST /api/v1/tag/delete-tag (auth).
//
// @Summary     Delete a tag
// @Description Deletes a tag. Transactions referencing it have their tag cleared. Requires ownership.
// @Tags        Tag
// @Accept      json
// @Produce     json
// @Param       request body     apptag.DeleteTagRequest true "Delete tag request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apptag.DeleteTagResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/tag/delete-tag [post]
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req apptag.DeleteTagRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.DeleteTag(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
