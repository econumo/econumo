// Package server assembles the full Econumo HTTP API from its modules. The
// wiring lives here (rather than in cmd/econumo) so the production binary AND
// the test harnesses build the IDENTICAL handler from the same code path — the
// engine-comparison suite (internal/test/enginecompare) exercises the real
// production router on both SQLite and PostgreSQL without re-wiring the modules
// (which would silently drift).
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
	"github.com/econumo/econumo/pkg/jwt"
)

// BuildAPI wires every resource module over the given (already opened+migrated)
// database and returns the full HTTP handler. The caller owns the DB, jwtSvc and
// clk so tests can inject deterministic ones and point at either engine; the
// engine is read from cfg.DatabaseDriver, which selects the per-engine sqlc query
// adapters in every repository constructor.
func BuildAPI(cfg config.Config, db *sql.DB, jwtSvc *jwt.JWT, clk Clock) http.Handler {
	txm := backend.NewTxManager(db)

	encodeSvc := auth.NewEncodeService(cfg.DataSalt)
	hasher := auth.NewPasswordHasher()

	userRepo := userrepo.NewRepo(cfg.DatabaseDriver, txm)
	userReadRepo := userrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	currencyLookup := currencyrepo.New(cfg.DatabaseDriver, txm)
	budgetExistence := userbudgetrepo.New(cfg.DatabaseDriver, txm)

	passwordReqRepo := passwordrequestrepo.New(cfg.DatabaseDriver, txm)
	resetMailer := mailer.NewResetSender(mailer.New(cfg.MailProvider, cfg.MailAPIKey), cfg.MailFrom, cfg.MailReplyTo)
	userSvc := appuser.NewService(
		userRepo, txm, encodeSvc, hasher, jwtSvc, currencyLookup, budgetExistence,
		passwordReqRepo, resetMailer, clk, cfg.AllowRegistration,
	)
	userReadSvc := appuser.NewReadService(userReadRepo, encodeSvc)
	userHandlers := handleruser.NewHandlers(userSvc, userReadSvc, cfg.IsDev(), clk)

	// Shared-account access resolver (account owner + connected-user grant role),
	// used by the category/tag create-for-account paths.
	accountAccessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(cfg.DatabaseDriver, txm))

	categoryRepo := categoryrepo.NewRepo(cfg.DatabaseDriver, txm)
	categoryReadRepo := categoryrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	categorySvc := appcategory.NewService(categoryRepo, txm, categoryRepo, clk, categoryReadRepo, accountAccessResolver)
	categoryReadSvc := appcategory.NewReadService(categoryReadRepo)
	categoryHandlers := handlercategory.NewHandlers(categorySvc, categoryReadSvc, cfg.IsDev())

	// Shared operation guard built here, reused by payee/account/transaction.
	opGuard := operationrepo.NewGuard(cfg.DatabaseDriver, txm)
	tagRepo := tagrepo.NewRepo(cfg.DatabaseDriver, txm)
	tagReadRepo := tagrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	tagSvc := apptag.NewService(tagRepo, txm, opGuard, clk, tagReadRepo, accountAccessResolver)
	tagReadSvc := apptag.NewReadService(tagReadRepo)
	tagHandlers := handlertag.NewHandlers(tagSvc, tagReadSvc, cfg.IsDev())

	payeeRepo := payeerepo.NewRepo(cfg.DatabaseDriver, txm)
	payeeReadRepo := payeerepo.NewReadRepo(cfg.DatabaseDriver, txm)
	payeeSvc := apppayee.NewService(payeeRepo, txm, opGuard, clk, payeeReadRepo, accountAccessResolver)
	payeeReadSvc := apppayee.NewReadService(payeeReadRepo)
	payeeHandlers := handlerpayee.NewHandlers(payeeSvc, payeeReadSvc, cfg.IsDev())

	currencyReadRepo := currencyrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	currencyReadSvc := appcurrency.NewReadService(currencyReadRepo)
	currencyHandlers := handlercurrency.NewHandlers(currencyReadSvc, cfg.IsDev())

	// Connection service is built first: the account result embeds sharedAccess[]
	// and delete-account revokes the caller's own access.
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

	transactionRepo := transactionrepo.NewRepo(cfg.DatabaseDriver, txm)
	txAccountResolver := transactionrepo.NewAccountResolver(accountSvc)
	txAccountGrants := transactionrepo.NewAccountGrants(connectionRepo)
	txVisible := transactionrepo.NewVisibleAccounts(accountSvc)
	txUserLookup := transactionrepo.NewUserLookup(userRepo)
	txExportLookup := transactionrepo.NewExportLookup(transactionRepo, categoryRepo, tagRepo, payeeRepo)
	txImportLookup := transactionrepo.NewImportLookup(
		accountSvc, accountAccessResolver, accountRepo, folderRepo, categorySvc, payeeSvc, tagSvc,
		categoryRepo, tagRepo, payeeRepo, currencyLookup, transactionRepo, cfg.CurrencyBase,
	)
	transactionSvc := apptransaction.NewService(
		transactionRepo, txAccountResolver, txAccountGrants, txVisible, txUserLookup, txExportLookup, txImportLookup, txm, opGuard, clk,
	)
	transactionHandlers := handlertransaction.NewHandlers(transactionSvc, cfg.IsDev())

	connectionHandlers := handlerconnection.NewHandlers(connectionSvc, cfg.IsDev())

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
		handleruser.RegisterAPI(userHandlers, jwtSvc, cfg.IsDev()),
		handlercategory.RegisterAPI(categoryHandlers, jwtSvc, cfg.IsDev()),
		handlertag.RegisterAPI(tagHandlers, jwtSvc, cfg.IsDev()),
		handlerpayee.RegisterAPI(payeeHandlers, jwtSvc, cfg.IsDev()),
		handlercurrency.RegisterAPI(currencyHandlers, jwtSvc, cfg.IsDev()),
		handleraccount.RegisterAPI(accountHandlers, jwtSvc, cfg.IsDev()),
		handlertransaction.RegisterAPI(transactionHandlers, jwtSvc, cfg.IsDev()),
		handlerconnection.RegisterAPI(connectionHandlers, jwtSvc, cfg.IsDev()),
		handlerbudget.RegisterAPI(budgetHandlers, jwtSvc, cfg.IsDev()),
		apidoc.RegisterAPI(),
	)

	return router.New(router.Deps{
		Cfg:         cfg,
		DB:          pinger{db},
		RegisterAPI: registerAPI,
	})
}

// Clock is the time seam BuildAPI threads into every module: the real clock in
// production, a fixed clock in tests for deterministic timestamps and JWT iat.
type Clock interface {
	Now() time.Time
}

type pinger struct{ db *sql.DB }

var _ router.Pinger = pinger{}

func (p pinger) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }
