package server_test

import (
	"context"
	"testing"
	"time"

	dombudget "github.com/econumo/econumo/internal/budget"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	revokerUSDID   = "dffc2a06-6f29-4704-8575-31709adee926"
	revokerUserA   = "11111111-1111-1111-1111-111111111111"
	revokerUserB   = "22222222-2222-2222-2222-222222222222"
	revokerBudgetA = "b0d00000-0000-0000-0000-00000000b0a1"
)

var revokerFixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func seedRevokerUser(t *testing.T, f *fixture.Builder, id string) {
	t.Helper()
	f.User(fixture.User{ID: id, Name: "u"})
}

func TestConnectionBudgetRevoker_RevokeBetween(t *testing.T) {
	db := dbtest.NewSQLite(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	seedRevokerUser(t, f, revokerUserA)
	seedRevokerUser(t, f, revokerUserB)

	budgets := budgetrepo.NewRepo("sqlite", db.TX)
	revoker := server.NewConnectionBudgetRevoker(budgets)

	// userA owns budgetA; userB has an access grant on it.
	b := &dombudget.Budget{
		ID: vo.MustParseId(revokerBudgetA), UserID: vo.MustParseId(revokerUserA), Name: "Shared",
		CurrencyID: vo.MustParseId(revokerUSDID), StartedAt: revokerFixedTime, CreatedAt: revokerFixedTime, UpdatedAt: revokerFixedTime,
	}
	if err := budgets.Save(ctx, b); err != nil {
		t.Fatalf("Save budget: %v", err)
	}
	access := &dombudget.BudgetAccess{
		ID: vo.MustParseId(revokerBudgetA), BudgetID: vo.MustParseId(revokerBudgetA), UserID: vo.MustParseId(revokerUserB),
		Role: dombudget.RoleUser, IsAccepted: true, CreatedAt: revokerFixedTime, UpdatedAt: revokerFixedTime,
	}
	if err := budgets.SaveAccess(ctx, access); err != nil {
		t.Fatalf("SaveAccess: %v", err)
	}

	// Sanity: the grant exists.
	if list, _ := budgets.ListAccess(ctx, vo.MustParseId(revokerBudgetA)); len(list) != 1 {
		t.Fatalf("precondition: want 1 access, got %d", len(list))
	}

	if err := revoker.RevokeBetween(ctx, vo.MustParseId(revokerUserA), vo.MustParseId(revokerUserB)); err != nil {
		t.Fatalf("RevokeBetween: %v", err)
	}

	list, err := budgets.ListAccess(ctx, vo.MustParseId(revokerBudgetA))
	if err != nil {
		t.Fatalf("ListAccess after revoke: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("want budget access revoked, still %d grants", len(list))
	}
}
