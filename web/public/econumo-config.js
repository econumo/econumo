// These are FALLBACK values only. They matter in exactly two contexts with no
// Go server in front of them: the mobile app's bundled Capacitor WebView
// (web/src/lib/appConfig.ts merges the running backend's own config over this
// on top), and `pnpm dev` without a backend running (vite.config.ts proxies
// /econumo-config.js to the Go server and falls back to this file when that
// proxy has nothing to talk to). A server-served instance NEVER reads this
// file — internal/web/router builds the whole document in Go and
// internal/web/spa writes it verbatim — so editing a value here has NO effect
// on a running instance; change the matching ECONUMO_* environment variable
// instead (see the "Web UI config" bullet in CLAUDE.md).
window.econumoConfig = {
  LILTAG_CONFIG_URL: '/liltag-config.json',
  LILTAG_CACHE_TTL: 0,
  ALLOW_REGISTRATION: true,
  INSTANCE_ID: '',
  BILLING_URL: '',
  AI_ENABLED: false,
  ALLOW_CUSTOM_API: true,
  VERSION: null,
  VERSION_LABEL: null,
  IMPORT_MATCHER: { matchDays: 3, tipDays: 5, tipTolerancePct: 20, tokenMinLength: 3 },
};
