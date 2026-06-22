// Command econumo is the Econumo HTTP server: it loads configuration, selects
// the database backend from the DATABASE_URL scheme, runs the baseline-aware
// migrations, builds the net/http router (API + SPA), and serves.
//
// Both database backends are linked into this single binary and chosen at
// runtime; the concrete backend packages register themselves via init() and are
// blank-imported below. CGO stays off everywhere (both drivers are pure Go).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/cli"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/server"

	"github.com/joho/godotenv"

	// Blank-import the concrete DB backends so their init() registers them in
	// the backend registry. Both are linked in; the DATABASE_URL scheme selects
	// one at runtime. CGO stays off (both drivers are pure Go).
	_ "github.com/econumo/econumo/internal/infra/storage/pgsql"
	_ "github.com/econumo/econumo/internal/infra/storage/sqlite"
)

func main() {
	// `econumo -healthcheck` probes the running server's health endpoint and
	// exits 0 (healthy) / 1 (not). It lets the distroless image (no shell, no
	// curl) self-report health to Docker. Honors PORT to find the local port.
	if len(os.Args) > 1 && (os.Args[1] == "-healthcheck" || os.Args[1] == "--healthcheck") {
		os.Exit(healthcheck())
	}

	// Route to the CLI (the `bin/console app:*` management commands ported from
	// Symfony) instead of starting the server when EITHER:
	//   - the binary was invoked through the `bin/console` symlink (argv[0]
	//     basename "console") — so a bare `bin/console` prints usage rather than
	//     accidentally booting a second server, matching Symfony's console; or
	//   - a non-flag first argument names a subcommand (e.g. `econumo app:...`).
	// A bare `econumo` and flags (leading '-', e.g. -healthcheck above) fall
	// through to the HTTP server.
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	// Load a local .env (if present) for BOTH the CLI and the server paths, so
	// `./econumo app:...` and `bin/console app:...` pick up DATABASE_URL etc. the
	// same way the server does. Must happen before the dispatch below, which
	// builds its config straight away. godotenv.Load never overrides real env
	// vars, so containerized/orchestrated deployments are unaffected.
	loadDotEnv()

	invokedAsConsole := filepath.Base(os.Args[0]) == "console"
	hasSubcommand := len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-")
	if invokedAsConsole || hasSubcommand {
		os.Exit(cli.Run(os.Args[1:]))
	}

	if err := run(); err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
}

// healthcheck GETs /_/health-check on the local listen port and returns a
// process exit code (0 healthy, 1 otherwise).
func healthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		return 1 // PORT is required; without it we cannot know where to probe
	}
	port = strings.TrimPrefix(port, ":")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/_/health-check")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// loadDotEnv loads a .env file from the working directory if it exists. It is a
// convenience for running the binary directly; godotenv.Load does NOT override
// variables already present in the environment, so real env vars (set by the
// shell, Docker, k8s, CI) always win. A missing .env is not an error.
func loadDotEnv() {
	const envFile = ".env"
	if _, err := os.Stat(envFile); err != nil {
		return // no .env present — nothing to load
	}
	if err := godotenv.Load(envFile); err != nil {
		slog.Warn("failed to load .env", "err", err)
		return
	}
	slog.Info("loaded .env", "file", envFile)
}

func run() error {
	ctx := context.Background()

	// .env is loaded once in main() (before CLI/server dispatch); nothing to do
	// here. godotenv.Load never overrides real env vars, so containerized /
	// orchestrated deployments (Docker compose env_file, k8s, CI) are unaffected.
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.Info("configuration loaded",
		"app_env", cfg.AppEnv,
		"database_driver", cfg.DatabaseDriver,
		"spa_dir", cfg.SPADir,
	)

	// Server-only requirements (the CLI path validated via config.Load does not
	// need these). PORT is never defaulted so the bound port is never an implicit
	// surprise; JWT_PUBLIC_KEY is needed to verify auth tokens.
	if cfg.Port == "" {
		return errors.New("PORT is required")
	}
	if cfg.JWTPublicKeyPath == "" {
		return errors.New("JWT_PUBLIC_KEY is required")
	}

	// Select the linked backend for the engine derived from the DATABASE_URL
	// scheme. A miss is fatal with a clear message listing what is actually
	// registered (e.g. if a backend package is not blank-imported).
	be, ok := backend.Get(cfg.DatabaseDriver)
	if !ok {
		return errors.New("no database backend registered for engine " +
			cfg.DatabaseDriver + " (from DATABASE_URL); registered backends: [" +
			strings.Join(backend.Registered(), ", ") +
			"] (is the backend package blank-imported in cmd/econumo?)")
	}

	// Open the database.
	db, err := be.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	// Run migrations. The runner imports the legacy versions first so an existing
	// production DB is left intact (those migrations are recognized as already
	// applied), then applies only genuinely new migrations.
	if err := migrate.Run(ctx, db, toMigrateMigrations(be.Migrations())); err != nil {
		return err
	}
	slog.Info("migrations applied", "backend", be.Name())

	// Construct the auth crypto + clock the API assembly needs, then build the
	// full handler. The module wiring lives in internal/server.BuildAPI so the
	// production binary and the test harnesses build the IDENTICAL router from one
	// code path (see internal/server for why).
	jwt, err := auth.NewJWT(cfg.JWTSecretKeyPath, cfg.JWTPublicKeyPath, cfg.JWTPassphrase)
	if err != nil {
		return err
	}

	handler := server.BuildAPI(cfg, db, jwt, clock.New())

	srv := &http.Server{
		Addr:              addr(cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// addr normalizes the configured port (e.g. "8080" or ":8080") into a listen
// address. The port is required and validated by config.Load, so it is never
// empty here.
func addr(port string) string {
	if strings.HasPrefix(port, ":") {
		return port
	}
	return ":" + port
}

// toMigrateMigrations adapts the backend's []backend.Migration (Version/Up) to
// the migrate runner's []migrate.Migration (Version/SQL). The two packages
// define their own Migration type to avoid an import cycle, so the field copy
// happens here at the wiring layer.
func toMigrateMigrations(in []backend.Migration) []migrate.Migration {
	out := make([]migrate.Migration, len(in))
	for i, m := range in {
		out[i] = migrate.Migration{Version: m.Version, SQL: m.Up}
	}
	return out
}
