package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestUpdateTheme_PersistsAndReplacesOption(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	for _, theme := range []string{"dark", "light", "system"} {
		status, env := h.do(t, http.MethodPost, "/api/v1/user/update-theme", token, map[string]string{"theme": theme})
		if status != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", theme, status, env.raw)
		}
		option := `{"name":"theme","value":"` + theme + `"}`
		if !strings.Contains(string(env.raw), option) {
			t.Fatalf("theme missing from write response: %s", env.raw)
		}
		// A separate read goes through the CQRS read path and the database,
		// rather than echoing the aggregate returned by the mutation.
		status, env = h.do(t, http.MethodGet, "/api/v1/user/get-user-data", token, nil)
		if status != http.StatusOK || !strings.Contains(string(env.raw), option) {
			t.Fatalf("saved theme missing from read: status = %d: %s", status, env.raw)
		}
	}
	var count int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM users_options WHERE user_id = ? AND name = 'theme'", seedUserID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("theme row count = %d, want 1", count)
	}
}

func TestUpdateTheme_RejectsInvalidAndUnauthenticatedWrites(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	for _, body := range []map[string]any{{}, {"theme": nil}, {"theme": ""}, {"theme": "sepia"}, {"theme": "Dark"}, {"theme": true}} {
		status, env := h.do(t, http.MethodPost, "/api/v1/user/update-theme", token, body)
		if status != http.StatusBadRequest {
			t.Fatalf("%v: status = %d, want 400: %s", body, status, env.raw)
		}
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-theme", "", map[string]string{"theme": "dark"})
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: status = %d, want 401: %s", status, env.raw)
	}
	var count int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM users_options WHERE name = 'theme'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected requests wrote %d theme rows", count)
	}
}
