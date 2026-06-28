package router

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/ui/httpx"
)

// Pinger is the minimal database-health contract the health-check needs. The
// storage backend's *sql.DB satisfies this (PingContext), but the router only
// depends on this narrow interface so it stays decoupled from database/sql.
type Pinger interface {
	Ping(ctx context.Context) error
}

// healthCheckHandler returns a handler for GET /health. It returns an
// OK envelope wrapping {"database": <bool>}. When db is
// nil the database is reported as healthy (true) — used before a backend is
// wired so the route is always mountable.
func healthCheckHandler(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbUp := true
		if db != nil {
			if err := db.Ping(r.Context()); err != nil {
				dbUp = false
			}
		}
		httpx.OK(w, map[string]bool{"database": dbUp})
	}
}
