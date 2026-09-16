package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
)

// driverStringLayouts are time.Time.String(), the form the driver stored every
// bound time.Time in before WithFrozenTimeFormat. A zone with no abbreviation
// (an RFC3339 offset parsed by the CSV importer) prints its offset twice. A
// monotonic " m=±..." suffix may follow either.
var driverStringLayouts = []string{
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999 -0700 -0700",
}

const normalizeBatchSize = 500

// NormalizeReport counts what NormalizeDatetimes did. Unparseable counts values
// left unchanged because they are longer than the frozen layout but are not in
// the driver's string form; the per-column detail is logged.
type NormalizeReport struct {
	Columns     int
	Rewritten   int
	Unparseable int
}

// NormalizeDatetimes rewrites DATETIME/TIMESTAMP values stored in the driver's
// time.Time.String() form to the frozen 'Y-m-d H:i:s' UTC layout. Columns are
// discovered from the live schema. schema_migrations is skipped: its first row
// anchors the instance id. Only values that parse with the exact driver layout
// change; anything else is counted and left as stored. Each update is guarded
// by the value it replaces, so a re-run, or a row written concurrently, is
// never clobbered.
func NormalizeDatetimes(ctx context.Context, db *sql.DB) (NormalizeReport, error) {
	var report NormalizeReport
	columns, err := datetimeColumns(ctx, db)
	if err != nil {
		return report, err
	}
	report.Columns = len(columns)
	for _, c := range columns {
		rewritten, unparseable, err := normalizeColumn(ctx, db, c)
		if err != nil {
			return report, fmt.Errorf("normalize %s.%s: %w", c.table, c.column, err)
		}
		report.Rewritten += rewritten
		report.Unparseable += unparseable
		if unparseable > 0 {
			slog.WarnContext(ctx, "normalize-sqlite-datetimes: unparseable values left unchanged",
				"table", c.table, "column", c.column, "count", unparseable)
		}
	}
	return report, nil
}

// byValue marks a table that cannot be paged by rowid: declared WITHOUT
// ROWID, or with a column of its own named rowid that shadows the implicit
// one. Its values are rewritten by distinct value instead.
type tableColumn struct {
	table, column string
	byValue       bool
}

func datetimeColumns(ctx context.Context, db *sql.DB) ([]tableColumn, error) {
	rows, err := db.QueryContext(ctx, `SELECT m.name, p.name,
		  upper(m.sql) LIKE '%WITHOUT%ROWID%'
		  OR EXISTS (SELECT 1 FROM pragma_table_info(m.name) q WHERE lower(q.name) = 'rowid')
		FROM sqlite_master m, pragma_table_info(m.name) p
		WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%' AND m.name <> 'schema_migrations'
		  AND upper(p.type) IN ('DATETIME', 'TIMESTAMP')
		ORDER BY m.name, p.cid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tableColumn
	for rows.Next() {
		var c tableColumn
		if err := rows.Scan(&c.table, &c.column, &c.byValue); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// storedValue is one candidate: key is the rowid, or the value itself for a
// byValue table (rewritten by value, rows at a time).
type storedValue struct {
	key  any
	raw  string
	rows int
}

// utcDriverWhere matches exactly the values parseDriverString accepts for a
// UTC value with no monotonic suffix: "Y-m-d H:i:s[.f] +0000 UTC" with a real
// calendar datetime (strftime round-trips it unchanged; Feb 30 does not) and a
// 1-9 digit fraction. For these the frozen value is the first 19 characters,
// so one UPDATE per column rewrites them; zoned values and any " m=" suffix go
// through the per-row parse, which validates the suffix. text is the value as
// TEXT.
func utcDriverWhere(text string) string {
	suffix := func(expr string) string { return expr + ` = ' +0000 UTC'` }
	fracLen := `instr(substr(` + text + `, 20), ' ')`
	return `length(` + text + `) > 19
	AND strftime('%Y-%m-%d %H:%M:%S', substr(` + text + `, 1, 19)) = substr(` + text + `, 1, 19)
	AND (` + suffix(`substr(`+text+`, 20)`) + `
		OR (substr(` + text + `, 20, 1) = '.'
			AND ` + fracLen + ` BETWEEN 3 AND 11
			AND substr(` + text + `, 21, ` + fracLen + ` - 2) NOT GLOB '*[^0-9]*'
			AND ` + suffix(`substr(`+text+`, 19 + `+fracLen+`)`) + `))`
}

func normalizeColumn(ctx context.Context, db *sql.DB, c tableColumn) (rewritten, unparseable int, err error) {
	t, col := quoteIdent(c.table), quoteIdent(c.column)
	text := `CAST(` + col + ` AS TEXT)`
	// A 'Y-m-d H:i:s' value is exactly 19 characters; anything longer is a
	// candidate.
	res, err := db.ExecContext(ctx, `UPDATE `+t+` SET `+col+` = substr(`+text+`, 1, 19) WHERE `+utcDriverWhere(text))
	if err != nil {
		return 0, 0, err
	}
	n, _ := res.RowsAffected()
	rewritten = int(n)

	if c.byValue {
		values, err := readValues(ctx, db, `SELECT `+text+`, COUNT(*) FROM `+t+` WHERE length(`+text+`) > 19 GROUP BY 1`)
		if err != nil {
			return rewritten, 0, err
		}
		n, u, err := rewriteBatch(ctx, db, `UPDATE `+t+` SET `+col+` = ? WHERE `+text+` = ?`, values,
			func(frozen string, v storedValue) []any { return []any{frozen, v.raw} })
		return rewritten + n, u, err
	}

	// rowid paging keeps each batch bounded and never revisits a row.
	selectBatch := `SELECT rowid, ` + text + ` FROM ` + t +
		` WHERE rowid > ? AND length(` + text + `) > 19 ORDER BY rowid LIMIT ?`
	update := `UPDATE ` + t + ` SET ` + col + ` = ? WHERE rowid = ? AND ` + text + ` = ?`
	args := func(frozen string, v storedValue) []any { return []any{frozen, v.key, v.raw} }

	after := int64(math.MinInt64)
	for {
		batch, err := readBatch(ctx, db, selectBatch, after)
		if err != nil {
			return rewritten, unparseable, err
		}
		if len(batch) == 0 {
			return rewritten, unparseable, nil
		}
		after = batch[len(batch)-1].key.(int64)

		n, u, err := rewriteBatch(ctx, db, update, batch, args)
		rewritten += n
		unparseable += u
		if err != nil {
			return rewritten, unparseable, err
		}
	}
}

// rewriteBatch parses each value in Go and rewrites the ones in the driver's
// form inside one transaction, through one prepared statement. Each update is
// guarded by the value it replaces, so a re-run, or a row written concurrently,
// is never clobbered. Returns how many rows changed and how many rows held a
// value that is not in the driver's form.
func rewriteBatch(ctx context.Context, db *sql.DB, update string, batch []storedValue, args func(frozen string, v storedValue) []any) (rewritten, unparseable int, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, update)
	if err != nil {
		return 0, 0, err
	}
	defer stmt.Close()
	for _, v := range batch {
		frozen, ok := parseDriverString(v.raw)
		if !ok {
			unparseable += v.rows
			continue
		}
		res, err := stmt.ExecContext(ctx, args(frozen, v)...)
		if err != nil {
			return rewritten, unparseable, err
		}
		n, _ := res.RowsAffected()
		rewritten += int(n)
	}
	return rewritten, unparseable, tx.Commit()
}

func readValues(ctx context.Context, db *sql.DB, query string) ([]storedValue, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storedValue
	for rows.Next() {
		var v storedValue
		if err := rows.Scan(&v.raw, &v.rows); err != nil {
			return nil, err
		}
		v.key = v.raw
		out = append(out, v)
	}
	return out, rows.Err()
}

func readBatch(ctx context.Context, db *sql.DB, query string, after int64) ([]storedValue, error) {
	rows, err := db.QueryContext(ctx, query, after, normalizeBatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storedValue
	for rows.Next() {
		var rowid int64
		var raw string
		if err := rows.Scan(&rowid, &raw); err != nil {
			return nil, err
		}
		out = append(out, storedValue{key: rowid, raw: raw, rows: 1})
	}
	return out, rows.Err()
}

func parseDriverString(raw string) (string, bool) {
	s := raw
	if i := strings.Index(s, " m="); i >= 0 {
		if !validMonotonic(s[i+3:]) {
			return "", false
		}
		s = s[:i]
	}
	for _, layout := range driverStringLayouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed.UTC().Format(datetime.Layout), true
		}
	}
	return "", false
}

// validMonotonic reports whether s is how time.Time.String prints a monotonic
// reading: a sign, the whole seconds, '.', and exactly nine digits.
func validMonotonic(s string) bool {
	if s == "" || (s[0] != '+' && s[0] != '-') {
		return false
	}
	secs, frac, ok := strings.Cut(s[1:], ".")
	return ok && secs != "" && len(frac) == 9 && digitsOnly(secs) && digitsOnly(frac)
}

func digitsOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
