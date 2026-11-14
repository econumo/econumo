package onboarding_test

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/ai"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

type iconCategories struct {
	onboarding.CategorySteps
	rows []model.CategoryResult
}

func (f iconCategories) OwnCategories(context.Context, vo.Id) ([]model.CategoryResult, error) {
	return f.rows, nil
}

func (iconCategories) ReplaceIcon(context.Context, vo.Id, vo.Id, string, string) (bool, error) {
	return true, nil
}

type iconAccounts struct {
	onboarding.AccountSteps
	rows []model.OnboardingAccount
}

func (f iconAccounts) OwnLiveAccounts(context.Context, vo.Id) ([]model.OnboardingAccount, error) {
	return f.rows, nil
}

func (iconAccounts) ReplaceIcon(context.Context, vo.Id, vo.Id, string, string) (bool, error) {
	return true, nil
}

type countingLimiter struct {
	mu     sync.Mutex
	scopes []string
}

func (*countingLimiter) Allow(context.Context, string, string) error { return nil }
func (l *countingLimiter) Reserve(scope, _ string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.scopes = append(l.scopes, scope)
	return nil
}

type recordedCall struct{ system, user string }

type recordingAI struct {
	mu    sync.Mutex
	calls []recordedCall
}

func (r *recordingAI) Complete(_ context.Context, _, system, user string, _ *ai.Schema) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCall{system, user})
	return `{}`, nil
}

// fakeTimers holds scheduled callbacks until the test fires them.
type fakeTimers struct {
	mu      sync.Mutex
	pending []*fakeTimer
	delays  []time.Duration
}

type fakeTimer struct {
	f       func()
	stopped bool
}

func (ft *fakeTimers) schedule(d time.Duration, f func()) func() bool {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	t := &fakeTimer{f: f}
	ft.pending = append(ft.pending, t)
	ft.delays = append(ft.delays, d)
	return func() bool {
		ft.mu.Lock()
		defer ft.mu.Unlock()
		was := !t.stopped
		t.stopped = true
		return was
	}
}

func (ft *fakeTimers) fire() {
	ft.mu.Lock()
	run := ft.pending
	ft.pending = nil
	ft.mu.Unlock()
	for _, t := range run {
		if !t.stopped {
			t.f()
		}
	}
}

var uuidPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

const (
	cat1  = "019a0000-0000-7000-8000-0000000000c1"
	cat2  = "019a0000-0000-7000-8000-0000000000c2"
	cat3  = "019a0000-0000-7000-8000-0000000000c3"
	acct1 = "019a0000-0000-7000-8000-0000000000a1"
	acct2 = "019a0000-0000-7000-8000-0000000000a2"
)

func newIconSvc(aiOff bool) (*onboarding.Service, *recordingAI, *countingLimiter, *fakeTimers) {
	return newIconSvcWith(fakeTimezones{aiOff: aiOff})
}

func newIconSvcWith(users onboarding.UserFacts) (*onboarding.Service, *recordingAI, *countingLimiter, *fakeTimers) {
	ai, lim, timers := &recordingAI{}, &countingLimiter{}, &fakeTimers{}
	svc := onboarding.NewService(&fakeBudgets{}, fakeAccounts{}, users)
	svc.SetAdvisor(ai, lim)
	svc.SetRunner(func(job func()) { job() })
	svc.SetScheduler(timers.schedule)
	var cats []model.CategoryResult
	for i, id := range []string{cat1, cat2, cat3} {
		cats = append(cats, model.CategoryResult{Id: id, Name: []string{"Food", "Rent", "Fun"}[i], Type: "expense", Icon: model.ImportCategoryIcon})
	}
	svc.SetSteps(onboarding.Steps{
		Categories: iconCategories{rows: cats},
		Accounts: iconAccounts{rows: []model.OnboardingAccount{
			{ID: acct1, OwnerID: me, Name: "Visa", CurrencyCode: "EUR", Icon: model.ImportAccountIcon},
			{ID: acct2, OwnerID: me, Name: "Cash", CurrencyCode: "EUR", Icon: model.ImportAccountIcon},
		}},
	})
	return svc, ai, lim, timers
}

// askedNames lists the names a prompt asks about, sorted.
func askedNames(t *testing.T, raw, key string) []string {
	t.Helper()
	var p map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	var rows []struct{ Name string }
	if err := json.Unmarshal(p[key], &rows); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.Name)
	}
	sort.Strings(out)
	return out
}

// A large CSV arrives in chunks; the icons of everything the chunks created
// are asked for once, after the last chunk, and count once against the icon
// cap (never the savings job's).
func TestIconReview_ChunksOfOneImportAskOnce(t *testing.T) {
	svc, ai, lim, timers := newIconSvc(false)
	ctx := reqctx.WithLanguage(context.Background(), "de")
	user := vo.MustParseId(me)
	svc.ReviewImportedIcons(ctx, user, ids(cat1), ids(acct1))
	svc.ReviewImportedIcons(ctx, user, ids(cat2), nil)
	svc.ReviewImportedIcons(ctx, user, ids(cat3), ids(acct2))
	if len(ai.calls) != 0 || len(lim.scopes) != 0 {
		t.Fatalf("asked before the import went quiet: %d calls, %v", len(ai.calls), lim.scopes)
	}
	for _, d := range timers.delays {
		if d != 15*time.Second {
			t.Fatalf("quiet period = %s", d)
		}
	}
	timers.fire()
	if len(ai.calls) != 2 {
		t.Fatalf("calls = %d, want one categories and one accounts request", len(ai.calls))
	}
	if len(lim.scopes) != 1 || lim.scopes[0] != onboarding.RateScopeIcons || onboarding.RateScopeIcons == onboarding.RateScopeClassify {
		t.Fatalf("reservations = %v", lim.scopes)
	}
	for _, c := range ai.calls {
		switch {
		case strings.Contains(c.system, `{"categories":`):
			if got := askedNames(t, c.user, "categories"); strings.Join(got, ",") != "Food,Fun,Rent" {
				t.Fatalf("categories asked = %v", got)
			}
		case strings.Contains(c.system, `{"accounts":`):
			if got := askedNames(t, c.user, "accounts"); strings.Join(got, ",") != "Cash,Visa" {
				t.Fatalf("accounts asked = %v", got)
			}
		default:
			t.Fatalf("unexpected request: %s", c.system)
		}
		if !strings.Contains(c.user, `"language":"de"`) {
			t.Fatalf("prompt = %s", c.user)
		}
	}

	// the next import starts a fresh review
	svc.ReviewImportedIcons(ctx, user, ids(cat1), nil)
	timers.fire()
	if len(ai.calls) != 3 || len(lim.scopes) != 2 {
		t.Fatalf("calls = %d, reservations = %v", len(ai.calls), lim.scopes)
	}
}

func TestIconReview_OptedOutAtTheEndAsksNothing(t *testing.T) {
	svc, ai, lim, timers := newIconSvc(true)
	svc.ReviewImportedIcons(context.Background(), vo.MustParseId(me), ids(cat1), ids(acct1))
	timers.fire()
	if len(ai.calls) != 0 || len(lim.scopes) != 0 {
		t.Fatalf("calls = %d, reservations = %v", len(ai.calls), lim.scopes)
	}
}

// jsonKeyPaths lists every key path in a JSON document ("accounts[].name").
func jsonKeyPaths(t *testing.T, raw string) string {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				path := k
				if prefix != "" {
					path = prefix + "." + k
				}
				seen[path] = true
				walk(path, child)
			}
		case []any:
			for _, child := range x {
				walk(prefix+"[]", child)
			}
		}
	}
	walk("", doc)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// Every onboarding prompt carries only names, types, currency codes, the
// language and the timezone, plus the short per-request keys its answer
// refers to; no entity id (no UUID) reaches a prompt.
func TestPrompts_SendOnlyTheAllowlist(t *testing.T) {
	svc, ai, _, timers := newIconSvc(false)
	ctx := reqctx.WithLanguage(context.Background(), "de")
	svc.ReviewImportedIcons(ctx, vo.MustParseId(me), ids(cat1), ids(acct1))
	timers.fire()
	c := replying(`{"savings":[]}`)
	csvc, _ := newSvc(c, nil)
	schedule(ctx, csvc, cash, deposit)

	want := map[string]string{
		"categories": "categories,categories[].key,categories[].name,categories[].type,language,timezone",
		"accounts":   "accounts,accounts[].currency,accounts[].key,accounts[].name,language,timezone",
	}
	if len(ai.calls) != 2 {
		t.Fatalf("calls = %d", len(ai.calls))
	}
	for _, call := range ai.calls {
		key := "accounts"
		if strings.Contains(call.system, `{"categories":`) {
			key = "categories"
		}
		if got := jsonKeyPaths(t, call.user); got != want[key] {
			t.Fatalf("%s prompt keys = %s", key, got)
		}
	}
	if got := jsonKeyPaths(t, c.user); got != want["accounts"] {
		t.Fatalf("savings prompt keys = %s", got)
	}
	for _, call := range append(ai.calls, recordedCall{user: c.user}) {
		if uuidPattern.MatchString(call.user) {
			t.Fatalf("prompt carries a UUID: %s", call.user)
		}
	}
}

// A chunk that creates nothing still shows the import is going on: it keeps
// a pending review waiting, and starts none of its own.
func TestIconReview_EmptyChunksKeepTheReviewWaiting(t *testing.T) {
	svc, ai, lim, timers := newIconSvc(false)
	ctx := context.Background()
	user := vo.MustParseId(me)
	svc.ReviewImportedIcons(ctx, user, nil, nil)
	timers.fire()
	if len(ai.calls) != 0 || len(lim.scopes) != 0 || len(timers.delays) != 0 {
		t.Fatalf("an empty chunk with nothing pending scheduled a review: %d calls, %d timers", len(ai.calls), len(timers.delays))
	}
	svc.ReviewImportedIcons(ctx, user, ids(cat1), nil)
	for range 2 {
		svc.ReviewImportedIcons(ctx, user, nil, nil)
	}
	svc.ReviewImportedIcons(ctx, user, ids(cat2), ids(acct1))
	timers.fire()
	if len(ai.calls) != 2 || len(lim.scopes) != 1 || len(timers.delays) != 4 {
		t.Fatalf("calls = %d, reservations = %v, timers = %d", len(ai.calls), lim.scopes, len(timers.delays))
	}
	for _, c := range ai.calls {
		if strings.Contains(c.system, `{"categories":`) {
			if got := askedNames(t, c.user, "categories"); strings.Join(got, ",") != "Food,Rent" {
				t.Fatalf("categories asked = %v", got)
			}
		}
	}
}

// flakyOptOut fails the opt-out read the first fails times.
type flakyOptOut struct {
	mu    sync.Mutex
	fails int
}

func (*flakyOptOut) StoredTimezone(context.Context, vo.Id) (string, error) { return "UTC", nil }
func (f *flakyOptOut) AIOptedOut(context.Context, vo.Id) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fails > 0 {
		f.fails--
		return false, errors.New("db down")
	}
	return false, nil
}

// A failed preference read keeps the pending review and tries once more; a
// second failure drops it. A real opt-out drops it at once.
func TestIconReview_TransientFailureRetriesOnce(t *testing.T) {
	user := vo.MustParseId(me)
	t.Run("recovers", func(t *testing.T) {
		svc, ai, lim, timers := newIconSvcWith(&flakyOptOut{fails: 1})
		svc.ReviewImportedIcons(context.Background(), user, ids(cat1), ids(acct1))
		timers.fire()
		if len(ai.calls) != 0 || len(timers.pending) != 1 {
			t.Fatalf("calls = %d, rescheduled = %d", len(ai.calls), len(timers.pending))
		}
		timers.fire()
		if len(ai.calls) != 2 || len(lim.scopes) != 1 {
			t.Fatalf("calls = %d, reservations = %v", len(ai.calls), lim.scopes)
		}
	})
	t.Run("gives up", func(t *testing.T) {
		svc, ai, _, timers := newIconSvcWith(&flakyOptOut{fails: 5})
		svc.ReviewImportedIcons(context.Background(), user, ids(cat1), nil)
		timers.fire()
		timers.fire()
		if len(ai.calls) != 0 || len(timers.pending) != 0 {
			t.Fatalf("calls = %d, still scheduled = %d", len(ai.calls), len(timers.pending))
		}
	})
	t.Run("opted out", func(t *testing.T) {
		svc, _, _, timers := newIconSvc(true)
		svc.ReviewImportedIcons(context.Background(), user, ids(cat1), nil)
		timers.fire()
		if len(timers.pending) != 0 {
			t.Fatalf("an opt-out rescheduled the review")
		}
	})
}

// A request that runs longer than the quiet period holds the earlier chunk's
// pending review; a failed request releases it too.
func TestIconReview_HeldWhileALaterRequestIsInFlight(t *testing.T) {
	svc, ai, _, timers := newIconSvc(false)
	ctx := context.Background()
	user := vo.MustParseId(me)
	svc.ReviewImportedIcons(ctx, user, ids(cat1), ids(acct1))
	release := svc.ImportStarted(user)
	timers.fire() // the old quiet period runs out during the slow request
	if len(ai.calls) != 0 {
		t.Fatalf("review fired mid-import: %d calls", len(ai.calls))
	}
	svc.ReviewImportedIcons(ctx, user, ids(cat2), nil)
	timers.fire()
	if len(ai.calls) != 0 {
		t.Fatalf("review fired while a request was still in flight: %d calls", len(ai.calls))
	}
	release()
	timers.fire()
	if len(ai.calls) != 2 {
		t.Fatalf("calls = %d, want one categories and one accounts request", len(ai.calls))
	}

	// a failing request releases without a commit and the pending ids still run
	svc.ReviewImportedIcons(ctx, user, ids(cat1), nil)
	release = svc.ImportStarted(user)
	release()
	release()
	timers.fire()
	if len(ai.calls) != 3 {
		t.Fatalf("calls = %d after a failed request", len(ai.calls))
	}
}

type replacement struct{ id, from, to string }

type recordingReplacer struct {
	mu   sync.Mutex
	done []replacement
}

func (r *recordingReplacer) ReplaceIcon(_ context.Context, _, id vo.Id, from, to string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = append(r.done, replacement{id.String(), from, to})
	return true, nil
}

type replacingCategories struct {
	iconCategories
	*recordingReplacer
}

func (r replacingCategories) ReplaceIcon(ctx context.Context, u, id vo.Id, from, to string) (bool, error) {
	return r.recordingReplacer.ReplaceIcon(ctx, u, id, from, to)
}

type replacingAccounts struct {
	iconAccounts
	*recordingReplacer
}

func (r replacingAccounts) ReplaceIcon(ctx context.Context, u, id vo.Id, from, to string) (bool, error) {
	return r.recordingReplacer.ReplaceIcon(ctx, u, id, from, to)
}

type routedAI struct{ categories, accounts string }

func (r routedAI) Complete(_ context.Context, _, system, _ string, _ *ai.Schema) (string, error) {
	if strings.Contains(system, `{"categories":`) {
		return r.categories, nil
	}
	return r.accounts, nil
}

// The reply's keys map back to the ids they stood for; a key the request never
// issued is dropped and counted in one WARN per request.
func TestIconReview_KeysMapBackToIDs(t *testing.T) {
	svc, _, _, timers := newIconSvc(false)
	cats, accts := &recordingReplacer{}, &recordingReplacer{}
	var catRows []model.CategoryResult
	for i, id := range []string{cat1, cat2} {
		catRows = append(catRows, model.CategoryResult{Id: id, Name: []string{"Food", "Rent"}[i], Type: "expense", Icon: model.ImportCategoryIcon})
	}
	svc.SetSteps(onboarding.Steps{
		Categories: replacingCategories{iconCategories{rows: catRows}, cats},
		Accounts: replacingAccounts{iconAccounts{rows: []model.OnboardingAccount{
			{ID: acct1, OwnerID: me, Name: "Visa", CurrencyCode: "EUR", Icon: model.ImportAccountIcon},
			{ID: acct2, OwnerID: me, Name: "Cash", CurrencyCode: "EUR", Icon: model.ImportAccountIcon},
		}}, accts},
	})
	svc.SetAdvisor(routedAI{
		categories: `{"categories":[{"key":"C2","icon":"home"},{"key":"c9","icon":"pets"},{"key":"` + cat1 + `","icon":"cake"}]}`,
		accounts:   `{"accounts":[{"key":"a1","icon":"credit_card"},{"key":"a2","icon":"payments"}]}`,
	}, &countingLimiter{})
	logs := captureLogs(t)
	svc.ReviewImportedIcons(context.Background(), vo.MustParseId(me), ids(cat1, cat2), ids(acct1, acct2))
	timers.fire()
	if len(cats.done) != 1 || cats.done[0] != (replacement{cat2, model.ImportCategoryIcon, "home"}) {
		t.Fatalf("category icons = %+v", cats.done)
	}
	sort.Slice(accts.done, func(i, j int) bool { return accts.done[i].id < accts.done[j].id })
	if len(accts.done) != 2 || accts.done[0] != (replacement{acct1, model.ImportAccountIcon, "credit_card"}) || accts.done[1] != (replacement{acct2, model.ImportAccountIcon, "payments"}) {
		t.Fatalf("account icons = %+v", accts.done)
	}
	out := logs.String()
	if strings.Count(out, `msg="ai reply named unknown keys"`) != 1 || !strings.Contains(out, "part=categories count=2") || strings.Contains(out, cat1) {
		t.Fatalf("logs = %s", out)
	}
}
