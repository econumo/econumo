package transaction_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// TestGetTransactionList_ValidationEnvelope verifies tier-1 validation matches
// PHP GetTransactionListV1Form: accountId must be a UUID, periodStart/periodEnd
// must be strict "Y-m-d H:i:s". Failures return the PHP envelope (message
// "Form validation error", code 400, per-field messages). Regression for the
// api-compare finding where Go returned code 0 / no field errors.
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
			msgs := env.Errors[tc.field]
			if len(msgs) == 0 || msgs[0] != tc.wantMsg {
				t.Errorf("errors[%q] = %v, want [%q]", tc.field, msgs, tc.wantMsg)
			}
		})
	}
}

// TestGetTransactionList_ValidParams: empty params and well-formed params both
// succeed (every field optional, like PHP).
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
// access to returns 403, matching PHP checkViewTransactionsAccess
// (AccessDeniedException). Regression for the finding where Go returned 400 with
// a raw i18n key.
func TestGetTransactionList_ForbiddenAccount(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	// A second user owns an account the seed user cannot see.
	const otherUser = "22222222-2222-2222-2222-222222222222"
	const otherAcct = "aaaa2222-0000-0000-0000-0000000000a2"
	now := time.Unix(1690000000, 0).UTC()
	if _, err := h.db.ExecContext(context.Background(),
		`INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
		 VALUES (?, ?, ?, 'Other', '', '', '', ?, ?, 1)`,
		otherUser, "ident-other", "enc-other", now, now,
	); err != nil {
		t.Fatalf("seed other user: %v", err)
	}
	if _, err := h.db.ExecContext(context.Background(),
		`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
		 VALUES (?, ?, ?, 'Theirs', 2, 'wallet', 0, ?, ?)`,
		otherAcct, usdID, otherUser, now, now,
	); err != nil {
		t.Fatalf("seed other account: %v", err)
	}

	status, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+otherAcct, tok, nil)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", status, env.raw)
	}
	if env.Success {
		t.Errorf("expected success=false for forbidden account")
	}
}
