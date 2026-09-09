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
//	/mcp              (*)    -> MCP endpoint (JSON-RPC over Streamable HTTP),
//	                            wrapped in the global chain plus the caller-
//	                            supplied auth/timezone-fallback handler; nil
//	                            Deps.MCP leaves it unmounted
//	/                 (*)    -> SPA file server with index.html fallback
//
// The auth middleware itself is built in the user module and is applied by
// the API registration func to the authenticated sub-group — the router only
// supplies the global chain (requestid -> recover -> cors -> timezone -> language).
package router

import (
	"io/fs"
	"net/http"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/spa"
	"github.com/econumo/econumo/web"
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
	// Cfg supplies the CORS allowlist and the SPA config-override values.
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

	// MCP is the fully-wrapped MCP endpoint handler (auth + timezone fallback
	// applied by the composition root). Nil = endpoint not mounted.
	MCP http.Handler

	// SPA is the filesystem the SPA catch-all serves. In production it is the
	// SPA embedded in the binary (web.DistFS); it is a seam for tests to inject
	// a fixture FS. Nil falls back to the embedded build.
	SPA fs.FS

	// SPAVersion is the version string merged into the served econumo-config.js
	// as VERSION: the running binary's own version, resolved by the composition
	// root and never overridable, because the SPA compares it as a real version
	// (analytics $app_version, the update check, the mobile app's compatibility
	// floors). Empty is tolerated: VERSION is still emitted, as JSON null
	// (never happens in production — server.BuildAPI always resolves a
	// non-empty value).
	SPAVersion string

	// SPAVersionLabel is merged as VERSION_LABEL, the version text the UI
	// DISPLAYS (the ECONUMO_VERSION override, handy for relabelling a
	// demo/staging box). Empty falls back to SPAVersion, so the key is always
	// present with a resolved value and a plain instance shows its real
	// version.
	SPAVersionLabel string

	// MinAppVersion is merged into the served econumo-config.js as
	// MIN_APP_VERSION — the oldest mobile-app build this backend accepts
	// (version.MinAppVersion, injected by the composition root like SPAVersion
	// so this leaf never imports the version package). The web SPA ships
	// embedded, so it can never be stale and ignores the key.
	MinAppVersion string

	// InstanceID is the per-deployment digest merged into the served
	// econumo-config.js as INSTANCE_ID (instance.ID, resolved by the
	// composition root against the migrated database). Empty (an unmigrated
	// database) is still emitted as "" — the key is always present — so the
	// SPA sends no instance.
	InstanceID string
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
		middleware.Recover,
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

	// MCP endpoint. Mounted at the root (outside /api: JSON-RPC, not the REST
	// contract, so the apiparity machinery must not scan it) but inside the
	// same global chain.
	if deps.MCP != nil {
		root.Handle("/mcp", global(deps.MCP))
	}

	// SPA catch-all. Not wrapped in the API global chain (static assets do not
	// need request-id/cors/timezone); spa.Handler refuses /api and /_ paths so
	// it never shadows the server-side groups.
	// The served econumo-config.js is now generated ENTIRELY here — spa.Handler
	// writes this map verbatim, with no merge against the dist file — so every
	// key ships with a default, matching exactly what web/public/econumo-config.js
	// hard-codes for the two contexts with no Go server in front of them (the
	// mobile app bundle, `pnpm dev` without a backend; see that file's header).
	// One rule: the backend value overwrites the default whenever it is present.
	allowCustomAPI := true
	if deps.Cfg.AllowCustomAPI != nil {
		allowCustomAPI = *deps.Cfg.AllowCustomAPI
	}
	liltagConfigURL := "/liltag-config.json"
	if deps.Cfg.LiltagConfigURL != "" {
		liltagConfigURL = deps.Cfg.LiltagConfigURL
	}
	// The dist default is the JS NUMBER 0, not the string "0" — LiltagCacheTTL
	// is a plain string field (a raw seconds value passed through to the SPA),
	// so an explicit override stays a JSON string while the untouched default
	// preserves the dist file's original type rather than tidying it to match.
	var liltagCacheTTL any = 0
	if deps.Cfg.LiltagCacheTTL != "" {
		liltagCacheTTL = deps.Cfg.LiltagCacheTTL
	}
	// The dist default is JS null. deps.SPAVersion is always non-empty in
	// production (server.BuildAPI resolves the binary version), so an empty
	// value here only happens when a caller builds Deps directly (tests).
	var version any
	if deps.SPAVersion != "" {
		version = deps.SPAVersion
	}
	versionLabel := version
	if deps.SPAVersionLabel != "" {
		versionLabel = deps.SPAVersionLabel
	}
	overrides := map[string]any{
		"ALLOW_REGISTRATION": deps.Cfg.AllowRegistration,
		// Present even when empty: the backend decides whether create-billing-link
		// works, so an empty value must switch the SPA's billing UI off rather than
		// leave a stale default pointing at a portal the server will not mint for.
		"BILLING_URL":       deps.Cfg.BillingURL,
		"ALLOW_CUSTOM_API":  allowCustomAPI,
		"LILTAG_CONFIG_URL": liltagConfigURL,
		"LILTAG_CACHE_TTL":  liltagCacheTTL,
		// Empty on a database that has not been migrated yet, in which case
		// the SPA sends no instance identifier — matching the dist default.
		"INSTANCE_ID":   deps.InstanceID,
		"VERSION":       version,
		"VERSION_LABEL": versionLabel,
	}
	// MIN_APP_VERSION is the one key that stays conditional: the app's
	// version-check treats a present-but-empty value differently from an
	// absent one, so an unset floor is omitted entirely rather than emitted
	// as "" (the dist file has no key for it either).
	if deps.MinAppVersion != "" {
		overrides["MIN_APP_VERSION"] = deps.MinAppVersion
	}
	// The SPA is always embedded in the binary; deps.SPA is the injection seam
	// for tests. A nil value falls back to the embedded FS.
	spaFS := deps.SPA
	if spaFS == nil {
		spaFS, _ = web.DistFS()
	}
	root.Handle("/", spa.Handler(spaFS, overrides))

	// Browser-hardening headers wrap the WHOLE tree — including the SPA catch-all,
	// which the per-subtree global chain deliberately skips — so the served HTML
	// carries them too (framing/clickjacking protection matters most there).
	return middleware.SecurityHeaders(root)
}
