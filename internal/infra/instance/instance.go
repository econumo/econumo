// Package instance derives a stable, per-deployment identifier for product
// analytics. It anchors on the earliest schema_migrations row, which every
// deployment stamps on its first boot of the Go binary and never rewrites —
// so the value exists before the first user, needs no storage of its own, and
// reveals nothing about the instance's hostname.
package instance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
)

// ID returns the 12-hex-character instance digest, or "" when the bookkeeping
// table holds no rows yet (a database created but never migrated).
func ID(ctx context.Context, db *sql.DB) (string, error) {
	var version string
	var appliedAt any
	row := db.QueryRowContext(ctx,
		`SELECT version, applied_at FROM schema_migrations ORDER BY applied_at ASC, version ASC LIMIT 1`)
	if err := row.Scan(&version, &appliedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("instance: read schema_migrations: %w", err)
	}
	stamp, err := normalize(appliedAt)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte("econumo:instance:v1:" + version + "|" + stamp))
	return hex.EncodeToString(sum[:])[:12], nil
}

// normalize collapses the two drivers' timestamp representations to the frozen
// layout, so the same instance hashes identically whichever engine it runs on
// and whichever way its driver round-trips a TIMESTAMP.
func normalize(v any) (string, error) {
	switch t := v.(type) {
	case time.Time:
		return t.UTC().Format(datetime.Layout), nil
	case []byte:
		return normalizeString(string(t))
	case string:
		return normalizeString(t)
	default:
		return "", fmt.Errorf("instance: unsupported applied_at type %T", v)
	}
}

func normalizeString(s string) (string, error) {
	for _, layout := range []string{datetime.Layout, time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999 -0700 MST"} {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed.UTC().Format(datetime.Layout), nil
		}
	}
	return "", fmt.Errorf("instance: unparseable applied_at %q", s)
}
