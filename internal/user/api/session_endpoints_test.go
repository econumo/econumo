package api_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

type sessionItem struct {
	Id         string `json:"id"`
	UserAgent  string `json:"userAgent"`
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt"`
	IsCurrent  bool   `json:"isCurrent"`
}

var datetimeShape = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)

func (h *harness) sessionList(t *testing.T, token string) []sessionItem {
	t.Helper()
	status, env := h.do(t, http.MethodGet, "/api/v1/user/get-session-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("get-session-list status = %d; body: %s", status, env.raw)
	}
	return mustUnmarshal[[]sessionItem](t, env.Data)
}

func TestGetSessionList_TwoSessionsOneCurrent(t *testing.T) {
	h := newHarness(t)
	tokA := h.issueToken(t)
	tokB := h.issueToken(t)

	items := h.sessionList(t, tokA)
	if len(items) != 2 {
		t.Fatalf("sessions = %d, want 2; %+v", len(items), items)
	}
	currents := 0
	for _, it := range items {
		if it.IsCurrent {
			currents++
		}
		if !datetimeShape.MatchString(it.CreatedAt) || !datetimeShape.MatchString(it.LastUsedAt) {
			t.Errorf("datetime shape mismatch: %+v", it)
		}
	}
	if currents != 1 {
		t.Fatalf("isCurrent count = %d, want exactly 1", currents)
	}

	// The other token sees ITSELF as current — a different row.
	var curA, curB string
	for _, it := range items {
		if it.IsCurrent {
			curA = it.Id
		}
	}
	for _, it := range h.sessionList(t, tokB) {
		if it.IsCurrent {
			curB = it.Id
		}
	}
	if curA == "" || curB == "" || curA == curB {
		t.Fatalf("current rows must differ: %q vs %q", curA, curB)
	}
}

func TestRevokeSession_OtherSession(t *testing.T) {
	h := newHarness(t)
	tokA := h.issueToken(t)
	tokB := h.issueToken(t)

	items := h.sessionList(t, tokA)
	var otherID string
	for _, it := range items {
		if !it.IsCurrent {
			otherID = it.Id
		}
	}
	if otherID == "" {
		t.Fatal("no other session found")
	}

	status, env := h.do(t, http.MethodPost, "/api/v1/user/revoke-session", tokA, map[string]string{"id": otherID})
	if status != http.StatusOK || !env.Success {
		t.Fatalf("revoke-session = %d %s", status, env.raw)
	}

	// The revoked token stops working; the current one still works.
	if status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", tokB, nil); status != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, want 401", status)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", tokA, nil); status != http.StatusOK {
		t.Fatalf("current token status = %d, want 200", status)
	}
	if got := h.sessionList(t, tokA); len(got) != 1 {
		t.Fatalf("live sessions after revoke = %d, want 1", len(got))
	}
}

func TestRevokeSession_CurrentIsAllowed(t *testing.T) {
	h := newHarness(t)
	tok := h.issueToken(t)
	items := h.sessionList(t, tok)
	if len(items) != 1 || !items[0].IsCurrent {
		t.Fatalf("unexpected list: %+v", items)
	}

	status, env := h.do(t, http.MethodPost, "/api/v1/user/revoke-session", tok, map[string]string{"id": items[0].Id})
	if status != http.StatusOK {
		t.Fatalf("revoke current = %d %s", status, env.raw)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", tok, nil); status != http.StatusUnauthorized {
		t.Fatalf("token after self-revoke = %d, want 401", status)
	}
}

func TestRevokeSession_ForeignAndUnknown400(t *testing.T) {
	h := newHarness(t)
	tok := h.issueToken(t)

	// A second user with their own session.
	otherUser := "99999999-9999-9999-9999-999999999999"
	f := fixture.New(t, h.tdb).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: otherUser, Email: "other-sess@example.test", Name: "Other"})
	otherTok := h.issueTokenFor(t, otherUser)
	foreign := h.sessionList(t, otherTok)[0].Id

	for name, id := range map[string]string{
		"foreign": foreign,
		"unknown": "00000000-0000-0000-0000-000000000009",
	} {
		// Domain not-found surfaces as 400 via the generic error envelope
		// (project-wide convention, see httpx.WriteError).
		status, env := h.do(t, http.MethodPost, "/api/v1/user/revoke-session", tok, map[string]string{"id": id})
		if status != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400; %s", name, status, env.raw)
		}
		if env.Message != "Session not found" {
			t.Errorf("%s: message = %q", name, env.Message)
		}
	}
	// The foreign session must still be live.
	if status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", otherTok, nil); status != http.StatusOK {
		t.Error("foreign session was revoked by a 404 path")
	}
}

func TestRevokeSession_BlankId400(t *testing.T) {
	h := newHarness(t)
	tok := h.issueToken(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/revoke-session", tok, map[string]string{"id": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; %s", status, env.raw)
	}
	if msgs := env.errorsMap()["id"]; len(msgs) == 0 || msgs[0] != "This value should not be blank." {
		t.Fatalf("id errors = %v", msgs)
	}
}

func TestRevokeOtherSessions(t *testing.T) {
	h := newHarness(t)
	tokA := h.issueToken(t)
	tokB := h.issueToken(t)
	tokC := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/revoke-other-sessions", tokA, nil)
	if status != http.StatusOK || !env.Success {
		t.Fatalf("revoke-other-sessions = %d %s", status, env.raw)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", tokA, nil); status != http.StatusOK {
		t.Fatal("current session must survive")
	}
	for _, tok := range []string{tokB, tokC} {
		if status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", tok, nil); status != http.StatusUnauthorized {
			t.Fatal("other sessions must be revoked")
		}
	}
	if got := h.sessionList(t, tokA); len(got) != 1 || !got[0].IsCurrent {
		t.Fatalf("after revoke-others: %+v", got)
	}
}
