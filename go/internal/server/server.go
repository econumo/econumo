// Package server assembles the full Econumo HTTP API from its modules. The
// wiring lives here (rather than in cmd/econumo) so that the production binary
// AND the test harnesses build the IDENTICAL handler from the same code path —
// the engine-comparison API suite (internal/test/enginecompare) depends on this
// to exercise the real production router on both SQLite and PostgreSQL without
// duplicating ~150 lines of module wiring (which would silently drift).
//
// BuildAPI takes an already-opened, already-migrated *sql.DB plus the auth/clock
// collaborators (injected so tests can pin deterministic JWT keys and time), and
// returns the same http.Handler cmd/econumo serves.
package server

import (
	"context"
	"database/sql"
	"net/http"
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
	"github.com/econumo/econumo/internal/infra/mailer"
	accountrepo "github.com/econumo/econumo/internal/infra/repo/account"
	budgetrepo "github.com/econumo/econumo/internal/infra/repo/budget"
	categoryrepo "github.com/econumo/econumo/internal/infra/repo/category"
	connectionrepo "github.com/econumo/econumo/internal/infra/repo/connection"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	operationrepo "github.com/econumo/econumo/internal/infra/repo/operation"
	passwordrequestrepo "github.com/econumo/econumo/internal/infra/repo/passwordrequest"
	payeerepo "github.com/econumo/econumo/internal/infra/repo/payee"
	tagrepo "github.com/econumo/econumo/internal/infra/repo/tag"
	transactionrepo "github.com/econumo/econumo/internal/infra/repo/transaction"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	userbudgetrepo "github.com/econumo/econumo/internal/infra/repo/userbudget"
	"github.com/econumo/econumo/internal/infra/storage/backend"
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
)

// BuildAPI wires every resource module over the given database and returns the
// full HTTP handler (global middleware chain + all /api/v1 routes + Swagger +
// SPA + health-check). The DB must already be opened and migrated. clk is the
// clock used for timestamps and JWT issuance time; jwt is the configured RS256
// signer/verifier. The engine is read from cfg.DatabaseDriver, which selects the
// per-engine sqlc query adapters in every repository constructor.
//
// This is the SAME assembly cmd/econumo uses; the only difference is that the
// caller owns opening the DB and constructing jwt/clk (so tests can inject
// deterministic ones and point at either engine).
//
// clk is a Clock (Now() time.Time) — clock.Real in production, a fixed clock in
// tests. The module services/handlers each take their own duck-typed Clock
// interface; this single value satisfies all of them.
func BuildAPI(cfg config.Config, db *sql.DB, jwt *auth.JWT, clk Clock) http.Handler {
	txm := backend.NewTxManager(db)

	encodeSvc := auth.NewEncodeService(cfg.DataSalt)
	hasher := auth.NewPasswordHasher()

	// User module.
	userRepo := userrepo.NewRepo(cfg.DatabaseDriver, txm)
	userReadRepo := userrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	currencyLookup := currencyrepo.New(cfg.DatabaseDriver, txm)
	budgetExistence := userbudgetrepo.New(cfg.DatabaseDriver, txm)

	passwordReqRepo := passwordrequestrepo.New(cfg.DatabaseDriver, txm)
	resetMailer := mailer.NewResetSender(mailer.New(cfg.ResendAPIKey), cfg.FromEmail, cfg.ReplyToEmail)
	userSvc := appuser.NewService(
		userRepo, txm, encodeSvc, hasher, jwt, currencyLookup, budgetExistence,
		passwordReqRepo, resetMailer, clk, cfg.AllowRegistration, cfg.ConnectUsers,
	)
	userReadSvc := appuser.NewReadService(userReadRepo, encodeSvc)
	userHandlers := handleruser.NewHandlers(userSvc, userReadSvc, cfg.IsDev(), clk)

	// Category module.
	categoryRepo := categoryrepo.NewRepo(cfg.DatabaseDriver, txm)
	categoryReadRepo := categoryrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	categorySvc := appcategory.NewService(categoryRepo, txm, categoryRepo, clk, categoryReadRepo)
	categoryReadSvc := appcategory.NewReadService(categoryReadRepo)
	categoryHandlers := handlercategory.NewHandlers(categorySvc, categoryReadSvc, cfg.IsDev())

	// Tag module (shared operation guard built here, reused by payee/account/transaction).
	opGuard := operationrepo.NewGuard(cfg.DatabaseDriver, txm)
	tagRepo := tagrepo.NewRepo(cfg.DatabaseDriver, txm)
	tagReadRepo := tagrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	tagSvc := apptag.NewService(tagRepo, txm, opGuard, clk, tagReadRepo)
	tagReadSvc := apptag.NewReadService(tagReadRepo)
	tagHandlers := handlertag.NewHandlers(tagSvc, tagReadSvc, cfg.IsDev())

	// Payee module.
	payeeRepo := payeerepo.NewRepo(cfg.DatabaseDriver, txm)
	payeeReadRepo := payeerepo.NewReadRepo(cfg.DatabaseDriver, txm)
	payeeSvc := apppayee.NewService(payeeRepo, txm, opGuard, clk, payeeReadRepo)
	payeeReadSvc := apppayee.NewReadService(payeeReadRepo)
	payeeHandlers := handlerpayee.NewHandlers(payeeSvc, payeeReadSvc, cfg.IsDev())

	// Currency module (read-only).
	currencyReadRepo := currencyrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	currencyReadSvc := appcurrency.NewReadService(currencyReadRepo)
	currencyHandlers := handlercurrency.NewHandlers(currencyReadSvc, cfg.IsDev())

	// Account + folder module; connection service is built first (account result
	// embeds sharedAccess[] and delete-account revokes the caller's own access).
	accountRepo := accountrepo.NewRepo(cfg.DatabaseDriver, txm)
	folderRepo := accountrepo.NewFolderRepo(cfg.DatabaseDriver, txm)
	accountCurrencyLookup := accountrepo.NewCurrencyLookup(currencyLookup)
	accountUserLookup := accountrepo.NewUserLookup(userRepo)
	connectionRepo := connectionrepo.NewRepo(cfg.DatabaseDriver, txm)
	connectionInviteRepo := connectionrepo.NewInviteRepo(cfg.DatabaseDriver, txm)
	connectionFolderPort := connectionrepo.NewFolderPort(folderRepo)
	connectionOptionPort := connectionrepo.NewOptionPort(accountRepo)
	connectionUserLookup := connectionrepo.NewUserLookup(userRepo)
	connectionBudgetRepo := budgetrepo.NewRepo(cfg.DatabaseDriver, txm)
	connectionBudgetRevoker := connectionrepo.NewBudgetAccessRevoker(connectionBudgetRepo)
	connectionSvc := appconnection.NewService(
		connectionRepo, connectionInviteRepo, connectionFolderPort, connectionOptionPort,
		connectionUserLookup, connectionBudgetRevoker, txm, clk,
	)
	accountSharedLookup := connectionrepo.NewSharedAccessLookup(connectionRepo)
	accountRevoker := connectionrepo.NewAccessRevoker(connectionRepo, connectionSvc)
	accountSvc := appaccount.NewService(
		accountRepo, folderRepo, accountCurrencyLookup, accountUserLookup, accountSharedLookup, accountRevoker, txm, opGuard, clk,
	)
	accountHandlers := handleraccount.NewHandlers(accountSvc, cfg.IsDev())

	// Transaction module.
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

	// Connection module handlers (service built above).
	connectionHandlers := handlerconnection.NewHandlers(connectionSvc, cfg.IsDev())

	// Budget module (heavy get-budget read runs the BudgetBuilder + convertor).
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

	return router.New(router.Deps{
		Cfg:         cfg,
		DB:          pinger{db},
		RegisterAPI: registerAPI,
	})
}

// Clock is the time seam BuildAPI threads into every module. clock.Real
// satisfies it in production; tests pass a fixed clock for deterministic
// timestamps and JWT issuance time.
type Clock interface {
	Now() time.Time
}

// pinger adapts *sql.DB to the router's Pinger interface (Ping(ctx) error).
type pinger struct{ db *sql.DB }

var _ router.Pinger = pinger{}

func (p pinger) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }
