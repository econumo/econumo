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

func newLink(srcID string, extTx string, txID *vo.Id, status string) *model.ImportTransactionLink {
	return &model.ImportTransactionLink{
		ID: vo.NewId(), SourceID: vo.MustParseId(srcID), ExternalAccountID: "card-1", ExternalTransactionID: extTx,
		TransactionID: txID, Status: status, ExternalPayee: "COFFEE SHOP", ExternalDescription: "coffee",
		ExternalAmount: "4.50000000", ExternalPostedAt: fixedTime, ImportedAt: fixedTime,
	}
}

func TestRepo_RunRoundTrip(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	run := &model.ImportRun{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), SourceID: src.ID, Provider: model.ImportProviderAppleWallet,
		Params: "{}", Status: model.ImportRunStatusRunning, StartedAt: fixedTime,
	}
	if err := repo.InsertRun(ctx, run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	fin := fixedTime.Add(2 * time.Second)
	run.Status, run.ImportedCount, run.MatchedCount, run.SkippedCount, run.FailedCount, run.FinishedAt = model.ImportRunStatusCompleted, 3, 1, 2, 0, &fin
	if err := repo.UpdateRun(ctx, run); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	got, err := repo.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != model.ImportRunStatusCompleted || got.ImportedCount != 3 || got.MatchedCount != 1 || got.SkippedCount != 2 || got.FailedCount != 0 ||
		got.FinishedAt == nil || !got.FinishedAt.Equal(fin) || got.Params != "{}" {
		t.Errorf("run after update: %+v", got)
	}
	if _, err := repo.GetRun(ctx, vo.NewId()); err == nil {
		t.Fatal("GetRun(miss) must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("GetRun(miss) = %T, want NotFound", err)
	}
}

func TestRepo_LinkLedger(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "iPhone"})
	txID := vo.MustParseId(f.Transaction(fixture.Transaction{UserID: userA, AccountID: acct1, Type: 0, Amount: "4.50000000", Description: "coffee"}))

	l := newLink(srcID, "ext-1", &txID, model.ImportLinkStatusLinked)
	if err := repo.InsertLink(ctx, l); err != nil {
		t.Fatalf("InsertLink: %v", err)
	}
	// Same external key twice is a ledger bug, not an upsert.
	if err := repo.InsertLink(ctx, newLink(srcID, "ext-1", &txID, model.ImportLinkStatusLinked)); err == nil {
		t.Fatal("duplicate external key must fail")
	}

	got, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-1")
	if err != nil {
		t.Fatalf("GetLinkByExternalKey: %v", err)
	}
	if got.TransactionID == nil || !got.TransactionID.Equal(txID) || vo.NewDecimal(got.ExternalAmount).String() != vo.NewDecimal("4.5").String() || !got.ExternalPostedAt.Equal(fixedTime) || !got.IsSeen() {
		t.Errorf("link mismatch: %+v", got)
	}
	if _, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-nope"); err == nil {
		t.Fatal("miss must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("miss = %T, want NotFound", err)
	}

	byTx, err := repo.ListLinksByTransaction(ctx, txID)
	if err != nil || len(byTx) != 1 || !byTx[0].ID.Equal(l.ID) {
		t.Fatalf("ListLinksByTransaction = %+v, %v; want the one link", byTx, err)
	}
}

// The regression this feature exists to prevent: deleting an imported
// transaction must NOT make the next sync re-create it. The FK is SET NULL,
// so the ledger row survives as a tombstone that still counts as seen.
func TestRepo_DeletedTransactionLeavesTombstone(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "iPhone"})
	txID := vo.MustParseId(f.Transaction(fixture.Transaction{UserID: userA, AccountID: acct1, Type: 0, Amount: "4.50000000", Description: "coffee"}))
	if err := repo.InsertLink(ctx, newLink(srcID, "ext-1", &txID, model.ImportLinkStatusLinked)); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Raw.ExecContext(ctx, db.Rebind("DELETE FROM transactions WHERE id = ?"), txID.String()); err != nil {
		t.Fatalf("delete transaction: %v", err)
	}

	got, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-1")
	if err != nil {
		t.Fatalf("ledger row must survive the transaction delete: %v", err)
	}
	if got.TransactionID != nil || got.Status != model.ImportLinkStatusLinked || !got.IsTombstone() || !got.IsSeen() {
		t.Errorf("tombstone mismatch: %+v", got)
	}
	if rows, err := repo.ListLinksByTransaction(ctx, txID); err != nil || len(rows) != 0 {
		t.Errorf("ListLinksByTransaction(deleted) = %d rows, %v; want none", len(rows), err)
	}
}

func TestRepo_QueuedLinkIsNotSeen(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "iPhone"})
	if err := repo.InsertLink(ctx, newLink(srcID, "ext-q", nil, model.ImportLinkStatusQueued)); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-q")
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSeen() {
		t.Error("a queued link must not count as seen")
	}
}

func TestRepo_SourceDeleteCascadesRunsAndLinks(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "iPhone"})
	run := &model.ImportRun{ID: vo.NewId(), UserID: vo.MustParseId(userA), SourceID: vo.MustParseId(srcID),
		Provider: model.ImportProviderAppleWallet, Params: "{}", Status: model.ImportRunStatusRunning, StartedAt: fixedTime}
	if err := repo.InsertRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	l := newLink(srcID, "ext-1", nil, model.ImportLinkStatusSkipped)
	l.RunID = &run.ID
	if err := repo.InsertLink(ctx, l); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Raw.ExecContext(ctx, db.Rebind("DELETE FROM import_sources WHERE id = ?"), srcID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetRun(ctx, run.ID); err == nil {
		t.Error("run must cascade with its source")
	}
	if _, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-1"); err == nil {
		t.Error("link must cascade with its source")
	}
}

// Deleting a run must not delete its ledger rows (SET NULL), or the history
// of what was imported would vanish with a cleanup.
func TestRepo_RunDeleteDetachesLinks(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "iPhone"})
	run := &model.ImportRun{ID: vo.NewId(), UserID: vo.MustParseId(userA), SourceID: vo.MustParseId(srcID),
		Provider: model.ImportProviderAppleWallet, Params: "{}", Status: model.ImportRunStatusCompleted, StartedAt: fixedTime}
	if err := repo.InsertRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	l := newLink(srcID, "ext-1", nil, model.ImportLinkStatusSkipped)
	l.RunID = &run.ID
	if err := repo.InsertLink(ctx, l); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Raw.ExecContext(ctx, db.Rebind("DELETE FROM import_runs WHERE id = ?"), run.ID.String()); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetLinkByExternalKey(ctx, vo.MustParseId(srcID), "card-1", "ext-1")
	if err != nil {
		t.Fatalf("link must survive run delete: %v", err)
	}
	if got.RunID != nil {
		t.Errorf("run_id must be NULL after run delete, got %v", got.RunID)
	}
}

// Deferred from the Task 4 review: a nullable FK round-trip with a real,
// non-nil value. UpdateEventStatus always writes run_id back (even unchanged),
// so this pins that a genuine run id survives the round trip, not just NULL.
func TestRepo_EventStatusUpdatePreservesRunID(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	run := &model.ImportRun{ID: vo.NewId(), UserID: vo.MustParseId(userA), SourceID: src.ID,
		Provider: model.ImportProviderAppleWallet, Params: "{}", Status: model.ImportRunStatusRunning, StartedAt: fixedTime}
	if err := repo.InsertRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	ev := &model.ImportEvent{
		ID: vo.NewId(), SourceID: src.ID, RunID: &run.ID, Payload: `{"amount":"4.50"}`, PayloadHash: "h-run",
		Status: model.ImportEventStatusProcessed, ReceivedAt: fixedTime,
	}
	if inserted, err := repo.InsertEvent(ctx, ev); err != nil || !inserted {
		t.Fatalf("InsertEvent = %v, %v; want inserted", inserted, err)
	}
	if err := repo.UpdateEventStatus(ctx, ev.ID, model.ImportEventStatusProcessed, nil); err != nil {
		t.Fatalf("UpdateEventStatus: %v", err)
	}
	got, err := repo.GetEvent(ctx, ev.ID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.RunID == nil || !got.RunID.Equal(run.ID) {
		t.Errorf("RunID = %v, want %v", got.RunID, run.ID)
	}
}
