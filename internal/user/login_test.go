package user_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestLoginPersistsLanguage(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)

	id, err := svc.AdminCreateUser(context.Background(), "Login Language", "login-lang@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	ctx := reqctx.WithLanguage(context.Background(), "ru")
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "login-lang@econumo.test", Password: "secretpass"}, "test-agent", time.Now()); err != nil {
		t.Fatalf("Login: %v", err)
	}

	var got string
	if err := db.Raw.QueryRowContext(context.Background(), db.Rebind("SELECT language FROM users WHERE id = ?"), id.String()).Scan(&got); err != nil {
		t.Fatalf("read back language: %v", err)
	}
	if got != "ru" {
		t.Fatalf("language = %q, want ru", got)
	}
}

func TestLoginDefaultsLanguageWithoutHeader(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)

	id, err := svc.AdminCreateUser(context.Background(), "Login Default", "login-default@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	if _, err := svc.Login(context.Background(), model.LoginRequest{Username: "login-default@econumo.test", Password: "secretpass"}, "test-agent", time.Now()); err != nil {
		t.Fatalf("Login: %v", err)
	}

	var got string
	if err := db.Raw.QueryRowContext(context.Background(), db.Rebind("SELECT language FROM users WHERE id = ?"), id.String()).Scan(&got); err != nil {
		t.Fatalf("read back language: %v", err)
	}
	if got != "en" {
		t.Fatalf("language = %q, want en", got)
	}
}
