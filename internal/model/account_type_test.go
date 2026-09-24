package model_test

import (
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestAccountTypeValid(t *testing.T) {
	for v, want := range map[int16]bool{0: false, 1: true, 2: true, 3: true, 4: false} {
		if got := model.AccountType(v).Valid(); got != want {
			t.Errorf("Valid(%d)=%v want %v", v, got, want)
		}
	}
	if model.TypeSavings != 3 {
		t.Fatalf("TypeSavings is frozen at 3")
	}
}

func TestAccountUpdateType(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	a := model.NewAccount(vo.NewId(), vo.NewId(), vo.NewId(), "Main", "wallet", now)
	later := now.Add(time.Hour)
	a.UpdateType(model.TypeCreditCard, later)
	if !a.UpdatedAt.Equal(now) {
		t.Fatalf("same type must not touch UpdatedAt")
	}
	a.UpdateType(model.TypeSavings, later)
	if a.Type != model.TypeSavings || !a.UpdatedAt.Equal(later) {
		t.Fatalf("got type %d updatedAt %v", a.Type, a.UpdatedAt)
	}
}

func TestAccountRequestTypeValidation(t *testing.T) {
	bad := 4
	create := model.CreateAccountRequest{Id: "x", Name: "Main", CurrencyId: "c", Icon: "wallet", Type: &bad}
	assertInvalidType(t, create.Validate())
	update := model.UpdateAccountRequest{Id: "x", Name: "Main", Icon: "wallet", UpdatedAt: "2026-09-01 00:00:00", Type: &bad}
	assertInvalidType(t, update.Validate())
	for _, ok := range []int{1, 2, 3} {
		v := ok
		create.Type, update.Type = &v, &v
		if err := create.Validate(); err != nil {
			t.Fatalf("create type %d: %v", ok, err)
		}
		if err := update.Validate(); err != nil {
			t.Fatalf("update type %d: %v", ok, err)
		}
	}
}

func assertInvalidType(t *testing.T, err error) {
	t.Helper()
	var v *errs.ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("want validation error, got %v", err)
	}
	for _, f := range v.Fields {
		if f.Key == "type" && f.Code == errs.CodeAccountInvalidType {
			return
		}
	}
	t.Fatalf("no type/%s field error in %+v", errs.CodeAccountInvalidType, v.Fields)
}
