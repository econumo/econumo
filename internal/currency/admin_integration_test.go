package currency_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	appcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func newCurrencySvc(t *testing.T, db *dbtest.DB) *appcurrency.WriteService {
	t.Helper()
	repo := currencyrepo.NewWriteRepo("sqlite", db.TX)
	return appcurrency.NewWriteService(repo, db.TX, clock.New())
}

func TestAddCurrency(t *testing.T) {
	db := dbtest.NewSQLite(t)
	svc := newCurrencySvc(t, db)
	ctx := context.Background()

	// USD is seeded by the baseline migration; AvailableCodes must include it.
	codes, err := svc.AvailableCodes(ctx)
	if err != nil {
		t.Fatalf("AvailableCodes: %v", err)
	}
	if !contains(codes, "USD") {
		t.Errorf("AvailableCodes = %v, want it to include USD", codes)
	}

	// New currency: symbol + fraction digits come from the ICU tables; lowercase
	// input is normalized to the stored uppercase code.
	created, err := svc.AddCurrency(ctx, "eur", nil, nil)
	if err != nil {
		t.Fatalf("AddCurrency(eur): %v", err)
	}
	if !created {
		t.Error("AddCurrency(eur) should report created")
	}
	sym, name, fd := readCurrency(t, db, "EUR")
	if sym != "€" {
		t.Errorf("EUR symbol = %q, want €", sym)
	}
	if name.Valid {
		t.Errorf("EUR name = %q, want NULL", name.String)
	}
	if fd != 2 {
		t.Errorf("EUR fraction_digits = %d, want 2", fd)
	}

	// Idempotent: a second add reports not-created and does not duplicate.
	created, err = svc.AddCurrency(ctx, "EUR", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("second AddCurrency(EUR) should report not-created")
	}

	// Explicit name + fraction-digits override the ICU defaults (BHD ICU = 3).
	customName, customFD := "Bahraini Dinar (custom)", 5
	if _, err := svc.AddCurrency(ctx, "BHD", &customName, &customFD); err != nil {
		t.Fatalf("AddCurrency(BHD): %v", err)
	}
	_, name, fd = readCurrency(t, db, "BHD")
	if !name.Valid || name.String != customName {
		t.Errorf("BHD name = %+v, want %q", name, customName)
	}
	if fd != 5 {
		t.Errorf("BHD fraction_digits = %d, want 5 (override)", fd)
	}

	// Invalid code -> validation error.
	if _, err := svc.AddCurrency(ctx, "TOOLONG", nil, nil); !isValidationErr(err) {
		t.Fatalf("AddCurrency(TOOLONG): want validation error, got %v", err)
	}
}

func TestUpdateRates(t *testing.T) {
	db := dbtest.NewSQLite(t)
	svc := newCurrencySvc(t, db)
	ctx := context.Background()

	if _, err := svc.AddCurrency(ctx, "EUR", nil, nil); err != nil {
		t.Fatal(err)
	}

	// A date with a wall-clock time: the stored published_at must collapse to Y-m-d.
	date := time.Date(2025, 4, 1, 13, 30, 0, 0, time.UTC)
	rates := []appcurrency.RateInput{
		{Code: "EUR", Base: "USD", Rate: "0.92000000", Date: date},
		{Code: "USD", Base: "USD", Rate: "1.00000000", Date: date},
		{Code: "ZZZ", Base: "USD", Rate: "9.99000000", Date: date}, // unknown -> skipped
	}
	n, err := svc.UpdateRates(ctx, rates)
	if err != nil {
		t.Fatalf("UpdateRates: %v", err)
	}
	if n != 2 {
		t.Errorf("updated %d, want 2 (EUR + USD; ZZZ skipped)", n)
	}
	if got := rateRowCount(t, db); got != 2 {
		t.Errorf("currencies_rates rows = %d, want 2", got)
	}
	// modernc stores DATE columns ISO8601; the wall-clock time (13:30) must be
	// truncated to the day so the value is stable for the per-day upsert.
	if pub := firstPublishedAt(t, db); !strings.HasPrefix(pub, "2025-04-01") || strings.Contains(pub, "13:30") {
		t.Errorf("published_at = %q, want the 2025-04-01 day at midnight", pub)
	}

	// Re-running with a changed EUR rate on the same date upserts (no new row).
	rates[0].Rate = "0.95000000"
	if _, err := svc.UpdateRates(ctx, rates); err != nil {
		t.Fatal(err)
	}
	if got := rateRowCount(t, db); got != 2 {
		t.Errorf("after upsert rows = %d, want 2 (no duplicate)", got)
	}
	// SQLite's NUMERIC affinity may store the value as REAL, so normalize both
	// sides to vo's canonical decimal form (as the read side does) before comparing.
	if got, want := vo.NewDecimal(eurRate(t, db)).String(), vo.NewDecimal("0.95000000").String(); got != want {
		t.Errorf("EUR rate = %q, want %q (updated)", got, want)
	}
}

func readCurrency(t *testing.T, db *dbtest.DB, code string) (symbol string, name sql.NullString, fractionDigits int) {
	t.Helper()
	row := db.Raw.QueryRow(`SELECT symbol, name, fraction_digits FROM currencies WHERE code = ?`, code)
	if err := row.Scan(&symbol, &name, &fractionDigits); err != nil {
		t.Fatalf("read currency %s: %v", code, err)
	}
	return symbol, name, fractionDigits
}

func rateRowCount(t *testing.T, db *dbtest.DB) int {
	t.Helper()
	var n int
	if err := db.Raw.QueryRow(`SELECT COUNT(*) FROM currencies_rates`).Scan(&n); err != nil {
		t.Fatalf("count rates: %v", err)
	}
	return n
}

func firstPublishedAt(t *testing.T, db *dbtest.DB) string {
	t.Helper()
	var pub string
	if err := db.Raw.QueryRow(`SELECT published_at FROM currencies_rates LIMIT 1`).Scan(&pub); err != nil {
		t.Fatalf("read published_at: %v", err)
	}
	return pub
}

func eurRate(t *testing.T, db *dbtest.DB) string {
	t.Helper()
	var rate string
	err := db.Raw.QueryRow(
		`SELECT rate FROM currencies_rates r JOIN currencies c ON c.id = r.currency_id WHERE c.code = 'EUR'`,
	).Scan(&rate)
	if err != nil {
		t.Fatalf("read EUR rate: %v", err)
	}
	return rate
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func isValidationErr(err error) bool {
	var v *errs.ValidationError
	return errors.As(err, &v)
}
