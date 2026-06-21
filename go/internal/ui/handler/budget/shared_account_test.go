package budget_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

const (
	otherUserID  = "22222222-2222-2222-2222-222222222222"
	sharedAcctID = "aaaa3333-0000-7000-8000-000000000003"
	ownedTxID    = "ffff2222-0000-7000-8000-000000000001"
	sharedTxID   = "ffff2222-0000-7000-8000-000000000002"
	c4BudgetID   = "bbbb4444-0000-7000-8000-000000000004"
	jan2026      = "2026-01-15"
)

// TestGetBudget_ExcludesSharedAccountBalance is the regression for the
// api-compare C4 finding: a budget's per-currency startBalance must sum only the
// accounts OWNED by the budget participants (PHP findByOwnersIds), NOT accounts
// merely shared with them via accounts_access. A shared account's balance was
// previously inflating the start balance.
func TestGetBudget_ExcludesSharedAccountBalance(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	ctx := context.Background()
	now := time.Unix(1690000000, 0).UTC()
	// A transaction BEFORE the period gives the seed user's OWN account a start
	// balance of 200 (income, type=1) on the baseline USD account.
	before := time.Date(2025, 12, 1, 12, 0, 0, 0, time.UTC)
	if _, err := h.db.ExecContext(ctx, `INSERT INTO transactions (id, user_id, account_id, type, amount, description, spent_at, created_at, updated_at) VALUES (?, ?, ?, 1, '200.00', '', ?, ?, ?)`,
		ownedTxID, seedUserID, accountID, before, now, now); err != nil {
		t.Fatalf("seed owned tx: %v", err)
	}

	// A second user owns a USD account they SHARE with the seed user, with a
	// before-period balance of 5000. It must NOT appear in the budget balance.
	if _, err := h.db.ExecContext(ctx, `INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active) VALUES (?, 'ident2', 'enc2', 'Other', '', '', '', ?, ?, 1)`,
		otherUserID, now, now); err != nil {
		t.Fatalf("seed other user: %v", err)
	}
	if _, err := h.db.ExecContext(ctx, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'Theirs', 2, 'wallet', 0, ?, ?)`,
		sharedAcctID, usdID, otherUserID, now, now); err != nil {
		t.Fatalf("seed shared account: %v", err)
	}
	// Grant the seed user access (role user=1) — makes it "available" but not owned.
	if _, err := h.db.ExecContext(ctx, `INSERT INTO accounts_access (account_id, user_id, role, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`,
		sharedAcctID, seedUserID, now, now); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	if _, err := h.db.ExecContext(ctx, `INSERT INTO transactions (id, user_id, account_id, type, amount, description, spent_at, created_at, updated_at) VALUES (?, ?, ?, 1, '5000.00', '', ?, ?, ?)`,
		sharedTxID, otherUserID, sharedAcctID, before, now, now); err != nil {
		t.Fatalf("seed shared tx: %v", err)
	}

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(c4BudgetID, "C4 Budget")); st != 200 {
		t.Fatalf("create=%d body=%s", st, e.raw)
	}

	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+c4BudgetID+"&date="+jan2026, tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, env.raw)
	}
	type budgetView struct {
		Item struct {
			Balances []struct {
				CurrencyId   string  `json:"currencyId"`
				StartBalance *string `json:"startBalance"`
			} `json:"balances"`
		} `json:"item"`
	}
	res := mustUnmarshal[budgetView](t, env.Data)

	var usdStart *string
	for _, b := range res.Item.Balances {
		if b.CurrencyId == usdID {
			usdStart = b.StartBalance
		}
	}
	if usdStart == nil {
		t.Fatalf("no USD balance in %+v", res.Item.Balances)
	}
	// Only the owned account's 200 counts; the shared account's 5000 is excluded.
	if *usdStart != "200" {
		t.Fatalf("USD startBalance=%q want 200 (shared account's 5000 must be excluded)", *usdStart)
	}
}
