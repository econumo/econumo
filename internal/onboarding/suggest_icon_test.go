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
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

func suggest(t *testing.T, svc *onboarding.Service, ctx context.Context, req model.OnboardingSuggestIconRequest) string {
	t.Helper()
	res, err := svc.SuggestIcon(ctx, vo.MustParseId(me), req)
	if err != nil {
		t.Fatalf("SuggestIcon: %v", err)
	}
	return res.Icon
}

var mortgage = model.OnboardingSuggestIconRequest{Kind: "category", Name: " Hypothek ", Type: "expense"}

func TestSuggestIcon_NoAISuggestsNothing(t *testing.T) {
	svc, _ := newSvc(nil, nil)
	if icon := suggest(t, svc, context.Background(), mortgage); icon != "" {
		t.Fatalf("icon = %q", icon)
	}
}

func TestSuggestIcon_ReturnsTheAIPick(t *testing.T) {
	c := replying("```json\n{\"icon\":\" house \"}\n```")
	svc, _ := newSvc(c, nil)
	ctx := reqctx.WithLanguage(context.Background(), "de")
	if icon := suggest(t, svc, ctx, mortgage); icon != "house" {
		t.Fatalf("icon = %q", icon)
	}
	var sent map[string]string
	if err := json.Unmarshal([]byte(c.user), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["name"] != "Hypothek" || sent["kind"] != "category" || sent["type"] != "expense" || sent["language"] != "de" || sent["timezone"] != "Europe/Zurich" {
		t.Fatalf("prompt = %s", c.user)
	}
	if !strings.Contains(c.system, "credit_card") {
		t.Fatal("the system prompt does not list the icon catalogue")
	}
}

func TestSuggestIcon_SendsTheAccountCurrency(t *testing.T) {
	c := replying(`{"icon":"savings"}`)
	svc, _ := newSvc(c, nil)
	req := model.OnboardingSuggestIconRequest{Kind: "account", Name: "Tagesgeld", Currency: "EUR"}
	if icon := suggest(t, svc, context.Background(), req); icon != "savings" {
		t.Fatalf("icon = %q", icon)
	}
	if !strings.Contains(c.user, `"currency":"EUR"`) || strings.Contains(c.user, `"type"`) {
		t.Fatalf("prompt = %s", c.user)
	}
}

func TestSuggestIcon_UnusableRepliesSuggestNothing(t *testing.T) {
	for name, c := range map[string]*fakeCompleter{
		"unknown icon": replying(`{"icon":"rocket_ship"}`),
		"malformed":    replying(`{"icon":`),
		"wrong shape":  replying(`{"icon":["home"]}`),
		"prose":        replying(`home`),
		"error":        {reply: func(context.Context) (string, error) { return "", errors.New("down") }},
	} {
		t.Run(name, func(t *testing.T) {
			svc, _ := newSvc(c, nil)
			if icon := suggest(t, svc, context.Background(), mortgage); icon != "" {
				t.Fatalf("icon = %q", icon)
			}
		})
	}
}

func TestSuggestIcon_GivesTheAIEightSeconds(t *testing.T) {
	var left time.Duration
	c := &fakeCompleter{reply: func(ctx context.Context) (string, error) {
		if d, ok := ctx.Deadline(); ok {
			left = time.Until(d)
		}
		return "", context.DeadlineExceeded
	}}
	svc, _ := newSvc(c, nil)
	if icon := suggest(t, svc, context.Background(), mortgage); icon != "" {
		t.Fatalf("icon = %q", icon)
	}
	if left <= 0 || left > 8*time.Second {
		t.Fatalf("time left = %v", left)
	}
}

func TestSuggestIcon_OverTheLimitSuggestsNothing(t *testing.T) {
	c := replying(`{"icon":"house"}`)
	svc, _ := newSvc(c, denyLimiter{})
	if icon := suggest(t, svc, context.Background(), mortgage); icon != "" || c.calls != 0 {
		t.Fatalf("icon = %q, calls %d", icon, c.calls)
	}
}

func TestSuggestIcon_CachesPerUserNameAndLanguage(t *testing.T) {
	c := replying(`{"icon":"house"}`)
	limiter := &countingLimiter{}
	svc, _ := newSvc(c, limiter)
	ctx := reqctx.WithLanguage(context.Background(), "de")
	suggest(t, svc, ctx, mortgage)
	again := mortgage
	again.Name = "hypothek"
	if icon := suggest(t, svc, ctx, again); icon != "house" || c.calls != 1 || limiter.n != 1 {
		t.Fatalf("icon %q, calls %d, counted %d", icon, c.calls, limiter.n)
	}
	// another language, kind, type or user asks again
	suggest(t, svc, reqctx.WithLanguage(context.Background(), "fr"), mortgage)
	income := mortgage
	income.Type = "income"
	suggest(t, svc, ctx, income)
	if _, err := svc.SuggestIcon(ctx, vo.MustParseId(theirs), mortgage); err != nil {
		t.Fatal(err)
	}
	if c.calls != 4 {
		t.Fatalf("calls = %d", c.calls)
	}
}

func TestSuggestIcon_FailuresAreNotCached(t *testing.T) {
	fail := true
	c := &fakeCompleter{reply: func(context.Context) (string, error) {
		if fail {
			return "", errors.New("down")
		}
		return `{"icon":"house"}`, nil
	}}
	svc, _ := newSvc(c, nil)
	suggest(t, svc, context.Background(), mortgage)
	fail = false
	if icon := suggest(t, svc, context.Background(), mortgage); icon != "house" || c.calls != 2 {
		t.Fatalf("icon %q, calls %d", icon, c.calls)
	}
}

func TestSuggestIcon_AClientGoneMidCallStillCachesTheAnswer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &fakeCompleter{reply: func(callCtx context.Context) (string, error) {
		cancel() // the client aborts while the provider is answering
		if err := callCtx.Err(); err != nil {
			return "", err
		}
		if _, ok := callCtx.Deadline(); !ok {
			return "", errors.New("no deadline")
		}
		return `{"icon":"house"}`, nil
	}}
	svc, _ := newSvc(c, nil)
	suggest(t, svc, ctx, mortgage)
	if icon := suggest(t, svc, context.Background(), mortgage); icon != "house" || c.calls != 1 {
		t.Fatalf("icon %q, calls %d", icon, c.calls)
	}
}
