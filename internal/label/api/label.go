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
var _ = model.CreateLabelResult{}

// CreateLabel handles POST /api/v1/label/create-label (auth).
//
// @Summary     Create a label
// @Description Creates a reporting label for the authenticated user. Idempotent on the request id; the name must be unique among the user's labels.
// @Tags        Label
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateLabelRequest true "Create label request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateLabelResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/label/create-label [post]
func (h *Handlers) CreateLabel(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateLabel)
}

// UpdateLabel handles POST /api/v1/label/update-label (auth).
//
// @Summary     Update a label
// @Description Updates a label's name. Requires ownership; the new name must be unique among the user's labels.
// @Tags        Label
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateLabelRequest true "Update label request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateLabelResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/label/update-label [post]
func (h *Handlers) UpdateLabel(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateLabel)
}

// ArchiveLabel handles POST /api/v1/label/archive-label (auth).
//
// @Summary     Archive a label
// @Description Marks a label archived. Requires ownership.
// @Tags        Label
// @Accept      json
// @Produce     json
// @Param       request body     model.ArchiveLabelRequest true "Archive label request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ArchiveLabelResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/label/archive-label [post]
func (h *Handlers) ArchiveLabel(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ArchiveLabel)
}

// UnarchiveLabel handles POST /api/v1/label/unarchive-label (auth).
//
// @Summary     Unarchive a label
// @Description Clears a label's archived flag. Requires ownership.
// @Tags        Label
// @Accept      json
// @Produce     json
// @Param       request body     model.UnarchiveLabelRequest true "Unarchive label request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UnarchiveLabelResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/label/unarchive-label [post]
func (h *Handlers) UnarchiveLabel(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UnarchiveLabel)
}

// DeleteLabel handles POST /api/v1/label/delete-label (auth).
//
// @Summary     Delete a label
// @Description Deletes a label. Transactions referencing it lose the association (transactions_labels cascades). Requires ownership.
// @Tags        Label
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteLabelRequest true "Delete label request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DeleteLabelResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/label/delete-label [post]
func (h *Handlers) DeleteLabel(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteLabel)
}

// MergeLabel handles POST /api/v1/label/merge-label (auth).
//
// @Summary     Merge two labels
// @Description Re-points every transaction and recurring template from sourceId to targetId, then deletes sourceId. Requires ownership of both. Cannot be undone.
// @Tags        Label
// @Accept      json
// @Produce     json
// @Param       request body     model.MergeLabelRequest true "Merge label request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.MergeLabelResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/label/merge-label [post]
func (h *Handlers) MergeLabel(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.MergeLabel)
}
