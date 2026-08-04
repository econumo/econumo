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
	"fmt"
	"log/slog"
	"net/http"

	appaccount "github.com/econumo/econumo/internal/account"
	handleraccount "github.com/econumo/econumo/internal/account/api"
	accountmcp "github.com/econumo/econumo/internal/account/mcp"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appadmin "github.com/econumo/econumo/internal/admin"
	handleradmin "github.com/econumo/econumo/internal/admin/api"
	appbudget "github.com/econumo/econumo/internal/budget"
	handlerbudget "github.com/econumo/econumo/internal/budget/api"
	budgetmcp "github.com/econumo/econumo/internal/budget/mcp"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	appcategory "github.com/econumo/econumo/internal/category"
	handlercategory "github.com/econumo/econumo/internal/category/api"
	categorymcp "github.com/econumo/econumo/internal/category/mcp"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	"github.com/econumo/econumo/internal/config"
	appconnection "github.com/econumo/econumo/internal/connection"
	handlerconnection "github.com/econumo/econumo/internal/connection/api"
	connectionmcp "github.com/econumo/econumo/internal/connection/mcp"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	appcurrency "github.com/econumo/econumo/internal/currency"
	handlercurrency "github.com/econumo/econumo/internal/currency/api"
	currencymcp "github.com/econumo/econumo/internal/currency/mcp"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/handoff"
	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/infra/mailer"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/infra/ratelimit"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	applabel "github.com/econumo/econumo/internal/label"
	handlerlabel "github.com/econumo/econumo/internal/label/api"
	labelrepo "github.com/econumo/econumo/internal/label/repo"
	"github.com/econumo/econumo/internal/model"
	apppayee "github.com/econumo/econumo/internal/payee"
	handlerpayee "github.com/econumo/econumo/internal/payee/api"
	payeemcp "github.com/econumo/econumo/internal/payee/mcp"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	apprecurring "github.com/econumo/econumo/internal/recurring"
	handlerrecurring "github.com/econumo/econumo/internal/recurring/api"
	recurringrepo "github.com/econumo/econumo/internal/recurring/repo"
	"github.com/econumo/econumo/internal/shared/port"
	appsystem "github.com/econumo/econumo/internal/system"
	handlersystem "github.com/econumo/econumo/internal/system/api"
	apptag "github.com/econumo/econumo/internal/tag"
	handlertag "github.com/econumo/econumo/internal/tag/api"
	tagmcp "github.com/econumo/econumo/internal/tag/mcp"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
	handlertransaction "github.com/econumo/econumo/internal/transaction/api"
	transactionmcp "github.com/econumo/econumo/internal/transaction/mcp"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
	appuser "github.com/econumo/econumo/internal/user"
	handleruser "github.com/econumo/econumo/internal/user/api"
	usermcp "github.com/econumo/econumo/internal/user/mcp"
	userrepo "github.com/econumo/econumo/internal/user/repo"
	"github.com/econumo/econumo/internal/version"
	"github.com/econumo/econumo/internal/web/apidoc"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
	"github.com/econumo/econumo/web"
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
	api, _, _, err := Build(cfg, db, seams)
	if err != nil {
		// Test-only entry: every harness uses the seeded USD base. serve calls
		// Build directly and reports the error.
		panic(err)
	}
	return api
}

// Build wires the service graph once and returns both edges: the public API
// handler and the private admin handler. They share one set of services rather
// than each opening its own, and serve decides whether to listen on the admin
// one (see cfg.AdminPort/AdminToken). The third return is the in-process
// currency-rate updater; serve starts its poller, BuildAPI and tests discard it.
func Build(cfg config.Config, db *sql.DB, seams Seams) (http.Handler, http.Handler, *appcurrency.RateUpdater, error) {
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
	// looked up by lower(email)). The salt is consumed only by the data:remove-salt
	// migration, so a still-salted database must be migrated before its users can log in.
	encodeSvc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()

	userRepo := userrepo.NewRepo(cfg.DatabaseDriver, txm)
	userReadRepo := userrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	accessTokens := userrepo.NewAccessTokenRepo(cfg.DatabaseDriver, txm)
	currencyLookup := currencyrepo.New(cfg.DatabaseDriver, txm)

	// The base currency is resolved exactly once: every stored rate is
	// denominated "X per 1 base unit", so consumers get the resolved row, not
	// a code to re-look-up. A bad code refuses to boot instead of failing at
	// runtime as lookup errors.
	baseID, err := currencyLookup.GetIDByCode(context.Background(), cfg.CurrencyBase)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ECONUMO_CURRENCY_BASE=%q does not match any global currency; add it first with currency:add", cfg.CurrencyBase)
	}
	base := model.BaseCurrency{ID: baseID, Code: cfg.CurrencyBase}

	budgetAccess := NewUserBudgetAccess(cfg.DatabaseDriver, txm)

	passwordReqRepo := userrepo.NewPasswordRequestRepo(cfg.DatabaseDriver, txm)
	emailVerificationRepo := userrepo.NewEmailVerificationRepo(cfg.DatabaseDriver, txm)
	emailChangeRepo := userrepo.NewEmailChangeRequestRepo(cfg.DatabaseDriver, txm)
	mailTransport := seams.Mailer
	if mailTransport == nil {
		mailTransport = mailer.New(cfg.MailProvider, cfg.MailAPIKey)
	}
	if cfg.AppURL != "" {
		mailTransport = mailer.WithAppLink(mailTransport, cfg.AppURL)
	}
	resetMailer := mailer.NewResetSender(mailTransport, cfg.MailFrom, cfg.MailReplyTo)
	verifyMailer := mailer.NewVerifySender(mailTransport, cfg.MailFrom, cfg.MailReplyTo)
	changeMailer := mailer.NewChangeEmailSender(mailTransport, cfg.MailFrom, cfg.MailReplyTo)
	authLimiter := ratelimit.New(ratelimit.Config{
		Limits: map[string]int{
			appuser.RateScopeLogin:              cfg.RateLimitLogin,
			appuser.RateScopeReset:              cfg.RateLimitReset,
			appuser.RateScopeRemind:             cfg.RateLimitRemind,
			appuser.RateScopeRegister:           cfg.RateLimitRegister,
			appuser.RateScopeVerifyEmail:        cfg.RateLimitVerifyEmail,
			appuser.RateScopeConfirmEmail:       cfg.RateLimitConfirmEmail,
			appuser.RateScopeRequestEmailChange: cfg.RateLimitRequestEmailChange,
			appuser.RateScopeConfirmEmailChange: cfg.RateLimitConfirmEmailChange,
			appconnection.RateScopeAcceptInvite: cfg.RateLimitAccept,
		},
		Window: cfg.RateLimitWindow,
		Global: cfg.RateLimitGlobal,
	}, clk)
	userSvc := appuser.NewService(
		userRepo, txm, encodeSvc, hasher, accessTokens, NewUserCurrencyLookup(currencyLookup), budgetAccess,
		passwordReqRepo, resetMailer, emailVerificationRepo, verifyMailer,
		emailChangeRepo, changeMailer,
		avatars, clk, authLimiter, cfg.AllowRegistration, cfg.TrialDays, cfg.EmailVerification,
	)
	userReadSvc := appuser.NewReadService(userReadRepo, encodeSvc, clk)
	billingSvc := appuser.NewBillingService(cfg.BillingURL, handoff.NewSigner(cfg.AdminToken), clk)
	userHandlers := handleruser.NewHandlers(userSvc, userReadSvc, clk, billingSvc)

	// Shared-account access resolver (account owner + connected-user grant role),
	// used by the category/tag create-for-account paths.
	accountAccessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(cfg.DatabaseDriver, txm))

	categoryRepo := categoryrepo.NewRepo(cfg.DatabaseDriver, txm)
	categoryReadRepo := categoryrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	categorySvc := appcategory.NewService(categoryRepo, txm, categoryRepo, clk, categoryReadRepo, accountAccessResolver)
	categoryReadSvc := appcategory.NewReadService(categoryReadRepo)
	categoryHandlers := handlercategory.NewHandlers(categorySvc, categoryReadSvc)

	// Shared operation guard built here, reused by payee/account/transaction.
	opGuard := operationrepo.NewGuard(cfg.DatabaseDriver, txm)
	tagRepo := tagrepo.NewRepo(cfg.DatabaseDriver, txm)
	tagReadRepo := tagrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	tagSvc := apptag.NewService(tagRepo, txm, opGuard, clk, tagReadRepo, accountAccessResolver)
	tagReadSvc := apptag.NewReadService(tagReadRepo)
	tagHandlers := handlertag.NewHandlers(tagSvc, tagReadSvc)

	labelRepo := labelrepo.NewRepo(cfg.DatabaseDriver, txm)
	labelReadRepo := labelrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	labelSvc := applabel.NewService(labelRepo, txm, opGuard, clk, labelReadRepo, accountAccessResolver)
	labelReadSvc := applabel.NewReadService(labelReadRepo)
	labelHandlers := handlerlabel.NewHandlers(labelSvc, labelReadSvc)

	payeeRepo := payeerepo.NewRepo(cfg.DatabaseDriver, txm)
	payeeReadRepo := payeerepo.NewReadRepo(cfg.DatabaseDriver, txm)
	payeeSvc := apppayee.NewService(payeeRepo, txm, opGuard, clk, payeeReadRepo, accountAccessResolver)
	payeeReadSvc := apppayee.NewReadService(payeeReadRepo)
	payeeHandlers := handlerpayee.NewHandlers(payeeSvc, payeeReadSvc)

	currencyReadRepo := currencyrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	currencyReadSvc := appcurrency.NewReadService(currencyReadRepo, base)
	currencyManageRepo := currencyrepo.NewManageRepo(cfg.DatabaseDriver, txm)
	currencyManageSvc := appcurrency.NewManageService(currencyManageRepo, txm, opGuard, clk,
		NewCurrencyProfileCurrency(userRepo), base)
	currencyHandlers := handlercurrency.NewHandlers(currencyReadSvc, currencyManageSvc)

	// In-process exchange-rate updater. Enabled only when a cadence is set AND an
	// OER token is present; serve starts its poller (Build's third return),
	// BuildAPI and tests discard it (a disabled updater never polls).
	currencyWriteRepo := currencyrepo.NewWriteRepo(cfg.DatabaseDriver, txm)
	// An admin MAY change the base; old rate rows stay untouched and inert
	// (they carry their own base_currency_id). This WARN is the only
	// acknowledgment of such a change.
	if latestBase, ok, lbErr := currencyWriteRepo.LatestRateBase(context.Background()); lbErr == nil && ok && latestBase != base.ID {
		slog.Warn("stored rates were written against a different base currency; conversions will use only rates written against the current base",
			"configured_base", base.Code, "stored_base_currency_id", latestBase)
	}
	currencyWriteSvc := appcurrency.NewWriteService(currencyWriteRepo, txm, clk)
	rateLoader := currencyrepo.NewLoader(cfg.OpenExchangeRatesToken, clk.Now)
	rateUpdaterEnabled := cfg.CurrencyUpdateIntervalDays > 0 && cfg.OpenExchangeRatesToken != ""
	if cfg.CurrencyUpdateIntervalDays > 0 && cfg.OpenExchangeRatesToken == "" {
		slog.Warn("ECONUMO_CURRENCY_UPDATE_INTERVAL is set but OPEN_EXCHANGE_RATES_TOKEN is empty; in-process rate updates are disabled")
	}
	rateUpdater := appcurrency.NewRateUpdater(
		rateUpdaterEnabled, cfg.CurrencyUpdateIntervalDays, cfg.CurrencyBase,
		rateLoader, currencyWriteSvc, clk,
	)

	updates := seams.Updates
	if updates == nil {
		updates = appsystem.NewService(false, appsystem.DefaultFeedURL)
	}
	systemHandlers := handlersystem.NewHandlers(updates)

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
	accountHandlers := handleraccount.NewHandlers(accountSvc)

	// Budget service is built before connection: delete-connection's unwind
	// removes budget memberships (access + seeded records) via RemoveMember.
	budgetRepo := budgetrepo.NewRepo(cfg.DatabaseDriver, txm)
	budgetReadRepo := budgetrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	rateProvider := currencyrepo.NewRateProvider(cfg.DatabaseDriver, txm, currencyLookup, base.ID)
	convertor := appcurrency.NewConvertor(rateProvider)
	budgetSvc := appbudget.NewService(
		budgetRepo, budgetReadRepo, convertor, rateProvider,
		NewBudgetUserLookup(userRepo, clk),
		NewBudgetAccountLookup(accountRepo),
		NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(NewBudgetCategoryMetadataLookup(categoryRepo), NewBudgetTagMetadataLookup(tagRepo), NewBudgetPayeeMetadataLookup(payeeRepo)),
		accountAccessResolver,
		txm, clk,
	)
	budgetHandlers := handlerbudget.NewHandlers(budgetSvc)

	connectionRepo := connectionrepo.NewRepo(cfg.DatabaseDriver, txm)
	connectionInviteRepo := connectionrepo.NewInviteRepo(cfg.DatabaseDriver, txm)
	connectionBudgetRevoker := NewConnectionBudgetRevoker(budgetRepo, budgetSvc)
	connectionSvc := appconnection.NewService(
		connectionRepo, connectionInviteRepo, userOwnerLookup,
		NewConnectionAccountAccessRevoker(accountSvc), connectionBudgetRevoker, authLimiter, txm, clk,
	)

	transactionRepo := transactionrepo.NewRepo(cfg.DatabaseDriver, txm)

	txExportLookup := transactionrepo.NewExportLookup(transactionRepo, NewTransactionCategoryNameLookup(categoryRepo), NewTransactionTagNameLookup(tagRepo), NewTransactionPayeeNameLookup(payeeRepo), NewTransactionLabelNameLookup(labelRepo))
	txImportAccounts := NewTransactionImportAccounts(accountSvc, accountRepo, folderRepo, currencyLookup, cfg.CurrencyBase)
	txImportCategories := NewTransactionImportCategories(categorySvc, categoryRepo)
	txImportTags := NewTransactionImportTags(tagSvc, tagRepo)
	txImportPayees := NewTransactionImportPayees(payeeSvc, payeeRepo)
	txImportLookup := transactionrepo.NewImportLookup(
		txImportAccounts, accountAccessResolver, txImportCategories, txImportPayees, txImportTags,
		transactionRepo,
	)
	// One label-ownership adapter, reused across every feature that validates
	// labelIds against the label repository (same pattern as userOwnerLookup
	// above): transaction and recurring both just want "who owns this label".
	labelOwnership := NewTransactionLabelOwnership(labelRepo)
	transactionSvc := apptransaction.NewService(
		transactionRepo, accountSvc, accountAccessResolver, accountSvc, userOwnerLookup, txExportLookup, txImportLookup,
		labelOwnership, txm, opGuard, clk,
	)
	transactionHandlers := handlertransaction.NewHandlers(transactionSvc)

	recurringRepo := recurringrepo.NewRepo(cfg.DatabaseDriver, txm)
	recurringSvc := apprecurring.NewService(recurringRepo, accountSvc, accountAccessResolver, accountSvc, transactionSvc, labelOwnership, txm, opGuard, clk)
	recurringHandlers := handlerrecurring.NewHandlers(recurringSvc)

	connectionHandlers := handlerconnection.NewHandlers(connectionSvc)

	authn := NewTimezoneTrackingAuthenticator(userSvc, userSvc)

	registerAPI := router.Compose(
		handleruser.RegisterAPI(userHandlers, authn),
		handlercategory.RegisterAPI(categoryHandlers, authn),
		handlertag.RegisterAPI(tagHandlers, authn),
		handlerlabel.RegisterAPI(labelHandlers, authn),
		handlerpayee.RegisterAPI(payeeHandlers, authn),
		handlercurrency.RegisterAPI(currencyHandlers, authn),
		handleraccount.RegisterAPI(accountHandlers, authn),
		handlertransaction.RegisterAPI(transactionHandlers, authn),
		handlerrecurring.RegisterAPI(recurringHandlers, authn),
		handlerconnection.RegisterAPI(connectionHandlers, authn),
		handlerbudget.RegisterAPI(budgetHandlers, authn),
		handlersystem.RegisterAPI(systemHandlers, authn),
		apidoc.RegisterAPI(),
	)

	adminSvc := appadmin.NewService(NewAdminUserAccess(userSvc), connectionRepo, clk)
	adminMux := http.NewServeMux()
	handleradmin.RegisterAdmin(handleradmin.NewHandlers(adminSvc))(adminMux)
	// No CORS (never browser-reached) and no timezone/language (nothing here is
	// user-facing; datetimes are frozen UTC).
	adminHandler := middleware.Chain(
		middleware.RequestID,
		middleware.AccessLog,
		middleware.Recover,
		middleware.AdminAuth(cfg.AdminToken),
	)(adminMux)

	mcpRegister := webmcp.Compose(
		categorymcp.Register(categoryReadSvc, categorySvc),
		tagmcp.Register(tagReadSvc, tagSvc),
		payeemcp.Register(payeeReadSvc, payeeSvc),
		accountmcp.Register(accountSvc),
		currencymcp.Register(currencyReadSvc),
		budgetmcp.Register(budgetSvc),
		usermcp.Register(userReadSvc),
		connectionmcp.Register(connectionSvc),
		transactionmcp.Register(transactionSvc),
	)
	mcpHandler := middleware.Chain(
		middleware.Auth(authn),
		timezoneFallback(userSvc),
	)(webmcp.NewHandler(mcpRegister))

	// The SPA is always embedded in the binary. The served econumo-config.js
	// reports the running binary's version, overridable via ECONUMO_VERSION
	// (handy for demo/staging environments).
	spaFS, _ := web.DistFS()
	spaVersion := cfg.Version
	if spaVersion == "" {
		spaVersion = version.Version
	}
	return router.New(router.Deps{
		Cfg:                cfg,
		DB:                 pinger{db},
		RegisterAPI:        registerAPI,
		SupportedLanguages: i18n.Supported,
		MCP:                mcpHandler,
		SPA:                spaFS,
		SPAVersion:         spaVersion,
	}), adminHandler, rateUpdater, nil
}

type pinger struct{ db *sql.DB }

var _ router.Pinger = pinger{}

func (p pinger) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }
