package mcp_test

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

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

func TestListTagsTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	f.Tag(fixture.Tag{UserID: userID, Name: "Vacation"})

	read := newReadService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	tagmcp.Register(read)(srv)

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
