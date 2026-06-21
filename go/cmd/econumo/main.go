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
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	appaccount "github.com/econumo/econumo/internal/app/account"
	appbudget "github.com/econumo/econumo/internal/app/budget"
	appcategory "github.com/econumo/econumo/internal/app/category"
	appconnection "github.com/econumo/econumo/internal/app/connection"
	appcurrency "github.com/econumo/econumo/internal/app/currency"
	apppayee "github.com/econumo/econumo/internal/app/payee"
	apptag "github.com/econumo/econumo/internal/app/tag"
	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/config"
	domcurrency "github.com/econumo/econumo/internal/domain/currency"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	accountrepo "github.com/econumo/econumo/internal/infra/repo/account"
	budgetrepo "github.com/econumo/econumo/internal/infra/repo/budget"
	categoryrepo "github.com/econumo/econumo/internal/infra/repo/category"
	connectionrepo "github.com/econumo/econumo/internal/infra/repo/connection"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	operationrepo "github.com/econumo/econumo/internal/infra/repo/operation"
	payeerepo "github.com/econumo/econumo/internal/infra/repo/payee"
	tagrepo "github.com/econumo/econumo/internal/infra/repo/tag"
	transactionrepo "github.com/econumo/econumo/internal/infra/repo/transaction"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/ui/apidoc"
	handleraccount "github.com/econumo/econumo/internal/ui/handler/account"
	handlerbudget "github.com/econumo/econumo/internal/ui/handler/budget"
	handlercategory "github.com/econumo/econumo/internal/ui/handler/category"
	handlerconnection "github.com/econumo/econumo/internal/ui/handler/connection"
	handlercurrency "github.com/econumo/econumo/internal/ui/handler/currency"
	handlerpayee "github.com/econumo/econumo/internal/ui/handler/payee"
	handlertag "github.com/econumo/econumo/internal/ui/handler/tag"
	handlertransaction "github.com/econumo/econumo/internal/ui/handler/transaction"
	handleruser "github.com/econumo/econumo/internal/ui/handler/user"
	"github.com/econumo/econumo/internal/ui/router"

	"github.com/joho/godotenv"

	// Blank-import the concrete DB backends so their init() registers them in
	// the backend registry. Both are linked in; the DATABASE_URL scheme selects
	// one at runtime. CGO stays off (both drivers are pure Go).
	_ "github.com/econumo/econumo/internal/infra/storage/pgsql"
	_ "github.com/econumo/econumo/internal/infra/storage/sqlite"
)

// defaultAddr is the listen address; overridable via the PORT env var.
const defaultAddr = ":8181"

func main() {
	// `econumo -healthcheck` probes the running server's health endpoint and
	// exits 0 (healthy) / 1 (not). It lets the distroless image (no shell, no
	// curl) self-report health to Docker. Honors PORT to find the local port.
	if len(os.Args) > 1 && (os.Args[1] == "-healthcheck" || os.Args[1] == "--healthcheck") {
		os.Exit(healthcheck())
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

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
		port = "8181"
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

	// Load a local .env into the process environment if one is present, for
	// convenience when running the binary directly (e.g. `go run`). This does NOT
	// override variables already set in the real environment, so containerized /
	// orchestrated deployments (Docker compose env_file, k8s, CI) — which inject
	// vars before the process starts — are never shadowed by a stray .env. In
	// those setups the file is typically absent anyway and this is a no-op.
	loadDotEnv()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.Info("configuration loaded",
		"app_env", cfg.AppEnv,
		"database_driver", cfg.DatabaseDriver,
		"spa_dir", cfg.SPADir,
	)

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

	// Build the user module: auth crypto, repositories (engine from the
	// DATABASE_URL scheme), the application service, and the HTTP handlers. The
	// router wraps the /api subtree in the global chain; the user module applies
	// JWT per-handler inside its RegisterAPI (public endpoints stay open).
	txm := backend.NewTxManager(db)

	encodeSvc := auth.NewEncodeService(cfg.DataSalt)
	hasher := auth.NewPasswordHasher()
	jwt, err := auth.NewJWT(cfg.JWTSecretKeyPath, cfg.JWTPublicKeyPath, cfg.JWTPassphrase)
	if err != nil {
		return err
	}

	userRepo := userrepo.NewRepo(cfg.DatabaseDriver, txm)
	userReadRepo := userrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	currencyLookup := currencyrepo.New(cfg.DatabaseDriver, txm)
	clk := clock.New()

	userSvc := appuser.NewService(
		userRepo, txm, encodeSvc, hasher, jwt, currencyLookup, clk,
		cfg.AllowRegistration, cfg.ConnectUsers,
	)
	userReadSvc := appuser.NewReadService(userReadRepo, encodeSvc)
	userHandlers := handleruser.NewHandlers(userSvc, userReadSvc, cfg.IsDev(), clk)

	// Category module: repositories (engine from the DATABASE_URL scheme), the
	// write-side service (the repo also satisfies the OperationGuard idempotency
	// seam), the read-side service, and the HTTP handlers. All 7 endpoints are
	// JWT-protected; the module applies JWT per-handler inside its RegisterAPI.
	categoryRepo := categoryrepo.NewRepo(cfg.DatabaseDriver, txm)
	categoryReadRepo := categoryrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	categorySvc := appcategory.NewService(categoryRepo, txm, categoryRepo, clk, categoryReadRepo)
	categoryReadSvc := appcategory.NewReadService(categoryReadRepo)
	categoryHandlers := handlercategory.NewHandlers(categorySvc, categoryReadSvc, cfg.IsDev())

	// Tag module: same shape as category, but a tag has no type/icon and delete is
	// unconditional. The create endpoint is idempotent via the SHARED operation
	// guard (operation_requests_ids), built once here and reusable by every module
	// that takes a client-supplied operation id.
	opGuard := operationrepo.NewGuard(cfg.DatabaseDriver, txm)
	tagRepo := tagrepo.NewRepo(cfg.DatabaseDriver, txm)
	tagReadRepo := tagrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	tagSvc := apptag.NewService(tagRepo, txm, opGuard, clk, tagReadRepo)
	tagReadSvc := apptag.NewReadService(tagReadRepo)
	tagHandlers := handlertag.NewHandlers(tagSvc, tagReadSvc, cfg.IsDev())

	// Payee module: 1:1 with tag (no type/icon, unconditional delete, per-owner
	// name uniqueness). Reuses the same shared operation guard for idempotent
	// create.
	payeeRepo := payeerepo.NewRepo(cfg.DatabaseDriver, txm)
	payeeReadRepo := payeerepo.NewReadRepo(cfg.DatabaseDriver, txm)
	payeeSvc := apppayee.NewService(payeeRepo, txm, opGuard, clk, payeeReadRepo)
	payeeReadSvc := apppayee.NewReadService(payeeReadRepo)
	payeeHandlers := handlerpayee.NewHandlers(payeeSvc, payeeReadSvc, cfg.IsDev())

	// Currency module: read-only (2 GET endpoints). The currency ReadRepo reuses
	// the same currencyrepo package as the user-module lookup. The display name is
	// resolved from the Intl table in the app layer (the stored name is NULL).
	currencyReadRepo := currencyrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	currencyReadSvc := appcurrency.NewReadService(currencyReadRepo)
	currencyHandlers := handlercurrency.NewHandlers(currencyReadSvc, cfg.IsDev())

	// Account + folder module: 12 endpoints. The account result embeds the owner
	// (via the user repo) and the full currency (via the currency lookup); the
	// balance is computed from the transactions table by the account repo. Create
	// is idempotent via the shared operation guard.
	accountRepo := accountrepo.NewRepo(cfg.DatabaseDriver, txm)
	folderRepo := accountrepo.NewFolderRepo(cfg.DatabaseDriver, txm)
	accountCurrencyLookup := accountrepo.NewCurrencyLookup(currencyLookup)
	accountUserLookup := accountrepo.NewUserLookup(userRepo)
	// Connection repo + service are constructed early so the account result can
	// embed sharedAccess[] (accounts_access grants) and so delete-account's
	// non-owner branch can revoke the caller's own access. The connection ports
	// reuse the account folder/options repos for their side effects.
	connectionRepo := connectionrepo.NewRepo(cfg.DatabaseDriver, txm)
	connectionFolderPort := connectionrepo.NewFolderPort(folderRepo)
	connectionOptionPort := connectionrepo.NewOptionPort(accountRepo)
	connectionUserLookup := connectionrepo.NewUserLookup(userRepo)
	connectionSvc := appconnection.NewService(
		connectionRepo, connectionFolderPort, connectionOptionPort, connectionUserLookup, txm, clk,
	)
	accountSharedLookup := connectionrepo.NewSharedAccessLookup(connectionRepo)
	accountRevoker := connectionrepo.NewAccessRevoker(connectionRepo, connectionSvc)
	accountSvc := appaccount.NewService(
		accountRepo, folderRepo, accountCurrencyLookup, accountUserLookup, accountSharedLookup, accountRevoker, txm, opGuard, clk,
	)
	accountHandlers := handleraccount.NewHandlers(accountSvc, cfg.IsDev())

	// Transaction module: create/update/delete/get-list (4 of 6; import+export
	// deferred). Its write results embed the account list (built by the account
	// service via the resolver adapter); access checks reduce to ownership.
	transactionRepo := transactionrepo.NewRepo(cfg.DatabaseDriver, txm)
	txAccountResolver := transactionrepo.NewAccountResolver(accountSvc)
	txVisible := transactionrepo.NewVisibleAccounts(accountSvc)
	txUserLookup := transactionrepo.NewUserLookup(userRepo)
	txExportLookup := transactionrepo.NewExportLookup(transactionRepo, categoryRepo, tagRepo, payeeRepo)
	txImportLookup := transactionrepo.NewImportLookup(
		accountSvc, accountRepo, folderRepo, categorySvc, payeeSvc, tagSvc,
		categoryRepo, tagRepo, payeeRepo, currencyLookup, transactionRepo, cfg.CurrencyBase,
	)
	transactionSvc := apptransaction.NewService(
		transactionRepo, txAccountResolver, txVisible, txUserLookup, txExportLookup, txImportLookup, txm, opGuard, clk,
	)
	transactionHandlers := handlertransaction.NewHandlers(transactionSvc, cfg.IsDev())

	// Connection module: 3 live endpoints (set/revoke account access, list) + 4
	// self-hosted 501 stubs (invites + delete-connection). The service +
	// connectionRepo are constructed above (account delete-account + sharedAccess[]
	// depend on them).
	connectionHandlers := handlerconnection.NewHandlers(connectionSvc, cfg.IsDev())

	// Budget module: the full 22-endpoint slice. The heavy get-budget read runs
	// the BudgetBuilder, which converts per-currency amounts through the currency
	// convertor (period-averaged rates). Cross-module data comes via adapters over
	// the user / account / category / tag / currency repos.
	budgetRepo := budgetrepo.NewRepo(cfg.DatabaseDriver, txm)
	budgetReadRepo := budgetrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	rateProvider := currencyrepo.NewRateProvider(cfg.DatabaseDriver, txm, currencyLookup, cfg.CurrencyBase)
	convertor := domcurrency.NewConvertor(rateProvider)
	budgetSvc := appbudget.NewService(
		budgetRepo, budgetReadRepo, convertor, rateProvider,
		budgetrepo.NewUserLookup(userRepo, clk),
		budgetrepo.NewAccountLookup(accountRepo),
		currencyLookup,
		budgetrepo.NewMetadataLookup(categoryRepo, tagRepo, payeeRepo),
		txm, clk,
	)
	budgetHandlers := handlerbudget.NewHandlers(budgetSvc, cfg.IsDev())

	// Compose the module route registrations plus the public Swagger routes into
	// the single RegisterAPI seam the router exposes.
	registerAPI := router.Compose(
		handleruser.RegisterAPI(userHandlers, jwt, cfg.IsDev()),
		handlercategory.RegisterAPI(categoryHandlers, jwt, cfg.IsDev()),
		handlertag.RegisterAPI(tagHandlers, jwt, cfg.IsDev()),
		handlerpayee.RegisterAPI(payeeHandlers, jwt, cfg.IsDev()),
		handlercurrency.RegisterAPI(currencyHandlers, jwt, cfg.IsDev()),
		handleraccount.RegisterAPI(accountHandlers, jwt, cfg.IsDev()),
		handlertransaction.RegisterAPI(transactionHandlers, jwt, cfg.IsDev()),
		handlerconnection.RegisterAPI(connectionHandlers, jwt, cfg.IsDev()),
		handlerbudget.RegisterAPI(budgetHandlers, jwt, cfg.IsDev()),
		apidoc.RegisterAPI(),
	)

	handler := router.New(router.Deps{
		Cfg:         cfg,
		DB:          pinger{db},
		RegisterAPI: registerAPI,
	})

	srv := &http.Server{
		Addr:              addr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// addr returns the listen address, honoring PORT (e.g. "8080" or ":8080").
func addr() string {
	port := os.Getenv("PORT")
	if port == "" {
		return defaultAddr
	}
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

// pinger adapts *sql.DB to the router's Pinger interface (Ping(ctx) error).
// database/sql exposes PingContext; the router uses the narrower Ping name so it
// does not depend on database/sql directly.
type pinger struct{ db *sql.DB }

// Compile-time assertion that pinger satisfies router.Pinger. This makes the
// implementation relationship explicit (Go interfaces are satisfied implicitly,
// so without this there is no visible link from the interface to its implementor)
// and lets editors resolve "go to implementation" unambiguously.
var _ router.Pinger = pinger{}

func (p pinger) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }
