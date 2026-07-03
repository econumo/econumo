package api

import (
	"net/http"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/endpoint"
)

var _ = apidoc.JsonResponseUnauthorized{}
var _ = appaccount.GetFolderListResult{}

// CreateFolder handles POST /api/v1/account/create-folder (auth).
//
// @Summary     Create a folder
// @Description Creates an account folder for the user (name unique among the user's folders).
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     appaccount.CreateFolderRequest true "Create folder request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appaccount.CreateFolderResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/create-folder [post]
func (h *Handlers) CreateFolder(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.CreateFolder)
}

// UpdateFolder handles POST /api/v1/account/update-folder (auth).
//
// @Summary     Update a folder
// @Description Renames a folder the user owns (name unique among the user's folders).
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     appaccount.UpdateFolderRequest true "Update folder request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appaccount.UpdateFolderResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/update-folder [post]
func (h *Handlers) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdateFolder)
}

// HideFolder handles POST /api/v1/account/hide-folder (auth).
//
// @Summary     Hide a folder
// @Description Marks a folder (and its accounts) hidden. Requires ownership.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     appaccount.HideFolderRequest true "Hide folder request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appaccount.HideFolderResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/hide-folder [post]
func (h *Handlers) HideFolder(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.HideFolder)
}

// ShowFolder handles POST /api/v1/account/show-folder (auth).
//
// @Summary     Show a folder
// @Description Clears a folder's hidden flag. Requires ownership.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     appaccount.ShowFolderRequest true "Show folder request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appaccount.ShowFolderResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/show-folder [post]
func (h *Handlers) ShowFolder(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.ShowFolder)
}

// ReplaceFolder handles POST /api/v1/account/replace-folder (auth).
//
// @Summary     Replace a folder
// @Description Moves a folder's accounts into another folder and deletes it. Requires ownership of both.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     appaccount.ReplaceFolderRequest true "Replace folder request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appaccount.ReplaceFolderResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/replace-folder [post]
func (h *Handlers) ReplaceFolder(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.ReplaceFolder)
}

// GetFolderList handles GET /api/v1/account/get-folder-list (auth).
//
// @Summary     Get the folder list
// @Description Returns the user's account folders ordered by position.
// @Tags        Account
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=appaccount.GetFolderListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/get-folder-list [get]
func (h *Handlers) GetFolderList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.svc.GetFolderList)
}

// OrderFolderList handles POST /api/v1/account/order-folder-list (auth).
//
// @Summary     Reorder the folder list
// @Description Applies position changes to the user's folders and returns the full ordered list.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     appaccount.OrderFolderListRequest true "Order folder list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appaccount.OrderFolderListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/order-folder-list [post]
func (h *Handlers) OrderFolderList(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.OrderFolderList)
}
