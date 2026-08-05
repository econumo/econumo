package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	applabel "github.com/econumo/econumo/internal/label"
	labelmcp "github.com/econumo/econumo/internal/label/mcp"
	labelrepo "github.com/econumo/econumo/internal/label/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
)

func newReadService(t *testing.T, db *dbtest.DB) *applabel.ReadService {
	t.Helper()
	return applabel.NewReadService(labelrepo.NewReadRepo(db.Engine, db.TX))
}

func newWriteService(t *testing.T, db *dbtest.DB) *applabel.Service {
	t.Helper()
	txm := db.TX
	repo := labelrepo.NewRepo(db.Engine, txm)
	accessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, txm))
	return applabel.NewService(repo, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(), labelrepo.NewReadRepo(db.Engine, txm), accessResolver)
}

func connectSession(t *testing.T, ctx context.Context, read *applabel.ReadService, write *applabel.Service) *sdk.ClientSession {
	t.Helper()
	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	labelmcp.Register(read, write)(srv)

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

func TestListLabelsTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	f.Label(fixture.Label{UserID: userID, Name: "Vacation"})

	read := newReadService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	labelmcp.Register(read, newWriteService(t, db))(srv)

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

	toolRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_labels", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list_labels: transport error: %v", err)
	}
	if toolRes.IsError {
		t.Fatalf("list_labels: unexpected error: %#v", toolRes.Content)
	}
	m, ok := toolRes.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("list_labels: structuredContent is not a map: %#v", toolRes.StructuredContent)
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("list_labels: missing items: %#v", m)
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["name"] != "Vacation" {
		t.Fatalf("list_labels: expected Vacation label, got: %#v", items)
	}
}

func TestLabelTools_FullFlow(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	read := newReadService(t, db)
	write := newWriteService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, read, write)

	createRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_label",
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

	listRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_labels", Arguments: map[string]any{}})
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
		t.Fatalf("list: created label not found: %#v", items)
	}

	updateRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "update_label",
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
		Name:      "set_label_archived",
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

	listRes2, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_labels", Arguments: map[string]any{}})
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
		t.Fatalf("list2: archived label not found: %#v", items2)
	}
	isArchived, _ := archived["isArchived"].(float64)
	if isArchived != 1 {
		t.Fatalf("archive: expected isArchived 1, got: %#v", archived["isArchived"])
	}

	unarchiveRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "set_label_archived",
		Arguments: map[string]any{"id": id, "archived": false},
	})
	if err != nil {
		t.Fatalf("unarchive: transport error: %v", err)
	}
	if unarchiveRes.IsError {
		t.Fatalf("unarchive: unexpected error: %#v", unarchiveRes.Content)
	}
	unarchData := structured(t, unarchiveRes)
	if unarchData["id"] != id || unarchData["archived"] != false {
		t.Fatalf("unarchive: unexpected result: %#v", unarchData)
	}

	listRes3, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_labels", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list3: transport error: %v", err)
	}
	items3, _ := structured(t, listRes3)["items"].([]any)
	var unarchived map[string]any
	for _, it := range items3 {
		if m, ok := it.(map[string]any); ok && m["id"] == id {
			unarchived = m
		}
	}
	if unarchived == nil {
		t.Fatalf("list3: label not found: %#v", items3)
	}
	isArchived2, _ := unarchived["isArchived"].(float64)
	if isArchived2 != 0 {
		t.Fatalf("unarchive: expected isArchived 0, got: %#v", unarchived["isArchived"])
	}
}

func TestLabelTools_CreateShortName_IsError(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	read := newReadService(t, db)
	write := newWriteService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, read, write)

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_label",
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

func TestLabelTools_CreateMintsOwnOperationID(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	read := newReadService(t, db)
	write := newWriteService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, read, write)

	// Two create_label calls with no client-supplied id must both succeed
	// (each mints its own fresh operation id server-side), producing two
	// distinct labels rather than the second being rejected as a replay of
	// the first's idempotency key.
	first, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "create_label", Arguments: map[string]any{"name": "Alpha"}})
	if err != nil || first.IsError {
		t.Fatalf("first create: err=%v res=%#v", err, first)
	}
	second, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "create_label", Arguments: map[string]any{"name": "Beta"}})
	if err != nil || second.IsError {
		t.Fatalf("second create: err=%v res=%#v", err, second)
	}

	firstID := structured(t, first)["item"].(map[string]any)["id"].(string)
	secondID := structured(t, second)["item"].(map[string]any)["id"].(string)
	if firstID == "" || secondID == "" {
		t.Fatalf("empty ids: first=%q second=%q", firstID, secondID)
	}
	if firstID == secondID {
		t.Fatalf("expected distinct operation ids to produce distinct labels, got same id %q twice", firstID)
	}
}
