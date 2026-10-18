// View-row and import/export shapes the transaction service's port interfaces
// exchange with their adapters (not wire DTOs unless noted — no JSON tags,
// except ImportResult which is the import-transaction-list response body).
package model

import (
	"strings"
	"unicode/utf8"

	"github.com/econumo/econumo/internal/shared/vo"
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

// MismatchedCategory is a transaction or recurring template whose category is
// of the other kind (an expense category on income or the reverse), as the
// category-type migration finds it.
type MismatchedCategory struct {
	ID             vo.Id
	CategoryName   string
	CategoryIncome bool
}

// MaxDescriptionLength is the transactions/recurring description column's
// width (VARCHAR(255) on PostgreSQL, which counts characters).
const MaxDescriptionLength = 255

// AppendNote adds note on its own line under desc, within max characters.
// The note is kept whole: when both do not fit, desc is shortened (ending in
// an ellipsis) to make room.
func AppendNote(desc, note string, max int) string {
	desc = strings.TrimRight(desc, " \t\r\n")
	note = truncateRunes(note, max)
	if desc == "" {
		return note
	}
	room := max - utf8.RuneCountInString(note) - 1
	if room <= 0 {
		return note
	}
	if utf8.RuneCountInString(desc) > room {
		desc = string([]rune(desc)[:room-1]) + "…"
	}
	return desc + "\n" + note
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
