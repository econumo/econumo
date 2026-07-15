package server_test

// Integration tests for the CurrencyProfileCurrency glue adapter
// (glue_currency.go) wired to the real user repository over a migrated
// in-memory SQLite.

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

const (
	currencyGlueUserA = "33333333-3333-3333-3333-333333333333"
	currencyGlueUserB = "44444444-4444-4444-4444-444444444444"
)

func TestCurrencyProfileCurrency_CurrencyCode(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: currencyGlueUserA, Name: "u"})
	f.DefaultOptions(currencyGlueUserA) // seeds currency=USD among the standard options

	users := userrepo.NewRepo("sqlite", db.TX)
	profile := server.NewCurrencyProfileCurrency(users)

	code, err := profile.CurrencyCode(context.Background(), currencyGlueUserA)
	if err != nil {
		t.Fatalf("CurrencyCode: %v", err)
	}
	if code != "USD" {
		t.Errorf("want USD from the seeded currency option, got %q", code)
	}
}

func TestCurrencyProfileCurrency_CurrencyCode_DefaultsWhenOptionMissing(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: currencyGlueUserB, Name: "u"})
	// No DefaultOptions seeded: the currency option row is absent.

	users := userrepo.NewRepo("sqlite", db.TX)
	profile := server.NewCurrencyProfileCurrency(users)

	code, err := profile.CurrencyCode(context.Background(), currencyGlueUserB)
	if err != nil {
		t.Fatalf("CurrencyCode: %v", err)
	}
	if code != model.DefaultCurrency {
		t.Errorf("want the domain default currency, got %q", code)
	}
}

func TestCurrencyProfileCurrency_CurrencyCode_InvalidID(t *testing.T) {
	db := dbtest.NewSQLite(t)
	users := userrepo.NewRepo("sqlite", db.TX)
	profile := server.NewCurrencyProfileCurrency(users)

	if _, err := profile.CurrencyCode(context.Background(), "not-a-uuid"); err == nil {
		t.Error("want an error for a malformed user id")
	}
}
