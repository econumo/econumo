package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	existenceUSDID    = "dffc2a06-6f29-4704-8575-31709adee926"
	existenceUserA    = "11111111-1111-1111-1111-111111111111"
	existenceBudgetID = "b0d00000-0000-0000-0000-00000000b001"
)

var existenceFixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func TestUserBudgetExistence_Exists(t *testing.T) {
	db := dbtest.NewSQLite(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	lookup := server.NewUserBudgetExistence("sqlite", db.TX)

	// Missing -> (false, nil).
	ok, err := lookup.Exists(ctx, existenceBudgetID)
	if err != nil {
		t.Fatalf("Exists missing: %v", err)
	}
	if ok {
		t.Error("want false for missing budget")
	}

	// Seed user + budget -> (true, nil).
	f.User(fixture.User{ID: existenceUserA, Name: "u"})
	f.Budget(fixture.Budget{ID: existenceBudgetID, UserID: existenceUserA, CurrencyID: existenceUSDID, Name: "B", StartedAt: existenceFixedTime})

	ok, err = lookup.Exists(ctx, existenceBudgetID)
	if err != nil {
		t.Fatalf("Exists present: %v", err)
	}
	if !ok {
		t.Error("want true for existing budget")
	}
}
