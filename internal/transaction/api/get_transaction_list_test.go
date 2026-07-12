package api_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const userID = seedUserID

// TestGetTransactionList_ValidationEnvelope verifies tier-1 validation:
// accountId must be a UUID, periodStart/periodEnd must be strict
// "2006-01-02 15:04:05". Failures return the validation envelope (message
// "Form validation error", code 400, per-field messages) — not the bare-500 form.
func TestGetTransactionList_ValidationEnvelope(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	cases := []struct {
		name    string
		query   string
		field   string
		wantMsg string
	}{
		{"bad accountId", "?accountId=not-a-uuid", "accountId", "This value is not a valid UUID."},
		{"date-only periodStart", "?periodStart=2020-01-01", "periodStart", "This value is not a valid datetime."},
		{"date-only periodEnd", "?periodEnd=2020-01-01", "periodEnd", "This value is not a valid datetime."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list"+tc.query, tok, nil)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
			}
			if env.Message != "Form validation error" {
				t.Errorf("message = %q, want %q", env.Message, "Form validation error")
			}
			if env.Code != 400 {
				t.Errorf("code = %d, want 400", env.Code)
			}
			msgs := env.errorsMap()[tc.field]
			if len(msgs) == 0 || msgs[0] != tc.wantMsg {
				t.Errorf("errors[%q] = %v, want [%q]", tc.field, msgs, tc.wantMsg)
			}
		})
	}
}

// TestGetTransactionList_ValidParams: empty params and well-formed params both
// succeed (every field is optional).
func TestGetTransactionList_ValidParams(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	for _, q := range []string{
		"",
		"?periodStart=" + url.QueryEscape("2020-01-01 00:00:00") + "&periodEnd=" + url.QueryEscape("2020-12-31 23:59:59"),
		"?accountId=" + accountID,
	} {
		status, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list"+q, tok, nil)
		if status != http.StatusOK {
			t.Fatalf("query %q: status = %d, want 200; body: %s", q, status, env.raw)
		}
	}
}

// TestGetTransactionList_ForbiddenAccount: requesting an account the user has no
// access to returns 403 (not a 400 with a raw i18n key).
func TestGetTransactionList_ForbiddenAccount(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	// A second user owns an account the seed user cannot see.
	const otherUser = "22222222-2222-2222-2222-222222222222"
	const otherAcct = "aaaa2222-0000-0000-0000-0000000000a2"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.User(fixture.User{ID: otherUser, Name: "Other"})
	f.Account(fixture.Account{ID: otherAcct, UserID: otherUser, CurrencyID: usdID, Name: "Theirs"})

	status, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+otherAcct, tok, nil)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", status, env.raw)
	}
	if env.Success {
		t.Errorf("expected success=false for forbidden account")
	}
}

// TestGetTransactionList_PageMode: limit+cursor walks the account newest-first
// with a stable envelope: items plus a "page" block.
func TestGetTransactionList_PageMode(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	// three rows with distinct dates so page boundaries are unambiguous
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-0000000000f1", UserID: userID, AccountID: accountID, Type: 0, Amount: "1.00000000", SpentAt: "2026-06-03 12:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-0000000000f2", UserID: userID, AccountID: accountID, Type: 0, Amount: "2.00000000", SpentAt: "2026-06-02 12:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-0000000000f3", UserID: userID, AccountID: accountID, Type: 0, Amount: "3.00000000", SpentAt: "2026-06-01 12:00:00"})

	status, env := h.do(t, http.MethodGet,
		"/api/v1/transaction/get-transaction-list?accountId="+accountID+"&limit=2", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d; body: %s", status, env.raw)
	}
	data := env.dataMap()
	page, ok := data["page"].(map[string]any)
	if !ok {
		t.Fatalf("no page block in %v", data)
	}
	if page["hasMore"] != true || page["nextCursor"] == nil {
		t.Fatalf("page = %v, want hasMore=true with a cursor", page)
	}

	status, env = h.do(t, http.MethodGet,
		"/api/v1/transaction/get-transaction-list?accountId="+accountID+"&limit=2&cursor="+url.QueryEscape(page["nextCursor"].(string)), tok, nil)
	if status != http.StatusOK {
		t.Fatalf("page 2 status = %d; body: %s", status, env.raw)
	}
	page2 := env.dataMap()["page"].(map[string]any)
	if page2["hasMore"] != false || page2["nextCursor"] != nil {
		t.Fatalf("page 2 = %v, want hasMore=false, nil cursor", page2)
	}
}

// TestGetTransactionList_BootMode: perAccountLimit returns deduped items plus
// per-account cursors, and NO page block.
func TestGetTransactionList_BootMode(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-0000000000f1", UserID: userID, AccountID: accountID, Type: 0, Amount: "1.00000000", SpentAt: "2026-06-03 12:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-0000000000f2", UserID: userID, AccountID: accountID, Type: 0, Amount: "2.00000000", SpentAt: "2026-06-02 12:00:00"})

	status, env := h.do(t, http.MethodGet,
		"/api/v1/transaction/get-transaction-list?perAccountLimit=1", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d; body: %s", status, env.raw)
	}
	data := env.dataMap()
	accounts, ok := data["accounts"].([]any)
	if !ok || len(accounts) == 0 {
		t.Fatalf("no accounts block in %v", data)
	}
	var entry map[string]any
	for _, a := range accounts {
		if m := a.(map[string]any); m["id"] == accountID {
			entry = m
		}
	}
	if entry == nil {
		t.Fatalf("no entry for %s in %v", accountID, accounts)
	}
	if entry["hasMore"] != true || entry["nextCursor"] == nil {
		t.Fatalf("entry = %v, want hasMore=true with a cursor", entry)
	}
	if _, hasPage := data["page"]; hasPage {
		t.Fatalf("boot mode must not include a page block: %v", data)
	}
}

// TestGetTransactionList_LegacyShapeUnchanged: a no-param call has ONLY items.
func TestGetTransactionList_LegacyShapeUnchanged(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d; body: %s", status, env.raw)
	}
	data := env.dataMap()
	for _, forbidden := range []string{"page", "accounts"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("legacy response leaked %q: %v", forbidden, data)
		}
	}
}

// TestGetTransactionList_BadCursor: a malformed cursor is a 400 validation error.
func TestGetTransactionList_BadCursor(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodGet,
		"/api/v1/transaction/get-transaction-list?accountId="+accountID+"&limit=2&cursor=not-a-cursor", tok, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	msgs := env.errorsMap()["cursor"]
	if len(msgs) == 0 || msgs[0] != "This value is not a valid cursor." {
		t.Fatalf("errors[cursor] = %v", msgs)
	}
}
