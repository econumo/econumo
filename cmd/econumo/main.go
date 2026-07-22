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
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/econumo/econumo/internal/cli"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/logging"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/system"
	"github.com/econumo/econumo/internal/version"

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

	case "version", "-version", "--version":
		fmt.Println("econumo " + version.Version)
		os.Exit(0)

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
	fmt.Fprintf(w, "  %-40s %s\n", "version", "Print the binary version")
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
		"database_driver", cfg.DatabaseDriver,
		"version", version.Version,
	)
	// The API ignores ECONUMO_DATA_SALT (it always runs salt-free). If the salt is
	// still set, a database written with it has unreadable emails / mismatched
	// identifiers, so those users cannot authenticate until migrated.
	if cfg.DataSalt != "" {
		slog.Warn("ECONUMO_DATA_SALT is set but ignored by the API; existing salted users " +
			"cannot authenticate until you run `econumo data:remove-salt`, then unset ECONUMO_DATA_SALT")
	}

	// Verification emails through the console transport only reach stdout, so
	// enabling the gate without a real mailer would strand new users.
	if cfg.EmailVerification && cfg.MailProvider == "console" {
		slog.Warn("ECONUMO_EMAIL_VERIFICATION is enabled but MAILER_DSN is the console transport; " +
			"verification codes will only be printed to the server log")
	}

	// Server-only requirement (the CLI path validated via config.Load does not
	// need it). PORT is never defaulted so the bound port is never an implicit
	// surprise.
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

	// Apply the SQLite busy_timeout from config (no-op for engines without the
	// knob) before opening the connection.
	if c, ok := be.(backend.BusyTimeoutConfigurer); ok {
		c.SetBusyTimeout(cfg.SQLiteBusyTimeout)
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

	updates := system.NewService(cfg.CheckUpdates, system.DefaultFeedURL)
	handler, adminHandler := server.Build(cfg, db, server.Seams{Updates: updates})
	updates.StartPolling(ctx)

	// newServer applies the shared timeouts: a slow or stalled request body must
	// not hold a connection/goroutine indefinitely, and WriteTimeout also bounds
	// slow-client response reads.
	newServer := func(port string, h http.Handler) *http.Server {
		return &http.Server{
			Addr:              addr(port),
			Handler:           h,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
	}

	servers := []*http.Server{newServer(cfg.Port, handler)}
	// The admin listener opens only when both variables are configured, so a
	// self-hosted instance never serves those routes at all. config.Load has
	// already rejected a half-configured pair.
	if cfg.AdminPort != "" && cfg.AdminToken != "" {
		adminSrv := newServer(cfg.AdminPort, adminHandler)
		adminSrv.Addr = adminAddr(cfg.AdminPort)
		servers = append(servers, adminSrv)
		slog.Info("admin listener enabled", "addr", adminSrv.Addr)
	}

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, len(servers))
	for _, s := range servers {
		go func(s *http.Server) {
			slog.Info("listening", "addr", s.Addr)
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
				return
			}
			errCh <- nil
		}(s)
	}

	var runErr error
	select {
	case <-sigCtx.Done():
		slog.Info("shutting down")
		// A listener failure that raced the signal still decides the exit code:
		// without this drain, select picking the signal branch would swallow a
		// buffered bind error and report a clean exit for a server that never
		// came up.
		select {
		case runErr = <-errCh:
		default:
		}
	case runErr = <-errCh:
		// One listener failing leaves a half-serving binary, which is worse than
		// exiting: bring the other down too so the supervisor restarts cleanly.
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, s := range servers {
		_ = s.Shutdown(shutdownCtx)
	}
	return runErr
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

// adminAddr additionally accepts a host-qualified value ("127.0.0.1:9090") so
// the private listener can be pinned to loopback or an internal interface on
// bare-host deployments; a bare port keeps the all-interfaces default that
// container deployments rely on. PORT deliberately does not get this: the
// healthcheck subcommand derives its probe URL from PORT and assumes it is a
// bare port.
func adminAddr(v string) string {
	if strings.Contains(v, ":") && !strings.HasPrefix(v, ":") {
		return v
	}
	return addr(v)
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
