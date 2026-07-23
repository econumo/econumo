package repo_test

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestWriteRepo_LatestRateDate(t *testing.T) {
	db := dbtest.New(t)
	w := currencyrepo.NewWriteRepo(db.Engine, db.TX)
	ctx := context.Background()

	// No rates yet -> ok=false.
	if _, ok, err := w.LatestRateDate(ctx); err != nil || ok {
		t.Fatalf("empty: ok=%v err=%v, want ok=false err=nil", ok, err)
	}

	// Seed two rates on different days; the newest wins.
	for _, d := range []time.Time{
		time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
	} {
		if err := w.UpsertRate(ctx, model.RateRow{
			ID:             vo.NewId().String(),
			CurrencyID:     seededUSD,
			BaseCurrencyID: seededUSD,
			Date:           d,
			Rate:           "1.00000000",
		}); err != nil {
			t.Fatalf("UpsertRate: %v", err)
		}
	}

	got, ok, err := w.LatestRateDate(ctx)
	if err != nil || !ok {
		t.Fatalf("LatestRateDate: ok=%v err=%v", ok, err)
	}
	if got.Format("2006-01-02") != "2026-07-22" {
		t.Errorf("LatestRateDate = %s, want 2026-07-22", got.Format("2006-01-02"))
	}
}
