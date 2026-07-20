// Package config loads runtime configuration from environment variables. All
// loading is plain stdlib (os.Getenv) — no third-party config library.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully-resolved application configuration.
type Config struct {
	// Database
	DatabaseURL    string // DSN passed to the selected Backend; its scheme picks the engine
	DatabaseDriver string // "sqlite" | "postgresql" — DERIVED from DatabaseURL's scheme

	// Econumo behavior
	CurrencyBase      string // default "USD"
	AllowRegistration bool
	DataSalt          string // ECONUMO_DATA_SALT. DEPRECATED and IGNORED by the API/repositories (they run salt-free); consumed only by the data:remove-salt migration to decrypt existing data. Unset it after migrating.
	SQLiteBusyTimeout int
	CheckUpdates      bool   // ECONUMO_CHECK_UPDATES: poll econumo.com for the latest release (default true)
	Analytics         bool   // ECONUMO_ANALYTICS: SPA sends anonymous product events to PostHog (default true)
	Trial             string // ECONUMO_TRIAL: "none" (default) or "end-of-next-month" — grants new registrations full access until the trial ends

	// Auth brute-force protection (see the 2026-07-09 auth-rate-limiting spec).
	// Counts are attempts per key per RateLimitWindow; 0 disables a check.
	RateLimitLogin    int           // ECONUMO_RATE_LIMIT_LOGIN: failed logins per username
	RateLimitReset    int           // ECONUMO_RATE_LIMIT_RESET: failed reset attempts per username
	RateLimitRemind   int           // ECONUMO_RATE_LIMIT_REMIND: remind requests per username
	RateLimitRegister int           // ECONUMO_RATE_LIMIT_REGISTER: register attempts per email
	RateLimitAccept   int           // ECONUMO_RATE_LIMIT_ACCEPT_INVITE: accept-invite attempts per user (short-code brute-force guard)
	RateLimitWindow   time.Duration // ECONUMO_RATE_LIMIT_WINDOW: sliding window (Go duration)
	RateLimitGlobal   int           // ECONUMO_RATE_LIMIT_GLOBAL: per-endpoint cap per minute

	// Mail — all DERIVED from MAILER_DSN, whose scheme selects the transport
	// (empty/console/log -> console to stdout; resend://<key> -> Resend).
	MailerDSN    string // raw MAILER_DSN, kept for reference/logging
	MailProvider string // "console" | "resend"
	MailAPIKey   string // transport credential (the Resend API key)
	MailFrom     string // from query param
	MailReplyTo  string // reply_to query param

	// HTTP
	Port               string   // PORT: HTTP listen port ("8181" or ":8181"); required, no default
	CORSAllowedOrigins []string // ECONUMO_CORS_ALLOW_ORIGIN: comma-separated allowlist; empty = same-domain only; "*" = allow all

	// Logging
	LogLevel string // ECONUMO_LOG_LEVEL: base slog level (debug|info|warn|error); default "info". Raised to DEBUG by -v/-vv/-vvv.

	// Integrations
	OpenExchangeRatesToken string

	// SPA
	SPADir string // path to web/dist (served directly by the Go binary)

	// Optional SPA config overrides merged into the served econumo-config.js.
	// Empty/nil = leave the dist file's value (the server does not enforce
	// these; they only reach the frontend).
	APIURL         string // ECONUMO_API_URL
	AllowCustomAPI *bool  // ECONUMO_ALLOW_CUSTOM_API
}

// Load reads and validates configuration from the environment.
func Load() (Config, error) {
	c := Config{
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		CurrencyBase:           getEnv("ECONUMO_CURRENCY_BASE", "USD"),
		AllowRegistration:      getBool("ECONUMO_ALLOW_REGISTRATION", false),
		MailerDSN:              os.Getenv("MAILER_DSN"),
		DataSalt:               os.Getenv("ECONUMO_DATA_SALT"),
		SQLiteBusyTimeout:      getInt("SQLITE_BUSY_TIMEOUT", 0),
		CheckUpdates:           getBool("ECONUMO_CHECK_UPDATES", true),
		Port:                   os.Getenv("PORT"),
		CORSAllowedOrigins:     getStringList("ECONUMO_CORS_ALLOW_ORIGIN", nil),
		LogLevel:               getEnv("ECONUMO_LOG_LEVEL", "info"),
		OpenExchangeRatesToken: os.Getenv("OPEN_EXCHANGE_RATES_TOKEN"),
		SPADir:                 getEnv("ECONUMO_WEB_DIST", "web/dist"),
	}

	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	// The database engine is derived from the URL scheme — the URL is the single
	// source of truth, so there is no separate DATABASE_DRIVER to drift from it.
	driver, err := driverFromURL(c.DatabaseURL)
	if err != nil {
		return Config{}, err
	}
	c.DatabaseDriver = driver

	// The mail transport is likewise derived from a single scheme-prefixed DSN.
	// An empty MAILER_DSN is valid (the console transport), so this only errors on
	// a malformed/unsupported DSN — surfacing the typo at boot, like DATABASE_URL.
	provider, apiKey, from, replyTo, err := parseMailerDSN(c.MailerDSN)
	if err != nil {
		return Config{}, err
	}
	c.MailProvider, c.MailAPIKey, c.MailFrom, c.MailReplyTo = provider, apiKey, from, replyTo

	// Strict parse (unlike the lenient getBool): a typo while trying to
	// DISABLE analytics must fail at boot, not silently leave it enabled.
	analytics, err := getBoolStrict("ECONUMO_ANALYTICS", true)
	if err != nil {
		return Config{}, err
	}
	c.Analytics = analytics

	c.Trial = getEnv("ECONUMO_TRIAL", "none")
	if c.Trial != "none" && c.Trial != "end-of-next-month" {
		return Config{}, fmt.Errorf("ECONUMO_TRIAL: invalid value %q (want none or end-of-next-month)", c.Trial)
	}

	if v := os.Getenv("ECONUMO_API_URL"); v != "" {
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Config{}, fmt.Errorf("ECONUMO_API_URL: not an absolute http(s) URL: %q", v)
		}
		c.APIURL = v
	}
	allowCustomAPI, err := getBoolOptional("ECONUMO_ALLOW_CUSTOM_API")
	if err != nil {
		return Config{}, err
	}
	c.AllowCustomAPI = allowCustomAPI

	// Rate-limit values fail at boot on a malformed value (unlike the lenient
	// getInt), because a typo here would silently disable brute-force protection.
	for _, p := range []struct {
		dst *int
		key string
		def int
	}{
		{&c.RateLimitLogin, "ECONUMO_RATE_LIMIT_LOGIN", 5},
		{&c.RateLimitReset, "ECONUMO_RATE_LIMIT_RESET", 5},
		{&c.RateLimitRemind, "ECONUMO_RATE_LIMIT_REMIND", 3},
		{&c.RateLimitRegister, "ECONUMO_RATE_LIMIT_REGISTER", 5},
		{&c.RateLimitAccept, "ECONUMO_RATE_LIMIT_ACCEPT_INVITE", 10},
		{&c.RateLimitGlobal, "ECONUMO_RATE_LIMIT_GLOBAL", 60},
	} {
		n, err := getIntStrict(p.key, p.def)
		if err != nil {
			return Config{}, err
		}
		*p.dst = n
	}
	window, werr := getDurationStrict("ECONUMO_RATE_LIMIT_WINDOW", 15*time.Minute)
	if werr != nil {
		return Config{}, werr
	}
	c.RateLimitWindow = window

	// PORT is required by the HTTP server only and is validated at server
	// startup; it is intentionally NOT required here because config.Load is also
	// the CLI's composition entry point, and those commands never bind a port.
	// Only DATABASE_URL is universally required.
	return c, nil
}

// driverFromURL maps a DATABASE_URL scheme to a backend driver name.
//
//	sqlite://...                  -> "sqlite"
//	postgres://... | postgresql://... -> "postgresql"
//
// The scheme is the single source of truth for the engine; there is no separate
// DATABASE_DRIVER env var.
func driverFromURL(url string) (string, error) {
	scheme, _, ok := strings.Cut(url, "://")
	if !ok {
		return "", fmt.Errorf("DATABASE_URL %q has no scheme (expected sqlite:// or postgresql://)", url)
	}
	switch strings.ToLower(scheme) {
	case "sqlite":
		return "sqlite", nil
	case "postgres", "postgresql":
		return "postgresql", nil
	default:
		return "", fmt.Errorf("unsupported DATABASE_URL scheme %q (want sqlite, postgres, or postgresql)", scheme)
	}
}

// parseMailerDSN maps a MAILER_DSN to the mail transport and envelope. The scheme
// selects the provider, the host carries the credential, and from / reply_to come
// from the query — mirroring how driverFromURL reads the DATABASE_URL scheme.
//
//	(empty)              -> console, no envelope
//	console:// | log://  -> console; from / reply_to from the query
//	resend://<api_key>   -> resend; from / reply_to from the query (api_key required)
//
// An empty DSN is the supported default (console), so only a malformed or
// unsupported DSN returns an error.
func parseMailerDSN(dsn string) (provider, apiKey, from, replyTo string, err error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "console", "", "", "", nil
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", "", "", "", fmt.Errorf("MAILER_DSN %q is not a valid URL: %w", dsn, err)
	}
	q := u.Query()
	from, replyTo = q.Get("from"), q.Get("reply_to")
	switch strings.ToLower(u.Scheme) {
	case "console", "log":
		return "console", "", from, replyTo, nil
	case "resend":
		// url.Parse keeps the host verbatim (no case-folding), so a "re_…" key
		// survives intact in u.Hostname().
		key := u.Hostname()
		if key == "" {
			return "", "", "", "", fmt.Errorf("MAILER_DSN %q is missing the Resend API key (want resend://<api_key>)", dsn)
		}
		return "resend", key, from, replyTo, nil
	default:
		return "", "", "", "", fmt.Errorf("unsupported MAILER_DSN scheme %q (want resend, console/log, or empty)", u.Scheme)
	}
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getBoolStrict(key string, def bool) (bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("%s: invalid boolean %q", key, v)
}

// getBoolOptional is the tri-state getBoolStrict: nil when the variable is
// unset/empty, an error on garbage (never a silent fallback).
func getBoolOptional(key string) (*bool, error) {
	if v, ok := os.LookupEnv(key); !ok || v == "" {
		return nil, nil
	}
	b, err := getBoolStrict(key, false)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func getBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// getStringList reads a comma-separated env var into a slice, trimming each item
// and dropping empties. An unset or all-empty value yields def.
func getStringList(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		if s := strings.TrimSpace(item); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func getInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// getIntStrict is getInt with a hard failure on malformed or negative input,
// for settings where a silent fallback would be a security downgrade.
func getIntStrict(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s %q is not a non-negative integer", key, v)
	}
	return n, nil
}

func getDurationStrict(key string, def time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s %q is not a positive Go duration (e.g. \"15m\")", key, v)
	}
	return d, nil
}
