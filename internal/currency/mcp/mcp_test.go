package mcp_test

import (
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appcurrency "github.com/econumo/econumo/internal/currency"
	currencymcp "github.com/econumo/econumo/internal/currency/mcp"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
)

func newCurrencyReadService(t *testing.T, db *dbtest.DB) *appcurrency.ReadService {
	t.Helper()
	return appcurrency.NewReadService(currencyrepo.NewReadRepo(db.Engine, db.TX))
}

func TestCurrenciesResource(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	eurID := f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	f.Rate(fixture.Rate{CurrencyID: eurID, Rate: "0.85000000"})

	read := newCurrencyReadService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	currencymcp.Register(read)(srv)

	ctx := mcptest.CtxWithUser(t, userID)

	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()

	client := sdk.NewClient(&sdk.Implementation{Name: "c", Version: "t"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: "econumo://currencies"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("contents: %+v", res.Contents)
	}
	text := res.Contents[0].Text
	if !strings.Contains(text, `"EUR"`) {
		t.Fatalf("expected EUR in resource text: %s", text)
	}
	if !strings.Contains(text, `"rates"`) || !strings.Contains(text, `"0.85`) {
		t.Fatalf("expected rates in resource text: %s", text)
	}
}
