package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appaccount "github.com/econumo/econumo/internal/account"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/server"
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
	accSvc := appaccount.NewService(
		accountrepo.NewRepo(db.Engine, txm), accountrepo.NewFolderRepo(db.Engine, txm),
		server.NewAccountCurrencyLookup(curLookup), server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm)),
		nil, nil, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(),
	)
	txRepo := transactionrepo.NewRepo(db.Engine, txm)
	accessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, txm))
	return apptransaction.NewService(
		txRepo, accSvc, accessResolver, accSvc,
		server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm)),
		nil, nil, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(),
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
