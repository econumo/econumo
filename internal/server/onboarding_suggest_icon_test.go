package server_test

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func suggestIconPath(kind, name string) string {
	return "/api/v1/onboarding/suggest-icon?kind=" + kind + "&name=" + url.QueryEscape(name)
}

func TestSuggestIcon_AnswersFromTheAIAndCaches(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	e.ai.reply = func() (string, error) { return `{"icon":"house"}`, nil }
	status, data := e.get(suggestIconPath("category", "Hypothek") + "&type=expense")
	if status != 200 || data["icon"] != "house" {
		t.Fatalf("status %d: %v", status, data)
	}
	if len(e.ai.prompts) != 1 || !strings.Contains(e.ai.prompts[0], `"name":"Hypothek"`) || !strings.Contains(e.ai.systems[0], `{"icon":`) {
		t.Fatalf("prompts = %v", e.ai.prompts)
	}
	if status, data := e.get(suggestIconPath("category", " hypothek") + "&type=expense"); status != 200 || data["icon"] != "house" || len(e.ai.prompts) != 1 {
		t.Fatalf("cached: status %d: %v, prompts %d", status, data, len(e.ai.prompts))
	}
	e.ai.reply = func() (string, error) { return `{"icon":"not_an_icon"}`, nil }
	if status, data := e.get(suggestIconPath("account", "Revolut") + "&currency=EUR"); status != 200 || data["icon"] != "" {
		t.Fatalf("unknown icon: status %d: %v", status, data)
	}
}

func TestSuggestIcon_OverThePerUserLimitAnswersNothing(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	e.ai.reply = func() (string, error) { return `{"icon":"star"}`, nil }
	// 60 a window, never the per-IP cap (also 60) first
	for i := range 60 {
		if status, data := e.get(suggestIconPath("account", "Account "+strconv.Itoa(i))); status != 200 || data["icon"] != "star" {
			t.Fatalf("call %d: status %d: %v", i, status, data)
		}
	}
	status, data := e.get(suggestIconPath("account", "One more"))
	if status != 200 || data["icon"] != "" || len(e.ai.prompts) != 60 {
		t.Fatalf("status %d: %v, prompts %d", status, data, len(e.ai.prompts))
	}
	// an answer already given still comes from the cache
	if status, data := e.get(suggestIconPath("account", "Account 7")); status != 200 || data["icon"] != "star" {
		t.Fatalf("cached: status %d: %v", status, data)
	}
}

func TestSuggestIcon_ValidatesTheQuery(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	for _, path := range []string{
		"/api/v1/onboarding/suggest-icon?name=Rent",
		suggestIconPath("payee", "Rent"),
		suggestIconPath("category", "  "),
		suggestIconPath("category", strings.Repeat("a", 65)),
	} {
		if status, _ := e.get(path); status != 400 {
			t.Fatalf("%s: status %d", path, status)
		}
	}
	if len(e.ai.prompts) != 0 {
		t.Fatalf("prompts = %v", e.ai.prompts)
	}
}
