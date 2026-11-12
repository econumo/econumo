package onboarding_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	me       = "11111111-1111-1111-1111-111111111111"
	budgetID = "019a0000-0000-7000-8000-0000000000b0"
	cash     = "019a0000-0000-7000-8000-000000000001"
	deposit  = "019a0000-0000-7000-8000-000000000002"
	gone     = "019a0000-0000-7000-8000-000000000003"
	theirs   = "019a0000-0000-7000-8000-000000000004"
)

type fakeBudgets struct {
	marked []vo.Id
}

func (f *fakeBudgets) MarkSavingsMembers(_ context.Context, _, _ vo.Id, ids []vo.Id) error {
	f.marked = append(f.marked, ids...)
	return nil
}

type fakeAccounts struct{}

func (fakeAccounts) AccountsByIDs(_ context.Context, ids []vo.Id) ([]model.OnboardingAccount, error) {
	all := map[string]model.OnboardingAccount{
		cash:    {ID: cash, OwnerID: me, Name: "Wallet", CurrencyCode: "EUR"},
		deposit: {ID: deposit, OwnerID: me, Name: "Tagesgeld", CurrencyCode: "CHF"},
		gone:    {ID: gone, OwnerID: me, Name: "Old savings", CurrencyCode: "EUR", IsDeleted: true},
		theirs:  {ID: theirs, OwnerID: "22222222-2222-2222-2222-222222222222", Name: "Their savings", CurrencyCode: "USD"},
	}
	var out []model.OnboardingAccount
	for _, id := range ids {
		if a, ok := all[id.String()]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

type fakeTimezones struct{ aiOff bool }

func (fakeTimezones) StoredTimezone(context.Context, vo.Id) (string, error) {
	return "Europe/Zurich", nil
}

func (f fakeTimezones) AIOptedOut(context.Context, vo.Id) (bool, error) { return f.aiOff, nil }

type fakeCompleter struct {
	calls int
	user  string
	reply func(ctx context.Context) (string, error)
}

func (f *fakeCompleter) Complete(ctx context.Context, _, _, user string) (string, error) {
	f.calls++
	f.user = user
	return f.reply(ctx)
}

type denyLimiter struct{}

func (denyLimiter) Allow(context.Context, string, string) error {
	return errs.NewTooManyRequests("Too many attempts. Try again later.")
}
func (denyLimiter) Reserve(string, string) error { return nil }

func replying(s string) *fakeCompleter {
	return &fakeCompleter{reply: func(context.Context) (string, error) { return s, nil }}
}

func newSvc(c onboarding.Completer, l onboarding.AttemptLimiter) (*onboarding.Service, *fakeBudgets) {
	b := &fakeBudgets{}
	svc := onboarding.NewService(b, fakeAccounts{}, fakeTimezones{})
	if c != nil {
		svc.SetAdvisor(c, l)
	}
	svc.SetRunner(func(job func()) { job() })
	return svc, b
}

var allIDs = []string{cash, deposit, gone, theirs}

func ids(raw ...string) []vo.Id {
	out := make([]vo.Id, 0, len(raw))
	for _, r := range raw {
		out = append(out, vo.MustParseId(r))
	}
	return out
}

func schedule(ctx context.Context, svc *onboarding.Service, accountIDs ...string) {
	svc.Schedule(ctx, vo.MustParseId(me), vo.MustParseId(budgetID), ids(accountIDs...))
}

func TestClassification_FlagsTheAIPicks(t *testing.T) {
	c := replying("```json\n{\"savings\":[\"" + strings.ToUpper(deposit) + "\",\"" + theirs + "\",\"nope\"]}\n```")
	svc, b := newSvc(c, nil)
	schedule(reqctx.WithLanguage(context.Background(), "de"), svc, allIDs...)
	if len(b.marked) != 1 || b.marked[0].String() != deposit {
		t.Fatalf("marked = %v", b.marked)
	}
	var prompt struct {
		Timezone, Language string
		Accounts           []struct{ ID, Name, Currency string }
	}
	if err := json.Unmarshal([]byte(c.user), &prompt); err != nil {
		t.Fatal(err)
	}
	if prompt.Timezone != "Europe/Zurich" || prompt.Language != "de" {
		t.Fatalf("context = %+v", prompt)
	}
	if len(prompt.Accounts) != 2 || prompt.Accounts[0].Currency != "EUR" || prompt.Accounts[1].Name != "Tagesgeld" || prompt.Accounts[1].Currency != "CHF" {
		t.Fatalf("accounts = %+v", prompt.Accounts)
	}
	for _, leak := range []string{theirs, gone, "Their savings", "Old savings", "profile", "USD"} {
		if strings.Contains(c.user, leak) {
			t.Fatalf("prompt carries %q: %s", leak, c.user)
		}
	}
}

func TestClassification_ExplicitTimezoneWins(t *testing.T) {
	c := replying(`{"savings":[]}`)
	svc, b := newSvc(c, nil)
	loc, _ := time.LoadLocation("Asia/Tokyo")
	schedule(reqctx.WithExplicitLocation(context.Background(), loc), svc, allIDs...)
	if !strings.Contains(c.user, `"timezone":"Asia/Tokyo"`) || len(b.marked) != 0 {
		t.Fatalf("prompt = %s, marked = %v", c.user, b.marked)
	}
}

func TestClassification_NoAIRunsNoJob(t *testing.T) {
	svc, b := newSvc(nil, nil)
	schedule(context.Background(), svc, allIDs...)
	if len(b.marked) != 0 {
		t.Fatalf("marked = %v", b.marked)
	}
}

func TestClassification_NoAccountsAsksNothing(t *testing.T) {
	c := replying(`{"savings":[]}`)
	svc, _ := newSvc(c, nil)
	schedule(context.Background(), svc)
	if c.calls != 0 {
		t.Fatal("AI asked with no new account")
	}
}

func TestClassification_FailuresChangeNothing(t *testing.T) {
	cases := map[string]*fakeCompleter{
		"malformed":   replying("the second one"),
		"wrong shape": replying(`{"savings":"all"}`),
		"error":       {reply: func(context.Context) (string, error) { return "", errors.New("boom") }},
		// The job's deadline is at most 15 s away; one that runs out flags nothing.
		"timeout": {reply: func(ctx context.Context) (string, error) {
			if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > 15*time.Second {
				return `{"savings":["` + deposit + `"]}`, nil
			}
			return "", context.DeadlineExceeded
		}},
		"panic": {reply: func(context.Context) (string, error) { panic("boom") }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			svc, b := newSvc(c, nil)
			schedule(context.Background(), svc, allIDs...)
			if len(b.marked) != 0 {
				t.Fatalf("marked = %v", b.marked)
			}
		})
	}
}

func TestClassification_OnlyForeignOrDeletedAsksNothing(t *testing.T) {
	c := replying(`{"savings":[]}`)
	svc, _ := newSvc(c, nil)
	schedule(context.Background(), svc, gone, theirs)
	if c.calls != 0 {
		t.Fatalf("AI asked %d times", c.calls)
	}
}

func TestClassification_OptedOutUserStartsNoJob(t *testing.T) {
	c := replying(`{"savings":["` + deposit + `"]}`)
	b := &fakeBudgets{}
	svc := onboarding.NewService(b, fakeAccounts{}, fakeTimezones{aiOff: true})
	svc.SetAdvisor(c, nil)
	svc.SetRunner(func(job func()) { job() })
	schedule(context.Background(), svc, allIDs...)
	if len(b.marked) != 0 || c.calls != 0 {
		t.Fatalf("marked = %v, calls = %d", b.marked, c.calls)
	}
}

// optOutAfter reads "on" for the first n preference reads, then "off".
type optOutAfter struct {
	fakeTimezones
	n, reads int
}

func (f *optOutAfter) AIOptedOut(context.Context, vo.Id) (bool, error) {
	f.reads++
	return f.reads > f.n, nil
}

func TestClassification_OptOutAfterSchedulingSendsNothing(t *testing.T) {
	c := replying(`{"savings":["` + deposit + `"]}`)
	b := &fakeBudgets{}
	svc := onboarding.NewService(b, fakeAccounts{}, &optOutAfter{n: 1})
	svc.SetAdvisor(c, nil)
	svc.SetRunner(func(job func()) { job() })
	schedule(context.Background(), svc, allIDs...)
	if len(b.marked) != 0 || c.calls != 0 {
		t.Fatalf("marked = %v, calls = %d", b.marked, c.calls)
	}
}

func TestClassification_OverTheCapSkipsTheJob(t *testing.T) {
	c := replying(`{"savings":["` + deposit + `"]}`)
	svc, b := newSvc(c, denyLimiter{})
	schedule(context.Background(), svc, allIDs...)
	if len(b.marked) != 0 || c.calls != 0 {
		t.Fatalf("marked = %v, calls = %d", b.marked, c.calls)
	}
}
