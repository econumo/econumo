package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appaccount "github.com/econumo/econumo/internal/account"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appcategory "github.com/econumo/econumo/internal/category"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	apppayee "github.com/econumo/econumo/internal/payee"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	apptag "github.com/econumo/econumo/internal/tag"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
	apptransaction "github.com/econumo/econumo/internal/transaction"
	transactionmcp "github.com/econumo/econumo/internal/transaction/mcp"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func newTransactionService(t *testing.T, db *dbtest.DB) *apptransaction.Service {
	t.Helper()
	txm := db.TX
	curLookup := currencyrepo.New(db.Engine, txm)
	accessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, txm))
	accSvc := appaccount.NewService(
		accountrepo.NewRepo(db.Engine, txm), accountrepo.NewFolderRepo(db.Engine, txm),
		accountrepo.NewAccessRepo(db.Engine, txm),
		server.NewAccountCurrencyLookup(curLookup), server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm)),
		accessResolver, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(),
	)
	txRepo := transactionrepo.NewRepo(db.Engine, txm)

	// The importer backs the owned-entity checks on create/update, so it must be
	// wired for real here — a nil one panics as soon as a category/payee/tag is set.
	catRepo := categoryrepo.NewRepo(db.Engine, txm)
	tgRepo := tagrepo.NewRepo(db.Engine, txm)
	pyRepo := payeerepo.NewRepo(db.Engine, txm)
	catSvc := appcategory.NewService(catRepo, txm, catRepo, clock.New(), categoryrepo.NewReadRepo(db.Engine, txm), accessResolver)
	tgSvc := apptag.NewService(tgRepo, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(), tagrepo.NewReadRepo(db.Engine, txm), accessResolver)
	pySvc := apppayee.NewService(pyRepo, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(), payeerepo.NewReadRepo(db.Engine, txm), accessResolver)
	txImport := transactionrepo.NewImportLookup(
		server.NewTransactionImportAccounts(accSvc, accountrepo.NewRepo(db.Engine, txm), accountrepo.NewFolderRepo(db.Engine, txm), curLookup, "USD"),
		accessResolver,
		server.NewTransactionImportCategories(catSvc, catRepo),
		server.NewTransactionImportPayees(pySvc, pyRepo),
		server.NewTransactionImportTags(tgSvc, tgRepo),
		txRepo,
	)
	txExport := transactionrepo.NewExportLookup(
		txRepo,
		server.NewTransactionCategoryNameLookup(catRepo),
		server.NewTransactionTagNameLookup(tgRepo),
		server.NewTransactionPayeeNameLookup(pyRepo),
	)
	return apptransaction.NewService(
		txRepo, accSvc, accessResolver, accSvc,
		server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm)),
		txExport, txImport, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(),
	)
}

func connectSession(t *testing.T, ctx context.Context, svc *apptransaction.Service) *sdk.ClientSession {
	t.Helper()
	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	transactionmcp.Register(svc)(srv)

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

func TestTransactionTools_CreateBogusCategory_IsError(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	accountID := f.Account(fixture.Account{UserID: userID})

	svc := newTransactionService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, svc)

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "create_transaction",
		Arguments: map[string]any{
			"type":        "expense",
			"amount":      "12.50",
			"account_id":  accountID,
			"date":        "2024-04-02",
			"category_id": "00000000-0000-0000-0000-000000000000",
		},
	})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected isError, got: %#v", res)
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok || strings.TrimSpace(text.Text) == "" {
		t.Fatalf("expected non-empty error text: %#v", res.Content)
	}
	for _, leak := range []string{"sql", "driver", "goroutine", "panic", "modernc.org"} {
		if strings.Contains(strings.ToLower(text.Text), leak) {
			t.Fatalf("error text leaked internals (%q): %s", leak, text.Text)
		}
	}
}

func TestTransactionTools_FullFlow(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	folderID := f.Folder(fixture.Folder{UserID: userID})
	accountID := f.Account(fixture.Account{UserID: userID})
	f.AccountInFolder(folderID, accountID)
	f.AccountOption(accountID, userID, 0)
	categoryID := f.Category(fixture.Category{UserID: userID, Type: 0})

	svc := newTransactionService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, svc)

	createRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "create_transaction",
		Arguments: map[string]any{
			"type":        "expense",
			"amount":      "12.50",
			"account_id":  accountID,
			"date":        "2024-04-02",
			"category_id": categoryID,
		},
	})
	if err != nil {
		t.Fatalf("create: transport error: %v", err)
	}
	if createRes.IsError {
		t.Fatalf("create: unexpected error: %#v", createRes.Content)
	}
	createData := structured(t, createRes)
	item, ok := createData["item"].(map[string]any)
	if !ok {
		t.Fatalf("create: missing item: %#v", createData)
	}
	txID, _ := item["id"].(string)
	if txID == "" {
		t.Fatalf("create: empty item id: %#v", item)
	}
	if typ, _ := item["type"].(string); typ != "expense" {
		t.Fatalf("create: type = %q, want expense", typ)
	}

	listRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_transactions", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list: transport error: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("list: unexpected error: %#v", listRes.Content)
	}
	items, _ := structured(t, listRes)["items"].([]any)
	found := false
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && m["id"] == txID {
			found = true
		}
	}
	if !found {
		t.Fatalf("list: created transaction not found: %#v", items)
	}

	updateRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "update_transaction",
		Arguments: map[string]any{
			"id":          txID,
			"type":        "expense",
			"amount":      "99.99",
			"account_id":  accountID,
			"date":        "2024-04-02",
			"category_id": categoryID,
		},
	})
	if err != nil {
		t.Fatalf("update: transport error: %v", err)
	}
	if updateRes.IsError {
		t.Fatalf("update: unexpected error: %#v", updateRes.Content)
	}
	updItem, ok := structured(t, updateRes)["item"].(map[string]any)
	if !ok {
		t.Fatalf("update: missing item: %#v", structured(t, updateRes))
	}
	if amt, _ := updItem["amount"].(string); amt != "99.99" {
		t.Fatalf("update: amount = %q, want 99.99", amt)
	}

	deleteRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "delete_transaction",
		Arguments: map[string]any{"id": txID},
	})
	if err != nil {
		t.Fatalf("delete: transport error: %v", err)
	}
	if deleteRes.IsError {
		t.Fatalf("delete: unexpected error: %#v", deleteRes.Content)
	}

	listRes2, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_transactions", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list2: transport error: %v", err)
	}
	if listRes2.IsError {
		t.Fatalf("list2: unexpected error: %#v", listRes2.Content)
	}
	items2, _ := structured(t, listRes2)["items"].([]any)
	if len(items2) != 0 {
		t.Fatalf("list2: expected empty, got: %#v", items2)
	}
}

func TestListTransactionsFilters(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	accountID1 := f.Account(fixture.Account{UserID: userID})
	accountID2 := f.Account(fixture.Account{UserID: userID})
	categoryID := f.Category(fixture.Category{UserID: userID, Type: 0})

	svc := newTransactionService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, svc)

	// Create first transaction on accountID1 at 2024-04-01 15:30:00
	tx1Res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "create_transaction",
		Arguments: map[string]any{
			"type":        "expense",
			"amount":      "12.50",
			"account_id":  accountID1,
			"date":        "2024-04-01 15:30:00",
			"category_id": categoryID,
		},
	})
	if err != nil {
		t.Fatalf("create tx1: transport error: %v", err)
	}
	if tx1Res.IsError {
		t.Fatalf("create tx1: unexpected error: %#v", tx1Res.Content)
	}
	tx1ID := structured(t, tx1Res)["item"].(map[string]any)["id"].(string)

	// Create second transaction on accountID1 at 2024-04-03
	tx2Res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "create_transaction",
		Arguments: map[string]any{
			"type":        "expense",
			"amount":      "25.00",
			"account_id":  accountID1,
			"date":        "2024-04-03",
			"category_id": categoryID,
		},
	})
	if err != nil {
		t.Fatalf("create tx2: transport error: %v", err)
	}
	if tx2Res.IsError {
		t.Fatalf("create tx2: unexpected error: %#v", tx2Res.Content)
	}
	tx2ID := structured(t, tx2Res)["item"].(map[string]any)["id"].(string)

	// Test period filtering: query 2024-04-01 only should return only tx1
	// (expand("2024-04-01", false) = "2024-04-01 00:00:00", expand("2024-04-01", true) = "2024-04-01 23:59:59")
	// tx1 at 15:30:00 is within this range; tx2 at 2024-04-03 is outside
	listRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "list_transactions",
		Arguments: map[string]any{
			"period_start": "2024-04-01",
			"period_end":   "2024-04-01",
		},
	})
	if err != nil {
		t.Fatalf("list period filter: transport error: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("list period filter: unexpected error: %#v", listRes.Content)
	}
	items := structured(t, listRes)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("list period filter: expected 1 item, got %d: %#v", len(items), items)
	}
	foundID := items[0].(map[string]any)["id"].(string)
	if foundID != tx1ID {
		t.Fatalf("list period filter: expected tx1 (%q), got %q", tx1ID, foundID)
	}
	// Verify tx2 is NOT in the filtered results
	if foundID == tx2ID {
		t.Fatalf("list period filter: unexpectedly returned tx2 (%q)", tx2ID)
	}

	// Test account_id filtering: query accountID2 (no transactions) should return empty
	listRes2, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "list_transactions",
		Arguments: map[string]any{
			"account_id": accountID2,
		},
	})
	if err != nil {
		t.Fatalf("list account filter: transport error: %v", err)
	}
	if listRes2.IsError {
		t.Fatalf("list account filter: unexpected error: %#v", listRes2.Content)
	}
	items2 := structured(t, listRes2)["items"].([]any)
	if len(items2) != 0 {
		t.Fatalf("list account filter: expected empty, got %d items: %#v", len(items2), items2)
	}

	// Combined account_id + full period: accountID1 windowed to 2024-04-01
	// returns only tx1 (tx2 on the same account is outside the window).
	listRes3, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name: "list_transactions",
		Arguments: map[string]any{
			"account_id":   accountID1,
			"period_start": "2024-04-01",
			"period_end":   "2024-04-01",
		},
	})
	if err != nil {
		t.Fatalf("list account+period filter: transport error: %v", err)
	}
	if listRes3.IsError {
		t.Fatalf("list account+period filter: unexpected error: %#v", listRes3.Content)
	}
	items3 := structured(t, listRes3)["items"].([]any)
	if len(items3) != 1 {
		t.Fatalf("list account+period filter: expected 1 item, got %d: %#v", len(items3), items3)
	}
	if id := items3[0].(map[string]any)["id"].(string); id != tx1ID {
		t.Fatalf("list account+period filter: expected tx1 (%q), got %q", tx1ID, id)
	}
}

func TestListTransactionsFilters_InvalidCombos(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	accountID := f.Account(fixture.Account{UserID: userID})

	svc := newTransactionService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, svc)

	// A lone period bound is rejected, with or without account_id.
	for name, args := range map[string]map[string]any{
		"lone period_start":          {"period_start": "2024-04-01"},
		"lone period_end":            {"period_end": "2024-04-01"},
		"account_id plus lone bound": {"account_id": accountID, "period_start": "2024-04-01"},
	} {
		res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_transactions", Arguments: args})
		if err != nil {
			t.Fatalf("%s: transport error: %v", name, err)
		}
		if !res.IsError {
			t.Fatalf("%s: expected isError, got: %#v", name, res)
		}
		text, ok := res.Content[0].(*sdk.TextContent)
		if !ok || text.Text != "period_start and period_end must be provided together" {
			t.Fatalf("%s: unexpected message: %#v", name, res.Content)
		}
	}
}
