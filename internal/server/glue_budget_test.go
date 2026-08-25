package server_test

// Integration tests for the BudgetUserLookup glue adapter
// (glue_budget_userlookup.go) wired to the real user repository over a
// migrated in-memory SQLite.

import (
	"context"
	"testing"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

const (
	glueUserA = "11111111-1111-1111-1111-111111111111"
	glueUserB = "22222222-2222-2222-2222-222222222222"
)

func TestBudgetUserLookup_CurrencyCode(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: glueUserA, Name: "u"})
	f.DefaultOptions(glueUserA) // seeds currency=USD among the standard options

	users := userrepo.NewRepo("sqlite", db.TX)
	lookup := server.NewBudgetUserLookup(users, clock.New())

	id, err := lookup.DefaultCurrencyID(context.Background(), glueUserA)
	if err != nil {
		t.Fatalf("DefaultCurrencyID: %v", err)
	}
	if id != fixture.USD {
		t.Errorf("want the seeded USD id from the currency option, got %q", id)
	}
}

// The migration guarantees every user holds a currency option; a user without
// one is data corruption and must surface as an error, not a silent fallback.
func TestBudgetUserLookup_CurrencyCode_ErrorWhenOptionMissing(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: glueUserB, Name: "u"})
	// No DefaultOptions seeded: the currency option row is absent.

	users := userrepo.NewRepo("sqlite", db.TX)
	lookup := server.NewBudgetUserLookup(users, clock.New())

	if _, err := lookup.DefaultCurrencyID(context.Background(), glueUserB); err == nil {
		t.Error("want an error for a user without a currency option")
	}
}

func TestBudgetUserLookup_CurrencyCode_InvalidID(t *testing.T) {
	db := dbtest.NewSQLite(t)
	users := userrepo.NewRepo("sqlite", db.TX)
	lookup := server.NewBudgetUserLookup(users, clock.New())

	if _, err := lookup.DefaultCurrencyID(context.Background(), "not-a-uuid"); err == nil {
		t.Error("want an error for a malformed user id")
	}
}

func TestBudgetAccountLookup_AccountsByIDs_IncludesDeletedInInputOrder(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := vo.NewId().String()
	f.User(fixture.User{ID: u, Email: "u@e.test", Name: "U", Password: "pw", Salt: "s"})
	live, dead := vo.NewId(), vo.NewId()
	f.Account(fixture.Account{ID: live.String(), UserID: u})
	f.Account(fixture.Account{ID: dead.String(), UserID: u, Deleted: true})
	l := server.NewBudgetAccountLookup(accountrepo.NewRepo(db.Engine, db.TX))
	got, err := l.AccountsByIDs(context.Background(), []vo.Id{dead, live})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != dead.String() || !got[0].IsDeleted || got[1].ID != live.String() || got[1].IsDeleted || got[0].OwnerID != u {
		t.Fatalf("got %+v", got)
	}
	if _, err := l.AccountsByIDs(context.Background(), []vo.Id{vo.NewId()}); err == nil {
		t.Fatal("unknown id must error")
	}
}

// OwnedLiveAccountIDs feeds the accept-access membership seeding: only the
// user's OWN live accounts — never a deleted one, never one merely shared with
// them.
func TestBudgetAccountLookup_OwnedLiveAccountIDs(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u, other := vo.NewId().String(), vo.NewId().String()
	f.User(fixture.User{ID: u, Email: "u@e.test", Name: "U", Password: "pw", Salt: "s"})
	f.User(fixture.User{ID: other, Email: "o@e.test", Name: "O", Password: "pw", Salt: "s"})
	live, dead, shared := vo.NewId(), vo.NewId(), vo.NewId()
	f.Account(fixture.Account{ID: live.String(), UserID: u})
	f.Account(fixture.Account{ID: dead.String(), UserID: u, Deleted: true})
	f.Account(fixture.Account{ID: shared.String(), UserID: other})
	f.AccountAccess(shared.String(), u, 1)

	l := server.NewBudgetAccountLookup(accountrepo.NewRepo(db.Engine, db.TX))
	got, err := l.OwnedLiveAccountIDs(context.Background(), vo.MustParseId(u))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Equal(live) {
		t.Fatalf("got %v want only the owned live account %s", got, live)
	}
}
