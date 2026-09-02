package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestCreateSource_IdempotentPerProvider(t *testing.T) {
	h := setup(t) // userA already has `source`
	uB := vo.MustParseId(userB)
	first, err := h.svc.CreateSource(context.Background(), uB, model.CreateImportSourceRequest{Provider: "apple-wallet", Name: "My iPhone"})
	if err != nil || first.Item.Provider != "apple-wallet" || first.Item.Name != "My iPhone" || first.Item.Status != "active" || first.Item.Cards == nil {
		t.Fatalf("create = %+v, %v", first, err)
	}
	second, err := h.svc.CreateSource(context.Background(), uB, model.CreateImportSourceRequest{Provider: "apple-wallet", Name: "Other name"})
	if err != nil || second.Item.Id != first.Item.Id || second.Item.Name != "My iPhone" {
		t.Fatalf("second create must return the existing source: %+v, %v", second, err)
	}
	if err := (model.CreateImportSourceRequest{Provider: "plaid", Name: "x"}).Validate(); err == nil {
		t.Fatal("unknown provider must fail validation")
	}
}

func TestGetSourceList_CardsUnionLedgerAndLinks(t *testing.T) {
	h := setup(t)
	ingest(t, h, tap)                                                                        // "Apple Card" queued (unmapped)
	ingest(t, h, `{"account":"Apple Card","amount":"2","currency":"USD","eventId":"evt-2"}`) // second tap on the same card
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: "Old Card", Mode: "ignore"})
	res, err := h.svc.GetSourceList(context.Background(), vo.MustParseId(userA))
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("list = %+v, %v", res, err)
	}
	cards := map[string]model.ImportCardResult{}
	for _, c := range res.Items[0].Cards {
		cards[c.ExternalAccountId] = c
	}
	if c := cards["Apple Card"]; c.State != "unmapped" || c.QueuedCount != 2 || c.TapCount != 2 || c.LastSeenAt == "" || c.AccountId != "" {
		t.Errorf("unmapped card = %+v", c)
	}
	if c := cards["Old Card"]; c.State != "ignored" || c.TapCount != 0 {
		t.Errorf("ignored card = %+v", c)
	}
	// another user sees nothing
	other, _ := h.svc.GetSourceList(context.Background(), vo.MustParseId(userB))
	if len(other.Items) != 0 {
		t.Errorf("foreign list = %+v", other)
	}
}

func TestDeleteSource(t *testing.T) {
	h := setup(t)
	ingest(t, h, tap)
	if _, err := h.svc.DeleteSource(context.Background(), vo.MustParseId(userB), model.DeleteImportSourceRequest{Id: source}); err == nil {
		t.Fatal("foreign delete must fail")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("foreign delete = %T, want NotFound", err)
	}
	res, err := h.svc.DeleteSource(context.Background(), vo.MustParseId(userA), model.DeleteImportSourceRequest{Id: source})
	if err != nil || len(res.Items) != 0 {
		t.Fatalf("delete = %+v, %v", res, err)
	}
	if _, err := h.repo.GetSource(context.Background(), vo.MustParseId(source)); err == nil {
		t.Error("source row must be gone (cascade removes events/links)")
	}
}
