package repo_test

import (
	"context"
	"strings"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func newManage(t *testing.T) (*currencyrepo.ManageRepo, *dbtest.DB, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	return currencyrepo.NewManageRepo(db.Engine, db.TX), db, fixture.New(t, db)
}

func TestManageRepo_InsertGetUpdateArchiveDelete(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	name := "Points"
	rec := model.CurrencyRecord{
		ID: fixture.NewID(), Code: "PTS", Symbol: "pts", Name: &name,
		FractionDigits: 0, UserID: &uid, CreatedAt: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
	}
	if err := r.InsertUserCurrency(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetCurrencyRecord(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "PTS" || got.UserID == nil || *got.UserID != uid || got.IsArchived {
		t.Fatalf("unexpected record: %+v", got)
	}
	if err := r.UpdateCurrencyDetails(ctx, rec.ID, "Kid points", "kp", 2, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.SetCurrencyArchived(ctx, rec.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetCurrencyRecord(ctx, rec.ID)
	if got.Name == nil || *got.Name != "Kid points" || got.Symbol != "kp" || got.FractionDigits != 2 || !got.IsArchived {
		t.Fatalf("update/archive not persisted: %+v", got)
	}
	if err := r.DeleteCurrency(ctx, rec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetCurrencyRecord(ctx, rec.ID); err == nil {
		t.Fatal("expected NotFound after delete")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestManageRepo_NameLength64Chars(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	name64 := strings.Repeat("n", 64)
	rec := model.CurrencyRecord{
		ID: fixture.NewID(), Code: "PTS", Symbol: "pts", Name: &name64,
		FractionDigits: 0, UserID: &uid, CreatedAt: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
	}
	if err := r.InsertUserCurrency(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetCurrencyRecord(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name == nil || *got.Name != name64 {
		t.Fatalf("Name = %v, want 64-char name round-tripped", got.Name)
	}

	name64b := strings.Repeat("m", 64)
	if err := r.UpdateCurrencyDetails(ctx, rec.ID, name64b, "kp", 2, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetCurrencyRecord(ctx, rec.ID)
	if got.Name == nil || *got.Name != name64b {
		t.Fatalf("updated Name = %v, want 64-char name round-tripped", got.Name)
	}
}

func TestManageRepo_CodeExistence(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	f.Currency(fixture.Currency{Code: "PTS", UserID: uid})
	if ok, _ := r.GlobalCodeExists(ctx, "USD"); !ok {
		t.Fatal("USD should exist globally")
	}
	if ok, _ := r.GlobalCodeExists(ctx, "PTS"); ok {
		t.Fatal("PTS is custom, not global")
	}
	if ok, _ := r.OwnerCodeExists(ctx, uid, "PTS"); !ok {
		t.Fatal("owner PTS should exist")
	}
	if ok, _ := r.OwnerCodeExists(ctx, fixture.NewID(), "PTS"); ok {
		t.Fatal("other user should not own PTS")
	}
}

func TestManageRepo_UsageCount(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	cid := f.Currency(fixture.Currency{Code: "PTS", UserID: uid})
	if n, _ := r.CountCurrencyUsage(ctx, cid); n != 0 {
		t.Fatalf("fresh currency usage = %d, want 0", n)
	}
	f.Account(fixture.Account{UserID: uid, CurrencyID: cid, Name: "Kid"})
	// The profile option stores the currency ID.
	f.Option(uid, "currency", &cid)
	if n, _ := r.CountCurrencyUsage(ctx, cid); n != 2 {
		t.Fatalf("usage = %d, want 2 (account + profile option)", n)
	}
}

// Same-code customs of DIFFERENT owners must not cross-block deletion: the
// census matches profile defaults by ID, not code.
func TestManageRepo_UsageCount_SameCodeOtherOwnerDefault(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	alice := f.User(fixture.User{Name: "Alice", Email: "alice@census.test"})
	bob := f.User(fixture.User{Name: "Bob", Email: "bob@census.test"})
	alicePTS := f.Currency(fixture.Currency{Code: "PTS", UserID: alice})
	bobPTS := f.Currency(fixture.Currency{Code: "PTS", UserID: bob})
	// Bob's profile default is HIS OWN PTS.
	f.Option(bob, "currency", &bobPTS)
	if n, _ := r.CountCurrencyUsage(ctx, alicePTS); n != 0 {
		t.Fatalf("usage of Alice's PTS = %d, want 0 (Bob's default is his own)", n)
	}
	if n, _ := r.CountCurrencyUsage(ctx, bobPTS); n != 1 {
		t.Fatalf("usage of Bob's PTS = %d, want 1 (his profile default)", n)
	}
}

func TestManageRepo_HideShowIdempotent(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	if err := r.HideCurrency(ctx, uid, seededUSD, now); err != nil {
		t.Fatal(err)
	}
	if err := r.HideCurrency(ctx, uid, seededUSD, now); err != nil {
		t.Fatal(err) // idempotent
	}
	if err := r.ShowCurrency(ctx, uid, seededUSD); err != nil {
		t.Fatal(err)
	}
	if err := r.ShowCurrency(ctx, uid, seededUSD); err != nil {
		t.Fatal(err) // idempotent
	}
}
