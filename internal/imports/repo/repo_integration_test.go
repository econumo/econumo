package repo_test

import (
	"context"
	"testing"
	"time"

	importsrepo "github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usdID = "dffc2a06-6f29-4704-8575-31709adee926" // seeded by the baseline migration
	userA = "0a000000-0000-0000-0000-000000000001"
	acct1 = "0a000000-0000-0000-0000-0000000000a1"
)

var fixedTime = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*importsrepo.Repo, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: userA, Email: "a@example.test", Name: "A"})
	f.Account(fixture.Account{ID: acct1, UserID: userA, CurrencyID: usdID, Name: "Card"})
	return importsrepo.NewRepo(db.Engine, db.TX), db
}

func newSource(id string) *model.ImportSource {
	return &model.ImportSource{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Provider: model.ImportProviderAppleWallet,
		Name: "iPhone", Status: model.ImportSourceStatusActive, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
}

func TestRepo_SourceRoundTrip(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatalf("InsertSource: %v", err)
	}
	got, err := repo.GetSource(ctx, src.ID)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got.Provider != model.ImportProviderAppleWallet || got.Name != "iPhone" || got.CredentialCiphertext != nil || got.LastSyncedAt != nil || !got.CreatedAt.Equal(fixedTime) {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if _, err := repo.GetSource(ctx, vo.NewId()); err == nil {
		t.Fatal("GetSource(miss) must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("GetSource(miss) = %T, want NotFound", err)
	}
}

func TestRepo_EventInboxDedupes(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	ev := &model.ImportEvent{
		ID: vo.NewId(), SourceID: src.ID, Payload: `{"amount":"4.50"}`, PayloadHash: "h1",
		Status: model.ImportEventStatusProcessed, ReceivedAt: fixedTime,
	}
	if inserted, err := repo.InsertEvent(ctx, ev); err != nil || !inserted {
		t.Fatalf("first InsertEvent = %v, %v; want inserted", inserted, err)
	}
	dup := &model.ImportEvent{
		ID: vo.NewId(), SourceID: src.ID, Payload: `{"amount":"4.50"}`, PayloadHash: "h1",
		Status: model.ImportEventStatusProcessed, ReceivedAt: fixedTime.Add(time.Minute),
	}
	if inserted, err := repo.InsertEvent(ctx, dup); err != nil || inserted {
		t.Fatalf("duplicate InsertEvent = %v, %v; want not inserted, no error", inserted, err)
	}
	// Same hash on ANOTHER source is a different inbox.
	src2 := newSource("0c000000-0000-0000-0000-000000000002")
	if err := repo.InsertSource(ctx, src2); err != nil {
		t.Fatal(err)
	}
	other := &model.ImportEvent{ID: vo.NewId(), SourceID: src2.ID, Payload: "x", PayloadHash: "h1",
		Status: model.ImportEventStatusProcessed, ReceivedAt: fixedTime}
	if inserted, err := repo.InsertEvent(ctx, other); err != nil || !inserted {
		t.Fatalf("other-source InsertEvent = %v, %v; want inserted", inserted, err)
	}

	msg := "bad json"
	if err := repo.UpdateEventStatus(ctx, ev.ID, model.ImportEventStatusFailed, &msg); err != nil {
		t.Fatalf("UpdateEventStatus: %v", err)
	}
	got, err := repo.GetEvent(ctx, ev.ID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.Status != model.ImportEventStatusFailed || got.ParseError == nil || *got.ParseError != msg || got.RunID != nil {
		t.Errorf("event after update: %+v", got)
	}
}

func TestRepo_SourceDeleteCascadesEvents(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	ev := &model.ImportEvent{ID: vo.NewId(), SourceID: src.ID, Payload: "p", PayloadHash: "h",
		Status: model.ImportEventStatusProcessed, ReceivedAt: fixedTime}
	if _, err := repo.InsertEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Raw.ExecContext(ctx, db.Rebind("DELETE FROM import_sources WHERE id = ?"), src.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetEvent(ctx, ev.ID); err == nil {
		t.Fatal("event must cascade with its source")
	}
}

// external_amount is NUMERIC(19,8) NOT NULL; omitting ExternalAmount must not
// leave the column empty (SQLite would accept "" but PostgreSQL rejects it as
// invalid numeric input). ListLinksByTransaction is a Task-5 stub, so this
// reads the row back with raw SQL instead of going through the repo.
func TestFixture_ImportTransactionLinkDefaultsExternalAmount(t *testing.T) {
	_, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA})
	linkID := f.ImportTransactionLink(fixture.ImportTransactionLink{
		SourceID:              srcID,
		ExternalAccountID:     "ext-acct-1",
		ExternalTransactionID: "ext-txn-1",
	})

	var amount string
	row := db.Raw.QueryRowContext(ctx, db.Rebind("SELECT external_amount FROM import_transaction_links WHERE id = ?"), linkID)
	if err := row.Scan(&amount); err != nil {
		t.Fatalf("scan external_amount: %v", err)
	}
	if vo.NewDecimal(amount).String() != vo.NewDecimal("0").String() {
		t.Errorf("external_amount default = %q, want normalized 0", amount)
	}
}
