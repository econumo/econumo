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

// driverStringLayout is time.Time.String(), the form the driver stored every
// bound time.Time in before WithFrozenTimeFormat. A monotonic " m=±..." suffix
// may follow it.
const driverStringLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

const normalizeBatchSize = 500

// NormalizeReport counts what NormalizeDatetimes did. Unparseable is keyed by
// "table.column" and holds values left unchanged because they are longer than
// the frozen layout but are not in the driver's string form.
type NormalizeReport struct {
	Columns     int
	Rewritten   int
	Unparseable map[string]int
}

// NormalizeDatetimes rewrites DATETIME/TIMESTAMP values stored in the driver's
// time.Time.String() form to the frozen 'Y-m-d H:i:s' UTC layout. Columns are
// discovered from the live schema. schema_migrations is skipped: its first row
// anchors the instance id. Only values that parse with the exact driver layout
// change; anything else is counted and left as stored. Each update is guarded
// by the value it replaces, so a re-run, or a row written concurrently, is
// never clobbered.
func NormalizeDatetimes(ctx context.Context, db *sql.DB) (NormalizeReport, error) {
	report := NormalizeReport{Unparseable: map[string]int{}}
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
		if unparseable > 0 {
			report.Unparseable[c.table+"."+c.column] = unparseable
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

func normalizeColumn(ctx context.Context, db *sql.DB, table, column string) (rewritten, unparseable int, err error) {
	t, c := quoteIdent(table), quoteIdent(column)
	// A 'Y-m-d H:i:s' value is exactly 19 characters; anything longer is a
	// candidate. rowid paging keeps each batch bounded and never revisits a row.
	selectBatch := `SELECT rowid, CAST(` + c + ` AS TEXT) FROM ` + t +
		` WHERE rowid > ? AND length(CAST(` + c + ` AS TEXT)) > 19 ORDER BY rowid LIMIT ?`
	update := `UPDATE ` + t + ` SET ` + c + ` = ? WHERE rowid = ? AND CAST(` + c + ` AS TEXT) = ?`

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

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return rewritten, unparseable, err
		}
		for _, v := range batch {
			frozen, ok := parseDriverString(v.raw)
			if !ok {
				unparseable++
				continue
			}
			res, err := tx.ExecContext(ctx, update, frozen, v.rowid, v.raw)
			if err != nil {
				_ = tx.Rollback()
				return rewritten, unparseable, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				rewritten++
			}
		}
		if err := tx.Commit(); err != nil {
			return rewritten, unparseable, err
		}
	}
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
	parsed, err := time.Parse(driverStringLayout, s)
	if err != nil {
		return "", false
	}
	return parsed.UTC().Format(datetime.Layout), true
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
