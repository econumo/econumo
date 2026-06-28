//go:build enginecompare

package enginecompare

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	appcategory "github.com/econumo/econumo/internal/app/category"
	domconnection "github.com/econumo/econumo/internal/domain/connection"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	accountrepo "github.com/econumo/econumo/internal/infra/repo/account"
	categoryrepo "github.com/econumo/econumo/internal/infra/repo/category"
	connectionrepo "github.com/econumo/econumo/internal/infra/repo/connection"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func mustID(t *testing.T, s string) vo.Id {
	t.Helper()
	id, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("parse id %q: %v", s, err)
	}
	return id
}

// TestEngines_CategoryOwnAndShared exercises the own+shared category read query
// — the `WHERE user_id = ? OR user_id IN (subquery over accounts_access)` shape
// that diverges by engine (placeholders + the param-reuse adapter). userB shares
// an account with userA, so userA's available list must include userB's
// categories. The snapshot is userA's list; both engines must agree.
func TestEngines_CategoryOwnAndShared(t *testing.T) {
	runOnBoth(t, func(t *testing.T, db *dbtest.DB) string {
		ctx := context.Background()
		f := fixture.New(t, db)
		f.User(fixture.User{ID: userA, Email: "a@test"})
		f.User(fixture.User{ID: userB, Email: "b@test"})

		const acct = "aaaa1111-0000-0000-0000-0000000000b1"
		f.Account(fixture.Account{ID: acct, CurrencyID: usdID, UserID: userB, Name: "Shared", Type: 2, Icon: "wallet"})
		f.AccountAccess(acct, userA, 1)

		f.Category(fixture.Category{ID: "c0000000-0000-0000-0000-0000000000a1", UserID: userA, Name: "Food", Position: 0, Type: 0, Icon: "i"})
		f.Category(fixture.Category{ID: "c0000000-0000-0000-0000-0000000000a2", UserID: userA, Name: "Salary", Position: 1, Type: 1, Icon: "i"})
		f.Category(fixture.Category{ID: "c0000000-0000-0000-0000-0000000000b1", UserID: userB, Name: "Rent", Position: 0, Type: 0, Icon: "i", Archived: true})

		rows, err := categoryrepo.NewReadRepo(db.Engine, db.TX).CategoryListView(ctx, userA)
		if err != nil {
			t.Fatalf("CategoryListView: %v", err)
		}
		return snapshotCategories(rows)
	})
}

func snapshotCategories(rows []appcategory.CategoryViewRow) string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s|%s|%s|pos=%d|type=%d|arch=%t|%s|%s",
			r.ID, r.UserID, r.Name, r.Position, r.Type, r.IsArchived, r.CreatedAt, r.UpdatedAt))
	}
	sort.Strings(out) // order-insensitive: ordering-of-ties is an accepted diff
	return fmt.Sprintf("%d rows\n%v", len(out), out)
}

// TestEngines_AccountBalances exercises the SUM-over-NUMERIC balance query,
// where sqlite returns a float (formatted to scale 8) and pgsql returns exact
// NUMERIC text. Seeded transactions (incl. a netting-to-zero pair) must produce
// the SAME balance strings on both engines.
func TestEngines_AccountBalances(t *testing.T) {
	runOnBoth(t, func(t *testing.T, db *dbtest.DB) string {
		ctx := context.Background()
		f := fixture.New(t, db)
		f.User(fixture.User{ID: userA, Email: "a@test"})
		const acct = "aaaa1111-0000-0000-0000-0000000000c1"
		f.Account(fixture.Account{ID: acct, CurrencyID: usdID, UserID: userA, Name: "Cash", Type: 2, Icon: "wallet"})
		amounts := []string{"100.00000000", "33.33000000", "-133.33000000", "0.10000000", "0.20000000"}
		for i, amt := range amounts {
			f.Transaction(fixture.Transaction{
				ID:        fmt.Sprintf("d0000000-0000-0000-0000-00000000000%d", i),
				UserID:    userA,
				AccountID: acct,
				Type:      1,
				Amount:    amt,
				SpentAt:   fixedTime,
			})
		}
		bals, err := accountrepo.NewRepo(db.Engine, db.TX).Balances(ctx, mustID(t, userA), fixedTime.AddDate(1, 0, 0))
		if err != nil {
			t.Fatalf("Balances: %v", err)
		}
		return fmt.Sprintf("balance[%s]=%s", acct, bals[acct])
	})
}

// TestEngines_ConnectionInviteByCode exercises the invite by-code lookup, whose
// expiry comparison is engine-specific (sqlite uses datetime() with a string
// bound; pgsql compares native timestamps). Look up by code before expiry
// (found) and after expiry (not found) — both engines must agree.
func TestEngines_ConnectionInviteByCode(t *testing.T) {
	runOnBoth(t, func(t *testing.T, db *dbtest.DB) string {
		ctx := context.Background()
		fixture.New(t, db).User(fixture.User{ID: userA, Email: "a@test"})
		repo := connectionrepo.NewInviteRepo(db.Engine, db.TX)

		inv := domconnection.NewConnectionInvite(mustID(t, userA))
		inv.GenerateNewCode(fixedTime) // expiry = fixedTime + 5min
		if err := repo.Save(ctx, inv); err != nil {
			t.Fatalf("Save invite: %v", err)
		}
		code := inv.Code()

		got, err := repo.GetByCode(ctx, code, fixedTime.Add(1*time.Minute))
		foundBefore := err == nil && got != nil && got.UserId().Equal(mustID(t, userA))

		_, errAfter := repo.GetByCode(ctx, code, fixedTime.Add(10*time.Minute))
		foundAfter := errAfter == nil

		return fmt.Sprintf("len(code)=%d foundBefore=%t foundAfter=%t", len([]rune(code.Value())), foundBefore, foundAfter)
	})
}
