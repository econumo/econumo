// View-row and import/export shapes the transaction service's port interfaces
// exchange with their adapters (not wire DTOs unless noted — no JSON tags,
// except ImportResult which is the import-transaction-list response body).
package model

// ExportAccount is one accessible account in the CSV export universe: id,
// name, and currency code.
type ExportAccount struct {
	ID           string
	Name         string
	CurrencyCode string
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
}

// ImportResult is the wire result: counts + an errors map (message ->
// [rowNumbers]).
type ImportResult struct {
	Imported int              `json:"imported"`
	Skipped  int              `json:"skipped"`
	Errors   map[string][]int `json:"errors"`
}
