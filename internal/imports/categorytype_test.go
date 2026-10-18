package imports_test

import (
	"context"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// refund is tap's income twin: same card and payee, money coming back.
var refund = strings.Replace(tap, `"eventId":"evt-1"`, `"eventId":"evt-refund","type":"income"`, 1)

// coffeeAndRefundRules gives userA an expense rule (Coffee + a payee) ahead
// of an income rule (Refunds) on the same payee text.
func coffeeAndRefundRules(t *testing.T, h *harness) (coffee, refunds, payee string) {
	t.Helper()
	uid := vo.MustParseId(userA)
	coffee = h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	refunds = h.f.Category(fixture.Category{UserID: userA, Name: "Refunds", Type: 1})
	payee = h.f.Payee(fixture.Payee{UserID: userA, Name: "Blue Bottle"})
	h.entities.add(h.entities.categories, uid, coffee, "Coffee")
	h.entities.categories[uid] = append(h.entities.categories[uid], model.ImportNamed{ID: refunds, Name: "Refunds", OwnerID: userA, Income: true})
	h.entities.add(h.entities.payees, uid, payee, "Blue Bottle")
	h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: coffee, PayeeID: payee, Priority: 0})
	h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: refunds, Priority: 1})
	return coffee, refunds, payee
}

func TestIngest_RuleCategoryOnlyClassifiesItsOwnKind(t *testing.T) {
	h := setup(t)
	coffee, refunds, payee := coffeeAndRefundRules(t, h)
	h.mapCard(t, "Apple Card")
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("expense tap: %s", res.Status)
	}
	if res := ingest(t, h, refund); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("refund tap: %s", res.Status)
	}
	spend, back := h.txns.created[0], h.txns.created[1]
	if spend.CategoryId == nil || *spend.CategoryId != coffee {
		t.Errorf("expense takes the expense category: %+v", spend)
	}
	// the higher rule's expense category is held back on the income tap, so
	// the lower income rule wins the field; the higher rule's payee still applies
	if back.Type != "income" || back.CategoryId == nil || *back.CategoryId != refunds {
		t.Errorf("income takes the income category: %+v", back)
	}
	if back.PayeeId == nil || *back.PayeeId != payee {
		t.Errorf("the payee applies whatever the kind: %+v", back)
	}
}

func TestPreviewAndApply_SkipRowsOfTheOtherKind(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	h.mapCard(t, "Apple Card")
	if res := ingest(t, h, refund); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("refund tap: %s", res.Status)
	}
	coffee := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.add(h.entities.categories, uid, coffee, "Coffee")
	spec := classifyReq(h, coffee)
	preview, err := h.svc.PreviewRule(context.Background(), uid, model.PreviewImportRuleRequest{ImportRuleSpec: spec.ImportRuleSpec, Scope: model.ImportRuleScopeAll})
	if err != nil || preview.Matched != 0 {
		t.Fatalf("an expense-only rule matches no income row: %+v %v", preview, err)
	}
	rule, err := h.svc.CreateRule(context.Background(), uid, spec)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule.Id, Scope: model.ImportRuleScopeAll, IncludeEdited: true})
	if err != nil || res.Updated != 0 || len(h.txns.replaced) != 0 {
		t.Fatalf("apply must leave the income row alone: %+v %v %+v", res, err, h.txns.replaced)
	}

	refunds := h.f.Category(fixture.Category{UserID: userA, Name: "Refunds", Type: 1})
	h.entities.categories[uid] = append(h.entities.categories[uid], model.ImportNamed{ID: refunds, Name: "Refunds", OwnerID: userA, Income: true})
	preview, err = h.svc.PreviewRule(context.Background(), uid, model.PreviewImportRuleRequest{ImportRuleSpec: classifyReq(h, refunds).ImportRuleSpec, Scope: model.ImportRuleScopeAll})
	if err != nil || preview.Matched != 1 {
		t.Fatalf("an income rule matches the income row: %+v %v", preview, err)
	}
}
