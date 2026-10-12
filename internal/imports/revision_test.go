package imports_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// isImported on the transaction list comes from import_transaction_links, so a
// link that appears or disappears moves the revision of the transaction's
// account even when the transaction row itself is untouched.

func TestRevision_AdoptingAHandEnteredTransaction_BumpsItsAccount(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	existing := vo.NewId()
	h.f.Transaction(fixture.Transaction{ID: existing.String(), UserID: userA, AccountID: acct1, Type: 0, Amount: "4.75000000", SpentAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)})
	h.txns.candidates = []*model.Transaction{{ID: existing, AccountID: vo.MustParseId(acct1), Type: model.TransactionTypeExpense, Amount: "4.75000000", SpentAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)}}
	before := h.f.Revisions(acct1, acctB)

	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusMatched {
		t.Fatalf("res = %+v, want matched", res)
	}

	h.f.RequireRevisionsMoved(before, acct1)
}

func TestRevision_ImportingAQueuedEvent_BumpsTheNewTransactionsAccount(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	ingest(t, h, tap)
	q, _ := h.svc.GetQueue(ctx, uA)
	before := h.f.Revisions(acct1, acctB)

	if _, err := h.svc.ImportQueuedEvent(ctx, uA, model.ImportQueuedEventRequest{LinkId: q.Queued[0].LinkId, Transaction: model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("4.75"), AccountId: acct1, Date: "2026-08-20 10:42:03",
	}}); err != nil {
		t.Fatalf("import: %v", err)
	}

	h.f.RequireRevisionsMoved(before, acct1)
}

func TestRevision_ImportingAQueuedEvent_ReportsTheCommittedRevision(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	ingest(t, h, tap)
	q, _ := h.svc.GetQueue(ctx, uA)

	res, err := h.svc.ImportQueuedEvent(ctx, uA, model.ImportQueuedEventRequest{LinkId: q.Queued[0].LinkId, Transaction: model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("4.75"), AccountId: acct1, Date: "2026-08-20 10:42:03",
	}})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	want := h.f.Revisions(acct1)[acct1]
	if len(res.Accounts) != 1 || res.Accounts[0].TransactionsRevision != want {
		t.Fatalf("accounts = %+v, want acct1 at the committed revision %d", res.Accounts, want)
	}
}

func TestRevision_DeletingASource_BumpsEveryAccountWithAnImportedTransaction(t *testing.T) {
	h := setup(t)
	acct2 := h.f.Account(fixture.Account{UserID: userA, CurrencyID: usdID, Name: "Second"})
	imported1 := h.f.Transaction(fixture.Transaction{UserID: userA, AccountID: acct1, Amount: "1"})
	imported2 := h.f.Transaction(fixture.Transaction{UserID: userA, AccountID: acct2, Amount: "2"})
	h.f.Transaction(fixture.Transaction{UserID: userB, AccountID: acctB, Amount: "3"})
	for i, id := range []string{imported1, imported2} {
		h.f.ImportTransactionLink(fixture.ImportTransactionLink{
			SourceID: source, ExternalAccountID: "Apple Card", ExternalTransactionID: string(rune('a' + i)), TransactionID: id,
		})
	}
	before := h.f.Revisions(acct1, acct2, acctB)

	if _, err := h.svc.DeleteSource(context.Background(), vo.MustParseId(userA), model.DeleteImportSourceRequest{Id: source}); err != nil {
		t.Fatalf("delete source: %v", err)
	}

	h.f.RequireRevisionsMoved(before, acct1, acct2)
}
