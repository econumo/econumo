package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	apptag "github.com/econumo/econumo/internal/tag"
	tagmcp "github.com/econumo/econumo/internal/tag/mcp"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
)

func newReadService(t *testing.T, db *dbtest.DB) *apptag.ReadService {
	t.Helper()
	return apptag.NewReadService(tagrepo.NewReadRepo(db.Engine, db.TX))
}

func newWriteService(t *testing.T, db *dbtest.DB) *apptag.Service {
	t.Helper()
	txm := db.TX
	repo := tagrepo.NewRepo(db.Engine, txm)
	accessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, txm))
	return apptag.NewService(repo, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(), tagrepo.NewReadRepo(db.Engine, txm), accessResolver)
}

func connectSession(t *testing.T, ctx context.Context, read *apptag.ReadService, write *apptag.Service) *sdk.ClientSession {
	t.Helper()
	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	tagmcp.Register(read, write)(srv)

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

func TestListTagsTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	f.Tag(fixture.Tag{UserID: userID, Name: "Vacation"})

	read := newReadService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	tagmcp.Register(read, newWriteService(t, db))(srv)

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

	toolRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_tags", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list_tags: transport error: %v", err)
	}
	if toolRes.IsError {
		t.Fatalf("list_tags: unexpected error: %#v", toolRes.Content)
	}
	m, ok := toolRes.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("list_tags: structuredContent is not a map: %#v", toolRes.StructuredContent)
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("list_tags: missing items: %#v", m)
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["name"] != "Vacation" {
		t.Fatalf("list_tags: expected Vacation tag, got: %#v", items)
	}
}

func TestTagTools_FullFlow(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	read := newReadService(t, db)
	write := newWriteService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, read, write)

	createRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_tag",
		Arguments: map[string]any{"name": "Business"},
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

	listRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_tags", Arguments: map[string]any{}})
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
		t.Fatalf("list: created tag not found: %#v", items)
	}

	updateRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "update_tag",
		Arguments: map[string]any{"id": id, "name": "Business Renamed"},
	})
	if err != nil {
		t.Fatalf("update: transport error: %v", err)
	}
	if updateRes.IsError {
		t.Fatalf("update: unexpected error: %#v", updateRes.Content)
	}
	updItem, ok := structured(t, updateRes)["item"].(map[string]any)
	if !ok || updItem["name"] != "Business Renamed" {
		t.Fatalf("update: unexpected item: %#v", structured(t, updateRes))
	}

	archiveRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "set_tag_archived",
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

	listRes2, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_tags", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list2: transport error: %v", err)
	}
	items2, _ := structured(t, listRes2)["items"].([]any)
	var archived map[string]any
	for _, it := range items2 {
		if m, ok := it.(map[string]any); ok && m["id"] == id {
			archived = m
		}
	}
	if archived == nil {
		t.Fatalf("list2: archived tag not found: %#v", items2)
	}
	isArchived, _ := archived["isArchived"].(float64)
	if isArchived != 1 {
		t.Fatalf("archive: expected isArchived 1, got: %#v", archived["isArchived"])
	}
}

func TestTagTools_CreateShortName_IsError(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	read := newReadService(t, db)
	write := newWriteService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, read, write)

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_tag",
		Arguments: map[string]any{"name": "ab"},
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
