// Import parsing helpers: a case-insensitive name cache (find-or-create dedup),
// CSV record parsing (header-keyed, UTF-8 BOM stripped), date parsing (a
// practical subset of PHP parseDate: Ymd, unix 10/13-digit, and the common
// explicit formats), and amount parsing (PHP parseAmount: currency-symbol strip,
// ./,-separator heuristics, ()-negatives). All stay byte-faithful to
// ImportTransactionService where it matters for the default frontend export
// format (single amount, Y-m-d H:i:s / Y-m-d).
package transaction

import (
	"bytes"
	"encoding/csv"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/datetime"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// nameCache maps a lowercased name to an entity view (ImportAccount or
// ImportNamed); it is mutated as new entities are created so later rows reuse
// them (PHP appends to the in-memory list).
type nameCache struct {
	m map[string]any
}

func newNameCache() *nameCache { return &nameCache{m: map[string]any{}} }

// newNamedCache builds a cache pre-populated from a slice of ImportNamed.
func newNamedCache(items []ImportNamed) *nameCache {
	c := newNameCache()
	for _, it := range items {
		c.put(it.Name, it)
	}
	return c
}

func (c *nameCache) get(name string) (any, bool) {
	v, ok := c.m[strings.ToLower(strings.TrimSpace(name))]
	return v, ok
}

// put stores under the lowercased name (first writer wins, matching PHP's
// linear find which returns the first case-insensitive match).
func (c *nameCache) put(name string, v any) {
	k := strings.ToLower(strings.TrimSpace(name))
	if _, exists := c.m[k]; !exists {
		c.m[k] = v
	}
}

// parseCSVRecords reads the CSV bytes header-first (setHeaderOffset 0) and
// returns the header (BOM-stripped) plus each subsequent row as a
// column-name -> value map. Fields are not pre-trimmed (fieldValue trims).
func parseCSVRecords(data []byte) (header []string, records []map[string]string, err error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1 // tolerate ragged rows like League\Csv
	r.TrimLeadingSpace = false

	rawHeader, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	header = make([]string, len(rawHeader))
	for i, h := range rawHeader {
		header[i] = stripUTF8BOM(h)
	}

	for {
		rec, rerr := r.Read()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, nil, rerr
		}
		row := make(map[string]string, len(header))
		for i, key := range header {
			if i < len(rec) {
				row[key] = rec[i]
			} else {
				row[key] = ""
			}
		}
		records = append(records, row)
	}
	return header, records, nil
}

// utf8BOM is the UTF-8 byte-order mark (EF BB BF).
const utf8BOM = "﻿"

// stripUTF8BOM removes a leading UTF-8 BOM from a header key.
func stripUTF8BOM(s string) string {
	return strings.TrimPrefix(s, utf8BOM)
}

// importDateFormats is the subset of PHP parseDate explicit formats, in PHP
// order, translated to Go layouts. Date-only formats parse at midnight (PHP '!'
// resets unspecified fields). Covers the default export format + common inputs.
var importDateFormats = []string{
	datetime.DateLayout,   // Y-m-d
	"02/01/2006",          // d/m/Y
	"01/02/2006",          // m/d/Y
	"2006/01/02",          // Y/m/d
	"02-01-2006",          // d-m-Y
	"01-02-2006",          // m-d-Y
	"02.01.2006",          // d.m.Y
	"01.02.2006",          // m.d.Y
	"2006.01.02",          // Y.m.d
	"20060102",            // Ymd
	datetime.Layout,       // Y-m-d H:i:s
	"2006-01-02 15:04",    // Y-m-d H:i
	"02/01/2006 15:04:05", // d/m/Y H:i:s
	"02/01/2006 15:04",    // d/m/Y H:i
	"01/02/2006 15:04:05", // m/d/Y H:i:s
	"01/02/2006 15:04",    // m/d/Y H:i
	"2006/01/02 15:04:05", // Y/m/d H:i:s
	"2006/01/02 15:04",    // Y/m/d H:i
	"2006-01-02T15:04:05", // Y-m-d\TH:i:s
	"2006-01-02T15:04",    // Y-m-d\TH:i
	time.RFC3339,          // Y-m-d\TH:i:sP
}

// parseImportDate parses a date string (PHP parseDate): trims quotes/space;
// 8-digit -> Ymd; 10/13-digit -> unix seconds/millis; else the explicit format
// list. Returns (time, false) when nothing matches.
func parseImportDate(s string) (time.Time, bool) {
	s = strings.Trim(s, " \t\n\r\x00\x0B\"'")
	if s == "" {
		return time.Time{}, false
	}

	if isAllDigits(s) {
		switch len(s) {
		case 8:
			if t, err := time.Parse("20060102", s); err == nil {
				return t, true
			}
		case 10, 13:
			n, err := strconv.ParseInt(s, 10, 64)
			if err == nil && n > 0 {
				if len(s) == 13 {
					n /= 1000
				}
				return time.Unix(n, 0).UTC(), true
			}
		}
	}

	for _, layout := range importDateFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// nonAmountChars matches everything except digits, dot, and comma (PHP
// preg_replace('/[^\d.,]/', ”)).
var nonAmountChars = regexp.MustCompile(`[^\d.,]`)

// parseImportAmount parses a localized amount string into a signed
// DecimalNumber (PHP parseAmount): negatives from a leading '-' or surrounding
// parentheses; strips currency symbols; resolves ./,-as decimal-vs-thousands
// separators by the last-separator heuristic. Returns (_, false) when not
// numeric.
func parseImportAmount(s string) (vo.DecimalNumber, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return vo.DecimalNumber{}, false
	}
	negative := strings.HasPrefix(trimmed, "-") ||
		(strings.Contains(trimmed, "(") && strings.Contains(trimmed, ")"))

	cleaned := nonAmountChars.ReplaceAllString(trimmed, "")
	if cleaned == "" {
		return vo.DecimalNumber{}, false
	}

	lastComma := strings.LastIndex(cleaned, ",")
	lastDot := strings.LastIndex(cleaned, ".")

	switch {
	case lastComma != -1 && lastDot != -1:
		if lastComma > lastDot {
			// comma is decimal: drop dots (thousands), comma -> dot
			cleaned = strings.ReplaceAll(cleaned, ".", "")
			cleaned = strings.ReplaceAll(cleaned, ",", ".")
		} else {
			// dot is decimal: drop commas (thousands)
			cleaned = strings.ReplaceAll(cleaned, ",", "")
		}
	case lastComma != -1:
		commaCount := strings.Count(cleaned, ",")
		if commaCount == 1 && len(cleaned)-lastComma-1 <= 2 {
			cleaned = strings.ReplaceAll(cleaned, ",", ".")
		} else {
			cleaned = strings.ReplaceAll(cleaned, ",", "")
		}
	}
	// strip any remaining commas (PHP final str_replace)
	cleaned = strings.ReplaceAll(cleaned, ",", "")

	if !isNumericDecimal(cleaned) {
		return vo.DecimalNumber{}, false
	}

	value := vo.NewDecimal(cleaned)
	if negative {
		value = negateDecimal(value)
	}
	return value, true
}

// isNumericDecimal reports whether s is a plain decimal numeric string (digits
// with at most one dot), mirroring PHP is_numeric for the cleaned value.
func isNumericDecimal(s string) bool {
	if s == "" {
		return false
	}
	dotSeen := false
	digits := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '.':
			if dotSeen {
				return false
			}
			dotSeen = true
		default:
			return false
		}
	}
	return digits > 0
}

// negateDecimal returns -v.
func negateDecimal(v vo.DecimalNumber) vo.DecimalNumber {
	return vo.NewDecimal("0").Sub(v)
}
