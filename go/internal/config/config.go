// Package config loads runtime configuration from environment variables. All
// loading is plain stdlib (os.Getenv) — no third-party config library.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the fully-resolved application configuration.
type Config struct {
	AppEnv string // "dev" | "prod" | "test"; controls stackTrace in the 500 envelope

	// Database
	DatabaseURL    string // DSN passed to the selected Backend; its scheme picks the engine
	DatabaseDriver string // "sqlite" | "postgresql" — DERIVED from DatabaseURL's scheme

	// Econumo behavior
	BaseURL           string
	CurrencyBase      string // default "USD"
	AllowRegistration bool
	FromEmail         string
	ReplyToEmail      string
	DataSalt          string // ECONUMO_DATA_SALT: AES key + md5 identifier salt
	SQLiteBusyTimeout int

	// Auth / JWT
	JWTSecretKeyPath string
	JWTPublicKeyPath string
	JWTPassphrase    string

	// HTTP
	Port            string // PORT: HTTP listen port ("8181" or ":8181"); required, no default
	CORSAllowOrigin string // default "*"

	// Integrations
	ResendAPIKey           string
	OpenExchangeRatesToken string

	// SPA
	SPADir string // path to web/dist/spa (served directly by the Go binary)
}

// IsDev reports whether stack traces should be exposed in the 500 envelope.
func (c Config) IsDev() bool { return c.AppEnv == "dev" }

// Load reads and validates configuration from the environment.
func Load() (Config, error) {
	c := Config{
		AppEnv:                 getEnv("APP_ENV", "prod"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		BaseURL:                os.Getenv("ECONUMO_BASE_URL"),
		CurrencyBase:           getEnv("ECONUMO_CURRENCY_BASE", "USD"),
		AllowRegistration:      getBool("ECONUMO_ALLOW_REGISTRATION", false),
		FromEmail:              os.Getenv("ECONUMO_FROM_EMAIL"),
		ReplyToEmail:           os.Getenv("ECONUMO_REPLY_TO_EMAIL"),
		DataSalt:               os.Getenv("ECONUMO_DATA_SALT"),
		SQLiteBusyTimeout:      getInt("ECONUMO_SQLITE_BUSY_TIMEOUT", 0),
		JWTSecretKeyPath:       os.Getenv("JWT_SECRET_KEY"),
		JWTPublicKeyPath:       os.Getenv("JWT_PUBLIC_KEY"),
		JWTPassphrase:          os.Getenv("JWT_PASSPHRASE"),
		Port:                   os.Getenv("PORT"),
		CORSAllowOrigin:        getEnv("CORS_ALLOW_ORIGIN", "*"),
		ResendAPIKey:           os.Getenv("RESEND_API_KEY"),
		OpenExchangeRatesToken: os.Getenv("OPEN_EXCHANGE_RATES_TOKEN"),
		SPADir:                 getEnv("ECONUMO_SPA_DIR", "web/dist/spa"),
	}

	// JWT key paths copied from a Symfony/lexik .env often contain the
	// "%kernel.project_dir%" placeholder (which Symfony resolves to the app root).
	// Expand it to the working directory so such a .env works here unchanged.
	c.JWTPublicKeyPath = ResolveProjectDir(c.JWTPublicKeyPath)
	c.JWTSecretKeyPath = ResolveProjectDir(c.JWTSecretKeyPath)

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
	// NOTE: PORT and JWT_PUBLIC_KEY are required by the HTTP server only, and are
	// validated at server startup (cmd/econumo run()). They are intentionally NOT
	// required here because config.Load is also the CLI's composition entry point
	// (bin/console app:*), and those commands neither bind a port nor issue JWTs.
	// Only DATABASE_URL (checked above) is universally required.
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

// projectDirPlaceholder is the Symfony container parameter commonly embedded in
// lexik JWT key paths (e.g. "%kernel.project_dir%/config/jwt/private.pem").
const projectDirPlaceholder = "%kernel.project_dir%"

// ResolveProjectDir expands the Symfony "%kernel.project_dir%" placeholder in a
// path to the process working directory (the app root — /var/www in the Docker
// image), so JWT key paths taken from a Symfony/lexik .env resolve here. A path
// without the placeholder is returned unchanged.
func ResolveProjectDir(path string) string {
	if !strings.Contains(path, projectDirPlaceholder) {
		return path
	}
	wd, err := os.Getwd()
	if err != nil || wd == "" {
		wd = "."
	}
	return strings.ReplaceAll(path, projectDirPlaceholder, wd)
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	// Accept common truthy/falsy string values.
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
