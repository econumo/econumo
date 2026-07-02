// Package datetime defines the canonical date/time layouts shared across the
// API wire contract and the persistence edge: a space-separated datetime with
// no timezone, and a date-only form. Kept in one place so the frozen wire
// format is defined once and reused everywhere.
package datetime

const (
	// Layout is the API/persistence datetime format: "2006-01-02 15:04:05"
	// (space separator, no timezone).
	Layout = "2006-01-02 15:04:05"
	// DateLayout is the date-only format: "2006-01-02".
	DateLayout = "2006-01-02"
)
