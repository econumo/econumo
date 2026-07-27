package mcp_test

import (
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

func TestListCurrenciesTool(t *testing.T) {
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

	toolRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_currencies", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list_currencies: transport error: %v", err)
	}
	if toolRes.IsError {
		t.Fatalf("list_currencies: unexpected error: %#v", toolRes.Content)
	}
	m, ok := toolRes.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("list_currencies: structuredContent is not a map: %#v", toolRes.StructuredContent)
	}
	currencies, ok := m["currencies"].([]any)
	if !ok || len(currencies) == 0 {
		t.Fatalf("list_currencies: missing currencies: %#v", m)
	}
	found := false
	for _, c := range currencies {
		cm, ok := c.(map[string]any)
		if ok && cm["code"] == "EUR" {
			found = true
		}
	}
	if !found {
		t.Fatalf("list_currencies: expected EUR currency, got: %#v", currencies)
	}
	rates, ok := m["rates"].([]any)
	if !ok || len(rates) == 0 {
		t.Fatalf("list_currencies: missing rates: %#v", m)
	}
}
