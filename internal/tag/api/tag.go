package api

import (
	"net/http"

	apptag "github.com/econumo/econumo/internal/tag"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/endpoint"
)

// _ keeps the apidoc/apptag import aliases visible to swag's annotation
// parser (this file's handler bodies no longer reference apptag types
// directly, since they delegate to method values).
var _ = apidoc.JsonResponseError{}
var _ = apptag.CreateTagResult{}

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
	endpoint.Handle(w, r, h.dev, h.svc.CreateTag)
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
	endpoint.Handle(w, r, h.dev, h.svc.UpdateTag)
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
	endpoint.Handle(w, r, h.dev, h.svc.ArchiveTag)
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
	endpoint.Handle(w, r, h.dev, h.svc.UnarchiveTag)
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
	endpoint.Handle(w, r, h.dev, h.svc.DeleteTag)
}
