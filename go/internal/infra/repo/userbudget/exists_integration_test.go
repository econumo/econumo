package userbudget_test

// Integration tests for the userbudget existence-probe Lookup against a real
// migrated in-memory SQLite.

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/repo/userbudget"
	"github.com/econumo/econumo/internal/testutil"
)

const (
	usdID    = "dffc2a06-6f29-4704-8575-31709adee926"
	userA    = "11111111-1111-1111-1111-111111111111"
	budgetID = "b0d00000-0000-0000-0000-00000000b001"
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func TestUserBudgetLookup_Exists(t *testing.T) {
	db := testutil.NewSQLite(t)
	ctx := context.Background()
	lookup := userbudget.New("sqlite", db.TX)

	// Missing -> (false, nil).
	ok, err := lookup.Exists(ctx, budgetID)
	if err != nil {
		t.Fatalf("Exists missing: %v", err)
	}
	if ok {
		t.Error("want false for missing budget")
	}

	// Seed user + budget -> (true, nil).
	db.Exec(t, `INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active) VALUES (?, ?, '', 'u', '', '', '', ?, ?, 1)`,
		userA, userA, fixedTime, fixedTime)
	db.Exec(t, `INSERT INTO budgets (id, user_id, currency_id, name, started_at, created_at, updated_at) VALUES (?, ?, ?, 'B', ?, ?, ?)`,
		budgetID, userA, usdID, fixedTime, fixedTime, fixedTime)

	ok, err = lookup.Exists(ctx, budgetID)
	if err != nil {
		t.Fatalf("Exists present: %v", err)
	}
	if !ok {
		t.Error("want true for existing budget")
	}
}
