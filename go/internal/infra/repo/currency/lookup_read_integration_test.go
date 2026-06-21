package currencyrepo_test

// Integration tests for the currency Lookup and ReadRepo against a real migrated
// in-memory SQLite (USD is seeded by the baseline migration). Note: the
// usdID/setup/toMigrations helpers in convertor_provider_test.go DELETE and
// re-seed currencies with a different id, so these tests use testutil's pristine
// migrated DB (seeded USD = dffc2a06...) and their own distinct identifiers.

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	"github.com/econumo/econumo/internal/testutil"
)

const seededUSD = "dffc2a06-6f29-4704-8575-31709adee926"

func TestCurrencyLookup_GetIDByCode(t *testing.T) {
	db := testutil.NewSQLite(t)
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
	db := testutil.NewSQLite(t)
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
	db := testutil.NewSQLite(t)
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
	db := testutil.NewSQLite(t)
	read := currencyrepo.NewReadRepo("sqlite", db.TX)
	ctx := context.Background()

	// Seed a second currency + a rate; the rate's NUMERIC(19,8) must normalize to
	// the PHP DecimalNumber wire form (trailing zeros trimmed).
	eur := "1ae5bfd5-03e8-412b-80d2-c0ecf3ce32fe"
	db.Exec(t, `INSERT INTO currencies (id, code, symbol, fraction_digits, created_at) VALUES (?, 'EUR', 'E', 2, ?)`, eur, fixedTimeStr)
	db.Exec(t, `INSERT INTO currencies_rates (id, currency_id, base_currency_id, rate, published_at) VALUES (?, ?, ?, '0.92000000', '2026-01-20')`,
		"10000000-0000-7000-8000-000000000099", eur, seededUSD)

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
	db := testutil.NewSQLite(t)
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

const fixedTimeStr = "2024-04-01 12:00:00"
