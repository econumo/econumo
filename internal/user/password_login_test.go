package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestPasswordLoginDisabled_RefusesEveryPasswordFlow(t *testing.T) {
	db := dbtest.New(t)
	svc, repo, _ := newTrialSvc(t, db, 0)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{
		Name: "Existing User", Email: "existing@econumo.test", Password: "secretpass",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	u, err := repo.GetByEmail(ctx, "existing@econumo.test")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}

	svc.DisablePasswordLogin()

	flows := map[string]func() error{
		"login": func() error {
			_, err := svc.Login(ctx, model.LoginRequest{Username: "existing@econumo.test", Password: "secretpass"}, "ua", time.Now())
			return err
		},
		"register": func() error {
			_, err := svc.Register(ctx, model.RegisterRequest{Name: "New User", Email: "new@econumo.test", Password: "secretpass"})
			return err
		},
		"remind-password": func() error {
			_, err := svc.RemindPassword(ctx, model.RemindPasswordRequest{Username: "existing@econumo.test"})
			return err
		},
		"reset-password": func() error {
			_, err := svc.ResetPassword(ctx, model.ResetPasswordRequest{Username: "existing@econumo.test", Code: "123456", Password: "newsecretpass"})
			return err
		},
		"update-password": func() error {
			_, err := svc.UpdatePassword(ctx, u.ID, vo.Id{}, model.UpdatePasswordRequest{OldPassword: "secretpass", NewPassword: "newsecretpass"})
			return err
		},
		"confirm-email": func() error {
			_, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "existing@econumo.test", Code: "123456"})
			return err
		},
		"resend-verification-code": func() error {
			_, _, err := svc.ResendVerificationCode(ctx, model.ResendVerificationCodeRequest{Username: "existing@econumo.test"})
			return err
		},
	}
	for name, call := range flows {
		t.Run(name, func(t *testing.T) {
			var ve *errs.ValidationError
			if err := call(); !errors.As(err, &ve) || ve.MsgCode != errs.CodeUserPasswordLoginDisabled {
				t.Fatalf("err = %v, want a %s validation error", err, errs.CodeUserPasswordLoginDisabled)
			}
		})
	}

	if _, err := repo.GetByEmail(ctx, "new@econumo.test"); err == nil {
		t.Fatal("register created a user while password login is disabled")
	}
}
