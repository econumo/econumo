package currency

import (
	"context"
	"log/slog"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/port"
)

// RateLoader fetches exchange rates for a date/base/symbol set. The Open
// Exchange Rates adapter (currency/repo.Loader) satisfies it; declared here so
// this package does not import its own repo subpackage.
type RateLoader interface {
	Load(ctx context.Context, date time.Time, base string, symbols []string) ([]model.RateInput, error)
}

// rateStore is the write-side surface the updater needs. *WriteService satisfies it.
type rateStore interface {
	LatestRateDate(ctx context.Context) (time.Time, bool, error)
	AvailableCodes(ctx context.Context) ([]string, error)
	UpdateRates(ctx context.Context, rates []model.RateInput) (int, error)
}

// RateUpdater periodically refreshes exchange rates in-process. Only serve
// starts it; when disabled (no interval, or no OER token) StartPolling is a
// no-op, so tests and the CLI stay hermetic.
type RateUpdater struct {
	enabled  bool
	interval time.Duration
	base     string
	loader   RateLoader
	svc      rateStore
	clock    port.Clock
}

// NewRateUpdater wires the updater. intervalDays is the refresh cadence in days;
// enabled must already fold in the OER-token presence.
func NewRateUpdater(enabled bool, intervalDays int, base string, loader RateLoader, svc rateStore, clock port.Clock) *RateUpdater {
	return &RateUpdater{
		enabled:  enabled,
		interval: time.Duration(intervalDays) * 24 * time.Hour,
		base:     base,
		loader:   loader,
		svc:      svc,
		clock:    clock,
	}
}

// StartPolling launches the background refresh (boot + every interval). No-op
// when disabled. Only serve calls this; it exits on ctx cancellation.
func (u *RateUpdater) StartPolling(ctx context.Context) {
	if !u.enabled {
		return
	}
	go func() {
		u.updateOnce(ctx)
		ticker := time.NewTicker(u.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				u.updateOnce(ctx)
			}
		}
	}()
}

// updateOnce refreshes rates unless the newest stored rate is already within the
// interval (DB-aware skip, so restart loops don't burn OER quota). All errors
// are logged and swallowed: a background job must never crash the server.
func (u *RateUpdater) updateOnce(ctx context.Context) {
	if !u.enabled {
		return
	}
	now := u.clock.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	if latest, ok, err := u.svc.LatestRateDate(ctx); err != nil {
		slog.Debug("rate updater: latest-date check failed", "err", err)
		// Fall through and try to fetch; a transient read error should not wedge
		// the schedule.
	} else if ok && today.Sub(latest) < u.interval {
		slog.Debug("rate updater: rates fresh, skipping", "latest", latest.Format("2006-01-02"))
		return
	}

	codes, err := u.svc.AvailableCodes(ctx)
	if err != nil {
		slog.Warn("rate updater: available-codes failed", "err", err)
		return
	}
	rates, err := u.loader.Load(ctx, now, u.base, codes)
	if err != nil {
		slog.Warn("rate updater: load failed", "err", err)
		return
	}
	n, err := u.svc.UpdateRates(ctx, rates)
	if err != nil {
		slog.Warn("rate updater: update failed", "err", err)
		return
	}
	slog.Info("rate updater: rates refreshed", "loaded", len(rates), "updated", n)
}
