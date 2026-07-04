package repo_test

import (
	"context"
	"testing"

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

func intp(v int) *int { return &v }

func mustID(t *testing.T, s string) vo.Id {
	t.Helper()
	id, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("ParseId(%q): %v", s, err)
	}
	return id
}
