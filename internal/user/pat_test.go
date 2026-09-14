package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// A PAT must be minted only while the credential that authenticated the
// request is still live. The reclaim revokes every token in the same
// transaction that bumps the credentials generation, so a request that was
// authenticated by the middleware before the reclaim lands must not still be
// able to write a fresh personal token behind it.
func TestCreatePersonalToken_RefusedWhenThePresentingSessionWasRevoked(t *testing.T) {
	svc, tokens, clk, uid := newAuthEnv(t)
	ctx := context.Background()

	login, err := svc.Login(ctx, model.LoginRequest{Username: "auth@econumo.test", Password: "secretpass"}, "ua", authT0)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	// Authenticate is what the auth middleware runs per request; its second
	// return value is the token id it stashes in the context.
	_, sessionTokenID, _, err := svc.Authenticate(ctx, login.Token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	// The reclaim lands after the middleware authenticated this request.
	if err := svc.RevokeTokensForTest(ctx, uid, vo.Id{}, clk.now, model.TokenKindSession, model.TokenKindPersonal); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	_, err = svc.CreatePersonalToken(ctx, uid, sessionTokenID, model.CreatePersonalTokenRequest{Name: "mcp"})
	var unauthorized *errs.UnauthorizedError
	if !errors.As(err, &unauthorized) || unauthorized.Msg != "Invalid access token" {
		t.Fatalf("want 401 Invalid access token, got %v", err)
	}

	rows, err := tokens.ListByUser(ctx, uid, model.TokenKindPersonal)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("a PAT row was written behind a revoked session: %d", len(rows))
	}
}

// A live session still mints a PAT exactly as before.
func TestCreatePersonalToken_SucceedsWhenThePresentingSessionIsLive(t *testing.T) {
	svc, tokens, _, uid := newAuthEnv(t)
	ctx := context.Background()

	login, err := svc.Login(ctx, model.LoginRequest{Username: "auth@econumo.test", Password: "secretpass"}, "ua", authT0)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	_, sessionTokenID, _, err := svc.Authenticate(ctx, login.Token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	res, err := svc.CreatePersonalToken(ctx, uid, sessionTokenID, model.CreatePersonalTokenRequest{Name: "mcp"})
	if err != nil {
		t.Fatalf("CreatePersonalToken: %v", err)
	}
	if res.Token == "" {
		t.Fatal("expected a raw token in the response")
	}

	rows, err := tokens.ListByUser(ctx, uid, model.TokenKindPersonal)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want exactly one PAT row, got %d", len(rows))
	}
}
