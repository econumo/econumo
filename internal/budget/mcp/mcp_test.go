package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appbudget "github.com/econumo/econumo/internal/budget"
	budgetmcp "github.com/econumo/econumo/internal/budget/mcp"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	domcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func newBudgetService(t *testing.T, db *dbtest.DB) *appbudget.Service {
	t.Helper()
	txm := db.TX
	userRepo := userrepo.NewRepo(db.Engine, txm)
	accountRepo := accountrepo.NewRepo(db.Engine, txm)
	categoryRepo := categoryrepo.NewRepo(db.Engine, txm)
	tagRepo := tagrepo.NewRepo(db.Engine, txm)
	payeeRepo := payeerepo.NewRepo(db.Engine, txm)
	currencyLookup := currencyrepo.New(db.Engine, txm)

	budgetRepo := budgetrepo.NewRepo(db.Engine, txm)
	budgetReadRepo := budgetrepo.NewReadRepo(db.Engine, txm)
	rateProvider := currencyrepo.NewRateProvider(db.Engine, txm, currencyLookup, "USD")
	convertor := domcurrency.NewConvertor(rateProvider)
	clk := clock.New()
	return appbudget.NewService(
		budgetRepo, budgetReadRepo, convertor, rateProvider,
		server.NewBudgetUserLookup(userRepo, clk),
		server.NewBudgetAccountLookup(accountRepo),
		currencyLookup,
		budgetrepo.NewMetadataLookup(
			server.NewBudgetCategoryMetadataLookup(categoryRepo),
			server.NewBudgetTagMetadataLookup(tagRepo),
			server.NewBudgetPayeeMetadataLookup(payeeRepo),
		),
		txm, clk,
	)
}

func connectBudgetSession(t *testing.T, ctx context.Context, svc *appbudget.Service) *sdk.ClientSession {
	t.Helper()
	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	budgetmcp.Register(svc)(srv)

	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	client := sdk.NewClient(&sdk.Implementation{Name: "c", Version: "t"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func structured(t *testing.T, res *sdk.CallToolResult) map[string]any {
	t.Helper()
	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent is not a map: %#v", res.StructuredContent)
	}
	return m
}

func TestBudgetsResourceAndGetBudgetTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	budgetID := f.Budget(fixture.Budget{UserID: userID, Name: "Household"})

	svc := newBudgetService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectBudgetSession(t, ctx, svc)

	res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: "econumo://budgets"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("contents: %+v", res.Contents)
	}
	text := res.Contents[0].Text
	if !strings.Contains(text, `"Household"`) {
		t.Fatalf("expected budget name in resource text: %s", text)
	}
	if !strings.Contains(text, budgetID) {
		t.Fatalf("expected budget id in resource text: %s", text)
	}

	getRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "get_budget",
		Arguments: map[string]any{
			"budget_id": budgetID,
			"month":     "2024-04",
		},
	})
	if err != nil {
		t.Fatalf("get_budget: transport error: %v", err)
	}
	if getRes.IsError {
		t.Fatalf("get_budget: unexpected error: %#v", getRes.Content)
	}
	item, ok := structured(t, getRes)["item"].(map[string]any)
	if !ok {
		t.Fatalf("get_budget: missing item: %#v", structured(t, getRes))
	}
	meta, ok := item["meta"].(map[string]any)
	if !ok {
		t.Fatalf("get_budget: missing item.meta: %#v", item)
	}
	if id, _ := meta["id"].(string); id != budgetID {
		t.Fatalf("get_budget: item.meta.id = %q, want %q", id, budgetID)
	}

	badRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "get_budget",
		Arguments: map[string]any{
			"budget_id": budgetID,
			"month":     "junk",
		},
	})
	if err != nil {
		t.Fatalf("get_budget bad month: transport error: %v", err)
	}
	if !badRes.IsError {
		t.Fatalf("get_budget bad month: expected isError, got: %#v", badRes)
	}
	badText, ok := badRes.Content[0].(*sdk.TextContent)
	if !ok || !strings.Contains(badText.Text, "month must be YYYY-MM") {
		t.Fatalf("get_budget bad month: expected validation message, got: %#v", badRes.Content)
	}
}
