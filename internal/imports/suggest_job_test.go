package imports_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/infra/ai"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

type completerCall struct {
	userID, system, user string
	schema               string
	deadline             time.Time
}

type fakeCompleter struct {
	calls []completerCall
	reply string
	// replies, when set, answers the n-th call with replies[n] (reply past its end)
	replies []string
	err     error
	panic   bool
	onCall  func()
}

func (c *fakeCompleter) Complete(ctx context.Context, userID, system, user string, schema *ai.Schema) (string, error) {
	deadline, _ := ctx.Deadline()
	call := completerCall{userID: userID, system: system, user: user, deadline: deadline}
	if schema != nil {
		call.schema = schema.Name
	}
	c.calls = append(c.calls, call)
	if c.onCall != nil {
		c.onCall()
	}
	if c.panic {
		panic("boom")
	}
	if n := len(c.calls) - 1; n < len(c.replies) {
		return c.replies[n], c.err
	}
	return c.reply, c.err
}

type ownerFact struct {
	language, timezone string
	readonly, aiOff    bool
}

type fakeFacts map[string]ownerFact

func (f fakeFacts) OwnerFacts(_ context.Context, userID vo.Id) (imports.OwnerInfo, error) {
	o := f[userID.String()]
	return imports.OwnerInfo{Language: o.language, Timezone: o.timezone, Readonly: o.readonly, AIOff: o.aiOff}, nil
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
	c := &fakeCompleter{reply: `{"rows":[]}`}
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
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`
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
	c := &fakeCompleter{reply: `{"rows":[]}`}
	h.wireSuggest(c, fakeFacts{userA: {language: "en"}})
	h.category(t, userA, "Shops")
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
	philz := h.payee(t, userA, "Philz Coffee")
	c.reply = `{"rows":[{"key":"0","matchText":"Philz","merchant":"Philz","categoryId":"` + other + `","payeeId":"` + philz + `"}]}`
	ingest(t, h, pushWithPayee("Philz", "evt-2"))
	if len(c.calls) != 2 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].CategoryID != nil || got[0].PayeeID == nil || got[0].PayeeID.String() != philz {
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
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + kaffee + `"}]}`
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

func TestSuggestSkipsOwnerWhoOptedOut(t *testing.T) {
	h, c, facts := suggestSetup(t)
	facts[userA] = ownerFact{language: "en", aiOff: true}
	h.mapCard(t, "Apple Card")
	h.category(t, userA, "Coffee")
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 0 || len(openSuggestions(t, h, userA)) != 0 {
		t.Fatalf("opted-out owner: calls = %+v", c.calls)
	}
}

// The owner's choice decides, whoever tapped: a partner who left AI on still
// sends nothing about the owner's account.
func TestSuggestSharedAccountOwnerOptedOut(t *testing.T) {
	h, c, facts := suggestSetup(t)
	facts[userB] = ownerFact{language: "de", aiOff: true}
	h.accounts.shared = true
	if _, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acctB}); err != nil {
		t.Fatal(err)
	}
	h.category(t, userB, "Kaffee")
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 0 || len(openSuggestions(t, h, userA)) != 0 || len(openSuggestions(t, h, userB)) != 0 {
		t.Fatalf("calls = %+v", c.calls)
	}
}

// optOutAfter answers "on" for the first n reads, then "opted out".
type optOutAfter struct {
	n, reads int
}

func (f *optOutAfter) OwnerFacts(context.Context, vo.Id) (imports.OwnerInfo, error) {
	f.reads++
	return imports.OwnerInfo{Language: "en", AIOff: f.reads > f.n}, nil
}

// The job's first look found AI on; the user opted out before the call.
func TestSuggestOptOutBeforeTheCallSendsNothing(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.wireSuggest(c, &optOutAfter{n: 1})
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 0 || len(openSuggestions(t, h, userA)) != 0 {
		t.Fatalf("calls = %+v", c.calls)
	}
}

// The user opted out while the provider was answering: nothing is stored.
func TestSuggestOptOutDuringTheCallStoresNothing(t *testing.T) {
	h, c, _ := suggestSetup(t)
	facts := &optOutAfter{n: 1 << 30}
	h.wireSuggest(c, facts)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`
	c.onCall = func() { facts.n = facts.reads }
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 1 || len(openSuggestions(t, h, userA)) != 0 {
		t.Fatalf("calls = %d, suggestions = %+v", len(c.calls), openSuggestions(t, h, userA))
	}
}

// The opt-out lands while the write commits (its in-transaction read still saw
// "on"): the post-commit check removes what the job stored.
func TestSuggestOptOutDuringTheWriteIsUndone(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.wireSuggest(c, &optOutAfter{n: 3})
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 1 || len(openSuggestions(t, h, userA)) != 0 {
		t.Fatalf("calls = %d, suggestions = %+v", len(c.calls), openSuggestions(t, h, userA))
	}
}

func TestSuggestRespectsCap(t *testing.T) {
	h, c, facts := suggestSetup(t)
	h.svc = imports.NewService(h.repo, h.accounts, h.conv, h.txns, h.txns, h.entities, denyScope(imports.RateScopeSuggest), h.db.TX, clock{now}, imports.DefaultMatcherConfig())
	registerParsers(h.svc)
	h.wireSuggest(c, facts)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 0 || len(openSuggestions(t, h, userA)) != 0 {
		t.Fatalf("over the cap: calls = %+v", c.calls)
	}
}

type brokenLimiter struct{}

func (brokenLimiter) Allow(_ context.Context, scope, _ string) error {
	if scope == imports.RateScopeSuggest {
		return errors.New("limiter down")
	}
	return nil
}
func (brokenLimiter) Reserve(string, string) error { return nil }

// A limiter that fails is not a rate limit: it is a WARN with the error type.
func TestSuggestLimiterFailureWarns(t *testing.T) {
	h, c, facts := suggestSetup(t)
	h.svc = imports.NewService(h.repo, h.accounts, h.conv, h.txns, h.txns, h.entities, brokenLimiter{}, h.db.TX, clock{now}, imports.DefaultMatcherConfig())
	registerParsers(h.svc)
	h.wireSuggest(c, facts)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	out := buf.String()
	if len(c.calls) != 0 || !strings.Contains(out, "level=WARN") || !strings.Contains(out, "import suggest failed") || !strings.Contains(out, "err_type=") || strings.Contains(out, "rate_limited") {
		t.Fatalf("calls = %d, log = %s", len(c.calls), out)
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

func (c *panicFor) Complete(ctx context.Context, userID, system, user string, schema *ai.Schema) (string, error) {
	if userID == c.owner {
		c.calls = append(c.calls, completerCall{userID: userID})
		panic("boom")
	}
	return c.fakeCompleter.Complete(ctx, userID, system, user, schema)
}

func TestSuggestPanicForOneOwnerStillServesOthers(t *testing.T) {
	h, p := bankHarness(t)
	h.accounts.shared = true
	kaffee := h.category(t, userB, "Kaffee")
	h.category(t, userA, "Coffee")
	c := &panicFor{owner: userA, fakeCompleter: fakeCompleter{reply: `{"rows":[{"key":"0","matchText":"Shop B","categoryId":"` + kaffee + `"}]}`}}
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
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","merchant":"Blue Bottle","payeeId":"` + py + `"}]}`
	ingest(t, h, tap)
	if len(c.calls) != 2 {
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
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","merchant":"Blue Bottle","payeeId":"` + py + `"}]}`
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 2 {
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
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","merchant":"Blue Bottle","payeeId":"` + py + `"}]}`
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 2 {
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

func (c *gatedCompleter) Complete(context.Context, string, string, string, *ai.Schema) (string, error) {
	c.mu.Lock()
	c.calls++
	first := c.calls == 1
	c.mu.Unlock()
	if !first {
		return c.reply, nil
	}
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
	c := &gatedCompleter{reply: `{"rows":[{"key":"0","matchText":"Blue Bottle","merchant":"Blue Bottle","payeeId":"` + py + `"}]}`, started: make(chan struct{}, 2)}
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
	if c.calls != 2 {
		t.Fatalf("calls = %d, want 2 (both steps once): the second job reuses the first one's suggestion", c.calls)
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
			c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cafe + `"}]}`
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
			c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`
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
		return `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`, nil
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

func (f completerFunc) Complete(ctx context.Context, _, _, _ string, _ *ai.Schema) (string, error) {
	return f(ctx)
}

func (h *harness) incomeCategory(t *testing.T, owner, name string) string {
	t.Helper()
	id := h.f.Category(fixture.Category{UserID: owner, Name: name, Type: 1})
	o := vo.MustParseId(owner)
	h.entities.categories[o] = append(h.entities.categories[o], model.ImportNamed{ID: id, Name: name, OwnerID: owner, Income: true})
	return id
}

// countingLimiter allows everything and counts each call per scope.
type countingLimiter struct {
	mu             sync.Mutex
	allow, reserve map[string]int
}

func (l *countingLimiter) Allow(_ context.Context, scope, _ string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.allow[scope]++
	return nil
}

func (l *countingLimiter) Reserve(scope, _ string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reserve[scope]++
	return nil
}

func withCountingLimiter(t *testing.T, h *harness, c imports.Completer, facts imports.OwnerFacts) *countingLimiter {
	t.Helper()
	l := &countingLimiter{allow: map[string]int{}, reserve: map[string]int{}}
	h.svc = imports.NewService(h.repo, h.accounts, h.conv, h.txns, h.txns, h.entities, l, h.db.TX, clock{now}, imports.DefaultMatcherConfig())
	registerParsers(h.svc)
	h.wireSuggest(c, facts)
	return l
}

func TestSuggestMixedBatchAsksOncePerTypeWithThatTypesCategories(t *testing.T) {
	h, p := bankHarness(t)
	c := &fakeCompleter{reply: `{"rows":[]}`}
	l := withCountingLimiter(t, h, c, fakeFacts{userA: {language: "en"}})
	h.svc.RegisterProvider(model.ImportProviderSimpleFIN, p)
	h.category(t, userA, "Groceries")
	h.incomeCategory(t, userA, "Salary")
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.txs = []model.ExternalTransaction{
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "Corner Shop"),
		extTx("ACT-1", "b", "900.00", now.AddDate(0, 0, -2).Unix(), "Acme Payroll"),
	}
	if _, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq()); err != nil {
		t.Fatal(err)
	}
	if len(c.calls) != 2 {
		t.Fatalf("calls = %d, want one per type", len(c.calls))
	}
	exp, inc := c.calls[0].user, c.calls[1].user
	if !strings.Contains(exp, "Groceries") || strings.Contains(exp, "Salary") || !strings.Contains(exp, "Corner Shop") || strings.Contains(exp, "Acme Payroll") {
		t.Fatalf("expense prompt = %s", exp)
	}
	if !strings.Contains(inc, "Salary") || strings.Contains(inc, "Groceries") || !strings.Contains(inc, "Acme Payroll") || strings.Contains(inc, "Corner Shop") {
		t.Fatalf("income prompt = %s", inc)
	}
	if l.allow[imports.RateScopeSuggest] != 1 || l.reserve[imports.RateScopeSuggest] != 1 {
		t.Fatalf("limiter allow = %d, reserve = %d, want one job", l.allow[imports.RateScopeSuggest], l.reserve[imports.RateScopeSuggest])
	}
}

func TestSuggestTwoStepPayee(t *testing.T) {
	h, c, facts := suggestSetup(t)
	l := withCountingLimiter(t, h, c, facts)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Cafes")
	bb := h.payee(t, userA, "Blue Bottle Coffee")
	h.payee(t, userA, "Starbucks")
	c.replies = []string{
		`{"rows":[{"key":"0","matchText":"Blue Bottle","merchant":"Blue Bottle","categoryId":"` + cat + `"}]}`,
		`{"rows":[{"key":"0","payeeId":"` + bb + `"}]}`,
	}
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 2 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	first, second := c.calls[0], c.calls[1]
	if strings.Contains(first.user, "Blue Bottle Coffee") || strings.Contains(first.user, "Starbucks") || strings.Contains(first.user, `"payees"`) || !strings.Contains(first.user, "Cafes") {
		t.Fatalf("step 1 prompt = %s", first.user)
	}
	if !strings.Contains(second.user, bb) || !strings.Contains(second.user, "Blue Bottle Coffee") || strings.Contains(second.user, "Starbucks") || strings.Contains(second.user, "Cafes") {
		t.Fatalf("step 2 prompt = %s", second.user)
	}
	if first.system == second.system {
		t.Fatal("the two steps share a system prompt")
	}
	if first.schema != "import_suggestions" || second.schema != "import_payees" {
		t.Fatalf("schemas = %q, %q", first.schema, second.schema)
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].PayeeID == nil || got[0].PayeeID.String() != bb || got[0].CategoryID == nil || got[0].CategoryID.String() != cat {
		t.Fatalf("suggestions = %+v", got)
	}
	if l.allow[imports.RateScopeSuggest] != 1 || l.reserve[imports.RateScopeSuggest] != 1 {
		t.Fatalf("limiter allow = %d, reserve = %d, want one job", l.allow[imports.RateScopeSuggest], l.reserve[imports.RateScopeSuggest])
	}
}

func TestSuggestNoStepTwoWithoutCandidates(t *testing.T) {
	for _, tc := range []struct{ name, reply string }{
		{"merchant_like_no_payee", `{"rows":[{"key":"0","matchText":"Blue Bottle","merchant":"Blue Bottle","categoryId":"%s"}]}`},
		{"no_merchant", `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"%s"}]}`},
	} {
		reply := tc.reply
		t.Run(tc.name, func(t *testing.T) {
			h, c, _ := suggestSetup(t)
			h.mapCard(t, "Apple Card")
			cat := h.category(t, userA, "Cafes")
			h.payee(t, userA, "Starbucks")
			c.reply = strings.Replace(reply, "%s", cat, 1)
			ingest(t, h, tap)
			if len(c.calls) != 1 {
				t.Fatalf("calls = %d, want no step 2", len(c.calls))
			}
			if got := openSuggestions(t, h, userA); len(got) != 1 || got[0].PayeeID != nil {
				t.Fatalf("suggestions = %+v", got)
			}
		})
	}
}

func TestSuggestStepTwoRejectsPayeeOutsideTheRowsCandidates(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Cafes")
	h.payee(t, userA, "Blue Bottle Coffee")
	sb := h.payee(t, userA, "Starbucks")
	c.replies = []string{
		`{"rows":[{"key":"0","matchText":"Blue Bottle","merchant":"Blue Bottle","categoryId":"` + cat + `"}]}`,
		`{"rows":[{"key":"0","payeeId":"` + sb + `"}]}`,
	}
	ingest(t, h, tap)
	if len(c.calls) != 2 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	if got := openSuggestions(t, h, userA); len(got) != 1 || got[0].PayeeID != nil || got[0].CategoryID == nil {
		t.Fatalf("suggestions = %+v", got)
	}
}

// A failed payee step keeps what step 1 found.
func TestSuggestStepTwoFailureKeepsCategory(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Cafes")
	h.payee(t, userA, "Blue Bottle Coffee")
	c.replies = []string{`{"rows":[{"key":"0","matchText":"Blue Bottle","merchant":"Blue Bottle","categoryId":"` + cat + `"}]}`, `not json`}
	ingest(t, h, tap)
	if len(c.calls) != 2 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	if got := openSuggestions(t, h, userA); len(got) != 1 || got[0].PayeeID != nil || got[0].CategoryID == nil || got[0].CategoryID.String() != cat {
		t.Fatalf("suggestions = %+v", got)
	}
}

// Reads: the job's first look, the check before the first call, then the one
// before step 2, which finds the owner opted out.
func TestSuggestOptOutBetweenStepsSkipsStepTwo(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.wireSuggest(c, &optOutAfter{n: 2})
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Cafes")
	h.payee(t, userA, "Blue Bottle Coffee")
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","merchant":"Blue Bottle","categoryId":"` + cat + `"}]}`
	ingest(t, h, tap)
	if len(c.calls) != 1 || len(openSuggestions(t, h, userA)) != 0 {
		t.Fatalf("calls = %d, suggestions = %+v", len(c.calls), openSuggestions(t, h, userA))
	}
}

func TestSuggestFindsPayeeBeyondFiveHundred(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	uA := vo.MustParseId(userA)
	for i := 0; i < 600; i++ {
		h.entities.add(h.entities.payees, uA, vo.NewId().String(), fmt.Sprintf("Aardvark %03d", i))
	}
	zoo := h.payee(t, userA, "Zoo Market")
	c.replies = []string{
		`{"rows":[{"key":"0","matchText":"Zoo","merchant":"Zoo Market"}]}`,
		`{"rows":[{"key":"0","payeeId":"` + zoo + `"}]}`,
	}
	ingest(t, h, pushWithPayee("ZOO MARKET 12", "evt-zoo"))
	if len(c.calls) != 2 || !strings.Contains(c.calls[1].user, zoo) {
		t.Fatalf("calls = %+v", c.calls)
	}
	if got := openSuggestions(t, h, userA); len(got) != 1 || got[0].PayeeID == nil || got[0].PayeeID.String() != zoo {
		t.Fatalf("suggestions = %+v", got)
	}
}

// One type's failed request does not cost the other type its answers.
func TestSuggestOneTypesFailureKeepsTheOther(t *testing.T) {
	h, p := bankHarness(t)
	c := &fakeCompleter{reply: `{"rows":[]}`}
	h.wireSuggest(c, fakeFacts{userA: {language: "en"}})
	h.svc.RegisterProvider(model.ImportProviderSimpleFIN, p)
	h.category(t, userA, "Groceries")
	salary := h.incomeCategory(t, userA, "Salary")
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.txs = []model.ExternalTransaction{
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "Corner Shop"),
		extTx("ACT-1", "b", "900.00", now.AddDate(0, 0, -2).Unix(), "Acme Payroll"),
	}
	c.replies = []string{`not json`, `{"rows":[{"key":"1","matchText":"Acme Payroll","categoryId":"` + salary + `"}]}`}
	if _, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq()); err != nil {
		t.Fatal(err)
	}
	if len(c.calls) != 2 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].CategoryID == nil || got[0].CategoryID.String() != salary || len(got[0].LinkIDs) != 1 {
		t.Fatalf("suggestions = %+v", got)
	}
}

// A row the job cannot read is skipped; the batch's other rows still go out.
func TestSuggestUnreadableRowSkipsOnlyThatRow(t *testing.T) {
	h, p := bankHarness(t)
	c := &fakeCompleter{reply: `{"rows":[]}`}
	h.wireSuggest(c, fakeFacts{userA: {language: "en"}})
	h.svc.RegisterProvider(model.ImportProviderSimpleFIN, p)
	h.category(t, userA, "Groceries")
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.txs = []model.ExternalTransaction{
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "Corner Shop"),
		extTx("ACT-1", "b", "-7.00", now.AddDate(0, 0, -2).Unix(), "Broken Row"),
	}
	h.txns.getFailOn = "Broken Row"
	if _, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq()); err != nil {
		t.Fatal(err)
	}
	if len(c.calls) != 1 || !strings.Contains(c.calls[0].user, "Corner Shop") || strings.Contains(c.calls[0].user, "Broken Row") {
		t.Fatalf("calls = %+v", c.calls)
	}
}

// A category archived while the AI answered is not stored, and an open
// suggestion whose only field it was takes no new rows.
func TestSuggestCategoryArchivedDuringTheCallIsDropped(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	coffee := h.category(t, userA, "Coffee")
	coffeeID := vo.MustParseId(coffee)
	old := &model.ImportSuggestion{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "PHILZ",
		CategoryID: &coffeeID, Status: model.ImportSuggestionStatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := h.repo.InsertSuggestion(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	uA := vo.MustParseId(userA)
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + coffee + `"}]}`
	c.onCall = func() {
		for i := range h.entities.categories[uA] {
			if h.entities.categories[uA][i].ID == coffee {
				h.entities.categories[uA][i].Archived = true
			}
		}
	}
	ingest(t, h, tap)
	if len(c.calls) != 1 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	for _, s := range openSuggestions(t, h, userA) {
		if s.ID != old.ID || len(s.LinkIDs) != 0 {
			t.Fatalf("nothing may be stored with the archived category: %+v", s)
		}
	}
}

// The AI sees the bank's payee text, never the amount or the date; a row
// with no payee text is not sent at all.
func TestSuggestSendsNoAmountDateOrPayeelessRow(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	h.category(t, userA, "Coffee")
	if res := ingest(t, h, pushWithPayee("", "evt-blank")); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 0 {
		t.Fatalf("a row with no payee text was sent: %+v", c.calls)
	}
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 1 {
		t.Fatalf("calls = %d", len(c.calls))
	}
	for _, leak := range []string{"4.75", "2026-08-20", "amount", "date", "description", `"A"`} {
		if strings.Contains(c.calls[0].user, leak) {
			t.Fatalf("prompt carries %q: %s", leak, c.calls[0].user)
		}
	}
	if !strings.Contains(c.calls[0].user, "Blue Bottle") {
		t.Fatalf("prompt = %s", c.calls[0].user)
	}
}

// A row an open suggestion already covers joins it without an AI call; an
// opt-out seen inside that join's transaction joins nothing, and one that
// lands right after it is undone like the job's other writes.
func TestSuggestPreAIJoinRespectsOptOut(t *testing.T) {
	for name, n := range map[string]int{"inside the join": 1, "after the join": 2} {
		t.Run(name, func(t *testing.T) {
			h, c, _ := suggestSetup(t)
			cat := h.category(t, userA, "Coffee")
			s, _ := seedSuggestion(t, h, c, cat, "")
			h.wireSuggest(c, &optOutAfter{n: n})
			calls := len(c.calls)
			if res := ingest(t, h, pushWithPayee("Blue Bottle", "evt-join")); res.Status != model.ImportIngestStatusCreated {
				t.Fatalf("status = %s", res.Status)
			}
			if len(c.calls) != calls {
				t.Fatalf("calls = %d, want none", len(c.calls)-calls)
			}
			got, err := h.repo.GetSuggestion(context.Background(), s.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.LinkIDs) != 1 {
				t.Fatalf("links = %v, want the seeded one only", got.LinkIDs)
			}
		})
	}
}

// The scan that groups a run's rows by owner has a generous deadline of its
// own, so a large sync cannot use up an owner's AI time before that owner's
// job starts; each owner's job keeps its own 60 s.
func TestSuggestScanHasItsOwnDeadline(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	h.category(t, userA, "Coffee")
	h.txns.readDeadlines = nil
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(h.txns.readDeadlines) == 0 || len(c.calls) != 1 {
		t.Fatalf("reads = %d, calls = %d", len(h.txns.readDeadlines), len(c.calls))
	}
	scan := h.txns.readDeadlines[0]
	if left := time.Until(scan); left < 4*time.Minute {
		t.Fatalf("scan deadline in %s, want minutes", left)
	}
	if left := time.Until(c.calls[0].deadline); left <= 0 || left > imports.SuggestTimeoutForTest {
		t.Fatalf("owner job deadline in %s, want its own %s", left, imports.SuggestTimeoutForTest)
	}
	if !c.calls[0].deadline.Before(scan) {
		t.Fatal("the owner's job runs under the scan's deadline")
	}
}

// A SimpleFIN row with no payee carries its memo in the payee field: it is not
// sent to the AI.
func TestSuggestSkipsASimpleFINRowWhosePayeeIsTheMemo(t *testing.T) {
	h, p := bankHarness(t)
	c := &fakeCompleter{reply: `{"rows":[]}`}
	l := withCountingLimiter(t, h, c, fakeFacts{userA: {language: "en"}})
	h.svc.RegisterProvider(model.ImportProviderSimpleFIN, p)
	h.category(t, userA, "Groceries")
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	memo := extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "")
	memo.Description = "Transfer to Anna rent 4417"
	memo.Raw, _ = json.Marshal(map[string]any{"id": "a", "posted": memo.Posted, "amount": "-5.00", "description": memo.Description})
	p.txs = []model.ExternalTransaction{memo}
	if _, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq()); err != nil {
		t.Fatal(err)
	}
	if len(c.calls) != 0 || l.reserve[imports.RateScopeSuggest] != 0 {
		t.Fatalf("calls = %+v, reservations = %v", c.calls, l.reserve)
	}
}

// A row nothing could be suggested for (no payees to pick from, no category
// of its type) costs no AI call and no cap slot.
func TestSuggestSkipsRowsNothingCanFill(t *testing.T) {
	h, _, _ := suggestSetup(t)
	c := &fakeCompleter{reply: `{"rows":[]}`}
	l := withCountingLimiter(t, h, c, fakeFacts{userA: {language: "en"}})
	h.mapCard(t, "Apple Card")
	h.incomeCategory(t, userA, "Salary")
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 0 || l.reserve[imports.RateScopeSuggest] != 0 {
		t.Fatalf("an expense with no expense category and no payees: calls = %d, reservations = %v", len(c.calls), l.reserve)
	}
	h.payee(t, userA, "Blue Bottle")
	if res := ingest(t, h, pushWithPayee("Philz", "evt-2")); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 1 || l.reserve[imports.RateScopeSuggest] != 1 {
		t.Fatalf("with a payee to pick: calls = %d, reservations = %v", len(c.calls), l.reserve)
	}
}

// optOutAfterDeadlines is optOutAfter that records each read's deadline.
type optOutAfterDeadlines struct {
	n, reads  int
	deadlines []time.Time
}

func (f *optOutAfterDeadlines) OwnerFacts(ctx context.Context, _ vo.Id) (imports.OwnerInfo, error) {
	f.reads++
	d, _ := ctx.Deadline()
	f.deadlines = append(f.deadlines, d)
	return imports.OwnerInfo{Language: "en", AIOff: f.reads > f.n}, nil
}

// The undo after the write runs under a short context of its own, not the
// owner's 60 s, which the AI call may have nearly used up.
func TestSuggestUndoRunsUnderItsOwnContext(t *testing.T) {
	h, c, _ := suggestSetup(t)
	facts := &optOutAfterDeadlines{n: 3}
	h.wireSuggest(c, facts)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(openSuggestions(t, h, userA)) != 0 || facts.reads != 4 {
		t.Fatalf("reads = %d, suggestions = %+v", facts.reads, openSuggestions(t, h, userA))
	}
	owner, undo := facts.deadlines[0], facts.deadlines[3]
	if left := time.Until(undo); left <= 0 || left > 11*time.Second {
		t.Fatalf("undo deadline in %s, want a fresh 10 s", left)
	}
	if !undo.Before(owner) {
		t.Fatalf("undo runs under the owner's deadline (%s vs %s)", undo, owner)
	}
}

func TestSuggestProviderStatusIsLogged(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	h.category(t, userA, "Coffee")
	c.err = &ai.HTTPError{StatusCode: 401, Dialect: "openai"}
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)
	ingest(t, h, tap)
	out := buf.String()
	if !strings.Contains(out, "status=401") || !strings.Contains(out, "err_type=*ai.HTTPError") {
		t.Fatalf("log = %s", out)
	}
}
