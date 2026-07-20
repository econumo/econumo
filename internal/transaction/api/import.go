package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// maxImportUpload bounds the multipart parse at 10M — a frozen upload-size limit.
const maxImportUpload = 10 << 20

// maxImportRequest caps the WHOLE multipart request. ParseMultipartForm's arg is
// only the in-memory threshold (excess spills to temp disk), so without a body
// cap an attacker could exhaust disk. The margin over maxImportUpload covers the
// multipart envelope and the small override fields.
const maxImportRequest = maxImportUpload + (1 << 20)

// ImportTransactionList handles POST /api/v1/transaction/import-transaction-list
// (auth, multipart/form-data). Form fields: file (CSV upload), mapping (JSON
// string), and optional overrides accountId/date/categoryId/description/payeeId/
// tagId. Returns the JSON envelope with data = {imported, skipped, errors}.
//
// @Summary     Import a transaction list from CSV
// @Description Imports transactions from an uploaded CSV using a field mapping (JSON) and optional per-import overrides; find-or-creates accounts/categories/payees/tags.
// @Tags        Transaction
// @Accept      multipart/form-data
// @Produce     json
// @Param       file       formData file   true  "CSV file"
// @Param       mapping    formData string true  "Field mapping JSON"
// @Param       accountId  formData string false "Override account id"
// @Param       date       formData string false "Override date (Y-m-d)"
// @Param       categoryId formData string false "Override category id"
// @Param       description formData string false "Override description"
// @Param       payeeId    formData string false "Override payee id"
// @Param       tagId      formData string false "Override tag id"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.ImportResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     402 {object} apidoc.JsonResponseError
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/transaction/import-transaction-list [post]
func (h *Handlers) ImportTransactionList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	// Bound the whole request body before parsing so oversized uploads are
	// rejected instead of spilled to disk.
	r.Body = http.MaxBytesReader(w, r.Body, maxImportRequest)
	if err := r.ParseMultipartForm(maxImportUpload); err != nil {
		httpx.WriteError(r.Context(), w, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "file", Message: "Please upload a valid CSV file", Code: errs.CodeTransactionInvalidImportFile}), h.dev)
		return
	}

	mapping, merr := parseImportMapping(r.FormValue("mapping"))
	if merr != nil {
		httpx.WriteError(r.Context(), w, merr, h.dev)
		return
	}

	req := model.ImportRequest{
		Mapping:     mapping,
		AccountId:   optFormValue(r, "accountId"),
		Date:        optFormValue(r, "date"),
		CategoryId:  optFormValue(r, "categoryId"),
		Description: optFormValue(r, "description"),
		PayeeId:     optFormValue(r, "payeeId"),
		TagId:       optFormValue(r, "tagId"),
	}

	// file (optional at this layer; the service reports "No file provided").
	if file, _, ferr := r.FormFile("file"); ferr == nil {
		defer file.Close()
		data, rerr := io.ReadAll(io.LimitReader(file, maxImportUpload))
		if rerr != nil {
			httpx.WriteError(r.Context(), w, rerr, h.dev)
			return
		}
		req.File = data
	}

	res, err := h.svc.ImportTransactionList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// parseImportMapping decodes the mapping JSON object into an ImportMapping. An
// invalid JSON object is a 400 ValidationError ("Invalid mapping JSON").
func parseImportMapping(raw string) (model.ImportMapping, error) {
	var m model.ImportMapping
	if raw == "" {
		return m, nil
	}
	// Tolerate null values in the object (the frontend sends nulls for unmapped
	// fields); decode into a string-pointer map first.
	var obj map[string]*string
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		// No Code here by design: Message is the raw json.Unmarshal error text,
		// which varies per malformed payload, so it cannot map to a fixed
		// catalogue string (the frozen "en value = exact message" invariant would
		// break). This is the import-parsing carve-out from the code-registration
		// task brief.
		return m, errs.NewValidation("Invalid mapping JSON",
			errs.FieldError{Key: "mapping", Message: err.Error()})
	}
	get := func(k string) string {
		if v, ok := obj[k]; ok && v != nil {
			return *v
		}
		return ""
	}
	m.Account = get("account")
	m.Date = get("date")
	m.Amount = get("amount")
	m.AmountInflow = get("amountInflow")
	m.AmountOutflow = get("amountOutflow")
	m.Description = get("description")
	m.Category = get("category")
	m.Payee = get("payee")
	m.Tag = get("tag")
	return m, nil
}

// optFormValue returns a pointer to the form value, or nil when the field is
// absent — the service distinguishes a missing override from a blank one only
// for description; ids treat blank as absent.
func optFormValue(r *http.Request, key string) *string {
	if _, ok := r.Form[key]; !ok {
		if r.MultipartForm == nil || r.MultipartForm.Value[key] == nil {
			return nil
		}
	}
	v := r.FormValue(key)
	return &v
}
