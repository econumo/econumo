package api

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"regexp"
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// accountIdPattern restricts the accountId query param to hex chars, commas,
// dashes, whitespace, or empty — a frozen client-facing validation contract.
var accountIdPattern = regexp.MustCompile(`^[0-9a-fA-F,\-\s]*$`)

// ExportTransactionList handles GET /api/v1/transaction/export-transaction-list
// (auth). It returns text/csv (NOT the JSON envelope): a header row plus one row
// per exported transaction. The optional accountId query param is a
// comma-separated id list restricting the export to those accounts; absent/blank
// exports all accessible accounts.
//
// @Summary     Export the transaction list as CSV
// @Description Returns a CSV (text/csv) of transactions on the selected accounts (comma-separated accountId), or all accessible accounts when omitted.
// @Tags        Transaction
// @Produce     text/csv
// @Param       accountId query string false "Comma-separated account ids"
// @Success     200 {string} string "CSV file"
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/transaction/export-transaction-list [get]
func (h *Handlers) ExportTransactionList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	raw := r.URL.Query().Get("accountId")
	if !accountIdPattern.MatchString(raw) {
		httpx.WriteError(w, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "accountId", Message: "This value is not valid.", Code: "REGEX_FAILED_ERROR"}), h.dev)
		return
	}

	accountIDs, err := parseExportAccountIDs(raw)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}

	rows, err := h.svc.ExportTransactionList(r.Context(), userID, accountIDs)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}

	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	if werr := cw.WriteAll(rows); werr != nil {
		httpx.WriteError(w, werr, h.dev)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=UTF-8")
	w.Header().Set("Content-Disposition", `attachment; filename="transactions.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// parseExportAccountIDs splits the comma-separated accountId param into a unique
// ordered id list: trim each, drop empties, dedupe; a blank/empty param yields
// nil (= export all accessible accounts).
func parseExportAccountIDs(raw string) ([]vo.Id, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	seen := make(map[string]struct{})
	var ids []vo.Id
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, dup := seen[part]; dup {
			continue
		}
		seen[part] = struct{}{}
		id, err := vo.ParseId(part)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return ids, nil
}
