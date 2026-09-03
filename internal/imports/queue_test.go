package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestGetQueue_ReasonsAndSections(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	ingest(t, h, tap) // unmapped
	h.mapCard(t, "GBP Card")
	ingest(t, h, `{"account":"GBP Card","amount":"5","currency":"GBP","eventId":"g1","type":"income"}`) // mapped, no rate
	h.mapCard(t, "Dead Card")
	failed := ingest(t, h, `{"account":"Dead Card","amount":"x","currency":"USD"}`) // failed
	q, err := h.svc.GetQueue(ctx, uA)
	if err != nil || len(q.Queued) != 2 || len(q.Skipped) != 0 || len(q.Failed) != 1 {
		t.Fatalf("queue = %+v, %v", q, err)
	}
	byCard := map[string]model.ImportQueuedEventResult{}
	for _, e := range q.Queued {
		byCard[e.ExternalAccountId] = e
	}
	if e := byCard["Apple Card"]; e.Reason != model.ImportQueueReasonUnmapped || e.Payee != "Blue Bottle" || e.Type != "expense" || e.Currency != "USD" || e.PostedAt != "2026-08-20 10:42:03" || e.AccountId != "" {
		t.Errorf("unmapped entry = %+v", e)
	}
	if e := byCard["GBP Card"]; e.Reason != model.ImportQueueReasonNoRate || e.Type != "income" || e.AccountId != acct1 {
		t.Errorf("no-rate entry = %+v", e)
	}
	if f := q.Failed[0]; f.EventId != failed.EventId || f.Error != "amount must be a positive number" || f.ReceivedAt == "" {
		t.Errorf("failed entry = %+v", f)
	}
	// a foreign user's queue is empty, and the slices are never null
	other, _ := h.svc.GetQueue(ctx, vo.MustParseId(userB))
	if other.Queued == nil || other.Skipped == nil || other.Failed == nil || len(other.Queued) != 0 {
		t.Errorf("foreign queue = %+v", other)
	}
	// account_deleted
	h.accounts.deleted = true
	h.mapCard(t, "Apple Card")
	q, _ = h.svc.GetQueue(ctx, uA)
	found := false
	for _, e := range q.Queued {
		if e.ExternalAccountId != "Apple Card" {
			continue
		}
		found = true
		if e.Reason != model.ImportQueueReasonAccountDeleted {
			t.Errorf("deleted-account reason = %+v", e)
		}
	}
	if !found {
		t.Errorf("the Apple Card tap must still be queued: %+v", q.Queued)
	}
}

func TestImportQueuedEvent(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	ingest(t, h, tap)
	q, _ := h.svc.GetQueue(ctx, uA)
	linkID := q.Queued[0].LinkId
	cat := "0b000000-0000-0000-0000-0000000000c1"
	res, err := h.svc.ImportQueuedEvent(ctx, uA, model.ImportQueuedEventRequest{LinkId: linkID, Transaction: model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("4.75"), AccountId: acct1, Date: "2026-08-20 10:42:03", CategoryId: &cat,
	}})
	if err != nil || res.Item.Id == "" {
		t.Fatalf("import = %+v, %v", res, err)
	}
	if res.Item.IsImported != 1 {
		t.Errorf("isImported = %d, want 1", res.Item.IsImported)
	}
	link, _ := h.repo.GetLink(ctx, vo.MustParseId(linkID))
	if link.Status != model.ImportLinkStatusLinked || link.TransactionID == nil || link.TransactionID.String() != res.Item.Id || link.AppliedCategoryID == nil || link.AppliedCategoryID.String() != cat {
		t.Errorf("link after import = %+v", link)
	}
	if q, _ := h.svc.GetQueue(ctx, uA); len(q.Queued) != 0 {
		t.Errorf("queue must drain: %+v", q.Queued)
	}
	// importing it again is refused
	_, err = h.svc.ImportQueuedEvent(ctx, uA, model.ImportQueuedEventRequest{LinkId: linkID, Transaction: model.CreateTransactionRequest{Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("4.75"), AccountId: acct1, Date: "2026-08-20 10:42:03"}})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeImportLinkNotQueued {
		t.Errorf("re-import = %v", err)
	}
	// foreign user: not found
	if _, err := h.svc.ImportQueuedEvent(ctx, vo.MustParseId(userB), model.ImportQueuedEventRequest{LinkId: linkID, Transaction: model.CreateTransactionRequest{Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("1"), AccountId: acctB, Date: "2026-08-20 10:42:03"}}); err == nil {
		t.Error("foreign import must fail")
	}
}

func TestSkipUnskipDiscard(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	ingest(t, h, tap)
	failed := ingest(t, h, `{"account":"Apple Card","amount":"x","currency":"USD"}`)
	q, _ := h.svc.GetQueue(ctx, uA)
	linkID := q.Queued[0].LinkId
	q, err := h.svc.SkipQueuedEvent(ctx, uA, model.ImportLinkActionRequest{LinkId: linkID})
	if err != nil || len(q.Queued) != 0 || len(q.Skipped) != 1 {
		t.Fatalf("skip = %+v, %v", q, err)
	}
	if _, err := h.svc.SkipQueuedEvent(ctx, uA, model.ImportLinkActionRequest{LinkId: linkID}); err == nil {
		t.Error("skipping a skipped row must fail (link_not_queued)")
	}
	q, err = h.svc.UnskipQueuedEvent(ctx, uA, model.ImportLinkActionRequest{LinkId: linkID})
	if err != nil || len(q.Queued) != 1 || len(q.Skipped) != 0 {
		t.Fatalf("unskip = %+v, %v", q, err)
	}
	if _, err := h.svc.UnskipQueuedEvent(ctx, uA, model.ImportLinkActionRequest{LinkId: linkID}); err == nil {
		t.Error("unskipping a queued row must fail (link_not_skipped)")
	}
	q, err = h.svc.DiscardEvent(ctx, uA, model.DiscardImportEventRequest{EventId: failed.EventId})
	if err != nil || len(q.Failed) != 0 {
		t.Fatalf("discard = %+v, %v", q, err)
	}
	if _, err := h.repo.GetEvent(ctx, vo.MustParseId(failed.EventId)); err == nil {
		t.Error("discarded event row must be deleted")
	}
	// discarding a processed event is refused: it is the dedupe memory
	ok := ingest(t, h, `{"account":"Apple Card","amount":"1","currency":"USD","eventId":"k"}`)
	if _, err := h.svc.DiscardEvent(ctx, uA, model.DiscardImportEventRequest{EventId: ok.EventId}); err == nil {
		t.Error("discard of a processed event must fail (event_not_failed)")
	}
}

func TestGetTransactionImportList(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	h.mapCard(t, "Apple Card")
	ingest(t, h, tap)
	links, _ := h.repo.ListLinksBySource(ctx, vo.MustParseId(source))
	txID := links[0].TransactionID.String()
	res, err := h.svc.GetTransactionImportList(ctx, uA, model.TransactionImportListRequest{TransactionId: txID})
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("list = %+v, %v", res, err)
	}
	it := res.Items[0]
	if it.Provider != "apple-wallet" || it.SourceName != "iPhone" || it.ExternalAccountId != "Apple Card" || it.ExternalPayee != "Blue Bottle" || it.ExternalCurrency != "USD" || it.Status != "linked" || it.ExternalPostedAt != "2026-08-20 10:42:03" || it.ImportedAt == "" {
		t.Errorf("item = %+v", it)
	}
	if other, _ := h.svc.GetTransactionImportList(ctx, vo.MustParseId(userB), model.TransactionImportListRequest{TransactionId: txID}); len(other.Items) != 0 {
		t.Errorf("foreign user sees no provenance: %+v", other)
	}
	if empty, err := h.svc.GetTransactionImportList(ctx, uA, model.TransactionImportListRequest{TransactionId: vo.NewId().String()}); err != nil || empty.Items == nil {
		t.Errorf("unknown transaction = %+v, %v (empty list, never null)", empty, err)
	}
}
