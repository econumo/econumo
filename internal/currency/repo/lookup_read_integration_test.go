package repo_test

// USD is seeded by the baseline migration. Note: the setup helper in
// convertor_provider_test.go DELETEs and re-seeds currencies with a different id,
// so these tests use dbtest's pristine migrated DB (seeded USD = dffc2a06...) and
// their own distinct identifiers.

import (
	"context"
	"errors"
	"testing"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const seededUSD = "dffc2a06-6f29-4704-8575-31709adee926"

func TestCurrencyLookup_GetIDByCode(t *testing.T) {
	db := dbtest.New(t)
	lookup := currencyrepo.New(db.Engine, db.TX)
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
	db := dbtest.New(t)
	lookup := currencyrepo.New(db.Engine, db.TX)
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

func TestLookup_GetIDByCodeForUser(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	lk := currencyrepo.New(db.Engine, db.TX)
	ctx := context.Background()
	// Explicit distinct ids: fixture identifiers derive from the id's leading 8
	// hex chars when the builder has no crypto, and UUIDv7's first 32 bits are
	// pure timestamp, so 2 fresh ids minted this fast can collide (see
	// TestReadRepo_UserCurrencyListScoping).
	uid := f.User(fixture.User{ID: "a1000000-0000-7000-8000-000000000001", Name: "A"})
	pts := f.Currency(fixture.Currency{Code: "PTS", UserID: uid})
	// Own custom resolves.
	got, err := lk.GetIDByCodeForUser(ctx, uid, "PTS")
	if err != nil || got != pts {
		t.Fatalf("own custom: got %q err %v", got, err)
	}
	// Global resolves for anyone.
	if got, err = lk.GetIDByCodeForUser(ctx, uid, "USD"); err != nil || got != seededUSD {
		t.Fatalf("global: got %q err %v", got, err)
	}
	// Foreign custom does NOT resolve.
	other := f.User(fixture.User{ID: "b1000000-0000-7000-8000-000000000002", Name: "B"})
	if _, err = lk.GetIDByCodeForUser(ctx, other, "PTS"); err == nil {
		t.Fatal("foreign custom code must not resolve")
	}
}

func TestLookup_EnsureUsable(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	lk := currencyrepo.New(db.Engine, db.TX)
	ctx := context.Background()
	alice := f.User(fixture.User{ID: "a2000000-0000-7000-8000-000000000001", Name: "Alice"})
	bob := f.User(fixture.User{ID: "b2000000-0000-7000-8000-000000000002", Name: "Bob"})
	pts := f.Currency(fixture.Currency{Code: "PTS", UserID: alice})
	old := f.Currency(fixture.Currency{Code: "OLD", UserID: alice, IsArchived: true})
	if err := lk.EnsureUsable(ctx, alice, seededUSD); err != nil {
		t.Errorf("global should be usable: %v", err)
	}
	if err := lk.EnsureUsable(ctx, alice, pts); err != nil {
		t.Errorf("own custom should be usable: %v", err)
	}
	if err := lk.EnsureUsable(ctx, bob, pts); err == nil {
		t.Error("foreign custom must be rejected")
	} else if v, ok := errs.AsValidation(err); !ok || v.Msg != "Validation failed" || v.Fields[0].Message != "Currency is not available" {
		t.Errorf("wrong error: %v", err)
	}
	if err := lk.EnsureUsable(ctx, alice, old); err == nil {
		t.Error("own archived custom must be rejected")
	}
	if err := lk.EnsureUsable(ctx, alice, fixture.NewID()); err == nil {
		t.Error("missing currency must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestCurrencyReadRepo_UserCurrencyListView(t *testing.T) {
	db := dbtest.New(t)
	read := currencyrepo.NewReadRepo(db.Engine, db.TX)
	f := fixture.New(t, db)
	ctx := context.Background()

	uid := f.User(fixture.User{Name: "A"})
	rows, err := read.UserCurrencyListView(ctx, uid)
	if err != nil {
		t.Fatalf("UserCurrencyListView: %v", err)
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
			if r.UserID != nil {
				t.Errorf("global USD should have nil UserID, got %v", *r.UserID)
			}
		}
	}
	if !foundUSD {
		t.Error("USD missing from currency list view")
	}
}

func TestReadRepo_UserCurrencyListScoping(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	read := currencyrepo.NewReadRepo(db.Engine, db.TX)
	ctx := context.Background()

	// Explicit distinct ids: fixture identifiers derive from the id's leading 8
	// hex chars when the builder has no crypto, and UUIDv7's first 32 bits are
	// pure timestamp, so 3 fresh ids minted this fast would collide.
	alice := f.User(fixture.User{ID: "a0000000-0000-7000-8000-000000000001", Name: "Alice"})
	bob := f.User(fixture.User{ID: "b0000000-0000-7000-8000-000000000002", Name: "Bob"})
	carol := f.User(fixture.User{ID: "c0000000-0000-7000-8000-000000000003", Name: "Carol"})
	ptsAlice := f.Currency(fixture.Currency{Code: "PTS", UserID: alice, Name: "Points"})
	ptsBob := f.Currency(fixture.Currency{Code: "PTS", UserID: bob, Name: "Bob points"})
	gemCarol := f.Currency(fixture.Currency{Code: "GEM", UserID: carol, Name: "Gems"})

	// Bob shares an account denominated in his PTS with Alice.
	acc := f.Account(fixture.Account{UserID: bob, CurrencyID: ptsBob, Name: "Kid"})
	f.AccountAccess(acc, alice, 1)
	// Carol shares a budget with Alice; one element uses Carol's GEM.
	bud := f.Budget(fixture.Budget{UserID: carol})
	f.BudgetElement(fixture.BudgetElement{BudgetID: bud, CurrencyID: gemCarol, ExternalID: fixture.NewID(), Type: 1})
	f.BudgetAccess(bud, alice, 1, true)

	rows, err := read.UserCurrencyListView(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r.ID] = true
	}
	if !ids[seededUSD] {
		t.Error("global USD missing")
	}
	if !ids[ptsAlice] {
		t.Error("own custom missing")
	}
	if !ids[ptsBob] {
		t.Error("shared-account custom missing")
	}
	if !ids[gemCarol] {
		t.Error("shared-budget element custom missing")
	}
	// Bob does NOT see Alice's or Carol's customs.
	rows, err = read.UserCurrencyListView(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == ptsAlice || r.ID == gemCarol {
			t.Errorf("bob sees foreign custom %s", r.Code)
		}
	}
}

func TestReadRepo_LatestRatePerCurrency(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	read := currencyrepo.NewReadRepo(db.Engine, db.TX)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	pts := f.Currency(fixture.Currency{Code: "PTS", UserID: uid})
	f.Rate(fixture.Rate{CurrencyID: pts, Rate: "100.00000000", PublishedAt: "2026-07-01"})
	f.Rate(fixture.Rate{CurrencyID: seededUSD, Rate: "1.00000000", PublishedAt: "2026-07-10"})
	rows, err := read.LatestCurrencyRateListView(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byCur := map[string]string{}
	for _, r := range rows {
		byCur[r.CurrencyID] = r.Rate
	}
	if byCur[pts] == "" {
		t.Fatal("backdated custom rate dropped by latest-rate query")
	}
	if byCur[seededUSD] == "" {
		t.Fatal("usd rate missing")
	}
}

func TestReadRepo_HiddenCurrencyIDs(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	read := currencyrepo.NewReadRepo(db.Engine, db.TX)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})

	hidden, err := read.HiddenCurrencyIDs(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden) != 0 {
		t.Fatalf("expected no hidden currencies, got %v", hidden)
	}

	f.HiddenCurrency(uid, seededUSD)
	hidden, err = read.HiddenCurrencyIDs(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden) != 1 || hidden[0] != seededUSD {
		t.Fatalf("want [%s], got %v", seededUSD, hidden)
	}
}

func TestCurrencyReadRepo_LatestRateListView(t *testing.T) {
	db := dbtest.New(t)
	read := currencyrepo.NewReadRepo(db.Engine, db.TX)
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
	db := dbtest.New(t)
	lookup := currencyrepo.New(db.Engine, db.TX)
	provider := currencyrepo.NewRateProvider(db.Engine, db.TX, lookup, "USD")
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
