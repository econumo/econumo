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
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
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
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, txm)),
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

func TestBudgetTools(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	budgetID := f.Budget(fixture.Budget{UserID: userID, Name: "Household"})

	svc := newBudgetService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectBudgetSession(t, ctx, svc)

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

	listRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_budgets", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list_budgets: transport error: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("list_budgets: unexpected error: %#v", listRes.Content)
	}
	items, ok := structured(t, listRes)["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("list_budgets: missing items: %#v", structured(t, listRes))
	}
	listItem, ok := items[0].(map[string]any)
	if !ok || listItem["name"] != "Household" {
		t.Fatalf("list_budgets: expected Household budget, got: %#v", items)
	}
}

func TestBudgetTools_BuildFlow(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	folderID := f.Folder(fixture.Folder{UserID: userID})
	accountID := f.Account(fixture.Account{UserID: userID})
	f.AccountInFolder(folderID, accountID)
	f.AccountOption(accountID, userID, 0)
	categoryID := f.Category(fixture.Category{UserID: userID, Type: 0})

	svc := newBudgetService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectBudgetSession(t, ctx, svc)

	createBudgetRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "create_budget",
		Arguments: map[string]any{
			"name":        "Groceries Budget",
			"currency_id": fixture.USD,
			"start_date":  "2024-04-01",
		},
	})
	if err != nil {
		t.Fatalf("create_budget: transport error: %v", err)
	}
	if createBudgetRes.IsError {
		t.Fatalf("create_budget: unexpected error: %#v", createBudgetRes.Content)
	}
	budgetItem, ok := structured(t, createBudgetRes)["item"].(map[string]any)
	if !ok {
		t.Fatalf("create_budget: missing item: %#v", structured(t, createBudgetRes))
	}
	budgetMeta, ok := budgetItem["meta"].(map[string]any)
	if !ok {
		t.Fatalf("create_budget: missing item.meta: %#v", budgetItem)
	}
	budgetID, _ := budgetMeta["id"].(string)
	if budgetID == "" {
		t.Fatalf("create_budget: empty budget id: %#v", budgetMeta)
	}
	if budgetMeta["name"] != "Groceries Budget" {
		t.Fatalf("create_budget: unexpected name: %#v", budgetMeta)
	}

	createFolderRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_folder",
		Arguments: map[string]any{"budget_id": budgetID, "name": "Bills"},
	})
	if err != nil {
		t.Fatalf("create_folder: transport error: %v", err)
	}
	if createFolderRes.IsError {
		t.Fatalf("create_folder: unexpected error: %#v", createFolderRes.Content)
	}
	folderItem, ok := structured(t, createFolderRes)["item"].(map[string]any)
	if !ok {
		t.Fatalf("create_folder: missing item: %#v", structured(t, createFolderRes))
	}
	elementFolderID, _ := folderItem["id"].(string)
	if elementFolderID == "" {
		t.Fatalf("create_folder: empty folder id: %#v", folderItem)
	}

	createEnvelopeRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "create_envelope",
		Arguments: map[string]any{
			"budget_id":    budgetID,
			"name":         "Groceries",
			"icon":         "cart",
			"currency_id":  fixture.USD,
			"folder_id":    elementFolderID,
			"category_ids": []string{categoryID},
		},
	})
	if err != nil {
		t.Fatalf("create_envelope: transport error: %v", err)
	}
	if createEnvelopeRes.IsError {
		t.Fatalf("create_envelope: unexpected error: %#v", createEnvelopeRes.Content)
	}
	envelopeItem, ok := structured(t, createEnvelopeRes)["item"].(map[string]any)
	if !ok {
		t.Fatalf("create_envelope: missing item: %#v", structured(t, createEnvelopeRes))
	}
	envelopeID, _ := envelopeItem["id"].(string)
	if envelopeID == "" {
		t.Fatalf("create_envelope: empty envelope id: %#v", envelopeItem)
	}
	children, ok := envelopeItem["children"].([]any)
	if !ok || len(children) != 1 {
		t.Fatalf("create_envelope: expected 1 child category, got: %#v", envelopeItem["children"])
	}
	if child, ok := children[0].(map[string]any); !ok || child["id"] != categoryID {
		t.Fatalf("create_envelope: expected child category %q, got: %#v", categoryID, children[0])
	}

	setLimitRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "set_limit",
		Arguments: map[string]any{
			"budget_id":  budgetID,
			"element_id": envelopeID,
			"month":      "2024-04",
			"amount":     "150.00",
		},
	})
	if err != nil {
		t.Fatalf("set_limit: transport error: %v", err)
	}
	if setLimitRes.IsError {
		t.Fatalf("set_limit: unexpected error: %#v", setLimitRes.Content)
	}
	limitData := structured(t, setLimitRes)
	if limitData["budget_id"] != budgetID || limitData["element_id"] != envelopeID ||
		limitData["month"] != "2024-04" || limitData["amount"] != "150.00" {
		t.Fatalf("set_limit: unexpected confirmation: %#v", limitData)
	}

	toggleRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "set_budget_account_included",
		Arguments: map[string]any{
			"budget_id":  budgetID,
			"account_id": accountID,
			"included":   false,
		},
	})
	if err != nil {
		t.Fatalf("set_budget_account_included: transport error: %v", err)
	}
	if toggleRes.IsError {
		t.Fatalf("set_budget_account_included: unexpected error: %#v", toggleRes.Content)
	}
	toggleData := structured(t, toggleRes)
	if toggleData["budget_id"] != budgetID || toggleData["account_id"] != accountID || toggleData["included"] != false {
		t.Fatalf("set_budget_account_included: unexpected confirmation: %#v", toggleData)
	}

	updateFolderRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "update_folder",
		Arguments: map[string]any{"budget_id": budgetID, "id": elementFolderID, "name": "Bills Renamed"},
	})
	if err != nil {
		t.Fatalf("update_folder: transport error: %v", err)
	}
	if updateFolderRes.IsError {
		t.Fatalf("update_folder: unexpected error: %#v", updateFolderRes.Content)
	}

	updateEnvelopeRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "update_envelope",
		Arguments: map[string]any{
			"budget_id":    budgetID,
			"id":           envelopeID,
			"name":         "Groceries Renamed",
			"icon":         "cart",
			"currency_id":  fixture.USD,
			"category_ids": []string{categoryID},
			"archived":     false,
		},
	})
	if err != nil {
		t.Fatalf("update_envelope: transport error: %v", err)
	}
	if updateEnvelopeRes.IsError {
		t.Fatalf("update_envelope: unexpected error: %#v", updateEnvelopeRes.Content)
	}

	// update_budget must NOT silently re-include the account excluded above:
	// ExcludedAccounts is authoritative on the wire (internal/budget/crud.go), so
	// the tool must round-trip the current excluded set.
	updateBudgetRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "update_budget",
		Arguments: map[string]any{
			"budget_id":   budgetID,
			"name":        "Groceries Budget Renamed",
			"currency_id": fixture.USD,
		},
	})
	if err != nil {
		t.Fatalf("update_budget: transport error: %v", err)
	}
	if updateBudgetRes.IsError {
		t.Fatalf("update_budget: unexpected error: %#v", updateBudgetRes.Content)
	}

	getRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "get_budget",
		Arguments: map[string]any{"budget_id": budgetID, "month": "2024-04"},
	})
	if err != nil {
		t.Fatalf("get_budget: transport error: %v", err)
	}
	if getRes.IsError {
		t.Fatalf("get_budget: unexpected error: %#v", getRes.Content)
	}
	getItem, ok := structured(t, getRes)["item"].(map[string]any)
	if !ok {
		t.Fatalf("get_budget: missing item: %#v", structured(t, getRes))
	}
	gotMeta, ok := getItem["meta"].(map[string]any)
	if !ok || gotMeta["name"] != "Groceries Budget Renamed" {
		t.Fatalf("get_budget: expected renamed budget, got: %#v", getItem["meta"])
	}
	filters, ok := getItem["filters"].(map[string]any)
	if !ok {
		t.Fatalf("get_budget: missing filters: %#v", getItem)
	}
	excluded, ok := filters["excludedAccountsIds"].([]any)
	if !ok || len(excluded) != 1 || excluded[0] != accountID {
		t.Fatalf("get_budget: expected account %q to stay excluded after update_budget, got: %#v", accountID, filters["excludedAccountsIds"])
	}
	structureData, ok := getItem["structure"].(map[string]any)
	if !ok {
		t.Fatalf("get_budget: missing structure: %#v", getItem)
	}
	folders, ok := structureData["folders"].([]any)
	if !ok || len(folders) == 0 {
		t.Fatalf("get_budget: missing folders: %#v", structureData)
	}
	foundFolder := false
	for _, fo := range folders {
		if m, ok := fo.(map[string]any); ok && m["id"] == elementFolderID && m["name"] == "Bills Renamed" {
			foundFolder = true
		}
	}
	if !foundFolder {
		t.Fatalf("get_budget: renamed folder not found: %#v", folders)
	}
	elements, ok := structureData["elements"].([]any)
	if !ok {
		t.Fatalf("get_budget: missing elements: %#v", structureData)
	}
	foundEnvelope := false
	for _, el := range elements {
		m, ok := el.(map[string]any)
		if !ok || m["id"] != envelopeID {
			continue
		}
		foundEnvelope = true
		if m["name"] != "Groceries Renamed" {
			t.Fatalf("get_budget: envelope name mismatch: %#v", m)
		}
		if m["budgeted"] != "150.00" && m["budgeted"] != "150" {
			t.Fatalf("get_budget: expected the set limit to show as budgeted, got: %#v", m["budgeted"])
		}
	}
	if !foundEnvelope {
		t.Fatalf("get_budget: renamed envelope not found: %#v", elements)
	}
}

func TestMoveElementTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	accountID := f.Account(fixture.Account{UserID: userID})
	f.AccountOption(accountID, userID, 0)
	categoryID := f.Category(fixture.Category{UserID: userID, Type: 0})

	svc := newBudgetService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectBudgetSession(t, ctx, svc)

	createBudgetRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_budget",
		Arguments: map[string]any{"name": "Bud", "currency_id": fixture.USD, "start_date": "2024-04-01"},
	})
	if err != nil || createBudgetRes.IsError {
		t.Fatalf("create_budget: %v %#v", err, createBudgetRes)
	}
	budgetID, _ := structured(t, createBudgetRes)["item"].(map[string]any)["meta"].(map[string]any)["id"].(string)

	folderRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_folder",
		Arguments: map[string]any{"budget_id": budgetID, "name": "Bills"},
	})
	if err != nil || folderRes.IsError {
		t.Fatalf("create_folder: %v %#v", err, folderRes)
	}
	folderID, _ := structured(t, folderRes)["item"].(map[string]any)["id"].(string)

	moveRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "move_element",
		Arguments: map[string]any{
			"budget_id": budgetID,
			"items":     []any{map[string]any{"element_id": categoryID, "folder_id": folderID, "position": 0}},
		},
	})
	if err != nil {
		t.Fatalf("move_element: transport error: %v", err)
	}
	if moveRes.IsError {
		t.Fatalf("move_element: unexpected error: %#v", moveRes.Content)
	}
	if got := structured(t, moveRes)["moved"]; got != float64(1) {
		t.Fatalf("move_element: moved = %#v, want 1", got)
	}

	getRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "get_budget",
		Arguments: map[string]any{"budget_id": budgetID, "month": "2024-04"},
	})
	if err != nil || getRes.IsError {
		t.Fatalf("get_budget: %v %#v", err, getRes)
	}
	structure := structured(t, getRes)["item"].(map[string]any)["structure"].(map[string]any)
	elements, _ := structure["elements"].([]any)
	found := false
	for _, e := range elements {
		el := e.(map[string]any)
		if el["id"] == categoryID {
			found = true
			if el["folderId"] != folderID {
				t.Fatalf("element folderId = %#v, want %q", el["folderId"], folderID)
			}
		}
	}
	if !found {
		t.Fatalf("moved category not found in structure: %#v", elements)
	}
}

func TestBudgetTools_SetLimitBeforeStart_IsError(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	svc := newBudgetService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectBudgetSession(t, ctx, svc)

	createBudgetRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "create_budget",
		Arguments: map[string]any{
			"name":        "Household",
			"currency_id": fixture.USD,
			"start_date":  "2024-04-01",
		},
	})
	if err != nil {
		t.Fatalf("create_budget: transport error: %v", err)
	}
	if createBudgetRes.IsError {
		t.Fatalf("create_budget: unexpected error: %#v", createBudgetRes.Content)
	}
	budgetItem, ok := structured(t, createBudgetRes)["item"].(map[string]any)
	if !ok {
		t.Fatalf("create_budget: missing item: %#v", structured(t, createBudgetRes))
	}
	budgetMeta, ok := budgetItem["meta"].(map[string]any)
	if !ok {
		t.Fatalf("create_budget: missing item.meta: %#v", budgetItem)
	}
	budgetID, _ := budgetMeta["id"].(string)
	if budgetID == "" {
		t.Fatalf("create_budget: empty budget id: %#v", budgetMeta)
	}

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "set_limit",
		Arguments: map[string]any{
			"budget_id":  budgetID,
			"element_id": "00000000-0000-0000-0000-000000000001",
			"month":      "2024-01",
			"amount":     "10.00",
		},
	})
	if err != nil {
		t.Fatalf("set_limit: transport error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("set_limit: expected isError for a month before the budget start, got: %#v", res)
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok || strings.TrimSpace(text.Text) == "" {
		t.Fatalf("set_limit: expected non-empty error text: %#v", res.Content)
	}
	for _, leak := range []string{"sql", "driver", "goroutine", "panic", "modernc.org"} {
		if strings.Contains(strings.ToLower(text.Text), leak) {
			t.Fatalf("set_limit: error text leaked internals (%q): %s", leak, text.Text)
		}
	}
}
