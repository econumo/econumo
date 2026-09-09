package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

func TestRuleRoutes_CreateListDelete(t *testing.T) {
	h := newHarness(t)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.categories = []string{cat}
	id := "0192b1e4-0000-7000-8000-000000000001"
	status, env := call(t, h, "POST", "/api/v1/import/create-rule", map[string]any{
		"id": id, "action": "classify", "matchField": "external_payee", "matchType": "contains", "matchValue": "Blue Bottle", "categoryId": cat, "priority": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("create: %d %v", status, env)
	}
	status, env = call(t, h, "GET", "/api/v1/import/get-rule-list", nil)
	items := env["data"].(map[string]any)["items"].([]any)
	if status != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["id"] != id {
		t.Fatalf("list: %d %v", status, env)
	}
	if status, env = call(t, h, "POST", "/api/v1/import/delete-rule", map[string]any{"id": id}); status != http.StatusOK {
		t.Fatalf("delete: %d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/delete-rule", map[string]any{"id": id})
	if status != http.StatusBadRequest || env["message"] != "Import rule not found" {
		t.Fatalf("second delete: %d %v", status, env)
	}
}

func TestRuleRoutes_ValidationAndScope(t *testing.T) {
	h := newHarness(t)
	status, env := call(t, h, "POST", "/api/v1/import/create-rule", map[string]any{
		"id": "0192b1e4-0000-7000-8000-000000000002", "action": "skip", "matchField": "external_payee", "matchType": "contains", "matchValue": "x", "categoryId": "anything",
	})
	if status != http.StatusBadRequest || env["errors"].(map[string]any)["categoryId"] == nil {
		t.Fatalf("skip with a target must fail on categoryId: %d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/preview-rule", map[string]any{
		"action": "classify", "matchField": "description", "matchType": "prefix", "matchValue": "x", "scope": "run",
	})
	if status != http.StatusBadRequest || env["errors"].(map[string]any)["runId"] == nil {
		t.Fatalf("scope=run without runId: %d %v", status, env)
	}
}

// The client mints the rule id (it is the create's idempotency key), so a
// blank one is a client bug, not an "assign one for me" request — the SPA
// mints a uuidv7 in useCreateImportRule. Frozen so the two halves cannot
// drift apart again: the DTO's own Validate() accepts an absent id, and only
// this check stops a blank one from reaching persistence.
func TestRuleRoutes_CreateRejectsABlankId(t *testing.T) {
	h := newHarness(t)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.categories = []string{cat}
	spec := map[string]any{
		"action": "classify", "matchField": "external_payee", "matchType": "contains", "matchValue": "Blue Bottle", "categoryId": cat, "priority": 1,
	}
	for _, name := range []string{"omitted", "blank", "whitespace"} {
		body := map[string]any{}
		for k, v := range spec {
			body[k] = v
		}
		switch name {
		case "blank":
			body["id"] = ""
		case "whitespace":
			body["id"] = "   "
		}
		status, env := call(t, h, "POST", "/api/v1/import/create-rule", body)
		if status != http.StatusBadRequest || env["errors"].(map[string]any)["id"] == nil {
			t.Fatalf("%s id: %d %v", name, status, env)
		}
	}
}
