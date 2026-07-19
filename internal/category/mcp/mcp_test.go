package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appcategory "github.com/econumo/econumo/internal/category"
	categorymcp "github.com/econumo/econumo/internal/category/mcp"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
)

func newReadService(t *testing.T, db *dbtest.DB) *appcategory.ReadService {
	t.Helper()
	return appcategory.NewReadService(categoryrepo.NewReadRepo(db.Engine, db.TX))
}

func newWriteService(t *testing.T, db *dbtest.DB) *appcategory.Service {
	t.Helper()
	txm := db.TX
	repo := categoryrepo.NewRepo(db.Engine, txm)
	accessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, txm))
	return appcategory.NewService(repo, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(), categoryrepo.NewReadRepo(db.Engine, txm), accessResolver)
}

func connectSession(t *testing.T, ctx context.Context, read *appcategory.ReadService, write *appcategory.Service) *sdk.ClientSession {
	t.Helper()
	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	categorymcp.Register(read, write)(srv)

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

func TestListCategoriesTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	f.Category(fixture.Category{UserID: userID, Name: "Groceries"})

	read := newReadService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	categorymcp.Register(read, newWriteService(t, db))(srv)

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

	toolRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_categories", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list_categories: transport error: %v", err)
	}
	if toolRes.IsError {
		t.Fatalf("list_categories: unexpected error: %#v", toolRes.Content)
	}
	m, ok := toolRes.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("list_categories: structuredContent is not a map: %#v", toolRes.StructuredContent)
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("list_categories: missing items: %#v", m)
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["name"] != "Groceries" {
		t.Fatalf("list_categories: expected Groceries category, got: %#v", items)
	}
}

func TestCategoryTools_FullFlow(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	read := newReadService(t, db)
	write := newWriteService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, read, write)

	createRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_category",
		Arguments: map[string]any{"name": "Utilities", "type": "expense", "icon": "bolt"},
	})
	if err != nil {
		t.Fatalf("create: transport error: %v", err)
	}
	if createRes.IsError {
		t.Fatalf("create: unexpected error: %#v", createRes.Content)
	}
	item, ok := structured(t, createRes)["item"].(map[string]any)
	if !ok {
		t.Fatalf("create: missing item: %#v", structured(t, createRes))
	}
	id, _ := item["id"].(string)
	if id == "" {
		t.Fatalf("create: empty item id: %#v", item)
	}

	listRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_categories", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list: transport error: %v", err)
	}
	items, _ := structured(t, listRes)["items"].([]any)
	found := false
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && m["id"] == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("list: created category not found: %#v", items)
	}

	updateRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "update_category",
		Arguments: map[string]any{"id": id, "name": "Utilities Renamed", "icon": "flash_on"},
	})
	if err != nil {
		t.Fatalf("update: transport error: %v", err)
	}
	if updateRes.IsError {
		t.Fatalf("update: unexpected error: %#v", updateRes.Content)
	}

	listRes2, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_categories", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list2: transport error: %v", err)
	}
	items2, _ := structured(t, listRes2)["items"].([]any)
	var updated map[string]any
	for _, it := range items2 {
		if m, ok := it.(map[string]any); ok && m["id"] == id {
			updated = m
		}
	}
	if updated == nil {
		t.Fatalf("list2: updated category not found: %#v", items2)
	}
	if updated["name"] != "Utilities Renamed" || updated["icon"] != "flash_on" {
		t.Fatalf("update: unexpected fields: %#v", updated)
	}

	archiveRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "set_category_archived",
		Arguments: map[string]any{"id": id, "archived": true},
	})
	if err != nil {
		t.Fatalf("archive: transport error: %v", err)
	}
	if archiveRes.IsError {
		t.Fatalf("archive: unexpected error: %#v", archiveRes.Content)
	}
	archData := structured(t, archiveRes)
	if archData["id"] != id || archData["archived"] != true {
		t.Fatalf("archive: unexpected result: %#v", archData)
	}

	listRes3, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_categories", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list3: transport error: %v", err)
	}
	items3, _ := structured(t, listRes3)["items"].([]any)
	var archived map[string]any
	for _, it := range items3 {
		if m, ok := it.(map[string]any); ok && m["id"] == id {
			archived = m
		}
	}
	if archived == nil {
		t.Fatalf("list3: archived category not found: %#v", items3)
	}
	isArchived, _ := archived["isArchived"].(float64)
	if isArchived != 1 {
		t.Fatalf("archive: expected isArchived 1, got: %#v", archived["isArchived"])
	}
}

func TestCategoryTools_CreateShortName_IsError(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	read := newReadService(t, db)
	write := newWriteService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, read, write)

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_category",
		Arguments: map[string]any{"name": "ab", "type": "expense"},
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
	if !strings.Contains(text.Text, "3-64") {
		t.Fatalf("expected localized length-validation message, got: %s", text.Text)
	}
	for _, leak := range []string{"sql", "driver", "goroutine", "panic", "modernc.org"} {
		if strings.Contains(strings.ToLower(text.Text), leak) {
			t.Fatalf("error text leaked internals (%q): %s", leak, text.Text)
		}
	}
}
