package imports_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// two imported Blue Bottle rows on the seeded source, one of them hand-edited afterwards
func seedTwoImports(t *testing.T, h *harness) (edited, untouched vo.Id) {
	t.Helper()
	h.mapCard(t, "Apple Card")
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("first: %s", res.Status)
	}
	second := strings.Replace(tap, `"eventId":"evt-1"`, `"eventId":"evt-2"`, 1)
	if res := ingest(t, h, second); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("second: %s", res.Status)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	other := h.f.Category(fixture.Category{UserID: userA, Name: "Groceries"})
	h.db.Raw.ExecContext(context.Background(), h.db.Rebind(`UPDATE transactions SET category_id = ? WHERE id = ?`), other, links[0].TransactionID.String())
	return links[0].ID, links[1].ID
}

func TestPreviewRule_CountsMatchesAndEdited(t *testing.T) {
	h := setup(t)
	seedTwoImports(t, h)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.add(h.entities.categories, vo.MustParseId(userA), cat, "Coffee")
	res, err := h.svc.PreviewRule(context.Background(), vo.MustParseId(userA), model.PreviewImportRuleRequest{
		ImportRuleSpec: classifyReq(h, cat).ImportRuleSpec, Scope: model.ImportRuleScopeSource, ScopeSourceId: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched != 2 || res.AlreadyEdited != 1 {
		t.Fatalf("got %+v", res)
	}
	none, _ := h.svc.PreviewRule(context.Background(), vo.MustParseId(userA), model.PreviewImportRuleRequest{
		ImportRuleSpec: model.ImportRuleSpec{Action: "skip", MatchField: "external_payee", MatchType: "exact", MatchValue: "nothing"},
		Scope:          model.ImportRuleScopeAll,
	})
	if none.Matched != 0 {
		t.Fatalf("got %+v", none)
	}
}

func TestApplyRule_SkipsEditedUnlessIncluded(t *testing.T) {
	h := setup(t)
	editedID, untouchedID := seedTwoImports(t, h)
	uid := vo.MustParseId(userA)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	label := h.f.Label(fixture.Label{UserID: userA, Name: "Work"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	h.entities.add(h.entities.labels, uid, label, "Work")
	req := classifyReq(h, cat)
	req.LabelIds = []string{label}
	rule, err := h.svc.CreateRule(context.Background(), uid, req)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule.Id, Scope: model.ImportRuleScopeAll})
	if err != nil || res.Updated != 1 || res.Skipped != 1 {
		t.Fatalf("first apply: %v %+v", err, res)
	}
	if len(h.txns.replaced) != 1 || *h.txns.replaced[0].CategoryId != cat || len(h.txns.replaced[0].LabelIds) != 1 {
		t.Fatalf("untouched row rewritten with the rule's targets: %+v", h.txns.replaced)
	}
	l, _ := h.repo.GetLink(context.Background(), untouchedID)
	if l.AppliedCategoryID == nil || l.AppliedCategoryID.String() != cat || l.AppliedRuleID == nil || l.AppliedRuleID.String() != rule.Id {
		t.Fatalf("applied snapshot after apply: %+v", l)
	}
	res, err = h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule.Id, Scope: model.ImportRuleScopeAll, IncludeEdited: true})
	if err != nil || res.Updated != 2 || res.Skipped != 0 {
		t.Fatalf("include-edited apply: %v %+v", err, res)
	}
	e, _ := h.repo.GetLink(context.Background(), editedID)
	if e.AppliedCategoryID == nil || e.AppliedCategoryID.String() != cat {
		t.Fatalf("edited row now applied: %+v", e)
	}
}

func TestApplyRule_RefusesSkipRulesAndForeignScope(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	skip := h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: "skip", MatchValue: "x"})
	_, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: skip, Scope: model.ImportRuleScopeAll})
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.MsgCode != errs.CodeImportRuleSkipNotApplicable {
		t.Fatalf("got %v", err)
	}
	theirs := h.f.ImportSource(fixture.ImportSource{UserID: userB, Provider: model.ImportProviderSimpleFIN, Name: "Theirs"})
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "x"})
	if _, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule, Scope: model.ImportRuleScopeSource, ScopeSourceId: theirs}); !isNotFound(err) {
		t.Fatalf("foreign source scope must be not-found: %v", err)
	}
}

// scope=run is the one scope whose ownership check is hand-rolled (the run
// carries the user id directly), so it gets its own guard.
func TestRuleScope_ForeignRunIsNotFound(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	theirSource := h.f.ImportSource(fixture.ImportSource{UserID: userB, Provider: model.ImportProviderSimpleFIN, Name: "Theirs"})
	theirRun := h.f.ImportRun(fixture.ImportRun{UserID: userB, SourceID: theirSource, Provider: model.ImportProviderSimpleFIN})
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "x"})
	if _, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule, Scope: model.ImportRuleScopeRun, RunId: theirRun}); !isNotFound(err) {
		t.Fatalf("apply: %v", err)
	}
	_, err := h.svc.PreviewRule(context.Background(), uid, model.PreviewImportRuleRequest{
		ImportRuleSpec: model.ImportRuleSpec{Action: "skip", MatchField: "external_payee", MatchType: "contains", MatchValue: "x"},
		Scope:          model.ImportRuleScopeRun, RunId: theirRun,
	})
	if !isNotFound(err) {
		t.Fatalf("preview: %v", err)
	}
}
