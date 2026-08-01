package server_test

// Integration tests for the BudgetUserLookup glue adapter
// (glue_budget_userlookup.go) wired to the real user repository over a
// migrated in-memory SQLite.

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

const (
	glueUserA = "11111111-1111-1111-1111-111111111111"
	glueUserB = "22222222-2222-2222-2222-222222222222"
)

func TestBudgetUserLookup_CurrencyCode(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: glueUserA, Name: "u"})
	f.DefaultOptions(glueUserA) // seeds currency=USD among the standard options

	users := userrepo.NewRepo("sqlite", db.TX)
	lookup := server.NewBudgetUserLookup(users, clock.New())

	id, err := lookup.DefaultCurrencyID(context.Background(), glueUserA)
	if err != nil {
		t.Fatalf("DefaultCurrencyID: %v", err)
	}
	if id != fixture.USD {
		t.Errorf("want the seeded USD id from the currency option, got %q", id)
	}
}

// The migration guarantees every user holds a currency option; a user without
// one is data corruption and must surface as an error, not a silent fallback.
func TestBudgetUserLookup_CurrencyCode_ErrorWhenOptionMissing(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: glueUserB, Name: "u"})
	// No DefaultOptions seeded: the currency option row is absent.

	users := userrepo.NewRepo("sqlite", db.TX)
	lookup := server.NewBudgetUserLookup(users, clock.New())

	if _, err := lookup.DefaultCurrencyID(context.Background(), glueUserB); err == nil {
		t.Error("want an error for a user without a currency option")
	}
}

func TestBudgetUserLookup_CurrencyCode_InvalidID(t *testing.T) {
	db := dbtest.NewSQLite(t)
	users := userrepo.NewRepo("sqlite", db.TX)
	lookup := server.NewBudgetUserLookup(users, clock.New())

	if _, err := lookup.DefaultCurrencyID(context.Background(), "not-a-uuid"); err == nil {
		t.Error("want an error for a malformed user id")
	}
}
