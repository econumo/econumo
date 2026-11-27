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

	"github.com/econumo/econumo/internal/config"
	appcurrency "github.com/econumo/econumo/internal/currency"
	appimports "github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/infra/mailer"
	appoauth "github.com/econumo/econumo/internal/oauth"
	apponboarding "github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/shared/port"
	appsystem "github.com/econumo/econumo/internal/system"
	appuser "github.com/econumo/econumo/internal/user"
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
	// ImportProviders overrides the pull-import providers keyed by
	// model.ImportProvider* name. nil registers the real SimpleFIN client;
	// tests inject a stub so no scenario reaches the network.
	ImportProviders map[string]appimports.Provider
	// OAuthProviders, when non-nil, are the provider clients to mount; serve
	// builds them once (so the boot probe warms the same discovery cache the
	// server uses) and tests inject fakes. nil builds them from cfg.
	OAuthProviders []appoauth.Provider
	// OAuthHTTPClient is the HTTP client for provider discovery/token/JWKS
	// calls when providers are built from cfg (nil = the default 10s client);
	// the apiparity harness maps a fixed literal issuer onto its fake.
	OAuthHTTPClient *http.Client
	// BillingHTTPClient is the client for billing's get-access when
	// ECONUMO_BILLING_URL is set (nil = a client with billing.Timeout); tests
	// answer through a stub transport.
	BillingHTTPClient *http.Client
	// OnboardingCompleter, when non-nil, is the AI provider the onboarding
	// savings classification asks instead of the configured one, and
	// OnboardingRunner replaces how its background job starts; tests use them
	// to observe the job inline without a network.
	OnboardingCompleter apponboarding.Completer
	OnboardingRunner    apponboarding.Runner
	// OnboardingScheduler replaces the timer that waits for a CSV import's
	// last chunk before its icon review; tests fire it themselves.
	OnboardingScheduler apponboarding.Scheduler
	// ImportsCompleter and ImportsJobRunner do the same for the import rule
	// suggestion job.
	ImportsCompleter appimports.Completer
	ImportsJobRunner func(func())
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
	b, err := BuildWithServices(cfg, db, seams)
	if err != nil {
		return nil, nil, nil, err
	}
	return b.API, b.Admin, b.Rates, nil
}

type pinger struct{ db *sql.DB }

var _ router.Pinger = pinger{}

func (p pinger) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }

// MailTransport builds the MAILER_DSN transport; the CLI container shares it.
func MailTransport(cfg config.Config) mailer.Mailer {
	return mailer.New(mailer.Transport{
		Provider: cfg.MailProvider,
		APIKey:   cfg.MailAPIKey,
		SMTP: mailer.SMTPConfig{
			Host:        cfg.MailSMTPHost,
			Port:        cfg.MailSMTPPort,
			Username:    cfg.MailSMTPUsername,
			Password:    cfg.MailSMTPPassword,
			ImplicitTLS: cfg.MailSMTPImplicitTLS,
			Insecure:    cfg.MailSMTPInsecure,
		},
	})
}
