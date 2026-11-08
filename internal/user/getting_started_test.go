package user_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestUpdateGettingStartedPersists(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Getting Started", "gs@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	v := "plan,!import"
	res, err := svc.UpdateGettingStarted(ctx, id, model.UpdateGettingStartedRequest{Value: &v})
	if err != nil {
		t.Fatalf("UpdateGettingStarted: %v", err)
	}
	var found bool
	for _, o := range res.User.Options {
		if o.Name == model.OptionGettingStarted {
			found = true
			if o.Value == nil || *o.Value != v {
				t.Fatalf("value = %v, want %q", o.Value, v)
			}
		}
	}
	if !found {
		t.Fatalf("getting_started missing from options: %+v", res.User.Options)
	}
}
