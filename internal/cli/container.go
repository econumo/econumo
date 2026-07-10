package cli

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	"github.com/econumo/econumo/internal/config"
	appcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/mailer"
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
	cfg      config.Config
	clk      clock.Real
	user     *appuser.Service
	currency *appcurrency.WriteService
	loader   *currencyrepo.Loader
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
	if c, ok := be.(backend.BusyTimeoutConfigurer); ok {
		c.SetBusyTimeout(cfg.SQLiteBusyTimeout)
	}
	db, err := be.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	slog.Debug("cli: database opened", "driver", cfg.DatabaseDriver)

	txm := backend.NewTxManager(db)
	// Salt-free, like the API: every CLI user command runs in plaintext mode so a
	// CLI-created/edited user is readable by the running server. Only data:remove-salt
	// consumes cfg.DataSalt, and it builds its own salted encoder from it.
	encodeSvc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	clk := clock.New()

	// User service. The CLI admin paths never issue JWTs, so the jwt collaborator
	// is nil (constructing a real one would require mounted RSA keys we don't need
	// here). currencyLookup + budgetExistence are required by the constructor but
	// are only exercised by the read/profile paths, not the admin mutators.
	userRepo := userrepo.NewRepo(cfg.DatabaseDriver, txm)
	currencyLookup := currencyrepo.New(cfg.DatabaseDriver, txm)
	budgetExistence := server.NewUserBudgetExistence(cfg.DatabaseDriver, txm)
	passwordReqRepo := userrepo.NewPasswordRequestRepo(cfg.DatabaseDriver, txm)
	resetMailer := mailer.NewResetSender(mailer.New(cfg.MailProvider, cfg.MailAPIKey), cfg.MailFrom, cfg.MailReplyTo)
	userSvc := appuser.NewService(
		userRepo, txm, encodeSvc, hasher, nil, currencyLookup, budgetExistence,
		passwordReqRepo, resetMailer, appuser.NewRandomAvatarPicker(), clk, cfg.AllowRegistration,
	)

	currencyWriteRepo := currencyrepo.NewWriteRepo(cfg.DatabaseDriver, txm)
	currencySvc := appcurrency.NewWriteService(currencyWriteRepo, txm, clk)
	loader := currencyrepo.NewLoader(cfg.OpenExchangeRatesToken, clk.Now)

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
