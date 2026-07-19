package user_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/dbtest"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func TestUpdateAvatar(t *testing.T) {
	db := dbtest.New(t)
	svc, enc, _ := newUserSvc(t, db)
	repo := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Avatar Updater", "upd@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	res, err := svc.UpdateAvatar(ctx, id, model.UpdateAvatarRequest{Icon: "pets", Color: "teal"})
	if err != nil {
		t.Fatalf("UpdateAvatar: %v", err)
	}
	if res.User.Avatar != "pets:teal" {
		t.Fatalf("result avatar = %q, want pets:teal", res.User.Avatar)
	}
	u, err := repo.GetByIdentifier(ctx, enc.Hash("upd@econumo.test"))
	if err != nil {
		t.Fatalf("GetByIdentifier: %v", err)
	}
	if u.Avatar != "pets:teal" {
		t.Fatalf("persisted avatar = %q, want pets:teal", u.Avatar)
	}
}

func TestUpdateAvatarRejectsBadValues(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Avatar Rejecter", "rej@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	cases := []struct {
		name string
		req  model.UpdateAvatarRequest
	}{
		{"bad icon format", model.UpdateAvatarRequest{Icon: "Not-Valid", Color: "teal"}},
		{"icon with colon", model.UpdateAvatarRequest{Icon: "face:extra", Color: "teal"}},
		{"unknown color", model.UpdateAvatarRequest{Icon: "face", Color: "neon"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.UpdateAvatar(ctx, id, tc.req); err == nil {
				t.Fatal("UpdateAvatar succeeded, want validation error")
			}
		})
	}
}
