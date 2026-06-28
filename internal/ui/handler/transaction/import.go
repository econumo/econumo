package transaction

import (
	"encoding/json"
	"io"
	"net/http"

	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// maxImportUpload bounds the multipart parse (PHP File constraint maxSize 10M).
const maxImportUpload = 10 << 20

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
// @Success     200 {object} apidoc.JsonResponseOk{data=apptransaction.ImportResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/transaction/import-transaction-list [post]
func (h *Handlers) ImportTransactionList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	if err := r.ParseMultipartForm(maxImportUpload); err != nil {
		httpx.WriteError(w, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "file", Message: "Please upload a valid CSV file"}), h.dev)
		return
	}

	// mapping: a JSON object string -> ImportMapping.
	mapping, merr := parseImportMapping(r.FormValue("mapping"))
	if merr != nil {
		httpx.WriteError(w, merr, h.dev)
		return
	}

	req := apptransaction.ImportRequest{
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
			httpx.WriteError(w, rerr, h.dev)
			return
		}
		req.File = data
	}

	res, err := h.svc.ImportTransactionList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// parseImportMapping decodes the mapping JSON object into an ImportMapping. An
// invalid JSON object is a 400 ValidationError (mirrors the PHP controller's
// "Invalid mapping JSON" branch).
func parseImportMapping(raw string) (apptransaction.ImportMapping, error) {
	var m apptransaction.ImportMapping
	if raw == "" {
		return m, nil
	}
	// Tolerate null values in the object (the frontend sends nulls for unmapped
	// fields); decode into a string-pointer map first.
	var obj map[string]*string
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
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
// absent (PHP distinguishes a missing override from a blank one only for
// description; ids treat blank as absent in the service).
func optFormValue(r *http.Request, key string) *string {
	if _, ok := r.Form[key]; !ok {
		if r.MultipartForm == nil || r.MultipartForm.Value[key] == nil {
			return nil
		}
	}
	v := r.FormValue(key)
	return &v
}
