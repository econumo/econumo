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

	appaccount "github.com/econumo/econumo/internal/account"
	handleraccount "github.com/econumo/econumo/internal/account/api"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appbudget "github.com/econumo/econumo/internal/budget"
	handlerbudget "github.com/econumo/econumo/internal/budget/api"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	appcategory "github.com/econumo/econumo/internal/category"
	handlercategory "github.com/econumo/econumo/internal/category/api"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	"github.com/econumo/econumo/internal/config"
	appconnection "github.com/econumo/econumo/internal/connection"
	handlerconnection "github.com/econumo/econumo/internal/connection/api"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	appcurrency "github.com/econumo/econumo/internal/currency"
	handlercurrency "github.com/econumo/econumo/internal/currency/api"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/infra/mailer"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/infra/ratelimit"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	apppayee "github.com/econumo/econumo/internal/payee"
	handlerpayee "github.com/econumo/econumo/internal/payee/api"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/shared/port"
	appsystem "github.com/econumo/econumo/internal/system"
	handlersystem "github.com/econumo/econumo/internal/system/api"
	apptag "github.com/econumo/econumo/internal/tag"
	handlertag "github.com/econumo/econumo/internal/tag/api"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
	handlertransaction "github.com/econumo/econumo/internal/transaction/api"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
	appuser "github.com/econumo/econumo/internal/user"
	handleruser "github.com/econumo/econumo/internal/user/api"
	userrepo "github.com/econumo/econumo/internal/user/repo"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/router"
)

// Seams are BuildAPI's injectable sources of nondeterminism. The zero value
// wires production defaults (real clock, random avatar picker); tests override
// them so responses stay byte-stable (golden files, engine comparison).
type Seams struct {
	Clock   port.Clock
	Avatars appuser.AvatarPicker
	// Updates is the release-check service. serve constructs an enabled one and
	// starts its poller; nil (every test and CLI path) wires a disabled service
	// that never polls, keeping responses deterministic and hermetic.
	Updates *appsystem.Service
	// Mailer overrides the reset-email transport. nil wires the config-derived
	// transport (console default / Resend); tests inject a recording transport to
	// capture the emitted reset code, which is no longer readable from the DB.
	Mailer mailer.Mailer
}

// BuildAPI wires every resource module over the given (already opened+migrated)
// database and returns the full HTTP handler. The caller owns the DB and the
// seams (clock, avatar picker) so tests can inject deterministic ones and point
// at either engine; the engine is read from cfg.DatabaseDriver, which selects
// the per-engine sqlc query adapters in every repository constructor.
func BuildAPI(cfg config.Config, db *sql.DB, seams Seams) http.Handler {
	clk := seams.Clock
	if clk == nil {
		clk = clock.New()
	}
	avatars := seams.Avatars
	if avatars == nil {
		avatars = appuser.NewRandomAvatarPicker()
	}
	txm := backend.NewTxManager(db)

	// The API ignores ECONUMO_DATA_SALT: it always runs salt-free (plaintext email,
	// md5(lower(email)) identifier). The salt is consumed only by the data:remove-salt
	// migration, so a still-salted database must be migrated before its users can log in.
	encodeSvc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()

	userRepo := userrepo.NewRepo(cfg.DatabaseDriver, txm)
	userReadRepo := userrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	accessTokens := userrepo.NewAccessTokenRepo(cfg.DatabaseDriver, txm)
	currencyLookup := currencyrepo.New(cfg.DatabaseDriver, txm)
	budgetAccess := NewUserBudgetAccess(cfg.DatabaseDriver, txm)

	passwordReqRepo := userrepo.NewPasswordRequestRepo(cfg.DatabaseDriver, txm)
	mailTransport := seams.Mailer
	if mailTransport == nil {
		mailTransport = mailer.New(cfg.MailProvider, cfg.MailAPIKey)
	}
	resetMailer := mailer.NewResetSender(mailTransport, cfg.MailFrom, cfg.MailReplyTo)
	authLimiter := ratelimit.New(ratelimit.Config{
		Limits: map[string]int{
			appuser.RateScopeLogin:              cfg.RateLimitLogin,
			appuser.RateScopeReset:              cfg.RateLimitReset,
			appuser.RateScopeRemind:             cfg.RateLimitRemind,
			appuser.RateScopeRegister:           cfg.RateLimitRegister,
			appconnection.RateScopeAcceptInvite: cfg.RateLimitAccept,
		},
		Window: cfg.RateLimitWindow,
		Global: cfg.RateLimitGlobal,
	}, clk)
	userSvc := appuser.NewService(
		userRepo, txm, encodeSvc, hasher, accessTokens, currencyLookup, budgetAccess,
		passwordReqRepo, resetMailer, avatars, clk, authLimiter, cfg.AllowRegistration, cfg.Trial,
	)
	userReadSvc := appuser.NewReadService(userReadRepo, encodeSvc, clk)
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

	updates := seams.Updates
	if updates == nil {
		updates = appsystem.NewService(false, appsystem.DefaultFeedURL)
	}
	systemHandlers := handlersystem.NewHandlers(updates, cfg.IsDev())

	accountRepo := accountrepo.NewRepo(cfg.DatabaseDriver, txm)
	folderRepo := accountrepo.NewFolderRepo(cfg.DatabaseDriver, txm)
	accountCurrencyLookup := NewAccountCurrencyLookup(currencyLookup)
	userOwnerLookup := NewUserOwnerLookup(userRepo)

	// Account service is built before connection: delete-connection's unwind
	// needs it (via ConnectionAccountAccessRevoker), and account is otherwise
	// self-sufficient — it no longer references connection.
	accountAccessRepo := accountrepo.NewAccessRepo(cfg.DatabaseDriver, txm)
	accountSvc := appaccount.NewService(
		accountRepo, folderRepo, accountAccessRepo, accountCurrencyLookup, userOwnerLookup, accountAccessResolver, txm, opGuard, clk,
	)
	accountHandlers := handleraccount.NewHandlers(accountSvc, cfg.IsDev())

	// Budget service is built before connection: delete-connection's unwind
	// removes budget memberships (access + seeded records) via RemoveMember.
	budgetRepo := budgetrepo.NewRepo(cfg.DatabaseDriver, txm)
	budgetReadRepo := budgetrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	rateProvider := currencyrepo.NewRateProvider(cfg.DatabaseDriver, txm, currencyLookup, cfg.CurrencyBase)
	convertor := appcurrency.NewConvertor(rateProvider)
	budgetSvc := appbudget.NewService(
		budgetRepo, budgetReadRepo, convertor, rateProvider,
		NewBudgetUserLookup(userRepo, clk),
		NewBudgetAccountLookup(accountRepo),
		currencyLookup,
		budgetrepo.NewMetadataLookup(NewBudgetCategoryMetadataLookup(categoryRepo), NewBudgetTagMetadataLookup(tagRepo), NewBudgetPayeeMetadataLookup(payeeRepo)),
		accountAccessResolver,
		txm, clk,
	)
	budgetHandlers := handlerbudget.NewHandlers(budgetSvc, cfg.IsDev())

	connectionRepo := connectionrepo.NewRepo(cfg.DatabaseDriver, txm)
	connectionInviteRepo := connectionrepo.NewInviteRepo(cfg.DatabaseDriver, txm)
	connectionBudgetRevoker := NewConnectionBudgetRevoker(budgetRepo, budgetSvc)
	connectionSvc := appconnection.NewService(
		connectionRepo, connectionInviteRepo, userOwnerLookup,
		NewConnectionAccountAccessRevoker(accountSvc), connectionBudgetRevoker, authLimiter, txm, clk,
	)

	transactionRepo := transactionrepo.NewRepo(cfg.DatabaseDriver, txm)

	txExportLookup := transactionrepo.NewExportLookup(transactionRepo, NewTransactionCategoryNameLookup(categoryRepo), NewTransactionTagNameLookup(tagRepo), NewTransactionPayeeNameLookup(payeeRepo))
	txImportAccounts := NewTransactionImportAccounts(accountSvc, accountRepo, folderRepo, currencyLookup, cfg.CurrencyBase)
	txImportCategories := NewTransactionImportCategories(categorySvc, categoryRepo)
	txImportTags := NewTransactionImportTags(tagSvc, tagRepo)
	txImportPayees := NewTransactionImportPayees(payeeSvc, payeeRepo)
	txImportLookup := transactionrepo.NewImportLookup(
		txImportAccounts, accountAccessResolver, txImportCategories, txImportPayees, txImportTags,
		transactionRepo,
	)
	transactionSvc := apptransaction.NewService(
		transactionRepo, accountSvc, accountAccessResolver, accountSvc, userOwnerLookup, txExportLookup, txImportLookup, txm, opGuard, clk,
	)
	transactionHandlers := handlertransaction.NewHandlers(transactionSvc, cfg.IsDev())

	connectionHandlers := handlerconnection.NewHandlers(connectionSvc, cfg.IsDev())

	authn := NewTimezoneTrackingAuthenticator(userSvc, userSvc)

	registerAPI := router.Compose(
		handleruser.RegisterAPI(userHandlers, authn, cfg.IsDev()),
		handlercategory.RegisterAPI(categoryHandlers, authn, cfg.IsDev()),
		handlertag.RegisterAPI(tagHandlers, authn, cfg.IsDev()),
		handlerpayee.RegisterAPI(payeeHandlers, authn, cfg.IsDev()),
		handlercurrency.RegisterAPI(currencyHandlers, authn, cfg.IsDev()),
		handleraccount.RegisterAPI(accountHandlers, authn, cfg.IsDev()),
		handlertransaction.RegisterAPI(transactionHandlers, authn, cfg.IsDev()),
		handlerconnection.RegisterAPI(connectionHandlers, authn, cfg.IsDev()),
		handlerbudget.RegisterAPI(budgetHandlers, authn, cfg.IsDev()),
		handlersystem.RegisterAPI(systemHandlers, authn, cfg.IsDev()),
		apidoc.RegisterAPI(),
	)

	return router.New(router.Deps{
		Cfg:                cfg,
		DB:                 pinger{db},
		RegisterAPI:        registerAPI,
		SupportedLanguages: i18n.Supported,
	})
}

type pinger struct{ db *sql.DB }

var _ router.Pinger = pinger{}

func (p pinger) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }
