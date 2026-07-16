package mcp_test

import (
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appcategory "github.com/econumo/econumo/internal/category"
	categorymcp "github.com/econumo/econumo/internal/category/mcp"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
)

func newReadService(t *testing.T, db *dbtest.DB) *appcategory.ReadService {
	t.Helper()
	return appcategory.NewReadService(categoryrepo.NewReadRepo(db.Engine, db.TX))
}

func TestCategoriesResource(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	f.Category(fixture.Category{UserID: userID, Name: "Groceries"})

	read := newReadService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	categorymcp.Register(read)(srv)

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

	res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: "econumo://categories"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 || !strings.Contains(res.Contents[0].Text, `"Groceries"`) {
		t.Fatalf("contents: %+v", res.Contents)
	}

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
