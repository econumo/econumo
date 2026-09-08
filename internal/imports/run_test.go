package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestGetRunListAndGetRun(t *testing.T) {
	h, p := bankHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.txs = []model.ExternalTransaction{extTx("ACT-1", "T1", "-1", 1755900000, "Coffee")}
	ctx := context.Background()
	res, err := h.svc.Sync(ctx, vo.MustParseId(userA), syncReq())
	if err != nil {
		t.Fatal(err)
	}
	list, err := h.svc.GetRunList(ctx, vo.MustParseId(userA), "")
	if err != nil || len(list.Items) != 1 || list.Items[0].Id != res.Run.Id || list.Items[0].SourceId != bankSource {
		t.Fatalf("list = %+v err %v", list, err)
	}
	filtered, err := h.svc.GetRunList(ctx, vo.MustParseId(userA), source)
	if err != nil || len(filtered.Items) != 0 {
		t.Fatalf("filtered = %+v err %v", filtered, err)
	}
	if _, err := h.svc.GetRunList(ctx, vo.MustParseId(userA), "not-a-uuid"); !isNotFound(err) {
		t.Fatalf("unknown source id must be not found, got %v", err)
	}
	one, err := h.svc.GetRun(ctx, vo.MustParseId(userA), res.Run.Id)
	if err != nil || one.Item.Id != res.Run.Id || len(one.Links) != 1 || one.Links[0].ExternalPayee != "Coffee" || one.Links[0].Status != model.ImportLinkStatusLinked || one.Links[0].TransactionId == "" {
		t.Fatalf("run = %+v err %v", one, err)
	}
	if _, err := h.svc.GetRun(ctx, vo.MustParseId(userB), res.Run.Id); !isNotFound(err) {
		t.Fatalf("foreign run must be not found, got %v", err)
	}
	if _, err := h.svc.GetRun(ctx, vo.MustParseId(userA), "nope"); !isNotFound(err) {
		t.Fatalf("bad id must be not found, got %v", err)
	}
}

func isNotFound(err error) bool {
	_, ok := errs.AsNotFound(err)
	return ok
}
