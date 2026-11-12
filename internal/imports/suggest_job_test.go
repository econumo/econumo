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

type completerCall struct{ userID, system, user string }

type fakeCompleter struct {
	calls []completerCall
	reply string
	err   error
	panic bool
}

func (c *fakeCompleter) Complete(_ context.Context, userID, system, user string) (string, error) {
	c.calls = append(c.calls, completerCall{userID: userID, system: system, user: user})
	if c.panic {
		panic("boom")
	}
	return c.reply, c.err
}

type ownerFact struct {
	language, timezone string
	readonly           bool
}

type fakeFacts map[string]ownerFact

func (f fakeFacts) OwnerFacts(_ context.Context, userID vo.Id) (string, string, bool, error) {
	o := f[userID.String()]
	return o.language, o.timezone, o.readonly, nil
}

// denyScope refuses one rate-limit scope and lets every other through.
type denyScope string

func (d denyScope) Allow(_ context.Context, scope, _ string) error {
	if scope == string(d) {
		return errs.NewTooManyRequests("Too many attempts. Try again later.")
	}
	return nil
}
func (d denyScope) Reserve(string, string) error { return nil }

func (h *harness) wireSuggest(c imports.Completer, facts imports.OwnerFacts) {
	h.svc.SetSuggester(c, facts)
	h.svc.SetJobRunner(func(run func()) { run() })
}

func suggestSetup(t *testing.T) (*harness, *fakeCompleter, fakeFacts) {
	t.Helper()
	h := setup(t)
	c := &fakeCompleter{reply: `{"rows":{}}`}
	facts := fakeFacts{userA: {language: "en", timezone: "America/Los_Angeles"}, userB: {language: "de", timezone: "Europe/Berlin"}}
	h.wireSuggest(c, facts)
	return h, c, facts
}

func (h *harness) category(t *testing.T, owner, name string) string {
	t.Helper()
	id := h.f.Category(fixture.Category{UserID: owner, Name: name})
	h.entities.add(h.entities.categories, vo.MustParseId(owner), id, name)
	return id
}

func (h *harness) payee(t *testing.T, owner, name string) string {
	t.Helper()
	id := h.f.Payee(fixture.Payee{UserID: owner, Name: name})
	h.entities.add(h.entities.payees, vo.MustParseId(owner), id, name)
	return id
}

func openSuggestions(t *testing.T, h *harness, owner string) []model.ImportSuggestion {
	t.Helper()
	list, err := h.repo.ListOpenSuggestionsByUser(context.Background(), vo.MustParseId(owner))
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func pushWithPayee(payee, eventID string) string {
	body := strings.Replace(tap, `"payee":"Blue Bottle"`, `"payee":"`+payee+`"`, 1)
	return strings.Replace(body, `"eventId":"evt-1"`, `"eventId":"`+eventID+`"`, 1)
}

func TestSuggestCreatesSuggestionForLivePush(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	c.reply = `{"rows":{"0":{"matchText":"Blue Bottle","categoryId":"` + cat + `"}}}`
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 1 || c.calls[0].userID != userA {
		t.Fatalf("calls = %+v", c.calls)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].CategoryID == nil || got[0].CategoryID.String() != cat || !strings.EqualFold(got[0].MatchText, "blue bottle") {
		t.Fatalf("suggestions = %+v", got)
	}
	if len(got[0].LinkIDs) != 1 || got[0].LinkIDs[0] != links[0].ID {
		t.Fatalf("links = %+v, want %s", got[0].LinkIDs, links[0].ID)
	}
	if got[0].CreatedAt.IsZero() || got[0].UserID.String() != userA {
		t.Fatalf("suggestion = %+v", got[0])
	}
}

func TestSuggestSkipsBackfill(t *testing.T) {
	h, p := bankHarness(t)
	c := &fakeCompleter{reply: `{"rows":{}}`}
	h.wireSuggest(c, fakeFacts{userA: {language: "en"}})
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.txs = []model.ExternalTransaction{
		extTx("ACT-1", "old", "-5.00", now.AddDate(0, 0, -30).Unix(), "Old Shop"),
		extTx("ACT-1", "new", "-6.00", now.AddDate(0, 0, -2).Unix(), "New Shop"),
	}
	if _, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq()); err != nil {
		t.Fatal(err)
	}
	if len(c.calls) != 1 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	if u := c.calls[0].user; !strings.Contains(u, "New Shop") || strings.Contains(u, "Old Shop") {
		t.Fatalf("prompt = %s", u)
	}
}

func TestSuggestNotForReplayOrRetry(t *testing.T) {
	h, c, facts := suggestSetup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()

	// LinkAccount's replay of a queued tap
	ingest(t, h, tap)
	if _, err := h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1}); err != nil {
		t.Fatal(err)
	}

	// ImportQueuedEvent on a tap from an unmapped card
	ingest(t, h, strings.Replace(pushWithPayee("Cafe", "evt-2"), `"account":"Apple Card"`, `"account":"Other Card"`, 1))
	q, err := h.svc.GetQueue(ctx, uA)
	if err != nil || len(q.Queued) != 1 {
		t.Fatalf("queue = %+v, %v", q, err)
	}
	if _, err := h.svc.ImportQueuedEvent(ctx, uA, model.ImportQueuedEventRequest{LinkId: q.Queued[0].LinkId, Transaction: model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("4.75"), AccountId: acct1, Date: "2026-08-20 10:42:03",
	}}); err != nil {
		t.Fatal(err)
	}

	// RetryEvent of an event that failed only for want of a parser
	h.svc = imports.NewService(h.repo, h.accounts, h.conv, h.txns, h.txns, h.entities, nil, h.db.TX, clock{now}, imports.DefaultMatcherConfig())
	h.wireSuggest(c, facts)
	failed := ingest(t, h, pushWithPayee("Bakery", "evt-3"))
	if failed.Status != model.ImportIngestStatusFailed {
		t.Fatalf("status = %s", failed.Status)
	}
	registerParsers(h.svc)
	res, err := h.svc.RetryEvent(ctx, uA, model.RetryImportEventRequest{EventId: failed.EventId})
	if err != nil || res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("retry = %+v, %v", res, err)
	}

	if len(h.txns.created) != 3 {
		t.Fatalf("created = %d, want 3", len(h.txns.created))
	}
	if len(c.calls) != 0 {
		t.Fatalf("completer must not be called: %+v", c.calls)
	}
}

func TestSuggestSkipsRowsRulesClassified(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	other := h.category(t, userA, "Food")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: cat, PayeeID: py})
	ingest(t, h, tap)
	if len(c.calls) != 0 {
		t.Fatalf("a fully classified row must not reach the AI: %+v", c.calls)
	}

	h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "philz", CategoryID: cat})
	c.reply = `{"rows":{"0":{"matchText":"Philz","categoryId":"` + other + `","payeeId":"` + py + `"}}}`
	ingest(t, h, pushWithPayee("Philz", "evt-2"))
	if len(c.calls) != 1 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].CategoryID != nil || got[0].PayeeID == nil || got[0].PayeeID.String() != py {
		t.Fatalf("only the payee may be suggested: %+v", got)
	}
}

func TestSuggestSharedAccountGoesToOwner(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.accounts.shared = true
	if _, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acctB}); err != nil {
		t.Fatal(err)
	}
	h.category(t, userA, "Coffee")
	kaffee := h.category(t, userB, "Kaffee")
	c.reply = `{"rows":{"0":{"matchText":"Blue Bottle","categoryId":"` + kaffee + `"}}}`
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 1 || c.calls[0].userID != userB {
		t.Fatalf("calls = %+v", c.calls)
	}
	u := c.calls[0].user
	if !strings.Contains(u, "Kaffee") || strings.Contains(u, `"Coffee"`) || !strings.Contains(u, `"language":"de"`) || !strings.Contains(u, `"account":"Other"`) {
		t.Fatalf("prompt = %s", u)
	}
	if got := openSuggestions(t, h, userB); len(got) != 1 || got[0].UserID.String() != userB {
		t.Fatalf("owner suggestions = %+v", got)
	}
	if got := openSuggestions(t, h, userA); len(got) != 0 {
		t.Fatalf("importer suggestions = %+v", got)
	}
}

func TestSuggestReusesOpenSuggestionWithoutAI(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	catID := vo.MustParseId(cat)
	open := &model.ImportSuggestion{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE",
		CategoryID: &catID, Status: model.ImportSuggestionStatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := h.repo.InsertSuggestion(context.Background(), open); err != nil {
		t.Fatal(err)
	}
	ingest(t, h, pushWithPayee("SQ *BLUE BOTTLE #12", "evt-9"))
	if len(c.calls) != 0 {
		t.Fatalf("an open suggestion covers the row: %+v", c.calls)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || len(got[0].LinkIDs) != 1 || got[0].LinkIDs[0] != links[0].ID {
		t.Fatalf("suggestions = %+v", got)
	}
}

func TestSuggestSkipsReadonlyOwner(t *testing.T) {
	h, c, facts := suggestSetup(t)
	facts[userA] = ownerFact{language: "en", readonly: true}
	h.mapCard(t, "Apple Card")
	h.category(t, userA, "Coffee")
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 0 || len(openSuggestions(t, h, userA)) != 0 {
		t.Fatalf("readonly owner: calls = %+v", c.calls)
	}
}

func TestSuggestRespectsCap(t *testing.T) {
	h, c, facts := suggestSetup(t)
	h.svc = imports.NewService(h.repo, h.accounts, h.conv, h.txns, h.txns, h.entities, denyScope(imports.RateScopeSuggest), h.db.TX, clock{now}, imports.DefaultMatcherConfig())
	registerParsers(h.svc)
	h.wireSuggest(c, facts)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	c.reply = `{"rows":{"0":{"matchText":"Blue Bottle","categoryId":"` + cat + `"}}}`
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 0 || len(openSuggestions(t, h, userA)) != 0 {
		t.Fatalf("over the cap: calls = %+v", c.calls)
	}
}

func TestSuggestJobFailureLeavesImportIntact(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	h.category(t, userA, "Coffee")
	c.err = errors.New("provider down")
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	c.err, c.panic = nil, true
	if res := ingest(t, h, pushWithPayee("Philz", "evt-2")); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 2 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	for _, req := range h.txns.created {
		if _, err := h.txns.GetByID(context.Background(), vo.MustParseId(req.Id)); err != nil {
			t.Fatalf("transaction %s: %v", req.Id, err)
		}
	}
	if len(h.txns.created) != 2 || len(openSuggestions(t, h, userA)) != 0 {
		t.Fatalf("created = %d", len(h.txns.created))
	}
}

func TestSuggestNoCompleterDoesNothing(t *testing.T) {
	h := setup(t)
	h.svc.SetJobRunner(func(run func()) { run() })
	h.mapCard(t, "Apple Card")
	h.category(t, userA, "Coffee")
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(openSuggestions(t, h, userA)) != 0 {
		t.Fatal("no AI, no suggestions")
	}
}
