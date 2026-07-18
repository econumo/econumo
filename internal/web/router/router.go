// Package router builds the top-level HTTP handler for the Econumo service: a
// net/http.ServeMux (Go 1.22+ method+pattern routing, no router dependency)
// that mounts the internal health-check, the SPA file server as the catch-all,
// and a seam for the API route groups added by the resource modules.
//
// Route layout:
//
//	/health           (GET)  -> health check, wrapped in the global chain
//	/api/...          (*)    -> API groups, wrapped in the global chain; the
//	                            module-supplied RegisterAPI seam attaches the
//	                            public group (login/register/remind/reset, plus
//	                            /api/doc) and the authenticated group here
//	/                 (*)    -> SPA file server with index.html fallback
//
// The auth middleware itself is built in the user module and is applied by
// the API registration func to the authenticated sub-group — the router only
// supplies the global chain (requestid -> recover -> cors -> timezone -> language).
package router

import (
	"net/http"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/spa"
)

// RegisterAPI is the seam through which resource modules attach their routes.
// It is called with the API mux (whose patterns are relative to "/api", e.g.
// "POST /api/v1/category/create-category") so a module can register both its
// public and authenticated endpoints. The user module supplies the token
// middleware and decides which handlers are wrapped with it; the router does
// not impose auth itself.
//
// Implementations register handlers on apiMux only. The global chain
// (requestid/recover/cors/timezone) is applied by the router around the whole
// /api subtree, so RegisterAPI handlers must not re-add it.
type RegisterAPI func(apiMux *http.ServeMux)

// Compose combines several RegisterAPI funcs into one that invokes each in
// order on the same mux. This is how multiple resource modules (user, category,
// …) plus the swagger routes are mounted through the router's single
// RegisterAPI seam without the router knowing about each module.
func Compose(fns ...RegisterAPI) RegisterAPI {
	return func(apiMux *http.ServeMux) {
		for _, fn := range fns {
			if fn != nil {
				fn(apiMux)
			}
		}
	}
}

// Deps carries the collaborators the router needs. All fields are optional at
// this foundational stage so the router is mountable before later phases wire
// the database and API modules.
type Deps struct {
	// Cfg supplies the CORS allowlist, dev flag, and the SPA directory.
	Cfg config.Config

	// DB is used by the health-check. May be nil (reports database: true).
	DB Pinger

	// RegisterAPI attaches API route groups onto the /api subtree. May be nil
	// until the resource modules are wired (Phase 2+), in which case only the
	// health-check and SPA are served.
	RegisterAPI RegisterAPI

	// SupportedLanguages lists the Accept-Language tags the Language middleware
	// recognizes (e.g. i18n.Supported from the composition root). The router
	// stays decoupled from the translation catalogue package; nil disables
	// language resolution (every request defaults to "en").
	SupportedLanguages []string
}

// New builds the root http.Handler from deps.
func New(deps Deps) http.Handler {
	root := http.NewServeMux()

	// Global middleware chain applied to the server-side route groups
	// (internal + API). Order is outer -> inner: requestid -> accesslog ->
	// recover -> cors -> timezone -> language. (auth is added per-group inside
	// RegisterAPI by the user module — see package doc.) AccessLog sits inside
	// RequestID (so the request_id is in context) and outside Recover (so it
	// observes the 500 that Recover writes for a panic).
	global := middleware.Chain(
		middleware.RequestID,
		middleware.AccessLog,
		middleware.Recover(deps.Cfg.IsDev()),
		middleware.CORS(deps.Cfg.CORSAllowedOrigins),
		middleware.Timezone,
		middleware.Language(deps.SupportedLanguages),
	)

	// Health check. Registered directly on root; the GET /health pattern is more
	// specific than the SPA "/" catch-all, so ServeMux routes it here. Wrapped in
	// the global chain (recover + requestid + cors apply here too).
	root.Handle("GET /health", global(healthCheckHandler(deps.DB)))

	// API subtree. Modules register their concrete routes via RegisterAPI; the
	// router wraps the whole subtree in the global chain. Public vs
	// authenticated grouping happens inside RegisterAPI (the public group:
	// login/register/remind-password/reset-password + /api/doc + /api/doc.json;
	// the authenticated group: the rest, behind the auth middleware supplied by
	// the user module).
	apiMux := http.NewServeMux()
	if deps.RegisterAPI != nil {
		deps.RegisterAPI(apiMux)
	}
	root.Handle("/api/", global(apiMux))

	// SPA catch-all. Not wrapped in the API global chain (static assets do not
	// need request-id/cors/timezone); spa.Handler refuses /api and /_ paths so
	// it never shadows the server-side groups.
	// Server-owned SPA config keys, merged into the served econumo-config.js so
	// the .env values reach the frontend (the dist file's static values are the
	// fallback for separately-hosted SPAs). ANALYTICS and ALLOW_REGISTRATION are
	// always server truth (the server enforces/owns them); the rest merge only
	// when explicitly configured, so a file-configured deployment is never
	// clobbered by a default.
	overrides := map[string]any{
		"ANALYTICS":          deps.Cfg.Analytics,
		"ALLOW_REGISTRATION": deps.Cfg.AllowRegistration,
	}
	if deps.Cfg.APIURL != "" {
		overrides["API_URL"] = deps.Cfg.APIURL
	}
	if deps.Cfg.AllowCustomAPI != nil {
		overrides["ALLOW_CUSTOM_API"] = *deps.Cfg.AllowCustomAPI
	}
	root.Handle("/", spa.Handler(deps.Cfg.SPADir, overrides))

	// Browser-hardening headers wrap the WHOLE tree — including the SPA catch-all,
	// which the per-subtree global chain deliberately skips — so the served HTML
	// carries them too (framing/clickjacking protection matters most there).
	return middleware.SecurityHeaders(root)
}
