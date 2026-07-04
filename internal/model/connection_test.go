package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
)

var (
	tn0 = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	tn1 = time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
)

func TestRole_AliasRoundTrip(t *testing.T) {
	for _, c := range []struct {
		role  Role
		alias string
		num   int16
	}{
		{RoleAdmin, "admin", 0}, {RoleUser, "user", 1}, {RoleGuest, "guest", 2},
	} {
		if c.role.Alias() != c.alias {
			t.Errorf("%d.Alias()=%q want %q", c.role, c.role.Alias(), c.alias)
		}
		if c.role.Int16() != c.num {
			t.Errorf("%s.Int16()=%d want %d", c.alias, c.role.Int16(), c.num)
		}
		got, err := RoleFromAlias(c.alias)
		if err != nil || got != c.role {
			t.Errorf("RoleFromAlias(%q)=%d err=%v", c.alias, got, err)
		}
	}
}

func TestRole_AliasOutOfRange(t *testing.T) {
	if Role(9).Alias() != "" || Role(-1).Alias() != "" {
		t.Fatal("out-of-range role alias should be empty")
	}
}

func TestRoleFromAlias_Invalid(t *testing.T) {
	_, err := RoleFromAlias("superadmin")
	if err == nil {
		t.Fatal("want error for unknown role")
	}
	if _, ok := err.(*errs.ValidationError); !ok {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
}

func TestAccountAccess_StructLiteral_RoundTripAndGetters(t *testing.T) {
	acc := mustID(t, "11111111-1111-1111-1111-111111111111")
	usr := mustID(t, "22222222-2222-2222-2222-222222222222")
	a := &AccountAccess{AccountID: acc, UserID: usr, Role: RoleGuest, CreatedAt: tn0, UpdatedAt: tn1}
	if !a.AccountID.Equal(acc) {
		t.Errorf("AccountID=%v want %v", a.AccountID, acc)
	}
	if !a.UserID.Equal(usr) {
		t.Errorf("UserID=%v want %v", a.UserID, usr)
	}
	if a.Role != RoleGuest {
		t.Errorf("Role=%d want guest", a.Role)
	}
	if !a.CreatedAt.Equal(tn0) || !a.UpdatedAt.Equal(tn1) {
		t.Errorf("timestamps: %v / %v want %v / %v", a.CreatedAt, a.UpdatedAt, tn0, tn1)
	}
}

func TestNewAccountAccess_CreatedEqualsUpdated(t *testing.T) {
	a := NewAccountAccess(
		mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), RoleAdmin, tn0)
	if !a.CreatedAt.Equal(tn0) || !a.UpdatedAt.Equal(tn0) {
		t.Errorf("new access: created=%v updated=%v want both %v", a.CreatedAt, a.UpdatedAt, tn0)
	}
}

func TestAccountAccess_UpdateRole_OnlyBumpsOnChange(t *testing.T) {
	a := NewAccountAccess(
		mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), RoleUser, tn0)
	a.UpdateRole(RoleUser, tn1)
	if !a.UpdatedAt.Equal(tn0) {
		t.Fatal("same-role update bumped updatedAt")
	}
	a.UpdateRole(RoleAdmin, tn1)
	if a.Role != RoleAdmin || !a.UpdatedAt.Equal(tn1) {
		t.Fatalf("role change: %d / %v", a.Role, a.UpdatedAt)
	}
}
