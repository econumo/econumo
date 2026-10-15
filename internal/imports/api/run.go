package api

import (
	"net/http"
	"strconv"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// GetRunList handles GET /api/v1/import/get-run-list (auth).
// Optional query param: sourceId.
//
// @Summary     List import runs
// @Description The caller's most recent 50 import runs, newest first, optionally filtered to one source.
// @Tags        Import
// @Produce     json
// @Param       sourceId query    string false "Source id"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportRunListResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-run-list [get]
func (h *Handlers) GetRunList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, err := h.svc.GetRunList(r.Context(), userID, r.URL.Query().Get("sourceId"))
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

// GetRun handles GET /api/v1/import/get-run (auth).
// Required query param: id.
//
// @Summary     Get one import run
// @Description The run summary plus every ledger row it wrote. A row whose transaction was deleted since keeps its external fields with an empty transactionId.
// @Tags        Import
// @Produce     json
// @Param       id query    string true "Run id"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportRunResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-run [get]
func (h *Handlers) GetRun(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, err := h.svc.GetRun(r.Context(), userID, r.URL.Query().Get("id"))
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

// GetSourceHistory handles GET /api/v1/import/get-source-history (auth).
// Query params: sourceId (required), offset (optional, default 0).
//
// @Summary     List a source's imported rows
// @Description One source's rows — for Apple Wallet, every tap received — newest first, 50 per page; hasMore says whether a page follows at offset+50. Each row carries its outcome: linked (an empty transactionId means the transaction was deleted since), queued or skipped. Payloads that could not be parsed are not rows; they are listed as failed by get-queued-event-list.
// @Tags        Import
// @Produce     json
// @Param       sourceId query    string true  "Source id"
// @Param       offset   query    int    false "Rows to skip"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportSourceHistoryResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-source-history [get]
func (h *Handlers) GetSourceHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			httpx.WriteError(r.Context(), w, errs.NewValidation("offset must be a non-negative number"))
			return
		}
		offset = n
	}
	res, err := h.svc.GetSourceHistory(r.Context(), userID, r.URL.Query().Get("sourceId"), offset)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
