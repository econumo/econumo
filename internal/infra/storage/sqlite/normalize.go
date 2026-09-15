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
		rewritten, unparseable, err := normalizeColumn(ctx, db, c.table, c.column)
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

type tableColumn struct{ table, column string }

func datetimeColumns(ctx context.Context, db *sql.DB) ([]tableColumn, error) {
	rows, err := db.QueryContext(ctx, `SELECT m.name, p.name
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
		if err := rows.Scan(&c.table, &c.column); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type storedValue struct {
	rowid int64
	raw   string
}

// utcDriverForm is the WHERE clause that matches exactly the values
// parseDriverString accepts for a UTC value: "Y-m-d H:i:s[.f] +0000 UTC[ m=...]"
// with a real calendar datetime (strftime round-trips it unchanged; Feb 30
// does not) and a 1-9 digit fraction. For these the frozen value is the first
// 19 characters, so one UPDATE per column rewrites them and only zoned values
// go through the per-row parse. %[1]s is the value as TEXT.
const utcDriverForm = `length(%[1]s) > 19
	AND strftime('%%Y-%%m-%%d %%H:%%M:%%S', substr(%[1]s, 1, 19)) = substr(%[1]s, 1, 19)
	AND (substr(%[1]s, 20) GLOB ' +0000 UTC*'
		OR (substr(%[1]s, 20, 1) = '.'
			AND instr(substr(%[1]s, 20), ' ') BETWEEN 3 AND 11
			AND substr(%[1]s, 21, instr(substr(%[1]s, 20), ' ') - 2) NOT GLOB '*[^0-9]*'
			AND substr(%[1]s, 19 + instr(substr(%[1]s, 20), ' ')) GLOB ' +0000 UTC*'))`

func normalizeColumn(ctx context.Context, db *sql.DB, table, column string) (rewritten, unparseable int, err error) {
	t, c := quoteIdent(table), quoteIdent(column)
	text := `CAST(` + c + ` AS TEXT)`
	// A 'Y-m-d H:i:s' value is exactly 19 characters; anything longer is a
	// candidate.
	res, err := db.ExecContext(ctx, `UPDATE `+t+` SET `+c+` = substr(`+text+`, 1, 19) WHERE `+fmt.Sprintf(utcDriverForm, text))
	if err != nil {
		return 0, 0, err
	}
	n, _ := res.RowsAffected()
	rewritten = int(n)

	// rowid paging keeps each batch bounded and never revisits a row.
	selectBatch := `SELECT rowid, ` + text + ` FROM ` + t +
		` WHERE rowid > ? AND length(` + text + `) > 19 ORDER BY rowid LIMIT ?`
	update := `UPDATE ` + t + ` SET ` + c + ` = ? WHERE rowid = ? AND ` + text + ` = ?`

	after := int64(math.MinInt64)
	for {
		batch, err := readBatch(ctx, db, selectBatch, after)
		if err != nil {
			return rewritten, unparseable, err
		}
		if len(batch) == 0 {
			return rewritten, unparseable, nil
		}
		after = batch[len(batch)-1].rowid

		n, err := rewriteBatch(ctx, db, update, batch)
		rewritten += n
		unparseable += len(batch) - n
		if err != nil {
			return rewritten, unparseable, err
		}
	}
}

// rewriteBatch parses each value in Go and rewrites the ones in the driver's
// form inside one transaction, through one prepared statement. Each update is
// guarded by the value it replaces, so a re-run, or a row written concurrently,
// is never clobbered. Returns how many rows changed.
func rewriteBatch(ctx context.Context, db *sql.DB, update string, batch []storedValue) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, update)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	rewritten := 0
	for _, v := range batch {
		frozen, ok := parseDriverString(v.raw)
		if !ok {
			continue
		}
		res, err := stmt.ExecContext(ctx, frozen, v.rowid, v.raw)
		if err != nil {
			return rewritten, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			rewritten++
		}
	}
	return rewritten, tx.Commit()
}

func readBatch(ctx context.Context, db *sql.DB, query string, after int64) ([]storedValue, error) {
	rows, err := db.QueryContext(ctx, query, after, normalizeBatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storedValue
	for rows.Next() {
		var v storedValue
		if err := rows.Scan(&v.rowid, &v.raw); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func parseDriverString(raw string) (string, bool) {
	s := raw
	if i := strings.Index(s, " m="); i >= 0 {
		s = s[:i]
	}
	for _, layout := range driverStringLayouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed.UTC().Format(datetime.Layout), true
		}
	}
	return "", false
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
