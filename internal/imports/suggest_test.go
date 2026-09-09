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

type fakeCompleter struct {
	reply    string
	err      error
	calls    int
	lastUser string
}

func (f *fakeCompleter) Complete(_ context.Context, _ string, user string) (string, error) {
	f.calls++
	f.lastUser = user
	return f.reply, f.err
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T: %v", err, err)
	}
	return ve.MsgCode
}

// seedUnclassifiedImports writes two Blue Bottle rows and leaves both
// unclassified (seedTwoImports files one of them under Groceries, which is
// exactly the history this test must not have).
func seedUnclassifiedImports(t *testing.T, h *harness) {
	t.Helper()
	h.mapCard(t, "Apple Card")
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("first: %s", res.Status)
	}
	second := strings.Replace(tap, `"eventId":"evt-1"`, `"eventId":"evt-2"`, 1)
	if res := ingest(t, h, second); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("second: %s", res.Status)
	}
}

// classify files a seeded import under a category the way the user would
// from the transaction dialog; fakeTxns.GetByID reads the real row back.
// linkID is what seedTwoImports returns — a ledger row id, not a transaction id.
func classify(t *testing.T, h *harness, linkID vo.Id, categoryID string) {
	t.Helper()
	link, err := h.repo.GetLink(context.Background(), linkID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Raw.ExecContext(context.Background(), h.db.Rebind("UPDATE transactions SET category_id = ? WHERE id = ?"), categoryID, link.TransactionID.String()); err != nil {
		t.Fatal(err)
	}
}

func TestSuggestRules_DisabledWithoutCompleter(t *testing.T) {
	h := setup(t)
	_, err := h.svc.SuggestRules(context.Background(), vo.MustParseId(userA), model.SuggestImportRulesRequest{})
	if got := codeOf(t, err); got != errs.CodeImportAiDisabled {
		t.Fatalf("code = %q", got)
	}
}

func TestSuggestRules_NoClassifiedImportsSkipsTheModel(t *testing.T) {
	h := setup(t)
	fc := &fakeCompleter{reply: `{"rules":[]}`}
	h.svc.SetCompleter(fc)
	seedUnclassifiedImports(t, h)
	res, err := h.svc.SuggestRules(context.Background(), vo.MustParseId(userA), model.SuggestImportRulesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 || fc.calls != 0 {
		t.Fatalf("items=%d calls=%d — an empty history must not spend a completion", len(res.Items), fc.calls)
	}
}

func TestSuggestRules_ValidatesEveryProposedRow(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	edited, _ := seedTwoImports(t, h)
	classify(t, h, edited, cat)

	hallucinated := vo.NewId().String()
	fc := &fakeCompleter{reply: "```json\n" + `{"rules":[
		{"matchField":"external_payee","matchType":"contains","matchValue":"Blue Bottle","categoryId":"` + cat + `","reason":"3 coffee purchases"},
		{"matchField":"external_payee","matchType":"contains","matchValue":"Blue Bottle","categoryId":"` + cat + `","reason":"duplicate of the first"},
		{"matchField":"external_payee","matchType":"exact","matchValue":"Ghost","categoryId":"` + hallucinated + `"},
		{"matchField":"external_payee","matchType":"regex","matchValue":"B.*","categoryId":"` + cat + `"},
		{"matchField":"external_payee","matchType":"exact","matchValue":"Nothing to set"},
		{"matchField":"external_payee","matchType":"prefix","matchValue":"  Skip me  ","action":"skip","categoryId":"` + cat + `"}
	]}` + "\n```"}
	h.svc.SetCompleter(fc)

	res, err := h.svc.SuggestRules(context.Background(), uid, model.SuggestImportRulesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if fc.calls != 1 {
		t.Fatalf("calls = %d, want exactly one completion", fc.calls)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %+v, want the deduped Blue Bottle row and the skip row re-typed as classify", res.Items)
	}
	first := res.Items[0]
	if first.Action != model.ImportRuleActionClassify || first.MatchType != model.ImportRuleMatchTypeContains || first.MatchValue != "Blue Bottle" || first.CategoryId == nil || *first.CategoryId != cat || first.Reason != "3 coffee purchases" {
		t.Fatalf("first = %+v", first)
	}
	if second := res.Items[1]; second.MatchValue != "Skip me" || second.Action != model.ImportRuleActionClassify {
		t.Fatalf("second = %+v (value trimmed, action forced to classify)", second)
	}
	if !strings.Contains(fc.lastUser, "Blue Bottle") || !strings.Contains(fc.lastUser, `"Coffee"`) {
		t.Fatalf("prompt must carry the payee strings and entity names: %s", fc.lastUser)
	}
	if strings.Contains(fc.lastUser, "4.75") {
		t.Fatalf("prompt must not carry amounts: %s", fc.lastUser)
	}
}

func TestSuggestRules_ModelFailuresAreCodedNot500(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	edited, _ := seedTwoImports(t, h)
	classify(t, h, edited, cat)

	for name, fc := range map[string]*fakeCompleter{
		"transport error": {err: errors.New("dial tcp: connection refused")},
		"not json":        {reply: "Sure! Here are some rules: none."},
		"wrong shape":     {reply: `{"rules":"nope"}`},
	} {
		h.svc.SetCompleter(fc)
		_, err := h.svc.SuggestRules(context.Background(), uid, model.SuggestImportRulesRequest{})
		if got := codeOf(t, err); got != errs.CodeImportAiUnavailable {
			t.Fatalf("%s: code = %q", name, got)
		}
	}
}

func TestSuggestRules_RateLimited(t *testing.T) {
	h := setup(t)
	h.withLimiter()
	h.svc.SetCompleter(&fakeCompleter{reply: `{"rules":[]}`})
	uid := vo.MustParseId(userA)
	for i := 0; i < 2; i++ {
		if _, err := h.svc.SuggestRules(context.Background(), uid, model.SuggestImportRulesRequest{}); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	_, err := h.svc.SuggestRules(context.Background(), uid, model.SuggestImportRulesRequest{})
	var tooMany *errs.TooManyRequestsError
	if !errors.As(err, &tooMany) {
		t.Fatalf("third call: want 429, got %v", err)
	}
	if h.lim.fail != 2 {
		t.Fatalf("every allowed call must count: fail=%d", h.lim.fail)
	}
}
