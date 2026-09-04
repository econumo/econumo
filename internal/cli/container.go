package cli

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	appaccount "github.com/econumo/econumo/internal/account"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	"github.com/econumo/econumo/internal/config"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	appcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/mailer"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/server"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// container is the CLI composition root: an opened DB plus the services the
// commands use. It mirrors the constructors server.BuildAPI uses so CLI behavior
// matches the HTTP API exactly. Unlike the server it does NOT run migrations: the
// CLI assumes an already-migrated database (the server migrates on boot).
type container struct {
	db       *sql.DB
	owned    bool // true when this container opened db itself (newContainer, not newContainerFor)
	cfg      config.Config
	clk      clock.Real
	user     *appuser.Service
	currency *appcurrency.WriteService
	loader   *currencyrepo.Loader
	account  *appaccount.Service
}

// newContainer loads config, opens the database, and wires the container's
// services over it. The database backends are registered by cmd/econumo's
// blank imports.
func newContainer(ctx context.Context) (*container, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	be, ok := backend.Get(cfg.DatabaseDriver)
	if !ok {
		return nil, errors.New("no database backend registered for engine " + cfg.DatabaseDriver +
			" (registered: [" + strings.Join(backend.Registered(), ", ") + "])")
	}
	if c, ok := be.(backend.BusyTimeoutConfigurer); ok {
		c.SetBusyTimeout(cfg.SQLiteBusyTimeout)
	}
	db, err := be.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	slog.Debug("cli: database opened", "driver", cfg.DatabaseDriver)

	c := newContainerFor(cfg, db)
	c.owned = true
	return c, nil
}

// newContainerFor wires the services over an already-open database. Used by
// newContainer and by the migration command runner, which shares serve's
// connection and must not close it.
func newContainerFor(cfg config.Config, db *sql.DB) *container {
	txm := backend.NewTxManager(db)
	// Salt-free, like the API: every CLI user command runs in plaintext mode so a
	// CLI-created/edited user is readable by the running server. Only data:remove-salt
	// consumes cfg.DataSalt, and it builds its own salted encoder from it.
	encodeSvc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	clk := clock.New()

	// User service. currencyLookup + budgetExistence are required by the
	// constructor but are only exercised by the read/profile paths, not the
	// admin mutators. The access-token repo IS needed: password changes and
	// deactivation revoke stored tokens.
	userRepo := userrepo.NewRepo(cfg.DatabaseDriver, txm)
	accessTokens := userrepo.NewAccessTokenRepo(cfg.DatabaseDriver, txm)
	currencyLookup := currencyrepo.New(cfg.DatabaseDriver, txm)
	budgetExistence := server.NewUserBudgetAccess(cfg.DatabaseDriver, txm)
	passwordReqRepo := userrepo.NewPasswordRequestRepo(cfg.DatabaseDriver, txm)
	emailVerificationRepo := userrepo.NewEmailVerificationRepo(cfg.DatabaseDriver, txm)
	emailChangeRepo := userrepo.NewEmailChangeRequestRepo(cfg.DatabaseDriver, txm)
	mailTransport := mailer.Mailer(mailer.New(cfg.MailProvider, cfg.MailAPIKey))
	if cfg.AppURL != "" {
		mailTransport = mailer.WithAppLink(mailTransport, cfg.AppURL)
	}
	resetMailer := mailer.NewResetSender(mailTransport, cfg.MailFrom, cfg.MailReplyTo)
	userSvc := appuser.NewService(
		userRepo, txm, encodeSvc, hasher, accessTokens, server.NewUserCurrencyLookup(currencyLookup), budgetExistence,
		passwordReqRepo, resetMailer, emailVerificationRepo, nil,
		emailChangeRepo, nil,
		appuser.NewRandomAvatarPicker(), clk, nil, cfg.AllowRegistration, cfg.TrialDays, cfg.EmailVerification, cfg.Analytics,
	)

	currencyWriteRepo := currencyrepo.NewWriteRepo(cfg.DatabaseDriver, txm)
	currencySvc := appcurrency.NewWriteService(currencyWriteRepo, txm, clk)
	loader := currencyrepo.NewLoader(cfg.OpenExchangeRatesToken, clk.Now)

	// Account service, mirroring server.BuildAPI's wiring (internal/server/server.go).
	// Needed for the migration:* commands (e.g. zero-deleted-accounts); no CLI
	// account:* commands exist yet.
	accountRepo := accountrepo.NewRepo(cfg.DatabaseDriver, txm)
	folderRepo := accountrepo.NewFolderRepo(cfg.DatabaseDriver, txm)
	accountAccessRepo := accountrepo.NewAccessRepo(cfg.DatabaseDriver, txm)
	accountAccessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(cfg.DatabaseDriver, txm))
	opGuard := operationrepo.NewGuard(cfg.DatabaseDriver, txm)
	accountSvc := appaccount.NewService(
		accountRepo, folderRepo, accountAccessRepo, server.NewAccountCurrencyLookup(currencyLookup), server.NewUserOwnerLookup(userRepo),
		accountAccessResolver, txm, opGuard, clk,
	)

	return &container{
		db:       db,
		cfg:      cfg,
		clk:      clk,
		user:     userSvc,
		currency: currencySvc,
		loader:   loader,
		account:  accountSvc,
	}
}

// Close releases the database connection this container opened. A no-op for a
// container built by newContainerFor over a caller-owned db (e.g. the
// migration command runner sharing serve's connection): that db outlives the
// container and must not be closed here.
func (c *container) Close() {
	if c.owned && c.db != nil {
		_ = c.db.Close()
	}
}
