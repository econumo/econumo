package budgetrepo_test

// Integration tests for the UserLookup adapter (adapters.go) wired to the real
// user repository over a migrated in-memory SQLite.

import (
	"context"
	"testing"

	domuser "github.com/econumo/econumo/internal/domain/user"
	"github.com/econumo/econumo/internal/infra/clock"
	budgetrepo "github.com/econumo/econumo/internal/infra/repo/budget"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestUserLookup_CurrencyCode(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: userA, Name: "u"})
	f.DefaultOptions(userA) // seeds currency=USD among the standard options

	users := userrepo.NewRepo("sqlite", db.TX)
	lookup := budgetrepo.NewUserLookup(users, clock.New())

	code, err := lookup.CurrencyCode(context.Background(), userA)
	if err != nil {
		t.Fatalf("CurrencyCode: %v", err)
	}
	if code != "USD" {
		t.Errorf("want USD from the seeded currency option, got %q", code)
	}
}

func TestUserLookup_CurrencyCode_DefaultsWhenOptionMissing(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: userB, Name: "u"})
	// No DefaultOptions seeded: the currency option row is absent.

	users := userrepo.NewRepo("sqlite", db.TX)
	lookup := budgetrepo.NewUserLookup(users, clock.New())

	code, err := lookup.CurrencyCode(context.Background(), userB)
	if err != nil {
		t.Fatalf("CurrencyCode: %v", err)
	}
	if code != domuser.DefaultCurrency {
		t.Errorf("want the domain default currency, got %q", code)
	}
}

func TestUserLookup_CurrencyCode_InvalidID(t *testing.T) {
	db := dbtest.NewSQLite(t)
	users := userrepo.NewRepo("sqlite", db.TX)
	lookup := budgetrepo.NewUserLookup(users, clock.New())

	if _, err := lookup.CurrencyCode(context.Background(), "not-a-uuid"); err == nil {
		t.Error("want an error for a malformed user id")
	}
}
