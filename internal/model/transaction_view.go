// View-row and import/export shapes the transaction service's port interfaces
// exchange with their adapters (not wire DTOs unless noted — no JSON tags,
// except ImportResult which is the import-transaction-list response body, and
// the suggest-import-columns request and result).
package model

import (
	"strings"
	"unicode/utf8"

	"github.com/econumo/econumo/internal/shared/errs"
)

// ExportAccount is one accessible account in the CSV export universe: id,
// name, and currency code.
type ExportAccount struct {
	ID           string
	Name         string
	CurrencyCode string
}

// ExportLabel is a label's name and sort key, resolved for the CSV export's
// labels column (list order, not id order, determines the export's join
// order). The key is an internal ordering token: only Name reaches the file.
type ExportLabel struct {
	Name    string
	SortKey string
}

// ImportAccount / ImportNamed are the lightweight entity views the CSV
// importer works with (id + name + owner for the belongs-to checks; a named
// entity carries no type since it's only matched by name).
type ImportAccount struct {
	ID      string
	Name    string
	OwnerID string
}

type ImportNamed struct {
	ID      string
	Name    string
	OwnerID string
	// Income is set on income categories (categories only).
	Income bool
	// Archived entries stay valid rule targets; only AI suggestions skip them.
	Archived bool
}

// ImportMapping maps logical fields to CSV column names. Empty string = unmapped.
// amountInflow/amountOutflow enable dual-amount mode (both required together).
type ImportMapping struct {
	Account       string
	Date          string
	Amount        string
	AmountInflow  string
	AmountOutflow string
	Description   string
	Category      string
	Payee         string
	Tag           string
	Labels        string
}

// ImportRequest is the decoded import request: the CSV bytes, the mapping, and
// the optional per-import overrides (nil pointer = not provided; a blank string
// is treated the same as absent for ids).
type ImportRequest struct {
	File        []byte
	Mapping     ImportMapping
	AccountId   *string
	Date        *string
	CategoryId  *string
	Description *string
	PayeeId     *string
	TagId       *string
	LabelIds    *string // comma-joined id list; applies to every row
	// LabelsSeparator splits the mapped labels cell; the caller may use any
	// delimiter (a CSV from another tool need not match the export default),
	// so this is read from the request rather than hardcoded.
	LabelsSeparator string
}

// ImportResult is the wire result: counts + an errors map (message ->
// [rowNumbers]).
type ImportResult struct {
	Imported int              `json:"imported"`
	Skipped  int              `json:"skipped"`
	Errors   map[string][]int `json:"errors"`
}

const (
	MaxImportColumns          = 60
	MaxImportColumnNameLength = 100
)

// ImportColumnFact is what the browser tells the column guesser about one CSV
// column: its header and a type computed from its cells. No cell value ever
// leaves the browser, so none can reach the AI.
type ImportColumnFact struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Negatives bool   `json:"negatives"`
}

// SuggestImportColumnsRequest is the decoded `columns` query parameter of
// suggest-import-columns, in file order.
type SuggestImportColumnsRequest struct {
	Columns []ImportColumnFact
}

func (r SuggestImportColumnsRequest) Validate() error {
	invalid := func(msg, code string) error {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "columns", Message: msg, Code: code})
	}
	if len(r.Columns) == 0 {
		return invalid("This value should not be blank.", errs.CodeIsBlank)
	}
	if len(r.Columns) > MaxImportColumns {
		return invalid("This value is too long.", errs.CodeTooLong)
	}
	for _, c := range r.Columns {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return invalid("This value should not be blank.", errs.CodeIsBlank)
		}
		if utf8.RuneCountInString(name) > MaxImportColumnNameLength {
			return invalid("This value is too long.", errs.CodeTooLong)
		}
		switch c.Type {
		case "date", "number", "text", "empty":
		default:
			return invalid("The value you selected is not a valid choice.", errs.CodeInvalidChoice)
		}
	}
	return nil
}

// SuggestImportColumnsResult maps a role (date, amount, amountIn, amountOut,
// description, payee, category, tag, labels, account) to the header the AI
// picked for it; empty when there is no suggestion.
type SuggestImportColumnsResult struct {
	Roles map[string]string `json:"roles"`
}
