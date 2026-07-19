package api_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

type patItem struct {
	Id         string  `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt string  `json:"lastUsedAt"`
	ExpiresAt  *string `json:"expiresAt"`
}

type createdPat struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	Token     string  `json:"token"`
	CreatedAt string  `json:"createdAt"`
	ExpiresAt *string `json:"expiresAt"`
}

var patTokenShape = regexp.MustCompile(`^eco_pat_[A-Za-z0-9_-]{43}$`)

func (h *harness) createPat(t *testing.T, token, name, expiresAt string) createdPat {
	t.Helper()
	status, env := h.do(t, http.MethodPost, "/api/v1/user/create-personal-token", token,
		map[string]string{"name": name, "expiresAt": expiresAt})
	if status != http.StatusOK {
		t.Fatalf("create-personal-token = %d %s", status, env.raw)
	}
	return mustUnmarshal[createdPat](t, env.Data)
}

func (h *harness) patList(t *testing.T, token string) []patItem {
	t.Helper()
	status, env := h.do(t, http.MethodGet, "/api/v1/user/get-personal-token-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("get-personal-token-list = %d %s", status, env.raw)
	}
	return mustUnmarshal[[]patItem](t, env.Data)
}

func TestCreatePersonalToken_NeverAndExpiring(t *testing.T) {
	h := newHarness(t)
	ses := h.issueToken(t)

	never := h.createPat(t, ses, "CI export", "")
	if !patTokenShape.MatchString(never.Token) {
		t.Errorf("token %q does not match eco_pat_<43 urlsafe chars>", never.Token)
	}
	if never.ExpiresAt != nil {
		t.Errorf("expiresAt = %v, want null", *never.ExpiresAt)
	}
	if never.Name != "CI export" || !datetimeShape.MatchString(never.CreatedAt) {
		t.Errorf("created pat mismatch: %+v", never)
	}

	expiring := h.createPat(t, ses, "Short lived", "2030-01-01 00:00:00")
	if expiring.ExpiresAt == nil || *expiring.ExpiresAt != "2030-01-01 00:00:00" {
		t.Errorf("expiresAt = %v, want echoed date", expiring.ExpiresAt)
	}

	// The list shows both, without any token material.
	items := h.patList(t, ses)
	if len(items) != 2 {
		t.Fatalf("pat list = %d, want 2", len(items))
	}
	if strings.Contains(string(mustRaw(t, h, ses)), "eco_pat_") {
		t.Error("list response must not contain raw tokens")
	}
}

// mustRaw returns the raw list body for token-leak scanning.
func mustRaw(t *testing.T, h *harness, token string) []byte {
	t.Helper()
	_, env := h.do(t, http.MethodGet, "/api/v1/user/get-personal-token-list", token, nil)
	return env.raw
}

func TestCreatePersonalToken_Validation(t *testing.T) {
	h := newHarness(t)
	ses := h.issueToken(t)

	for name, body := range map[string]map[string]string{
		"blank-name": {"name": "", "expiresAt": ""},
		"long-name":  {"name": strings.Repeat("x", 65), "expiresAt": ""},
		"bad-date":   {"name": "ok", "expiresAt": "tomorrow"},
	} {
		status, env := h.do(t, http.MethodPost, "/api/v1/user/create-personal-token", ses, body)
		if status != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400; %s", name, status, env.raw)
		}
	}

	status, env := h.do(t, http.MethodPost, "/api/v1/user/create-personal-token", ses,
		map[string]string{"name": "Expired", "expiresAt": "2020-01-01 00:00:00"})
	if status != http.StatusBadRequest {
		t.Fatalf("past date: status = %d; %s", status, env.raw)
	}
	if env.Message != "Expiration date must be in the future" {
		t.Errorf("past date message = %q", env.Message)
	}
}

func TestPersonalToken_AuthenticatesAndRevokes(t *testing.T) {
	h := newHarness(t)
	ses := h.issueToken(t)
	pat := h.createPat(t, ses, "integration", "")

	// The PAT authenticates like a session.
	if status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", pat.Token, nil); status != http.StatusOK {
		t.Fatalf("PAT auth status = %d, want 200", status)
	}
	// But never appears in the sessions list.
	for _, s := range h.sessionList(t, ses) {
		if s.Id == pat.Id {
			t.Error("PAT leaked into the sessions list")
		}
	}

	status, env := h.do(t, http.MethodPost, "/api/v1/user/revoke-personal-token", ses, map[string]string{"id": pat.Id})
	if status != http.StatusOK {
		t.Fatalf("revoke-personal-token = %d %s", status, env.raw)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", pat.Token, nil); status != http.StatusUnauthorized {
		t.Fatal("revoked PAT must 401")
	}
	if items := h.patList(t, ses); len(items) != 0 {
		t.Fatalf("pat list after revoke = %d, want 0", len(items))
	}
}

func TestRevokePersonalToken_NotFoundCases(t *testing.T) {
	h := newHarness(t)
	ses := h.issueToken(t)
	sessions := h.sessionList(t, ses)

	for name, id := range map[string]string{
		"unknown":    "00000000-0000-0000-0000-000000000009",
		"session-id": sessions[0].Id, // a session is not a personal token
	} {
		status, env := h.do(t, http.MethodPost, "/api/v1/user/revoke-personal-token", ses, map[string]string{"id": id})
		if status != http.StatusBadRequest || env.Message != "Token not found" {
			t.Errorf("%s: status = %d message = %q", name, status, env.Message)
		}
	}
	// The session used above must still work (not revoked via the PAT path).
	if status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", ses, nil); status != http.StatusOK {
		t.Fatal("session must survive a failed PAT revoke")
	}
}
