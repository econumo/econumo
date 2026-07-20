package mcp_test

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appaccount "github.com/econumo/econumo/internal/account"
	accountmcp "github.com/econumo/econumo/internal/account/mcp"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func newAccountService(t *testing.T, db *dbtest.DB) *appaccount.Service {
	t.Helper()
	txm := db.TX
	repo := accountrepo.NewRepo(db.Engine, txm)
	folderRepo := accountrepo.NewFolderRepo(db.Engine, txm)
	curLookup := currencyrepo.New(db.Engine, txm)
	accCur := server.NewAccountCurrencyLookup(curLookup)
	accUser := server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm))
	accessRepo := accountrepo.NewAccessRepo(db.Engine, txm)
	opGuard := operationrepo.NewGuard(db.Engine, txm)
	return appaccount.NewService(repo, folderRepo, accessRepo, accCur, accUser, nil, txm, opGuard, clock.New())
}

func TestListAccountsTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	f.Account(fixture.Account{UserID: userID, Name: "Checking"})

	svc := newAccountService(t, db)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	accountmcp.Register(svc)(srv)

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

	toolRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_accounts", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list_accounts: transport error: %v", err)
	}
	if toolRes.IsError {
		t.Fatalf("list_accounts: unexpected error: %#v", toolRes.Content)
	}
	m, ok := toolRes.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("list_accounts: structuredContent is not a map: %#v", toolRes.StructuredContent)
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("list_accounts: missing items: %#v", m)
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["name"] != "Checking" {
		t.Fatalf("list_accounts: expected Checking account, got: %#v", items)
	}
}
