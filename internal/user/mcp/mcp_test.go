package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
	appuser "github.com/econumo/econumo/internal/user"
	usermcp "github.com/econumo/econumo/internal/user/mcp"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// stubConnectionLister returns one fixed connection regardless of the
// requesting user, so the test can assert its presence in the resource text
// without wiring the connection feature.
type stubConnectionLister struct{}

func (stubConnectionLister) GetConnectionList(ctx context.Context, userID vo.Id) (*model.GetConnectionListResult, error) {
	return &model.GetConnectionListResult{
		Items: []model.ConnectionResult{
			{
				User: model.UserResult{
					Id:     vo.NewId().Value(),
					Name:   "Connected Friend",
					Avatar: "diamond:sky",
				},
				SharedAccounts: []model.AccountAccessResult{},
			},
		},
	}, nil
}

func TestUserResource(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	const email = "profile-owner@example.test"
	userID := f.User(fixture.User{Email: email})

	readRepo := userrepo.NewReadRepo(db.Engine, db.TX)
	encode := auth.NewEncodeService("")
	readSvc := appuser.NewReadService(readRepo, encode)

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	usermcp.Register(readSvc, stubConnectionLister{})(srv)

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

	res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: "econumo://user"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("contents: %+v", res.Contents)
	}
	text := res.Contents[0].Text
	if !strings.Contains(text, email) {
		t.Fatalf("expected user email in resource text: %s", text)
	}
	if !strings.Contains(text, "Connected Friend") {
		t.Fatalf("expected connection name in resource text: %s", text)
	}
}
