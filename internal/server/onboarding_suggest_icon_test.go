package server_test

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
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

func TestApplySuggestedIcon_ChangesOnlyAnUnchangedIcon(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	cat := e.f.Category(fixture.Category{UserID: e.user, Name: "Mortgage", Icon: "home"})
	const path = "/api/v1/onboarding/apply-suggested-icon"
	// the icon moved since the client read it: nothing is written
	status, data := e.post(path, map[string]any{"categoryId": cat, "icon": "house", "expectedIcon": "wallet"})
	if status != 200 || data["applied"] != false || e.icon("categories", cat) != "home" {
		t.Fatalf("stale: status %d: %v, icon %s", status, data, e.icon("categories", cat))
	}
	status, data = e.post(path, map[string]any{"categoryId": cat, "icon": "house", "expectedIcon": "home"})
	if status != 200 || data["applied"] != true || e.icon("categories", cat) != "house" {
		t.Fatalf("status %d: %v, icon %s", status, data, e.icon("categories", cat))
	}
	if name := e.str(`SELECT name FROM categories WHERE id = ?`, cat); name != "Mortgage" {
		t.Fatalf("name = %s", name)
	}
}

func TestApplySuggestedIcon_Refusals(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	other := e.f.User(fixture.User{Name: "Other", Email: "other@example.test"})
	theirs := e.f.Category(fixture.Category{UserID: other, Name: "Theirs", Icon: "home"})
	mine := e.f.Category(fixture.Category{UserID: e.user, Name: "Mine", Icon: "home"})
	const path = "/api/v1/onboarding/apply-suggested-icon"
	if status, data := e.post(path, map[string]any{"categoryId": theirs, "icon": "house", "expectedIcon": "home"}); status != 400 || data["message"] != "Category not found" {
		t.Fatalf("foreign: status %d: %v", status, data)
	}
	if status, data := e.post(path, map[string]any{"categoryId": mine, "icon": "rocket_ship", "expectedIcon": "home"}); status != 400 || !strings.Contains(fmt.Sprint(data["errors"]), "icon") {
		t.Fatalf("unknown icon: status %d: %v", status, data)
	}
	if status, _ := e.post(path, map[string]any{"categoryId": "nope", "icon": "house", "expectedIcon": "home"}); status != 400 {
		t.Fatalf("bad id: status %d", status)
	}
	if e.icon("categories", theirs) != "home" || e.icon("categories", mine) != "home" {
		t.Fatal("an icon changed")
	}
}
