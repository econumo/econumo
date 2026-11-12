package imports_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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

// panicFor panics for one owner and replies for every other.
type panicFor struct {
	fakeCompleter
	owner string
}

func (c *panicFor) Complete(ctx context.Context, userID, system, user string) (string, error) {
	if userID == c.owner {
		c.calls = append(c.calls, completerCall{userID: userID})
		panic("boom")
	}
	return c.fakeCompleter.Complete(ctx, userID, system, user)
}

func TestSuggestPanicForOneOwnerStillServesOthers(t *testing.T) {
	h, p := bankHarness(t)
	h.accounts.shared = true
	kaffee := h.category(t, userB, "Kaffee")
	h.category(t, userA, "Coffee")
	c := &panicFor{owner: userA, fakeCompleter: fakeCompleter{reply: `{"rows":{"0":{"matchText":"Shop B","categoryId":"` + kaffee + `"}}}`}}
	h.wireSuggest(c, fakeFacts{userA: {language: "en"}, userB: {language: "de"}})
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-2", AccountID: acctB})
	p.accounts = append(p.accounts, model.ExternalAccount{ID: "ACT-2", Name: "Shared", Currency: "USD", Balance: "1", OrgName: "Big Bank"})
	p.txs = []model.ExternalTransaction{
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "Shop A"),
		extTx("ACT-2", "b", "-6.00", now.AddDate(0, 0, -1).Unix(), "Shop B"),
	}
	if _, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq()); err != nil {
		t.Fatal(err)
	}
	if len(c.calls) != 2 || c.calls[0].userID != userA || c.calls[1].userID != userB {
		t.Fatalf("calls = %+v", c.calls)
	}
	if got := openSuggestions(t, h, userB); len(got) != 1 {
		t.Fatalf("owner B suggestions = %+v", got)
	}
}

func TestSuggestOpenSuggestionOfOtherTypeDoesNotCover(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	pyID := vo.MustParseId(py)
	income := &model.ImportSuggestion{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE",
		PayeeID: &pyID, Type: model.TransactionTypeIncome, Status: model.ImportSuggestionStatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := h.repo.InsertSuggestion(context.Background(), income); err != nil {
		t.Fatal(err)
	}
	c.reply = `{"rows":{"0":{"matchText":"Blue Bottle","payeeId":"` + py + `"}}}`
	ingest(t, h, tap)
	if len(c.calls) != 1 {
		t.Fatalf("an income suggestion must not cover an expense row: calls = %d", len(c.calls))
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 2 {
		t.Fatalf("suggestions = %+v", got)
	}
	for _, s := range got {
		if s.ID == income.ID && len(s.LinkIDs) != 0 {
			t.Fatalf("the income suggestion absorbed an expense row: %+v", s)
		}
		if s.ID != income.ID && (s.Type != model.TransactionTypeExpense || len(s.LinkIDs) != 1) {
			t.Fatalf("new suggestion = %+v", s)
		}
	}
}

// vanishingRepo deletes the suggestions a chosen ListOpenSuggestionsByUser
// call returns, right after reading them: a list read's cleanup landing
// between the job's read and its write. hideFirst makes the first read see
// nothing.
type vanishingRepo struct {
	imports.Repository
	deleteOn  int
	hideFirst bool
	calls     int
}

func (r *vanishingRepo) ListOpenSuggestionsByUser(ctx context.Context, userID vo.Id) ([]model.ImportSuggestion, error) {
	r.calls++
	if r.hideFirst && r.calls == 1 {
		return nil, nil
	}
	list, err := r.Repository.ListOpenSuggestionsByUser(ctx, userID)
	if err != nil || r.calls != r.deleteOn {
		return list, err
	}
	for _, s := range list {
		if err := r.Repository.DeleteSuggestion(ctx, s.ID); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func seedPayeeSuggestion(t *testing.T, h *harness, py string) vo.Id {
	t.Helper()
	pyID := vo.MustParseId(py)
	s := &model.ImportSuggestion{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE",
		PayeeID: &pyID, Status: model.ImportSuggestionStatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := h.repo.InsertSuggestion(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return s.ID
}

func TestSuggestCoveringSuggestionDeletedSendsRowsToAI(t *testing.T) {
	h, c, facts := suggestSetup(t)
	h.withRepo(&vanishingRepo{Repository: h.repo, deleteOn: 1})
	h.wireSuggest(c, facts)
	h.mapCard(t, "Apple Card")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	old := seedPayeeSuggestion(t, h, py)
	c.reply = `{"rows":{"0":{"matchText":"Blue Bottle","payeeId":"` + py + `"}}}`
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 1 {
		t.Fatalf("the row must go to the AI once its suggestion is gone: calls = %d", len(c.calls))
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].ID == old || len(got[0].LinkIDs) != 1 {
		t.Fatalf("suggestions = %+v", got)
	}
}

func TestSuggestJoinTargetDeletedOpensFreshSuggestion(t *testing.T) {
	h, c, facts := suggestSetup(t)
	h.withRepo(&vanishingRepo{Repository: h.repo, deleteOn: 2, hideFirst: true})
	h.wireSuggest(c, facts)
	h.mapCard(t, "Apple Card")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	old := seedPayeeSuggestion(t, h, py)
	c.reply = `{"rows":{"0":{"matchText":"Blue Bottle","payeeId":"` + py + `"}}}`
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 1 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].ID == old || len(got[0].LinkIDs) != 1 || got[0].PayeeID == nil || got[0].PayeeID.String() != py {
		t.Fatalf("suggestions = %+v", got)
	}
}

// gatedCompleter answers only once both jobs have started, so without the
// per-owner lock both would ask the AI before either wrote.
type gatedCompleter struct {
	mu      sync.Mutex
	calls   int
	reply   string
	started chan struct{}
}

func (c *gatedCompleter) Complete(context.Context, string, string, string) (string, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	for i := 0; i < 2; i++ {
		select {
		case <-c.started:
		case <-time.After(2 * time.Second):
		}
	}
	time.Sleep(100 * time.Millisecond)
	return c.reply, nil
}

func TestSuggestJobsForOneOwnerRunOneAtATime(t *testing.T) {
	h, _, facts := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	c := &gatedCompleter{reply: `{"rows":{"0":{"matchText":"Blue Bottle","payeeId":"` + py + `"}}}`, started: make(chan struct{}, 2)}
	h.svc.SetSuggester(c, facts)
	var queued []func()
	h.svc.SetJobRunner(func(run func()) { queued = append(queued, run) })
	ingest(t, h, tap)
	ingest(t, h, pushWithPayee("Blue Bottle", "evt-2"))
	if len(queued) != 2 {
		t.Fatalf("jobs = %d", len(queued))
	}
	var wg sync.WaitGroup
	for _, run := range queued {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.started <- struct{}{}
			run()
		}()
	}
	wg.Wait()
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || len(got[0].LinkIDs) != 2 {
		t.Fatalf("suggestions = %+v", got)
	}
	if c.calls != 1 {
		t.Fatalf("calls = %d, want 1: the second job reuses the first one's suggestion", c.calls)
	}
}

func TestSuggestIgnoresOpenSuggestionWithArchivedCategory(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hideFirst bool
	}{{"before the AI call", false}, {"after the AI call", true}} {
		t.Run(tc.name, func(t *testing.T) {
			h, c, facts := suggestSetup(t)
			if tc.hideFirst {
				h.withRepo(&vanishingRepo{Repository: h.repo, hideFirst: true})
				h.wireSuggest(c, facts)
			}
			h.mapCard(t, "Apple Card")
			coffee := h.category(t, userA, "Coffee")
			cafe := h.category(t, userA, "Cafe")
			coffeeID := vo.MustParseId(coffee)
			old := &model.ImportSuggestion{
				ID: vo.NewId(), UserID: vo.MustParseId(userA), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE",
				CategoryID: &coffeeID, Status: model.ImportSuggestionStatusOpen, CreatedAt: now, UpdatedAt: now,
			}
			if err := h.repo.InsertSuggestion(context.Background(), old); err != nil {
				t.Fatal(err)
			}
			uA := vo.MustParseId(userA)
			for i := range h.entities.categories[uA] {
				if h.entities.categories[uA][i].ID == coffee {
					h.entities.categories[uA][i].Archived = true
				}
			}
			c.reply = `{"rows":{"0":{"matchText":"Blue Bottle","categoryId":"` + cafe + `"}}}`
			ingest(t, h, tap)
			if len(c.calls) != 1 {
				t.Fatalf("calls = %d", len(c.calls))
			}
			for _, s := range openSuggestions(t, h, userA) {
				if s.ID == old.ID && len(s.LinkIDs) != 0 {
					t.Fatalf("the row joined a suggestion with an archived category: %+v", s)
				}
				if s.ID != old.ID && (s.CategoryID == nil || s.CategoryID.String() != cafe || len(s.LinkIDs) != 1) {
					t.Fatalf("new suggestion = %+v", s)
				}
			}
			if items := suggestionList(t, h, userA); len(items) != 1 || items[0].CategoryId != cafe {
				t.Fatalf("list = %+v", items)
			}
		})
	}
}

// The 7-day cut reads the import time as a wall clock in the owner's zone,
// as the posted time is.
func TestSuggestAgeUsesOwnersWallClock(t *testing.T) {
	for _, tc := range []struct {
		name, zone, occurredAt string
		want                   bool
	}{
		// now = 2026-08-20 12:00 UTC = 05:00 in Los Angeles; posted 6 d 23 h before
		{"inside the week in Los Angeles", "America/Los_Angeles", "2026-08-13T06:00:00-07:00", true},
		// now = 21:00 in Tokyo; posted 7 d 1 h before
		{"outside the week in Tokyo", "Asia/Tokyo", "2026-08-13T20:00:00+09:00", false},
		{"unknown zone reads as UTC", "Not/AZone", "2026-08-13T13:00:00Z", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, c, facts := suggestSetup(t)
			facts[userA] = ownerFact{language: "en", timezone: tc.zone}
			h.mapCard(t, "Apple Card")
			cat := h.category(t, userA, "Coffee")
			c.reply = `{"rows":{"0":{"matchText":"Blue Bottle","categoryId":"` + cat + `"}}}`
			ingest(t, h, strings.Replace(tap, `"occurredAt":"2026-08-20T10:42:03-07:00"`, `"occurredAt":"`+tc.occurredAt+`"`, 1))
			if got := len(c.calls) == 1; got != tc.want {
				t.Fatalf("asked the AI = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSuggestTimeoutStartsOnceTheOwnerLockIsHeld(t *testing.T) {
	h, _, facts := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	var deadline time.Time
	c := completerFunc(func(ctx context.Context) (string, error) {
		deadline, _ = ctx.Deadline()
		return `{"rows":{"0":{"matchText":"Blue Bottle","categoryId":"` + cat + `"}}}`, nil
	})
	h.svc.SetSuggester(c, facts)
	done := make(chan struct{})
	h.svc.SetJobRunner(func(run func()) { go func() { defer close(done); run() }() })
	unlock := h.svc.LockSuggestOwnerForTest(vo.MustParseId(userA))
	ingest(t, h, tap)
	time.Sleep(500 * time.Millisecond)
	released := time.Now()
	unlock()
	<-done
	if deadline.IsZero() || deadline.Before(released.Add(imports.SuggestTimeoutForTest-100*time.Millisecond)) {
		t.Fatalf("deadline %v: the timeout ran while the job waited for the lock (released %v)", deadline, released)
	}
}

type completerFunc func(ctx context.Context) (string, error)

func (f completerFunc) Complete(ctx context.Context, _, _, _ string) (string, error) { return f(ctx) }
