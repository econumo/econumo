package budget

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
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
func agg(t *testing.T, grants ...*model.BudgetAccess) *budgetAggregate {
	t.Helper()
	b := model.NewBudget(mustID(t, budgetID), mustID(t, ownerID), "B",
		mustID(t, "44444444-4444-4444-4444-444444444444"), now, now)
	return &budgetAggregate{budget: b, access: grants}
}

// grant builds an access row for a user with a role + accepted flag.
func grant(t *testing.T, userID string, role model.BudgetRole, accepted bool) *model.BudgetAccess {
	t.Helper()
	a := model.NewBudgetAccess(
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
	if err != nil || r != model.BudgetRoleOwner {
		t.Fatalf("owner role=%d err=%v want owner", r, err)
	}
}

func TestBudgetRole_AcceptedGrant(t *testing.T) {
	s := &Service{}
	a := agg(t, grant(t, otherID, model.BudgetRoleAdmin, true))
	r, err := s.budgetRole(a, mustID(t, otherID))
	if err != nil || r != model.BudgetRoleAdmin {
		t.Fatalf("accepted admin role=%d err=%v", r, err)
	}
}

func TestBudgetRole_UnacceptedGrant_AccessDenied(t *testing.T) {
	s := &Service{}
	a := agg(t, grant(t, otherID, model.BudgetRoleAdmin, false))
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
			a:    agg(t, grant(t, otherID, model.BudgetRoleAdmin, true)),
			user: other,
			want: want{true, true, true, true, false, true},
		},
		{
			name: "accepted user",
			a:    agg(t, grant(t, otherID, model.BudgetRoleUser, true)),
			user: other,
			// user can update/edit but NOT delete/reset; not share.
			want: want{true, false, true, false, false, true},
		},
		{
			name: "accepted guest",
			a:    agg(t, grant(t, otherID, model.BudgetRoleGuest, true)),
			user: other,
			// guest reads only.
			want: want{true, false, false, false, false, true},
		},
		{
			name: "unaccepted user (invited, pending)",
			a:    agg(t, grant(t, otherID, model.BudgetRoleUser, false)),
			user: other,
			// no accepted access -> read/del/upd/share false; can accept (pending
			// row exists) + can decline (row exists).
			want: want{false, false, false, false, true, true},
		},
		{
			name: "stranger (no row)",
			a:    agg(t),
			user: stranger,
			// no access at all -> everything false (canShare fails closed).
			want: want{false, false, false, false, false, false},
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

// canShare fails closed: only an accepted owner/admin may share. A guest, a
// pending invitee, and a stranger are all denied.
func TestCanShare_FailsClosed(t *testing.T) {
	s := &Service{}
	// accepted guest: real role, no share right -> false.
	guest := agg(t, grant(t, otherID, model.BudgetRoleGuest, true))
	if s.canShare(guest, mustID(t, otherID)) {
		t.Error("accepted guest must NOT canShare")
	}
	// pending (unaccepted) grant -> budgetRole errors -> canShare returns false.
	pending := agg(t, grant(t, otherID, model.BudgetRoleGuest, false))
	if s.canShare(pending, mustID(t, otherID)) {
		t.Error("pending grant must NOT canShare (fails closed)")
	}
	// stranger with no row -> false.
	if s.canShare(agg(t), mustID(t, "99999999-9999-9999-9999-999999999999")) {
		t.Error("stranger must NOT canShare (fails closed)")
	}
	// accepted admin -> true.
	admin := agg(t, grant(t, otherID, model.BudgetRoleAdmin, true))
	if !s.canShare(admin, mustID(t, otherID)) {
		t.Error("accepted admin must canShare")
	}
}

func isAccessDenied(err error) bool {
	_, ok := err.(*errs.AccessDeniedError)
	return ok
}
