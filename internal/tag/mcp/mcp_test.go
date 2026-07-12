package mcp_test

import (
	"strings"
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

func TestTagsResource(t *testing.T) {
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

	res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: "econumo://tags"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 || !strings.Contains(res.Contents[0].Text, `"Vacation"`) {
		t.Fatalf("contents: %+v", res.Contents)
	}
}
