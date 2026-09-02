package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func cardByID(res *model.UpdateImportAccountResult, ext string) model.ImportCardResult {
	for _, c := range res.Item.Cards {
		if c.ExternalAccountId == ext {
			return c
		}
	}
	return model.ImportCardResult{}
}

func TestLinkAccount_RunsConversionOverQueuedTaps(t *testing.T) {
	h := setup(t)
	ingest(t, h, tap) // queued: unmapped
	ingest(t, h, `{"account":"apple card","payee":"Shop","amount":"2","currency":"USD","occurredAt":"2026-08-19T09:00:00Z","eventId":"evt-2"}`)
	res, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1})
	if err != nil {
		t.Fatalf("LinkAccount: %v", err)
	}
	if res.Run == nil || res.Run.Status != model.ImportRunStatusCompleted || res.Run.ImportedCount != 2 || res.Run.MatchedCount != 0 || res.Run.SkippedCount != 0 {
		t.Fatalf("run = %+v", res.Run)
	}
	if len(h.txns.created) != 2 {
		t.Fatalf("conversion must create both queued taps: %d", len(h.txns.created))
	}
	if c := cardByID(res, "Apple Card"); c.State != "mapped" || c.AccountId != acct1 || c.QueuedCount != 0 || c.TapCount != 2 {
		t.Errorf("card after link = %+v", c)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	for _, l := range links {
		if l.Status != model.ImportLinkStatusLinked || l.TransactionID == nil || l.RunID == nil {
			t.Errorf("link must be linked + stamped with the run: %+v", l)
		}
	}
	// the next tap goes straight through
	if r := ingest(t, h, `{"account":"Apple Card","amount":"3","currency":"USD","eventId":"evt-3"}`); r.Status != model.ImportIngestStatusCreated {
		t.Errorf("post-link ingest = %+v", r)
	}
}

func TestLinkAccount_Rejections(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	// foreign account
	_, err := h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acctB})
	if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("foreign account = %v, want NotFound", err)
	}
	// deleted account
	h.accounts.deleted = true
	_, err = h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeTransactionAccountDeleted {
		t.Errorf("deleted account = %v", err)
	}
	h.accounts.deleted = false
	// currency mismatch: the card's ledger is EUR, the account is USD
	ingest(t, h, `{"account":"Euro Card","amount":"5","currency":"EUR","eventId":"e1"}`)
	_, err = h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Euro Card", AccountId: acct1})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeImportCurrencyMismatch {
		t.Errorf("currency mismatch = %v", err)
	}
	// duplicate (case-insensitive)
	if _, err := h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1}); err != nil {
		t.Fatalf("first link: %v", err)
	}
	_, err = h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "APPLE CARD", AccountId: acct1})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeImportAccountLinkExists {
		t.Errorf("duplicate = %v", err)
	}
	// blank / foreign source
	if _, err := h.svc.LinkAccount(ctx, vo.MustParseId(userB), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "X", AccountId: acctB}); err == nil {
		t.Error("foreign source must fail")
	}
}

func TestLinkAccount_NoRateStaysQueued(t *testing.T) {
	h := setup(t)
	ingest(t, h, `{"account":"GBP Card","amount":"5","currency":"GBP","eventId":"g1"}`)
	// mapping an all-GBP card onto a USD account is refused (mismatch) — so map a mixed card instead
	ingest(t, h, `{"account":"Mixed","amount":"5","currency":"GBP","eventId":"m1"}`)
	ingest(t, h, `{"account":"Mixed","amount":"6","currency":"USD","eventId":"m2"}`)
	res, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Mixed", AccountId: acct1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.ImportedCount != 1 || res.Run.SkippedCount != 1 || res.Run.Status != model.ImportRunStatusPartial {
		t.Errorf("run = %+v", res.Run)
	}
	if c := cardByID(res, "Mixed"); c.QueuedCount != 1 {
		t.Errorf("no-rate tap must stay queued: %+v", c)
	}
}

func TestIgnoreAndUnlinkAccount(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	ingest(t, h, tap)
	res, err := h.svc.IgnoreAccount(ctx, uA, model.ImportAccountActionRequest{SourceId: source, ExternalAccountId: "Apple Card"})
	if err != nil || res.Run != nil || cardByID(res, "Apple Card").State != "ignored" || cardByID(res, "Apple Card").QueuedCount != 0 {
		t.Fatalf("ignore = %+v, %v", res, err)
	}
	links, _ := h.repo.ListLinksBySource(ctx, vo.MustParseId(source))
	if links[0].Status != model.ImportLinkStatusSkipped {
		t.Errorf("queued tap must become skipped on ignore: %+v", links[0])
	}
	if r := ingest(t, h, `{"account":"Apple Card","amount":"9","currency":"USD","eventId":"evt-9"}`); r.Status != model.ImportIngestStatusSkipped {
		t.Errorf("ignored card ingest = %+v", r)
	}
	// map-instead: link over an ignored card flips it to import and converts nothing (all rows are skipped now)
	linked, err := h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1})
	if err != nil || cardByID(linked, "Apple Card").State != "mapped" || linked.Run.ImportedCount != 0 {
		t.Fatalf("map-instead = %+v, %v", linked, err)
	}
	// unlink forgets the mapping; queued rows and their events go, seen rows stay
	ingest(t, h, `{"account":"Fresh","amount":"1","currency":"USD","eventId":"f1"}`) // queued on another card
	un, err := h.svc.UnlinkAccount(ctx, uA, model.ImportAccountActionRequest{SourceId: source, ExternalAccountId: "Fresh"})
	if err != nil || cardByID(un, "Fresh").ExternalAccountId != "" {
		t.Fatalf("unlink must drop an unmapped card entirely: %+v, %v", un, err)
	}
	if evs, _ := h.repo.ListEventsBySourceStatus(ctx, vo.MustParseId(source), model.ImportEventStatusProcessed); len(evs) != 2 {
		t.Errorf("only the unlinked card's event may disappear (2 Apple Card events remain): %d", len(evs))
	}
	if r := ingest(t, h, `{"account":"Fresh","amount":"1","currency":"USD","eventId":"f1"}`); r.Status != model.ImportIngestStatusQueued {
		t.Errorf("a re-fired tap on a forgotten card is new again: %+v", r)
	}
	un, _ = h.svc.UnlinkAccount(ctx, uA, model.ImportAccountActionRequest{SourceId: source, ExternalAccountId: "Apple Card"})
	if c := cardByID(un, "Apple Card"); c.State != "unmapped" || c.TapCount != 2 {
		t.Errorf("unlink keeps seen rows: %+v", c)
	}
}
