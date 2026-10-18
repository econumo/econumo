package imports_test

import (
	"context"
	"strings"
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
	// userA's own rule names userA's category, which a transaction on
	// userB's account cannot carry, and userB has no rules: unclassified
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

// userB's Cafe rule on userB's own Cafe category.
func ownerCafeRule(t *testing.T, h *harness) (cat, rule string) {
	t.Helper()
	cat = h.f.Category(fixture.Category{UserID: userB, Name: "Cafe"})
	h.entities.add(h.entities.categories, vo.MustParseId(userB), cat, "Cafe")
	rule = h.f.ImportRule(fixture.ImportRule{UserID: userB, MatchValue: "blue bottle", CategoryID: cat})
	return cat, rule
}

func TestIngest_SharedAccountTakesTheOwnersRules(t *testing.T) {
	h := setup(t)
	coffeeRule(t, h)
	cafe, rule := ownerCafeRule(t, h)
	h.accounts.shared = true
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: "Apple Card", AccountID: acctB})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	req := h.txns.created[0]
	if req.AccountId != acctB || req.CategoryId == nil || *req.CategoryId != cafe {
		t.Fatalf("the account owner's rule must classify the tap: %+v", req)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if links[0].AppliedRuleID == nil || links[0].AppliedRuleID.String() != rule {
		t.Fatalf("applied rule = %v, want the owner's %s", links[0].AppliedRuleID, rule)
	}
}

func TestIngest_SharedAccountTakesTheOwnersSkipRules(t *testing.T) {
	h := setup(t)
	h.accounts.shared = true
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: "Apple Card", AccountID: acctB})
	// the importer's own skip rule does not decide what lands on userB's account
	h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: "skip", MatchValue: "blue bottle"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s, want created: userA's skip rule must not apply on userB's account", res.Status)
	}
	h.f.ImportRule(fixture.ImportRule{UserID: userB, Action: "skip", MatchValue: "blue bottle"})
	second := strings.Replace(tap, "evt-1", "evt-2", 1)
	if res := ingest(t, h, second); res.Status != model.ImportIngestStatusSkipped {
		t.Fatalf("status = %s, want skipped by userB's rule", res.Status)
	}
}

func TestLinkAccount_SharedAccountReplaysWithTheOwnersRules(t *testing.T) {
	h := setup(t)
	cafe, _ := ownerCafeRule(t, h)
	ingest(t, h, tap) // queued: unmapped
	h.accounts.shared = true
	if _, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acctB}); err != nil {
		t.Fatal(err)
	}
	if req := h.txns.created[0]; req.CategoryId == nil || *req.CategoryId != cafe {
		t.Fatalf("created = %+v", req)
	}
}

// The owner's rules classify what a connection imports into the owner's
// account, so applying one reaches those rows as well as the owner's own.
func TestApplyRule_OwnerReachesRowsOthersImportedIntoTheirAccount(t *testing.T) {
	h := setup(t)
	h.accounts.shared = true
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: "Apple Card", AccountID: acctB})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	owner := vo.MustParseId(userB)
	cafe := h.f.Category(fixture.Category{UserID: userB, Name: "Cafe"})
	h.entities.add(h.entities.categories, owner, cafe, "Cafe")
	spec := classifyReq(h, cafe).ImportRuleSpec
	preview, err := h.svc.PreviewRule(context.Background(), owner, model.PreviewImportRuleRequest{ImportRuleSpec: spec, Scope: model.ImportRuleScopeAll})
	if err != nil || preview.Matched != 1 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	rule, err := h.svc.CreateRule(context.Background(), owner, classifyReq(h, cafe))
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.ApplyRule(context.Background(), owner, model.ApplyImportRuleRequest{RuleId: rule.Id, Scope: model.ImportRuleScopeAll})
	if err != nil || res.Updated != 1 {
		t.Fatalf("apply = %+v, %v", res, err)
	}
}
