package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// userA's Blue Bottle classify rule, targeting userA's own Coffee category.
func coffeeRule(t *testing.T, h *harness) (cat string) {
	t.Helper()
	cat = h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.add(h.entities.categories, vo.MustParseId(userA), cat, "Coffee")
	h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: cat})
	return cat
}

func TestLinkAccount_SharedAccountWithWriteGrant(t *testing.T) {
	h := setup(t)
	coffeeRule(t, h)
	ingest(t, h, tap) // queued: unmapped
	h.accounts.shared = true
	res, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acctB})
	if err != nil {
		t.Fatalf("LinkAccount on a shared account: %v", err)
	}
	if res.Run == nil || res.Run.ImportedCount != 1 {
		t.Fatalf("run = %+v", res.Run)
	}
	// the rule's category is userA's, which a transaction on userB's account
	// cannot carry: the tap imports unclassified instead of failing
	req := h.txns.created[0]
	if req.AccountId != acctB || req.CategoryId != nil || req.PayeeId != nil || req.TagId != nil || len(req.LabelIds) != 0 {
		t.Fatalf("created = %+v", req)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if links[0].AppliedRuleID != nil || links[0].AppliedCategoryID != nil {
		t.Fatalf("applied snapshot must be empty: %+v", links[0])
	}
}

func TestIngest_RevokedShareQueues(t *testing.T) {
	h := setup(t)
	h.accounts.shared = true
	if _, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acctB}); err != nil {
		t.Fatal(err)
	}
	h.accounts.shared = false
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusQueued {
		t.Fatalf("status = %s", res.Status)
	}
	if len(h.txns.created) != 0 {
		t.Fatalf("nothing may be created without write access: %+v", h.txns.created)
	}
	q, err := h.svc.GetQueue(context.Background(), vo.MustParseId(userA))
	if err != nil || len(q.Queued) != 1 {
		t.Fatalf("queue = %+v, %v", q, err)
	}
	if e := q.Queued[0]; e.Reason != model.ImportQueueReasonNoAccess || e.AccountId != acctB {
		t.Errorf("entry = %+v", e)
	}
}

func TestApplyRule_LeavesSharedAccountRowsAlone(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	h.accounts.shared = true
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: "Apple Card", AccountID: acctB})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	preview, err := h.svc.PreviewRule(context.Background(), uid, model.PreviewImportRuleRequest{
		ImportRuleSpec: classifyReq(h, cat).ImportRuleSpec, Scope: model.ImportRuleScopeAll,
	})
	if err != nil || preview.Matched != 0 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	rule, err := h.svc.CreateRule(context.Background(), uid, classifyReq(h, cat))
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule.Id, Scope: model.ImportRuleScopeAll})
	if err != nil || res.Updated != 0 || len(h.txns.replaced) != 0 {
		t.Fatalf("apply = %+v, %v, replaced %+v", res, err, h.txns.replaced)
	}
}
