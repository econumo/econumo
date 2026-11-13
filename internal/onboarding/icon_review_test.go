package onboarding_test

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

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

func (r *recordingAI) Complete(_ context.Context, _, system, user string) (string, error) {
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

const (
	cat1  = "019a0000-0000-7000-8000-0000000000c1"
	cat2  = "019a0000-0000-7000-8000-0000000000c2"
	cat3  = "019a0000-0000-7000-8000-0000000000c3"
	acct1 = "019a0000-0000-7000-8000-0000000000a1"
	acct2 = "019a0000-0000-7000-8000-0000000000a2"
)

func newIconSvc(aiOff bool) (*onboarding.Service, *recordingAI, *countingLimiter, *fakeTimers) {
	ai, lim, timers := &recordingAI{}, &countingLimiter{}, &fakeTimers{}
	svc := onboarding.NewService(&fakeBudgets{}, fakeAccounts{}, fakeTimezones{aiOff: aiOff})
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

func askedIDs(t *testing.T, raw, key string) []string {
	t.Helper()
	var p map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	var rows []struct{ ID string }
	if err := json.Unmarshal(p[key], &rows); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.ID)
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
			if got := askedIDs(t, c.user, "categories"); strings.Join(got, ",") != strings.Join([]string{cat1, cat2, cat3}, ",") {
				t.Fatalf("categories asked = %v", got)
			}
		case strings.Contains(c.system, `{"accounts":`):
			if got := askedIDs(t, c.user, "accounts"); strings.Join(got, ",") != strings.Join([]string{acct1, acct2}, ",") {
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
// language and the timezone, plus the ids its answer refers to.
func TestPrompts_SendOnlyTheAllowlist(t *testing.T) {
	svc, ai, _, timers := newIconSvc(false)
	ctx := reqctx.WithLanguage(context.Background(), "de")
	svc.ReviewImportedIcons(ctx, vo.MustParseId(me), ids(cat1), ids(acct1))
	timers.fire()
	c := replying(`{"savings":[]}`)
	csvc, _ := newSvc(c, nil)
	schedule(ctx, csvc, cash, deposit)

	want := map[string]string{
		"categories": "categories,categories[].id,categories[].name,categories[].type,language,timezone",
		"accounts":   "accounts,accounts[].currency,accounts[].id,accounts[].name,language,timezone",
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
}
