package cli

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	appcurrency "github.com/econumo/econumo/internal/app/currency"
	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/infra/openexchangerates"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	passwordrequestrepo "github.com/econumo/econumo/internal/infra/repo/passwordrequest"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	userbudgetrepo "github.com/econumo/econumo/internal/infra/repo/userbudget"
	"github.com/econumo/econumo/internal/infra/storage/backend"
)

// container is the CLI composition root: an opened DB plus the services the
// commands use. It mirrors the constructors server.BuildAPI uses so CLI behavior
// matches the HTTP API exactly. Unlike the server it does NOT run migrations —
// the CLI assumes an already-migrated database (the server migrates on boot, and
// PHP's commands likewise assume a migrated schema).
type container struct {
	db       *sql.DB
	cfg      config.Config
	clk      clock.Real
	user     *appuser.Service
	currency *appcurrency.WriteService
	loader   *openexchangerates.Loader
}

// newContainer loads config, opens the database, and wires the user + currency
// services. The database backends are registered by cmd/econumo's blank imports.
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
	db, err := be.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	slog.Debug("cli: database opened", "driver", cfg.DatabaseDriver)

	txm := backend.NewTxManager(db)
	encodeSvc := auth.NewEncodeService(cfg.DataSalt)
	hasher := auth.NewPasswordHasher()
	clk := clock.New()

	// User service. The CLI admin paths never issue JWTs, so the jwt collaborator
	// is nil (constructing a real one would require mounted RSA keys we don't need
	// here). currencyLookup + budgetExistence are required by the constructor but
	// are only exercised by the read/profile paths, not the admin mutators.
	userRepo := userrepo.NewRepo(cfg.DatabaseDriver, txm)
	currencyLookup := currencyrepo.New(cfg.DatabaseDriver, txm)
	budgetExistence := userbudgetrepo.New(cfg.DatabaseDriver, txm)
	passwordReqRepo := passwordrequestrepo.New(cfg.DatabaseDriver, txm)
	resetMailer := mailer.NewResetSender(mailer.New(cfg.ResendAPIKey), cfg.FromEmail, cfg.ReplyToEmail)
	userSvc := appuser.NewService(
		userRepo, txm, encodeSvc, hasher, nil, currencyLookup, budgetExistence,
		passwordReqRepo, resetMailer, clk, cfg.AllowRegistration, cfg.ConnectUsers,
	)

	// Currency write service + the Open Exchange Rates loader.
	currencyWriteRepo := currencyrepo.NewWriteRepo(cfg.DatabaseDriver, txm)
	currencySvc := appcurrency.NewWriteService(currencyWriteRepo, txm, clk)
	loader := openexchangerates.NewLoader(cfg.OpenExchangeRatesToken, clk.Now)

	return &container{
		db:       db,
		cfg:      cfg,
		clk:      clk,
		user:     userSvc,
		currency: currencySvc,
		loader:   loader,
	}, nil
}

// Close releases the database connection.
func (c *container) Close() {
	if c.db != nil {
		_ = c.db.Close()
	}
}
