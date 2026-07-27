package repo_test

import (
	"context"
	"testing"
	"time"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/shared/vo"
)

// TestAccessResolver_RoleMatrix pins the shared-account permission matrix at its
// source: AccountAccessResolver.HasWriteGrant / HasAdminGrant translate a
// stored grant role into write/admin authority. The feature use-cases consume
// these booleans, so this is the one place the role -> permission rule itself is
// exercised end to end against a real grant row.
//
//	role   | HasWriteGrant | HasAdminGrant
//	admin  |     yes       |     yes
//	user   |     yes       |     no
//	guest  |     no        |     no
//	(none) |     no        |     no
func TestAccessResolver_RoleMatrix(t *testing.T) {
	const roleAdmin, roleUser, roleGuest = 0, 1, 2

	cases := []struct {
		name      string
		grant     *int // nil = no grant seeded for userB on acctA
		wantWrite bool
		wantAdmin bool
	}{
		{"admin grant", intp(roleAdmin), true, true},
		{"user grant", intp(roleUser), true, false},
		{"guest grant", intp(roleGuest), false, false},
		{"no grant", nil, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo, _, f := newRepo(t)
			if c.grant != nil {
				f.AccountAccess(acctA, userB, *c.grant)
			}
			resolver := connectionrepo.NewAccountAccessResolver(repo)
			accountID := mustID(t, acctA)
			userID := mustID(t, userB)

			gotWrite, err := resolver.HasWriteGrant(context.Background(), accountID, userID)
			if err != nil {
				t.Fatalf("HasWriteGrant: %v", err)
			}
			if gotWrite != c.wantWrite {
				t.Errorf("HasWriteGrant = %v, want %v", gotWrite, c.wantWrite)
			}

			gotAdmin, err := resolver.HasAdminGrant(context.Background(), accountID, userID)
			if err != nil {
				t.Fatalf("HasAdminGrant: %v", err)
			}
			if gotAdmin != c.wantAdmin {
				t.Errorf("HasAdminGrant = %v, want %v", gotAdmin, c.wantAdmin)
			}
		})
	}
}

// TestAccessResolver_PendingGrant_NoAccess pins the is_accepted guard: an
// otherwise write/admin-capable role (admin) confers NO access while the grant
// is pending, and full access once accepted. Regression guard against
// HasWriteGrant/HasAdminGrant checking only the role and ignoring is_accepted.
func TestAccessResolver_PendingGrant_NoAccess(t *testing.T) {
	const roleAdmin = 0
	repo, _, f := newRepo(t)
	f.AccountAccessPending(acctA, userB, roleAdmin)
	resolver := connectionrepo.NewAccountAccessResolver(repo)
	accountID := mustID(t, acctA)
	userID := mustID(t, userB)

	gotWrite, err := resolver.HasWriteGrant(context.Background(), accountID, userID)
	if err != nil {
		t.Fatalf("HasWriteGrant: %v", err)
	}
	if gotWrite {
		t.Errorf("HasWriteGrant = true for a pending admin grant, want false")
	}
	gotAdmin, err := resolver.HasAdminGrant(context.Background(), accountID, userID)
	if err != nil {
		t.Fatalf("HasAdminGrant: %v", err)
	}
	if gotAdmin {
		t.Errorf("HasAdminGrant = true for a pending admin grant, want false")
	}

	// Accept the grant (via the entity's own Accept, not a re-seed — the PK is
	// (account_id, user_id), so a second insert would conflict): both checks
	// must flip to true.
	access, err := repo.Get(context.Background(), accountID, userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	access.Accept(time.Now())
	if err := repo.Save(context.Background(), access); err != nil {
		t.Fatalf("Save (accept): %v", err)
	}

	gotWrite, err = resolver.HasWriteGrant(context.Background(), accountID, userID)
	if err != nil {
		t.Fatalf("HasWriteGrant (accepted): %v", err)
	}
	if !gotWrite {
		t.Errorf("HasWriteGrant = false for an accepted admin grant, want true")
	}
	gotAdmin, err = resolver.HasAdminGrant(context.Background(), accountID, userID)
	if err != nil {
		t.Fatalf("HasAdminGrant (accepted): %v", err)
	}
	if !gotAdmin {
		t.Errorf("HasAdminGrant = false for an accepted admin grant, want true")
	}
}

func intp(v int) *int { return &v }

func mustID(t *testing.T, s string) vo.Id {
	t.Helper()
	id, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("ParseId(%q): %v", s, err)
	}
	return id
}
