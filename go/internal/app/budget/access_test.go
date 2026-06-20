package budget

import (
	"testing"
	"time"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

func mustID(t *testing.T, s string) vo.Id {
	t.Helper()
	v, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("parse id %q: %v", s, err)
	}
	return v
}

var (
	ownerID  = "11111111-1111-1111-1111-111111111111"
	otherID  = "22222222-2222-2222-2222-222222222222"
	budgetID = "33333333-3333-3333-3333-333333333333"
	now      = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
)

// agg builds a budgetAggregate owned by ownerID with the given access grants.
func agg(t *testing.T, grants ...*dombudget.BudgetAccess) *budgetAggregate {
	t.Helper()
	b := dombudget.NewBudget(mustID(t, budgetID), mustID(t, ownerID), "B",
		mustID(t, "44444444-4444-4444-4444-444444444444"), now, now)
	return &budgetAggregate{budget: b, access: grants}
}

// grant builds an access row for a user with a role + accepted flag.
func grant(t *testing.T, userID string, role dombudget.UserRole, accepted bool) *dombudget.BudgetAccess {
	t.Helper()
	a := dombudget.NewBudgetAccess(
		mustID(t, "55555555-5555-5555-5555-555555555555"),
		mustID(t, budgetID), mustID(t, userID), role, now)
	if accepted {
		a.Accept(now)
	}
	return a
}

func TestBudgetRole_Owner(t *testing.T) {
	s := &Service{}
	r, err := s.budgetRole(agg(t), mustID(t, ownerID))
	if err != nil || r != dombudget.RoleOwner {
		t.Fatalf("owner role=%d err=%v want owner", r, err)
	}
}

func TestBudgetRole_AcceptedGrant(t *testing.T) {
	s := &Service{}
	a := agg(t, grant(t, otherID, dombudget.RoleAdmin, true))
	r, err := s.budgetRole(a, mustID(t, otherID))
	if err != nil || r != dombudget.RoleAdmin {
		t.Fatalf("accepted admin role=%d err=%v", r, err)
	}
}

func TestBudgetRole_UnacceptedGrant_AccessDenied(t *testing.T) {
	s := &Service{}
	a := agg(t, grant(t, otherID, dombudget.RoleAdmin, false))
	_, err := s.budgetRole(a, mustID(t, otherID))
	if !isAccessDenied(err) {
		t.Fatalf("unaccepted grant should be AccessDenied, got %v", err)
	}
}

func TestBudgetRole_NoGrant_AccessDenied(t *testing.T) {
	s := &Service{}
	_, err := s.budgetRole(agg(t), mustID(t, otherID))
	if !isAccessDenied(err) {
		t.Fatalf("stranger should be AccessDenied, got %v", err)
	}
}

// The full permission matrix: for each (role, scenario) assert each can* verb.
func TestAccessMatrix(t *testing.T) {
	s := &Service{}
	owner := mustID(t, ownerID)
	other := mustID(t, otherID)
	stranger := mustID(t, "99999999-9999-9999-9999-999999999999")

	type want struct {
		read, del, upd, share, accept, decline bool
	}
	cases := []struct {
		name string
		a    *budgetAggregate
		user vo.Id
		want want
	}{
		{
			name: "owner",
			a:    agg(t),
			user: owner,
			//      read  del   upd   share accept decline
			want: want{true, true, true, true, false, false},
		},
		{
			name: "accepted admin",
			a:    agg(t, grant(t, otherID, dombudget.RoleAdmin, true)),
			user: other,
			want: want{true, true, true, true, false, true},
		},
		{
			name: "accepted user",
			a:    agg(t, grant(t, otherID, dombudget.RoleUser, true)),
			user: other,
			// user can update/edit but NOT delete/reset; not share.
			want: want{true, false, true, false, false, true},
		},
		{
			name: "accepted guest",
			a:    agg(t, grant(t, otherID, dombudget.RoleGuest, true)),
			user: other,
			// guest reads only.
			want: want{true, false, false, false, false, true},
		},
		{
			name: "unaccepted user (invited, pending)",
			a:    agg(t, grant(t, otherID, dombudget.RoleUser, false)),
			user: other,
			// no accepted access -> read/del/upd false; canShare TRUE (PHP quirk);
			// can accept (pending row exists) + can decline (row exists).
			want: want{false, false, false, true, true, true},
		},
		{
			name: "stranger (no row)",
			a:    agg(t),
			user: stranger,
			// canShare TRUE via the AccessDenied catch quirk.
			want: want{false, false, false, true, false, false},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := s.canRead(c.a, c.user); got != c.want.read {
				t.Errorf("canRead=%v want %v", got, c.want.read)
			}
			if got := s.canDelete(c.a, c.user); got != c.want.del {
				t.Errorf("canDelete=%v want %v", got, c.want.del)
			}
			if got := s.canUpdate(c.a, c.user); got != c.want.upd {
				t.Errorf("canUpdate=%v want %v", got, c.want.upd)
			}
			if got := s.canShare(c.a, c.user); got != c.want.share {
				t.Errorf("canShare=%v want %v", got, c.want.share)
			}
			if got := s.canAccept(c.a, c.user); got != c.want.accept {
				t.Errorf("canAccept=%v want %v", got, c.want.accept)
			}
			if got := s.canDecline(c.a, c.user); got != c.want.decline {
				t.Errorf("canDecline=%v want %v", got, c.want.decline)
			}
		})
	}
}

// canShare's AccessDenied-catch quirk: a guest (accepted, no share right) returns
// FALSE, but anyone WITHOUT accepted access returns TRUE. Pin both halves.
func TestCanShare_Quirk(t *testing.T) {
	s := &Service{}
	// accepted guest: real role, no share -> false.
	guest := agg(t, grant(t, otherID, dombudget.RoleGuest, true))
	if s.canShare(guest, mustID(t, otherID)) {
		t.Error("accepted guest must NOT canShare")
	}
	// pending (unaccepted) grant -> budgetRole errors -> canShare returns true.
	pending := agg(t, grant(t, otherID, dombudget.RoleGuest, false))
	if !s.canShare(pending, mustID(t, otherID)) {
		t.Error("pending grant should canShare via the AccessDenied catch quirk")
	}
}

func isAccessDenied(err error) bool {
	_, ok := err.(*errs.AccessDeniedError)
	return ok
}
