package mcp_test

import (
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appconnection "github.com/econumo/econumo/internal/connection"
	connectionmcp "github.com/econumo/econumo/internal/connection/mcp"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func newConnectionService(t *testing.T, db *dbtest.DB) *appconnection.Service {
	t.Helper()
	txm := db.TX
	userRepo := userrepo.NewRepo(db.Engine, txm)
	// The revokers and the limiter are only reached by the invite/revoke write
	// paths; this suite exercises the list read, so they stay nil.
	return appconnection.NewService(
		connectionrepo.NewRepo(db.Engine, txm),
		connectionrepo.NewInviteRepo(db.Engine, txm),
		server.NewUserOwnerLookup(userRepo),
		nil, nil, nil,
		txm, clock.New(),
	)
}

func TestListConnectionsTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)

	const (
		userID   = "11111111-1111-1111-1111-111111111111"
		friendID = "22222222-2222-2222-2222-222222222222"
	)
	f.User(fixture.User{ID: userID, Email: "owner@example.test"})
	f.User(fixture.User{ID: friendID, Email: "friend@example.test", Name: "Connected Friend"})
	f.Connect(userID, friendID)

	const usdID = "dffc2a06-6f29-4704-8575-31709adee926" // seeded by baseline migration
	folderID := f.Folder(fixture.Folder{UserID: friendID, Name: "Main", Position: 0})
	accountID := f.Account(fixture.Account{UserID: friendID, CurrencyID: usdID, Name: "Shared Cash", Type: 2, Icon: "wallet"})
	f.AccountInFolder(folderID, accountID)
	f.AccountOption(accountID, friendID, 0)
	f.AccountAccess(accountID, userID, 1)

	svc := newConnectionService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	connectionmcp.Register(svc)(srv)

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

	toolRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_connections", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list_connections: transport error: %v", err)
	}
	if toolRes.IsError {
		t.Fatalf("list_connections: unexpected error: %#v", toolRes.Content)
	}
	toolText, ok := toolRes.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("list_connections: content not text: %#v", toolRes.Content)
	}
	if !strings.Contains(toolText.Text, "Connected Friend") {
		t.Fatalf("list_connections: expected connected user name: %s", toolText.Text)
	}
	if !strings.Contains(toolText.Text, accountID) {
		t.Fatalf("list_connections: expected shared account id: %s", toolText.Text)
	}
}
