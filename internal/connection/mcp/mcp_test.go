package mcp_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
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
	folderRepo := accountrepo.NewFolderRepo(db.Engine, txm)
	accountRepo := accountrepo.NewRepo(db.Engine, txm)
	userRepo := userrepo.NewRepo(db.Engine, txm)
	budgetRepo := budgetrepo.NewRepo(db.Engine, txm)
	return appconnection.NewService(
		connectionrepo.NewRepo(db.Engine, txm),
		connectionrepo.NewInviteRepo(db.Engine, txm),
		server.NewConnectionFolderPort(folderRepo),
		accountRepo,
		server.NewUserOwnerLookup(userRepo),
		server.NewConnectionBudgetRevoker(budgetRepo),
		txm, clock.New(),
	)
}

func TestConnectionsResourceAndListConnectionsTool(t *testing.T) {
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

	res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: "econumo://connections"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("contents: %+v", res.Contents)
	}
	resourceText := res.Contents[0].Text
	if !strings.Contains(resourceText, "Connected Friend") {
		t.Fatalf("expected connected user name in resource text: %s", resourceText)
	}
	if !strings.Contains(resourceText, accountID) {
		t.Fatalf("expected shared account id in resource text: %s", resourceText)
	}

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

	var resourceDoc, toolDoc any
	if err := json.Unmarshal([]byte(resourceText), &resourceDoc); err != nil {
		t.Fatalf("decode resource: %v", err)
	}
	if err := json.Unmarshal([]byte(toolText.Text), &toolDoc); err != nil {
		t.Fatalf("decode tool: %v", err)
	}
	if !reflect.DeepEqual(resourceDoc, toolDoc) {
		t.Fatalf("resource and tool payloads differ:\nresource: %s\ntool: %s", resourceText, toolText.Text)
	}
}
