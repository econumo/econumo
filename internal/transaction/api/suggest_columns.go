package api

import (
	"encoding/json"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// SuggestImportColumns handles GET /api/v1/transaction/suggest-import-columns (auth).
//
// @Summary     Suggest CSV import column roles
// @Description Asks the server's AI provider which CSV header holds the date, amount (or money in and out), description, payee, category, tag, labels and account. Only the header names, their browser-computed types and the caller's language are sent; never a cell value. Answers empty roles when there is no suggestion: no AI provider configured, AI turned off for the caller, read-only access, the caller over their per-user limit, or a failed, late (8 s) or unusable reply. A header not in the request is never suggested, and each header plays at most one role.
// @Tags        Transaction
// @Produce     json
// @Param       columns query    string true "JSON array (1-60) of {name, type: date|number|text|empty, negatives} in file order; names 1-100 characters after trimming"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.SuggestImportColumnsResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/transaction/suggest-import-columns [get]
func (h *Handlers) SuggestImportColumns(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	var req model.SuggestImportColumnsRequest
	if raw := r.URL.Query().Get("columns"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &req.Columns); err != nil {
			httpx.WriteError(r.Context(), w, errs.NewValidation("Validation failed",
				errs.FieldError{Key: "columns", Message: "This value is not valid.", Code: errs.CodeInvalidFormat}))
			return
		}
	}
	if err := req.Validate(); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.SuggestImportColumns(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
