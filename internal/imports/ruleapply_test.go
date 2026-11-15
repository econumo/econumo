package imports_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/imports"
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
	if res.Matched != 2 || res.Changes != 2 || res.AlreadyEdited != 1 {
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
	if err != nil || res.Updated != 1 || res.Skipped != 0 {
		t.Fatalf("include-edited apply (the row the first apply classified no longer changes): %v %+v", err, res)
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

// failingLabelsRepo is the real repo with one read broken, so a transient DB
// failure is observable end to end.
type failingLabelsRepo struct {
	imports.Repository
	err error
}

func (r failingLabelsRepo) ListLinkAppliedLabels(context.Context, vo.Id) ([]vo.Id, error) {
	return nil, r.err
}

// A failed applied-label read used to read as "no labels applied", which makes
// every row look edited: apply then answered 200 with updated=0 and told the
// user it had skipped edits they never made.
func TestPreviewAndApplyRule_PropagateAppliedLabelReadFailure(t *testing.T) {
	h := setup(t)
	seedTwoImports(t, h)
	uid := vo.MustParseId(userA)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	req := classifyReq(h, cat)
	rule, err := h.svc.CreateRule(context.Background(), uid, req)
	if err != nil {
		t.Fatal(err)
	}

	boom := errors.New("database is locked")
	h.withRepo(failingLabelsRepo{Repository: h.repo, err: boom})

	if _, err := h.svc.PreviewRule(context.Background(), uid, model.PreviewImportRuleRequest{
		ImportRuleSpec: req.ImportRuleSpec, Scope: model.ImportRuleScopeAll,
	}); !errors.Is(err, boom) {
		t.Fatalf("preview must surface the read failure, got %v", err)
	}
	res, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule.Id, Scope: model.ImportRuleScopeAll})
	if !errors.Is(err, boom) {
		t.Fatalf("apply must surface the read failure, got %v (res %+v)", err, res)
	}
}

func TestPreviewRule_IsRateLimited(t *testing.T) {
	h := setup(t)
	h.withLimiter()
	uid := vo.MustParseId(userA)
	spec := model.PreviewImportRuleRequest{
		ImportRuleSpec: model.ImportRuleSpec{Action: model.ImportRuleActionSkip, MatchField: model.ImportRuleMatchFieldExternalPayee, MatchType: model.ImportRuleMatchTypeContains, MatchValue: "x"},
		Scope:          model.ImportRuleScopeAll,
	}
	for i := 0; i < 2; i++ {
		if _, err := h.svc.PreviewRule(context.Background(), uid, spec); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	_, err := h.svc.PreviewRule(context.Background(), uid, spec)
	if _, ok := errs.AsTooManyRequests(err); !ok {
		t.Fatalf("the third preview must be rate limited, got %v", err)
	}
	if h.lim.fail != 2 {
		t.Fatalf("every allowed preview counts toward the cap, got %d", h.lim.fail)
	}
}

// row A already carries the rule's category and payee (set by hand, so its
// applied snapshot differs), row B is empty
func seedClassifiedAndEmpty(t *testing.T, h *harness) (spec model.ImportRuleSpec) {
	t.Helper()
	seedTwoImports(t, h)
	uid := vo.MustParseId(userA)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	payee := h.f.Payee(fixture.Payee{UserID: userA, Name: "Blue Bottle"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	h.entities.add(h.entities.payees, uid, payee, "Blue Bottle")
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	h.db.Raw.ExecContext(context.Background(), h.db.Rebind(`UPDATE transactions SET category_id = ?, payee_id = ? WHERE id = ?`), cat, payee, links[0].TransactionID.String())
	h.db.Raw.ExecContext(context.Background(), h.db.Rebind(`UPDATE transactions SET category_id = NULL, payee_id = NULL WHERE id = ?`), links[1].TransactionID.String())
	spec = classifyReq(h, cat).ImportRuleSpec
	spec.PayeeId = &payee
	return spec
}

// The live count keeps the already classified row; only changes leaves it out.
func TestPreviewRuleCountsChangesApart(t *testing.T) {
	h := setup(t)
	spec := seedClassifiedAndEmpty(t, h)
	res, err := h.svc.PreviewRule(context.Background(), vo.MustParseId(userA), model.PreviewImportRuleRequest{
		ImportRuleSpec: spec, Scope: model.ImportRuleScopeSource, ScopeSourceId: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched != 2 || res.Changes != 1 || res.AlreadyEdited != 0 {
		t.Fatalf("got %+v", res)
	}
}

func TestApplyRuleLeavesUnchangedRowsUncounted(t *testing.T) {
	h := setup(t)
	spec := seedClassifiedAndEmpty(t, h)
	uid := vo.MustParseId(userA)
	rule, err := h.svc.CreateRule(context.Background(), uid, model.CreateImportRuleRequest{Id: vo.NewId().String(), ImportRuleSpec: spec})
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule.Id, Scope: model.ImportRuleScopeAll})
	if err != nil || res.Updated != 1 || res.Skipped != 0 {
		t.Fatalf("apply: %v %+v", err, res)
	}
	if len(h.txns.replaced) != 1 {
		t.Fatalf("the already classified row was rewritten: %+v", h.txns.replaced)
	}
}

// A hand-edited row that already carries the rule's targets needs no
// rewrite, but include-edited still makes the rule its applied baseline, so
// the snapshot no longer reads as an edit.
func TestApplyRule_IncludeEditedRestampsAnEditedRowThatAlreadyMatches(t *testing.T) {
	h := setup(t)
	editedID, _ := seedTwoImports(t, h)
	uid := vo.MustParseId(userA)
	e, _ := h.repo.GetLink(context.Background(), editedID)
	var groceries string
	if err := h.db.Raw.QueryRowContext(context.Background(), h.db.Rebind(`SELECT category_id FROM transactions WHERE id = ?`), e.TransactionID.String()).Scan(&groceries); err != nil {
		t.Fatal(err)
	}
	h.entities.add(h.entities.categories, uid, groceries, "Groceries")
	rule, err := h.svc.CreateRule(context.Background(), uid, classifyReq(h, groceries))
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule.Id, Scope: model.ImportRuleScopeAll, IncludeEdited: true})
	if err != nil || res.Updated != 1 {
		t.Fatalf("apply: %v %+v", err, res)
	}
	if len(h.txns.replaced) != 1 || h.txns.replaced[0].Id == e.TransactionID.String() {
		t.Fatalf("the edited row was rewritten: %+v", h.txns.replaced)
	}
	e, _ = h.repo.GetLink(context.Background(), editedID)
	if e.AppliedCategoryID == nil || e.AppliedCategoryID.String() != groceries || e.AppliedRuleID == nil || e.AppliedRuleID.String() != rule.Id {
		t.Fatalf("edited row's snapshot: %+v", e)
	}
	preview, err := h.svc.PreviewRule(context.Background(), uid, model.PreviewImportRuleRequest{ImportRuleSpec: classifyReq(h, groceries).ImportRuleSpec, Scope: model.ImportRuleScopeAll})
	if err != nil || preview.AlreadyEdited != 0 {
		t.Fatalf("preview: %v %+v", err, preview)
	}
}
