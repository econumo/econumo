package mcp_test

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

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

func TestListPayeesTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	f.Payee(fixture.Payee{UserID: userID, Name: "Landlord"})

	read := newReadService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	payeemcp.Register(read)(srv)

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
