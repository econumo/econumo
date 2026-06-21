package connectionrepo_test

// Integration test for the BudgetAccessRevoker: it removes budget-sharing
// between two users in both directions, driven by a real budget repo against a
// migrated in-memory SQLite.

import (
	"context"
	"testing"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	budgetrepo "github.com/econumo/econumo/internal/infra/repo/budget"
	connectionrepo "github.com/econumo/econumo/internal/infra/repo/connection"
	"github.com/econumo/econumo/internal/test/dbtest"
)

const budgetA = "b0d00000-0000-0000-0000-00000000b0a1"

func TestBudgetAccessRevoker_RevokeBetween(t *testing.T) {
	db := dbtest.NewSQLite(t)
	ctx := context.Background()
	seedUser(t, db, userA)
	seedUser(t, db, userB)

	budgets := budgetrepo.NewRepo("sqlite", db.TX)
	revoker := connectionrepo.NewBudgetAccessRevoker(budgets)

	// userA owns budgetA; userB has an access grant on it.
	b := dombudget.FromState(vo.MustParseId(budgetA), vo.MustParseId(userA), "Shared", vo.MustParseId(usdID), fixedTime, fixedTime, fixedTime)
	if err := budgets.Save(ctx, b); err != nil {
		t.Fatalf("Save budget: %v", err)
	}
	access := dombudget.AccessFromState(vo.MustParseId(budgetA), vo.MustParseId(budgetA), vo.MustParseId(userB), dombudget.RoleUser, true, fixedTime, fixedTime)
	if err := budgets.SaveAccess(ctx, access); err != nil {
		t.Fatalf("SaveAccess: %v", err)
	}

	// Sanity: the grant exists.
	if list, _ := budgets.ListAccess(ctx, vo.MustParseId(budgetA)); len(list) != 1 {
		t.Fatalf("precondition: want 1 access, got %d", len(list))
	}

	if err := revoker.RevokeBetween(ctx, vo.MustParseId(userA), vo.MustParseId(userB)); err != nil {
		t.Fatalf("RevokeBetween: %v", err)
	}

	list, err := budgets.ListAccess(ctx, vo.MustParseId(budgetA))
	if err != nil {
		t.Fatalf("ListAccess after revoke: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("want budget access revoked, still %d grants", len(list))
	}
}
