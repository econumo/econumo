package server_test

// Integration tests for the CurrencyProfileCurrency glue adapter
// (glue_currency.go) wired to the real user repository over a migrated
// in-memory SQLite.

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

const (
	currencyGlueUserA = "33333333-3333-3333-3333-333333333333"
	currencyGlueUserB = "44444444-4444-4444-4444-444444444444"
)

func TestCurrencyProfileCurrency_CurrencyID(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: currencyGlueUserA, Name: "u"})
	f.DefaultOptions(currencyGlueUserA) // seeds the USD id among the standard options

	users := userrepo.NewRepo("sqlite", db.TX)
	profile := server.NewCurrencyProfileCurrency(users)

	id, err := profile.CurrencyID(context.Background(), currencyGlueUserA)
	if err != nil {
		t.Fatalf("CurrencyID: %v", err)
	}
	if id != fixture.USD {
		t.Errorf("want the seeded USD id from the currency option, got %q", id)
	}
}

func TestCurrencyProfileCurrency_CurrencyID_EmptyWhenOptionMissing(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: currencyGlueUserB, Name: "u"})
	// No DefaultOptions seeded: the currency option row is absent.

	users := userrepo.NewRepo("sqlite", db.TX)
	profile := server.NewCurrencyProfileCurrency(users)

	id, err := profile.CurrencyID(context.Background(), currencyGlueUserB)
	if err != nil {
		t.Fatalf("CurrencyID: %v", err)
	}
	if id != "" {
		t.Errorf(`want "" (no default set; guards skip), got %q`, id)
	}
}

func TestCurrencyProfileCurrency_CurrencyID_InvalidID(t *testing.T) {
	db := dbtest.NewSQLite(t)
	users := userrepo.NewRepo("sqlite", db.TX)
	profile := server.NewCurrencyProfileCurrency(users)

	if _, err := profile.CurrencyID(context.Background(), "not-a-uuid"); err == nil {
		t.Error("want an error for a malformed user id")
	}
}
