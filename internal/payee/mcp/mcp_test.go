package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	apppayee "github.com/econumo/econumo/internal/payee"
	payeemcp "github.com/econumo/econumo/internal/payee/mcp"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
)

func newReadService(t *testing.T, db *dbtest.DB) *apppayee.ReadService {
	t.Helper()
	return apppayee.NewReadService(payeerepo.NewReadRepo(db.Engine, db.TX))
}

func newWriteService(t *testing.T, db *dbtest.DB) *apppayee.Service {
	t.Helper()
	txm := db.TX
	repo := payeerepo.NewRepo(db.Engine, txm)
	accessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, txm))
	return apppayee.NewService(repo, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(), payeerepo.NewReadRepo(db.Engine, txm), accessResolver)
}

func connectSession(t *testing.T, ctx context.Context, read *apppayee.ReadService, write *apppayee.Service) *sdk.ClientSession {
	t.Helper()
	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	payeemcp.Register(read, write)(srv)

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

func TestListPayeesTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	f.Payee(fixture.Payee{UserID: userID, Name: "Landlord"})

	read := newReadService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	payeemcp.Register(read, newWriteService(t, db))(srv)

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

	toolRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_payees", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list_payees: transport error: %v", err)
	}
	if toolRes.IsError {
		t.Fatalf("list_payees: unexpected error: %#v", toolRes.Content)
	}
	m, ok := toolRes.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("list_payees: structuredContent is not a map: %#v", toolRes.StructuredContent)
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("list_payees: missing items: %#v", m)
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["name"] != "Landlord" {
		t.Fatalf("list_payees: expected Landlord payee, got: %#v", items)
	}
}

func TestPayeeTools_FullFlow(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	read := newReadService(t, db)
	write := newWriteService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, read, write)

	createRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_payee",
		Arguments: map[string]any{"name": "Grocer"},
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

	listRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_payees", Arguments: map[string]any{}})
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
		t.Fatalf("list: created payee not found: %#v", items)
	}

	updateRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "update_payee",
		Arguments: map[string]any{"id": id, "name": "Grocer Renamed"},
	})
	if err != nil {
		t.Fatalf("update: transport error: %v", err)
	}
	if updateRes.IsError {
		t.Fatalf("update: unexpected error: %#v", updateRes.Content)
	}

	listRes2, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_payees", Arguments: map[string]any{}})
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
		t.Fatalf("list2: updated payee not found: %#v", items2)
	}
	if updated["name"] != "Grocer Renamed" {
		t.Fatalf("update: unexpected name: %#v", updated)
	}

	archiveRes, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "set_payee_archived",
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

	listRes3, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_payees", Arguments: map[string]any{}})
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
		t.Fatalf("list3: archived payee not found: %#v", items3)
	}
	isArchived, _ := archived["isArchived"].(float64)
	if isArchived != 1 {
		t.Fatalf("archive: expected isArchived 1, got: %#v", archived["isArchived"])
	}
}

func TestPayeeTools_CreateShortName_IsError(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	read := newReadService(t, db)
	write := newWriteService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectSession(t, ctx, read, write)

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "create_payee",
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
