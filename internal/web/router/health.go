package router

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/web/httpx"
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
//
// The "admin" key is present only when the admin listener is configured, so a
// cloud monitor can assert it while a self-hosted /health shows no trace of an
// admin surface it does not have. Its value is true by construction: serve
// ties the listeners together (either failing brings the process down), so a
// served /health implies the admin listener is up. Only the enabled flag is
// exposed — never the admin address, which a public endpoint must not reveal.
func healthCheckHandler(db Pinger, adminEnabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbUp := true
		if db != nil {
			if err := db.Ping(r.Context()); err != nil {
				dbUp = false
			}
		}
		data := map[string]bool{"database": dbUp}
		if adminEnabled {
			data["admin"] = true
		}
		httpx.OK(w, data)
	}
}
