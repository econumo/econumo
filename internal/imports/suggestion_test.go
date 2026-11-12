package imports_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/imports"
	importsrepo "github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// seedSuggestion imports one Blue Bottle tap per payee text through the AI
// job, which answers with the given category and payee; later taps join the
// open suggestion without a new call.
func seedSuggestion(t *testing.T, h *harness, c *fakeCompleter, cat, payee string, payees ...string) (model.ImportSuggestion, []model.ImportTransactionLink) {
	t.Helper()
	h.mapCard(t, "Apple Card")
	reply := `{"rows":{"0":{"matchText":"Blue Bottle"`
	if cat != "" {
		reply += `,"categoryId":"` + cat + `"`
	}
	if payee != "" {
		reply += `,"payeeId":"` + payee + `"`
	}
	c.reply = reply + `}}}`
	if len(payees) == 0 {
		payees = []string{"Blue Bottle"}
	}
	for i, p := range payees {
		if res := ingest(t, h, pushWithPayee(p, "evt-s"+string(rune('a'+i)))); res.Status != model.ImportIngestStatusCreated {
			t.Fatalf("ingest %d: %s", i, res.Status)
		}
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || len(got[0].LinkIDs) != len(payees) {
		t.Fatalf("seeded suggestions = %+v", got)
	}
	links, err := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if err != nil {
		t.Fatal(err)
	}
	return got[0], links
}

func (h *harness) setTx(t *testing.T, txID *vo.Id, column, value string) {
	t.Helper()
	if _, err := h.db.Raw.ExecContext(context.Background(), h.db.Rebind(`UPDATE transactions SET `+column+` = ? WHERE id = ?`), value, txID.String()); err != nil {
		t.Fatal(err)
	}
}

func suggestionList(t *testing.T, h *harness, user string) []model.ImportSuggestionResult {
	t.Helper()
	res, err := h.svc.GetSuggestionList(context.Background(), vo.MustParseId(user))
	if err != nil {
		t.Fatal(err)
	}
	return res.Items
}

func wantNotFound(t *testing.T, err error) {
	t.Helper()
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestSuggestionListOwnerOnly(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, links := seedSuggestion(t, h, c, cat, "")
	items := suggestionList(t, h, userA)
	if len(items) != 1 || items[0].Id != s.ID.String() || items[0].CategoryId != cat || items[0].PayeeId != "" || items[0].MatchField != model.ImportRuleMatchFieldExternalPayee || items[0].Type != "expense" {
		t.Fatalf("owner list = %+v", items)
	}
	tx := items[0].Transactions
	if len(tx) != 1 || tx[0].TransactionId != links[0].TransactionID.String() || tx[0].AccountId != acct1 || tx[0].Currency != "USD" || tx[0].ExternalPayee != "Blue Bottle" || tx[0].Date == "" || tx[0].Amount == "" {
		t.Fatalf("transactions = %+v", tx)
	}

	if got := suggestionList(t, h, userB); len(got) != 0 {
		t.Fatalf("partner list = %+v", got)
	}
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userB, MatchValue: "blue bottle"})
	_, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userB), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
	wantNotFound(t, err)
	_, err = h.svc.DismissSuggestion(context.Background(), vo.MustParseId(userB), model.DismissImportSuggestionRequest{Id: s.ID.String()})
	wantNotFound(t, err)
	if got := openSuggestions(t, h, userA); len(got) != 1 {
		t.Fatalf("the owner's suggestion must survive: %+v", got)
	}
}

func TestSuggestionListDropsFilledTransactions(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	other := h.category(t, userA, "Food")
	s, links := seedSuggestion(t, h, c, cat, py, "Blue Bottle", "Blue Bottle Mission")

	h.setTx(t, links[0].TransactionID, "category_id", other)
	h.setTx(t, links[0].TransactionID, "payee_id", py)
	items := suggestionList(t, h, userA)
	if len(items) != 1 || len(items[0].Transactions) != 1 || items[0].Transactions[0].TransactionId != links[1].TransactionID.String() {
		t.Fatalf("list = %+v", items)
	}

	// one field still empty keeps the row pending
	h.setTx(t, links[1].TransactionID, "category_id", other)
	if items = suggestionList(t, h, userA); len(items) != 1 || len(items[0].Transactions) != 1 {
		t.Fatalf("list = %+v", items)
	}
	h.setTx(t, links[1].TransactionID, "payee_id", py)
	if items = suggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("list = %+v", items)
	}
	_, err := h.repo.GetSuggestion(context.Background(), s.ID)
	wantNotFound(t, err)
}

func TestSuggestionListDropsArchivedCategory(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	seedSuggestion(t, h, c, cat, "")
	uA := vo.MustParseId(userA)
	for i := range h.entities.categories[uA] {
		h.entities.categories[uA][i].Archived = true
	}
	if items := suggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("list = %+v", items)
	}
}

func TestSuggestionListDropsArchivedPayee(t *testing.T) {
	h, c, _ := suggestSetup(t)
	py := h.payee(t, userA, "Blue Bottle Coffee")
	seedSuggestion(t, h, c, "", py)
	uA := vo.MustParseId(userA)
	for i := range h.entities.payees[uA] {
		h.entities.payees[uA][i].Archived = true
	}
	if items := suggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("list = %+v", items)
	}
}

func TestSuggestionListLooksUpEachAccountOnce(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	seedSuggestion(t, h, c, cat, "", "Blue Bottle", "Blue Bottle Mission", "Blue Bottle Hayes")
	h.accounts.ownerCalls = 0
	if items := suggestionList(t, h, userA); len(items) != 1 || len(items[0].Transactions) != 3 {
		t.Fatalf("list = %+v", items)
	}
	if h.accounts.ownerCalls != 1 {
		t.Fatalf("AccountOwner calls = %d, want 1", h.accounts.ownerCalls)
	}
}

// failingDelete makes the lazy cleanup fail.
type failingDelete struct{ *importsrepo.Repo }

func (failingDelete) DeleteSuggestion(context.Context, vo.Id) error { return errors.New("db down") }

func TestSuggestionListCleanupFailureStillAnswers(t *testing.T) {
	h, c, facts := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	other := h.category(t, userA, "Food")
	_, links := seedSuggestion(t, h, c, cat, "")
	h.setTx(t, links[0].TransactionID, "category_id", other)
	h.svc = imports.NewService(failingDelete{h.repo}, h.accounts, h.conv, h.txns, h.txns, h.entities, nil, h.db.TX, clock{now}, imports.DefaultMatcherConfig())
	h.wireSuggest(c, facts)
	res, err := h.svc.GetSuggestionList(context.Background(), vo.MustParseId(userA))
	if err != nil || len(res.Items) != 0 {
		t.Fatalf("list = %+v, %v", res, err)
	}
}

func TestAcceptFillsOnlyEmptyFields(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	mine := h.payee(t, userA, "My Cafe")
	s, links := seedSuggestion(t, h, c, cat, py)
	h.setTx(t, links[0].TransactionID, "payee_id", mine)
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: cat, PayeeID: py})

	res, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated = %d", res.Updated)
	}
	tx, err := h.txns.GetByID(context.Background(), *links[0].TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if tx.CategoryID == nil || tx.CategoryID.String() != cat || tx.PayeeID == nil || tx.PayeeID.String() != mine {
		t.Fatalf("transaction = %+v", tx)
	}
	_, err = h.repo.GetSuggestion(context.Background(), s.ID)
	wantNotFound(t, err)
	link, err := h.repo.GetLink(context.Background(), links[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	// The hand-set payee stays out of the snapshot, so the row still reads
	// as edited.
	if link.AppliedCategoryID == nil || link.AppliedCategoryID.String() != cat || link.AppliedPayeeID != nil || link.AppliedRuleID == nil || link.AppliedRuleID.String() != rule {
		t.Fatalf("applied = %+v", link)
	}
	if items := suggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("an accepted suggestion is not listed: %+v", items)
	}
	uA := vo.MustParseId(userA)
	preview, err := h.svc.PreviewRule(context.Background(), uA, model.PreviewImportRuleRequest{ImportRuleSpec: model.ImportRuleSpec{
		Action: model.ImportRuleActionClassify, MatchField: model.ImportRuleMatchFieldExternalPayee, MatchType: model.ImportRuleMatchTypeContains,
		MatchValue: "blue bottle", PayeeId: &py,
	}, Scope: model.ImportRuleScopeAll})
	if err != nil || preview.Matched != 1 || preview.AlreadyEdited != 1 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	applied, err := h.svc.ApplyRule(context.Background(), uA, model.ApplyImportRuleRequest{RuleId: rule, Scope: model.ImportRuleScopeAll})
	if err != nil || applied.Updated != 0 || applied.Skipped != 1 {
		t.Fatalf("apply = %+v, %v", applied, err)
	}
	if tx, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID); tx.PayeeID == nil || tx.PayeeID.String() != mine {
		t.Fatalf("apply overwrote the hand-set payee: %+v", tx)
	}
}

func TestAcceptSkipsRowsRuleDoesNotMatch(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, links := seedSuggestion(t, h, c, cat, "", "Blue Bottle", "SQ *BLUE BOTTLE #12")
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "sq *blue bottle", CategoryID: cat})

	res, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated = %d", res.Updated)
	}
	first, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID)
	second, _ := h.txns.GetByID(context.Background(), *links[1].TransactionID)
	if first.CategoryID != nil {
		t.Fatalf("the rule does not match the first row: %+v", first)
	}
	if second.CategoryID == nil || second.CategoryID.String() != cat {
		t.Fatalf("second = %+v", second)
	}
	// accepting again is not an open suggestion any more
	_, err = h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
	wantNotFound(t, err)
}

func TestAcceptRejectsForeignRule(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, links := seedSuggestion(t, h, c, cat, "")
	foreign := h.f.ImportRule(fixture.ImportRule{UserID: userB, MatchValue: "blue bottle"})
	_, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: foreign})
	wantNotFound(t, err)

	skip := h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: model.ImportRuleActionSkip, MatchValue: "blue bottle"})
	if _, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: skip}); err == nil {
		t.Fatal("a skip rule cannot accept a suggestion")
	}
	tx, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID)
	if tx.CategoryID != nil {
		t.Fatalf("transaction = %+v", tx)
	}
	if got := openSuggestions(t, h, userA); len(got) != 1 {
		t.Fatalf("the suggestion stays open: %+v", got)
	}
}

func TestDismissForgets(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, _ := seedSuggestion(t, h, c, cat, "")
	res, err := h.svc.DismissSuggestion(context.Background(), vo.MustParseId(userA), model.DismissImportSuggestionRequest{Id: s.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("list after dismiss = %+v", res.Items)
	}
	_, err = h.repo.GetSuggestion(context.Background(), s.ID)
	wantNotFound(t, err)

	if res := ingest(t, h, pushWithPayee("Blue Bottle", "evt-later")); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 2 {
		t.Fatalf("a dismissed merchant goes to the AI again: calls = %d", len(c.calls))
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].ID == s.ID {
		t.Fatalf("suggestions = %+v", got)
	}
}

func matchTextLink(t *testing.T, h *harness, payee string) model.ImportTransactionLink {
	t.Helper()
	h.mapCard(t, "Apple Card")
	if res := ingest(t, h, pushWithPayee(payee, "evt-m1")); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("ingest: %s", res.Status)
	}
	links, err := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if err != nil || len(links) != 1 {
		t.Fatalf("links = %v, %v", links, err)
	}
	return links[0]
}

func suggestMatchText(h *harness, user string, l model.ImportTransactionLink) (*model.SuggestImportMatchTextResult, error) {
	return h.svc.SuggestMatchText(context.Background(), vo.MustParseId(user), model.SuggestImportMatchTextRequest{LinkId: l.ID.String()})
}

func TestSuggestMatchTextValid(t *testing.T) {
	h, c, _ := suggestSetup(t)
	l := matchTextLink(t, h, "SQ *BLUE BOTTLE COFFEE #0423")
	c.calls, c.reply = nil, `{"matchText":"BLUE BOTTLE COFFEE"}`
	res, err := suggestMatchText(h, userA, l)
	if err != nil {
		t.Fatal(err)
	}
	if res.MatchField != model.ImportRuleMatchFieldExternalPayee || res.MatchText != "BLUE BOTTLE COFFEE" {
		t.Fatalf("result = %+v", res)
	}
	if len(c.calls) != 1 || c.calls[0].userID != userA || !strings.Contains(c.calls[0].user, "SQ *BLUE BOTTLE COFFEE #0423") {
		t.Fatalf("calls = %+v", c.calls)
	}
}

func TestSuggestMatchTextRejectsInvented(t *testing.T) {
	h, c, _ := suggestSetup(t)
	l := matchTextLink(t, h, "SQ *BLUE BOTTLE COFFEE #0423")
	c.reply = `{"matchText":"STARBUCKS"}`
	res, err := suggestMatchText(h, userA, l)
	if err != nil || res.MatchField != "" || res.MatchText != "" {
		t.Fatalf("result = %+v, %v", res, err)
	}
}

func TestSuggestMatchTextNoAI(t *testing.T) {
	h := setup(t)
	h.withLimiter()
	l := matchTextLink(t, h, "Blue Bottle")
	before := *h.lim
	res, err := suggestMatchText(h, userA, l)
	if err != nil || res.MatchField != "" || res.MatchText != "" {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if *h.lim != before {
		t.Fatalf("limiter touched: %+v", h.lim)
	}
}

func TestSuggestMatchTextOptedOut(t *testing.T) {
	h, c, _ := suggestSetup(t)
	l := matchTextLink(t, h, "Blue Bottle")
	h.withLimiter()
	h.wireSuggest(c, fakeFacts{userA: {language: "en", aiOff: true}})
	c.calls, c.reply = nil, `{"matchText":"Blue Bottle"}`
	before := *h.lim
	res, err := suggestMatchText(h, userA, l)
	if err != nil || res.MatchField != "" || res.MatchText != "" {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if len(c.calls) != 0 || *h.lim != before {
		t.Fatalf("calls = %+v, limiter = %+v", c.calls, h.lim)
	}
}

// Opting out hides the open suggestions and drops them, so turning AI back on
// later brings only suggestions for new imports.
func TestSuggestionListEmptyAndClearedWhenOptedOut(t *testing.T) {
	h, c, facts := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	seedSuggestion(t, h, c, cat, "")
	facts[userA] = ownerFact{language: "en", aiOff: true}
	if got := suggestionList(t, h, userA); len(got) != 0 {
		t.Fatalf("list = %+v", got)
	}
	if got := openSuggestions(t, h, userA); len(got) != 0 {
		t.Fatalf("open suggestions kept: %+v", got)
	}
	facts[userA] = ownerFact{language: "en"}
	if got := suggestionList(t, h, userA); len(got) != 0 {
		t.Fatalf("list after opting back in = %+v", got)
	}
}

func TestClearSuggestionsDropsTheUsersOpenSuggestions(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	seedSuggestion(t, h, c, cat, "")
	if err := h.svc.ClearSuggestions(context.Background(), vo.MustParseId(userB)); err != nil {
		t.Fatal(err)
	}
	if got := openSuggestions(t, h, userA); len(got) != 1 {
		t.Fatalf("another user's clear touched these: %+v", got)
	}
	if err := h.svc.ClearSuggestions(context.Background(), vo.MustParseId(userA)); err != nil {
		t.Fatal(err)
	}
	if got := openSuggestions(t, h, userA); len(got) != 0 {
		t.Fatalf("open suggestions kept: %+v", got)
	}
}

func TestSuggestMatchTextCountsLimiter(t *testing.T) {
	h, c, _ := suggestSetup(t)
	l := matchTextLink(t, h, "Blue Bottle")
	h.withLimiter()
	h.wireSuggest(c, fakeFacts{userA: {language: "en"}})
	c.reply = `{"matchText":"Blue Bottle"}`
	if _, err := suggestMatchText(h, userA, l); err != nil {
		t.Fatal(err)
	}
	if h.lim.allow != 1 || h.lim.fail != 1 {
		t.Fatalf("limiter = %+v", h.lim)
	}
}

func TestSuggestMatchTextForeignLink(t *testing.T) {
	h, _, _ := suggestSetup(t)
	l := matchTextLink(t, h, "Blue Bottle")
	_, err := suggestMatchText(h, userB, l)
	wantNotFound(t, err)
	_, err = h.svc.SuggestMatchText(context.Background(), vo.MustParseId(userA), model.SuggestImportMatchTextRequest{LinkId: "0d000000-0000-0000-0000-0000000000ff"})
	wantNotFound(t, err)
}

func TestSuggestMatchTextCompleterError(t *testing.T) {
	h, c, _ := suggestSetup(t)
	l := matchTextLink(t, h, "Blue Bottle")
	c.err = errors.New("provider down")
	res, err := suggestMatchText(h, userA, l)
	if err != nil || res.MatchField != "" || res.MatchText != "" {
		t.Fatalf("result = %+v, %v", res, err)
	}
}

func TestSuggestionListKeepsOnlyRowsOfStoredType(t *testing.T) {
	h, c, _ := suggestSetup(t)
	py := h.payee(t, userA, "Blue Bottle Coffee")
	_, links := seedSuggestion(t, h, c, "", py, "Blue Bottle", "SQ *BLUE BOTTLE #12")
	h.setTx(t, links[0].TransactionID, "type", "1")
	items := suggestionList(t, h, userA)
	if len(items) != 1 || items[0].Type != "expense" || len(items[0].Transactions) != 1 || items[0].Transactions[0].TransactionId != links[1].TransactionID.String() {
		t.Fatalf("list = %+v", items)
	}
	h.setTx(t, links[1].TransactionID, "type", "1")
	if items := suggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("an expense suggestion must not offer income rows: %+v", items)
	}
}

func TestAcceptSkipsRowsOfAnotherType(t *testing.T) {
	h, c, _ := suggestSetup(t)
	py := h.payee(t, userA, "Blue Bottle Coffee")
	s, links := seedSuggestion(t, h, c, "", py)
	h.setTx(t, links[0].TransactionID, "type", "1")
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", PayeeID: py})
	res, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 0 {
		t.Fatalf("updated = %d", res.Updated)
	}
	if tx, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID); tx.PayeeID != nil {
		t.Fatalf("an income row took an expense suggestion's payee: %+v", tx)
	}
}

// joinAfterRead runs hook once, right after the suggestion read named by
// on ("get" or "list") returned: a job joining a row in between.
type joinAfterRead struct {
	imports.Repository
	on   string
	hook func(ctx context.Context)
}

func (r *joinAfterRead) fire(ctx context.Context, on string) {
	if r.on == on && r.hook != nil {
		h := r.hook
		r.hook = nil
		h(ctx)
	}
}

func (r *joinAfterRead) GetSuggestion(ctx context.Context, id vo.Id) (*model.ImportSuggestion, error) {
	s, err := r.Repository.GetSuggestion(ctx, id)
	r.fire(ctx, "get")
	return s, err
}

func (r *joinAfterRead) ListOpenSuggestionsByUser(ctx context.Context, userID vo.Id) ([]model.ImportSuggestion, error) {
	list, err := r.Repository.ListOpenSuggestionsByUser(ctx, userID)
	r.fire(ctx, "list")
	return list, err
}

// lateRow seeds a suggestion over one Blue Bottle tap and imports a second
// tap with the job off, so its link is not on the suggestion yet.
func lateRow(t *testing.T, h *harness, c *fakeCompleter, cat string) (model.ImportSuggestion, model.ImportTransactionLink, model.ImportTransactionLink) {
	t.Helper()
	s, links := seedSuggestion(t, h, c, cat, "")
	h.svc.SetJobRunner(func(func()) {})
	ingest(t, h, pushWithPayee("Blue Bottle", "evt-late"))
	all, err := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if err != nil || len(all) != 2 {
		t.Fatalf("links = %v, %v", all, err)
	}
	late := all[0]
	if late.ID == links[0].ID {
		late = all[1]
	}
	return s, links[0], late
}

func TestSuggestionListCleanupKeepsRowsJoinedMeanwhile(t *testing.T) {
	h, c, facts := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, first, late := lateRow(t, h, c, cat)
	h.setTx(t, first.TransactionID, "category_id", cat)
	rp := &joinAfterRead{Repository: h.repo, on: "list", hook: func(ctx context.Context) {
		if ok, err := h.repo.AddSuggestionLinks(ctx, s.ID, []vo.Id{late.ID}, now); err != nil || !ok {
			t.Fatalf("join: %v, %v", ok, err)
		}
	}}
	h.withRepo(rp)
	h.wireSuggest(c, facts)
	if items := suggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("the read saw nothing pending: %+v", items)
	}
	items := suggestionList(t, h, userA)
	if len(items) != 1 || items[0].Id != s.ID.String() || len(items[0].Transactions) != 1 || items[0].Transactions[0].TransactionId != late.TransactionID.String() {
		t.Fatalf("the joined row must survive the cleanup: %+v", items)
	}
}

func TestAcceptKeepsRowsJoinedMeanwhile(t *testing.T) {
	h, c, facts := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, first, late := lateRow(t, h, c, cat)
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: cat})
	rp := &joinAfterRead{Repository: h.repo, on: "get", hook: func(ctx context.Context) {
		if ok, err := h.repo.AddSuggestionLinks(ctx, s.ID, []vo.Id{late.ID}, now); err != nil || !ok {
			t.Fatalf("join: %v, %v", ok, err)
		}
	}}
	h.withRepo(rp)
	h.wireSuggest(c, facts)
	res, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
	if err != nil || res.Updated != 1 {
		t.Fatalf("accept = %+v, %v", res, err)
	}
	if tx, _ := h.txns.GetByID(context.Background(), *first.TransactionID); tx.CategoryID == nil {
		t.Fatalf("the accepted row was not filled: %+v", tx)
	}
	got, err := h.repo.GetSuggestion(context.Background(), s.ID)
	if err != nil || len(got.LinkIDs) != 1 || got.LinkIDs[0] != late.ID {
		t.Fatalf("the suggestion stays open for the row joined meanwhile: %+v, %v", got, err)
	}
}

func TestDismissKeepsRowsJoinedMeanwhile(t *testing.T) {
	h, c, facts := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, _, late := lateRow(t, h, c, cat)
	rp := &joinAfterRead{Repository: h.repo, on: "get", hook: func(ctx context.Context) {
		if ok, err := h.repo.AddSuggestionLinks(ctx, s.ID, []vo.Id{late.ID}, now); err != nil || !ok {
			t.Fatalf("join: %v, %v", ok, err)
		}
	}}
	h.withRepo(rp)
	h.wireSuggest(c, facts)
	if _, err := h.svc.DismissSuggestion(context.Background(), vo.MustParseId(userA), model.DismissImportSuggestionRequest{Id: s.ID.String()}); err != nil {
		t.Fatal(err)
	}
	got, err := h.repo.GetSuggestion(context.Background(), s.ID)
	if err != nil || len(got.LinkIDs) != 1 || got.LinkIDs[0] != late.ID {
		t.Fatalf("the suggestion stays open for the row joined meanwhile: %+v, %v", got, err)
	}
}
