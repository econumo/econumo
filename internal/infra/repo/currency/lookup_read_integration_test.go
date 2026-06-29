package currencyrepo_test

// USD is seeded by the baseline migration. Note: the setup helper in
// convertor_provider_test.go DELETEs and re-seeds currencies with a different id,
// so these tests use dbtest's pristine migrated DB (seeded USD = dffc2a06...) and
// their own distinct identifiers.

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const seededUSD = "dffc2a06-6f29-4704-8575-31709adee926"

func TestCurrencyLookup_GetIDByCode(t *testing.T) {
	db := dbtest.NewSQLite(t)
	lookup := currencyrepo.New("sqlite", db.TX)
	ctx := context.Background()

	id, err := lookup.GetIDByCode(ctx, "USD")
	if err != nil {
		t.Fatalf("GetIDByCode(USD): %v", err)
	}
	if id != seededUSD {
		t.Errorf("want %s, got %s", seededUSD, id)
	}

	_, err = lookup.GetIDByCode(ctx, "ZZZ")
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound for unknown code, got %v", err)
	}

	if lookup.DefaultCode() != "USD" {
		t.Errorf("DefaultCode = %q, want USD", lookup.DefaultCode())
	}
}

func TestCurrencyLookup_GetByID(t *testing.T) {
	db := dbtest.NewSQLite(t)
	lookup := currencyrepo.New("sqlite", db.TX)
	ctx := context.Background()

	v, err := lookup.GetByID(ctx, seededUSD)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if v.Code != "USD" || v.Symbol != "$" {
		t.Errorf("currency view mismatch: %+v", v)
	}
	if v.Name == "" {
		t.Error("expected a resolved display name")
	}

	_, err = lookup.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound for missing currency, got %v", err)
	}
}

func TestCurrencyReadRepo_CurrencyListView(t *testing.T) {
	db := dbtest.NewSQLite(t)
	read := currencyrepo.NewReadRepo("sqlite", db.TX)
	ctx := context.Background()

	rows, err := read.CurrencyListView(ctx)
	if err != nil {
		t.Fatalf("CurrencyListView: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least the seeded USD currency")
	}
	var foundUSD bool
	for _, r := range rows {
		if r.Code == "USD" {
			foundUSD = true
			if r.ID != seededUSD {
				t.Errorf("USD id mismatch: %q", r.ID)
			}
		}
	}
	if !foundUSD {
		t.Error("USD missing from currency list view")
	}
}

func TestCurrencyReadRepo_LatestRateListView(t *testing.T) {
	db := dbtest.NewSQLite(t)
	read := currencyrepo.NewReadRepo("sqlite", db.TX)
	ctx := context.Background()

	// Seed a second currency + a rate; the rate's NUMERIC(19,8) must normalize to
	// the frozen wire form (trailing zeros trimmed).
	eur := "1ae5bfd5-03e8-412b-80d2-c0ecf3ce32fe"
	f := fixture.New(t, db)
	f.Currency(fixture.Currency{ID: eur, Code: "EUR", Symbol: "E"})
	f.Rate(fixture.Rate{ID: "10000000-0000-7000-8000-000000000099", CurrencyID: eur, BaseCurrencyID: seededUSD, Rate: "0.92000000", PublishedAt: "2026-01-20"})

	rows, err := read.LatestCurrencyRateListView(ctx)
	if err != nil {
		t.Fatalf("LatestCurrencyRateListView: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 latest rate, got %d", len(rows))
	}
	if rows[0].Rate != "0.92" {
		t.Errorf("rate not normalized: %q", rows[0].Rate)
	}
	if rows[0].UpdatedAt != "2026-01-20 00:00:00" {
		t.Errorf("publishedAt format mismatch: %q", rows[0].UpdatedAt)
	}
}

func TestRateProvider_FractionDigitsAndBase(t *testing.T) {
	db := dbtest.NewSQLite(t)
	lookup := currencyrepo.New("sqlite", db.TX)
	provider := currencyrepo.NewRateProvider("sqlite", db.TX, lookup, "USD")
	ctx := context.Background()

	baseID, err := provider.BaseCurrencyID(ctx)
	if err != nil {
		t.Fatalf("BaseCurrencyID: %v", err)
	}
	if baseID.String() != seededUSD {
		t.Errorf("base id mismatch: %s", baseID)
	}
	fd, err := provider.FractionDigits(ctx, baseID)
	if err != nil {
		t.Fatalf("FractionDigits: %v", err)
	}
	if fd != 2 {
		t.Errorf("want 2 fraction digits, got %d", fd)
	}
}
