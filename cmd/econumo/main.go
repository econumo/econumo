// Command econumo is the Econumo binary. It is subcommand-driven:
//
//	econumo serve                  run the HTTP API + SPA server
//	econumo healthcheck            probe a running server's health endpoint (exit 0/1)
//	econumo <resource>:<action>    management commands, e.g. user:create (see cli)
//
// `serve` is explicit so a bare invocation can never accidentally start a second
// server — with no command it prints usage and exits.
//
// Both database backends are linked into this single binary and chosen at
// runtime; the concrete backend packages register themselves via init() and are
// blank-imported below. CGO stays off everywhere (both drivers are pure Go).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/cli"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/logging"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/pkg/jwt"

	"github.com/joho/godotenv"

	// Embed the IANA timezone database into the binary. The production image is
	// distroless (no system /usr/share/zoneinfo), so without this time.LoadLocation
	// would fail for the X-Timezone header and the per-user day boundary (account
	// balance "as of end of today") would silently fall back to UTC.
	_ "time/tzdata"

	// Blank-import the concrete DB backends so their init() registers them in
	// the backend registry. Both are linked in; the DATABASE_URL scheme selects
	// one at runtime. CGO stays off (both drivers are pure Go).
	_ "github.com/econumo/econumo/internal/infra/storage/pgsql"
	_ "github.com/econumo/econumo/internal/infra/storage/sqlite"
)

func main() {
	args := os.Args[1:]

	// No command: print usage and exit. Deliberately does NOT start the server,
	// so a stray `econumo` can't boot a second instance.
	if len(args) == 0 {
		printUsage(os.Stderr)
		os.Exit(2)
	}

	switch args[0] {
	case "serve":
		// Server path: bootstrap INFO logging for the earliest diagnostics, then
		// load .env and run. run() re-applies the logger from ECONUMO_LOG_LEVEL plus any
		// -v/-vv/-vvv/-q flags once config is loaded. Returning from run() (server
		// stopped) exits 0; an error exits 1.
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
		loadDotEnv()
		if err := run(args[1:]); err != nil {
			slog.Error("startup failed", "err", err)
			os.Exit(1)
		}

	case "healthcheck", "-healthcheck", "--healthcheck":
		// Probe the running server's health endpoint and exit 0 (healthy) / 1.
		// Lets the distroless image (no shell, no curl) self-report to Docker.
		// `-healthcheck` is kept as an alias for any older healthcheck config.
		os.Exit(healthcheck())

	case "help", "-h", "--help":
		printUsage(os.Stdout)
		os.Exit(0)

	default:
		// Management commands (the resource:action set, e.g. user:create). Apply
		// verbosity (-v/-vv/-vvv/-q) FIRST so it also governs the startup logs (.env
		// load, database open); it strips those flags from the args the command sees.
		cmdArgs := cli.ConfigureLogging(args)
		loadDotEnv()
		os.Exit(cli.Run(cmdArgs))
	}
}

// printUsage writes the top-level command listing: the binary's own commands
// (serve, healthcheck) plus the management commands owned by the cli package.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: econumo <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Server:")
	fmt.Fprintf(w, "  %-40s %s\n", "serve", "Start the HTTP API + SPA server")
	fmt.Fprintf(w, "  %-40s %s\n", "healthcheck", "Probe a running server's health endpoint (exit 0/1)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Management commands:")
	cli.WriteCommandList(w)
}

// healthcheck GETs /health on the local listen port and returns a
// process exit code (0 healthy, 1 otherwise).
func healthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		return 1 // PORT is required; without it we cannot know where to probe
	}
	port = strings.TrimPrefix(port, ":")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
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

func run(serveArgs []string) error {
	ctx := context.Background()

	// .env is loaded once in main() (before CLI/server dispatch); nothing to do
	// here. godotenv.Load never overrides real env vars, so containerized /
	// orchestrated deployments (Docker compose env_file, k8s, CI) are unaffected.
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Re-apply the logger now that ECONUMO_LOG_LEVEL is known: the baseline is
	// cfg.LogLevel (default info), raised to DEBUG by -v/-vv/-vvv on the serve
	// command line (flags win); -q silences. From here on every log honors it.
	logging.Setup(cfg.LogLevel, serveArgs)
	slog.Info("configuration loaded",
		"debug", cfg.Debug,
		"database_driver", cfg.DatabaseDriver,
		"spa_dir", cfg.SPADir,
	)

	// Server-only requirements (the CLI path validated via config.Load does not
	// need these). PORT is never defaulted so the bound port is never an implicit
	// surprise; the JWT public key is needed to verify auth tokens.
	if cfg.Port == "" {
		return errors.New("PORT is required")
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

	// Generate the JWT keypair on first boot if it is missing (no keys are
	// committed or baked into the image). force=false: an existing keypair is left
	// untouched so a restart never invalidates issued tokens. Persist the key
	// directory on a volume to keep tokens valid across restarts. Same path the
	// jwt:generate CLI command uses.
	passphrase, _, err := jwt.EnsureKeypair(cfg.JWTPrivateKeyPath, cfg.JWTPublicKeyPath, cfg.JWTPassphrase, false)
	if err != nil {
		return err
	}
	jwtSvc, err := jwt.New(cfg.JWTPrivateKeyPath, cfg.JWTPublicKeyPath, passphrase)
	if err != nil {
		return err
	}

	handler := server.BuildAPI(cfg, db, jwtSvc, clock.New())

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
