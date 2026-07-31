package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	existenceUSDID    = "dffc2a06-6f29-4704-8575-31709adee926"
	existenceUserA    = "11111111-1111-1111-1111-111111111111"
	existenceUserB    = "22222222-2222-2222-2222-222222222222"
	existenceBudgetID = "b0d00000-0000-0000-0000-00000000b001"
)

var existenceFixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func TestUserBudgetAccess_HasAccess(t *testing.T) {
	db := dbtest.NewSQLite(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	lookup := server.NewUserBudgetAccess("sqlite", db.TX)
	userA := vo.MustParseId(existenceUserA)
	userB := vo.MustParseId(existenceUserB)

	// Missing budget -> (false, nil).
	if ok, err := lookup.HasAccess(ctx, userA, existenceBudgetID); err != nil || ok {
		t.Fatalf("HasAccess missing = (%v, %v), want (false, nil)", ok, err)
	}

	f.User(fixture.User{ID: existenceUserA, Name: "a"})
	f.User(fixture.User{ID: existenceUserB, Name: "b"})
	f.Budget(fixture.Budget{ID: existenceBudgetID, UserID: existenceUserA, CurrencyID: existenceUSDID, Name: "B", StartedAt: existenceFixedTime})

	// Owner -> true.
	if ok, err := lookup.HasAccess(ctx, userA, existenceBudgetID); err != nil || !ok {
		t.Fatalf("owner HasAccess = (%v, %v), want (true, nil)", ok, err)
	}

	// A stranger (no share) -> false: a foreign budget id must not be storable.
	if ok, err := lookup.HasAccess(ctx, userB, existenceBudgetID); err != nil || ok {
		t.Fatalf("stranger HasAccess = (%v, %v), want (false, nil)", ok, err)
	}

	// A pending (unaccepted) share -> still false.
	f.BudgetAccess(existenceBudgetID, existenceUserB, 1, false)
	if ok, err := lookup.HasAccess(ctx, userB, existenceBudgetID); err != nil || ok {
		t.Fatalf("pending-share HasAccess = (%v, %v), want (false, nil)", ok, err)
	}

	// Accepting the share -> true.
	if _, err := db.Raw.Exec(db.Rebind(`UPDATE budgets_access SET is_accepted = 1 WHERE budget_id = ? AND user_id = ?`), existenceBudgetID, existenceUserB); err != nil {
		t.Fatalf("accept share: %v", err)
	}
	if ok, err := lookup.HasAccess(ctx, userB, existenceBudgetID); err != nil || !ok {
		t.Fatalf("accepted-share HasAccess = (%v, %v), want (true, nil)", ok, err)
	}
}
