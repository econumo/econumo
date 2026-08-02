package server_test

// Boot-time base-currency behavior: an unresolvable ECONUMO_CURRENCY_BASE
// refuses to serve, and rates stored against a different base draw a WARN
// (nothing is rewritten -- old rows are inert by design).

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

func baseTestConfig(engine string) config.Config {
	return config.Config{
		DatabaseDriver:     engine,
		CurrencyBase:       "USD",
		AllowRegistration:  true,
		CORSAllowedOrigins: []string{"*"},
		RateLimitLogin:     5,
		RateLimitReset:     5,
		RateLimitRemind:    3,
		RateLimitRegister:  5,
		RateLimitWindow:    15 * time.Minute,
		RateLimitGlobal:    60,
	}
}

func TestBuild_UnknownBaseCurrency_Fails(t *testing.T) {
	db := dbtest.NewSQLite(t)
	cfg := baseTestConfig(db.Engine)
	cfg.CurrencyBase = "XXX"
	_, _, _, err := server.Build(cfg, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)})
	if err == nil {
		t.Fatal("Build with an unknown base code must fail")
	}
	want := `ECONUMO_CURRENCY_BASE="XXX" does not match any global currency; add it first with currency:add`
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}

// warnRecorder captures WARN-level slog output for the drift assertion.
type warnRecorder struct {
	slog.Handler
	msgs *[]string
}

func (h warnRecorder) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		*h.msgs = append(*h.msgs, r.Message)
	}
	return nil
}

func captureWarns(t *testing.T) *[]string {
	t.Helper()
	msgs := &[]string{}
	prev := slog.Default()
	slog.SetDefault(slog.New(warnRecorder{Handler: prev.Handler(), msgs: msgs}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return msgs
}

func TestBuild_RateBaseDrift_Warns(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	eur := f.Currency(fixture.Currency{Code: "EUR", Symbol: "E"})
	f.Rate(fixture.Rate{CurrencyID: eur, BaseCurrencyID: eur, PublishedAt: "2026-07-01"})

	msgs := captureWarns(t)
	if _, _, _, err := server.Build(baseTestConfig(db.Engine), db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*msgs, "\n")
	if !strings.Contains(joined, "different base currency") {
		t.Fatalf("want a rate-base drift WARN, got warns: %q", joined)
	}
}

func TestBuild_NoRates_NoDriftWarn(t *testing.T) {
	db := dbtest.NewSQLite(t)
	msgs := captureWarns(t)
	if _, _, _, err := server.Build(baseTestConfig(db.Engine), db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}); err != nil {
		t.Fatal(err)
	}
	for _, m := range *msgs {
		if strings.Contains(m, "different base currency") {
			t.Fatalf("unexpected drift WARN: %q", m)
		}
	}
}
