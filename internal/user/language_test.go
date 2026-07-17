package user_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestUpdateLanguage(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Language Tester", "lang@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	res, err := svc.UpdateLanguage(ctx, id, model.UpdateLanguageRequest{Language: "ru"})
	if err != nil || res.User.Id == "" {
		t.Fatalf("UpdateLanguage: res=%v err=%v", res, err)
	}

	var got string
	if err := db.Raw.QueryRowContext(ctx, db.Rebind("SELECT language FROM users WHERE id = ?"), id.String()).Scan(&got); err != nil {
		t.Fatalf("read back language: %v", err)
	}
	if got != "ru" {
		t.Fatalf("language = %q, want ru", got)
	}
}

func TestUpdateLanguageRejectsUnsupported(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Language Rejecter", "lang-bad@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	_, err = svc.UpdateLanguage(ctx, id, model.UpdateLanguageRequest{Language: "xx"})
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("want ValidationError, got %v", err)
	}
	if v.Fields[0].Code != errs.CodeUserLanguageInvalid {
		t.Fatalf("code = %q", v.Fields[0].Code)
	}
}
