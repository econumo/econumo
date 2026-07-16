package mcp_test

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
	appuser "github.com/econumo/econumo/internal/user"
	usermcp "github.com/econumo/econumo/internal/user/mcp"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func TestGetUserTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	const email = "profile-owner@example.test"
	userID := f.User(fixture.User{Email: email})

	readRepo := userrepo.NewReadRepo(db.Engine, db.TX)
	encode := auth.NewEncodeService("")
	readSvc := appuser.NewReadService(readRepo, encode)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	usermcp.Register(readSvc)(srv)

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

	toolRes, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "get_user", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("get_user: transport error: %v", err)
	}
	if toolRes.IsError {
		t.Fatalf("get_user: unexpected error: %#v", toolRes.Content)
	}
	m, ok := toolRes.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("get_user: structuredContent is not a map: %#v", toolRes.StructuredContent)
	}
	u, ok := m["user"].(map[string]any)
	if !ok || u["email"] != email {
		t.Fatalf("get_user: expected user email %q, got: %#v", email, m)
	}
	if _, ok := m["connections"]; ok {
		t.Fatalf("get_user: connections key should be gone: %#v", m)
	}
}
