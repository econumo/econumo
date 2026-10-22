package server_test

// Integration tests for the BudgetUserLookup glue adapter
// (glue_budget_userlookup.go) wired to the real user repository over a
// migrated in-memory SQLite.

import (
	"context"
	"testing"
	"time"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/model"
	recurringrepo "github.com/econumo/econumo/internal/recurring/repo"
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
	f.Account(fixture.Account{ID: live.String(), UserID: u, Name: "Rainy day", Icon: "savings"})
	f.Account(fixture.Account{ID: dead.String(), UserID: u, Deleted: true})
	l := server.NewBudgetAccountLookup(accountrepo.NewRepo(db.Engine, db.TX))
	got, err := l.AccountsByIDs(context.Background(), []vo.Id{dead, live})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != dead.String() || !got[0].IsDeleted || got[1].ID != live.String() || got[1].IsDeleted || got[0].OwnerID != u {
		t.Fatalf("got %+v", got)
	}
	if got[1].Name != "Rainy day" || got[1].Icon != "savings" {
		t.Fatalf("member view = %+v, want name/icon carried through", got[1])
	}
	if got[0].Name != "Account" || got[0].Icon != "wallet" {
		t.Fatalf("regular member view = %+v, want the fixture defaults", got[0])
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

func TestBudgetRecurringSource_ListsSourceAndRecipientWithLabels(t *testing.T) {
	const (
		owner    = "0197d000-0000-7000-8000-000000000001"
		guest    = "0197d000-0000-7000-8000-000000000002"
		writer   = "0197d000-0000-7000-8000-000000000003"
		member   = "0197d000-0000-7000-8000-0000000000a1"
		outside  = "0197d000-0000-7000-8000-0000000000a2"
		label    = "0197d000-0000-7000-8000-0000000000b1"
		expense  = "0197d000-0000-7000-8000-0000000000c1"
		transfer = "0197d000-0000-7000-8000-0000000000c2"
	)
	db := dbtest.New(t)
	f := fixture.New(t, db)
	for _, u := range []string{owner, guest, writer} {
		f.User(fixture.User{ID: u, Email: u + "@e.test"})
	}
	f.Account(fixture.Account{ID: member, UserID: owner})
	f.Account(fixture.Account{ID: outside, UserID: owner})
	f.Label(fixture.Label{ID: label, UserID: owner})
	f.AccountAccess(member, guest, 2)
	f.AccountAccess(member, writer, 1)

	ctx := context.Background()
	repo := recurringrepo.NewRepo(db.Engine, db.TX)
	next := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	mk := func(id, account string, typ model.TransactionType) *model.RecurringTransaction {
		return model.NewRecurringTransaction(model.RecurringNewState{
			ID: vo.MustParseId(id), UserID: vo.MustParseId(owner), Type: typ, AccountID: vo.MustParseId(account),
			Amount: "10", Schedule: model.RecurringScheduleMonthly, NextPaymentAt: next, CreatedAt: next,
		})
	}
	if err := repo.Save(ctx, mk(expense, member, model.TransactionTypeExpense)); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(expense), []vo.Id{vo.MustParseId(label)}); err != nil {
		t.Fatal(err)
	}
	in := mk(transfer, outside, model.TransactionTypeTransfer)
	recip := vo.MustParseId(member)
	in.AccountRecipID = &recip
	if err := repo.Save(ctx, in); err != nil {
		t.Fatal(err)
	}

	src := server.NewBudgetRecurringSource(repo, server.NewBudgetAccountLookup(accountrepo.NewRepo(db.Engine, db.TX)),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX)))
	items, err := src.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(member)})
	if err != nil {
		t.Fatalf("ListByAccountIDs: %v", err)
	}
	byID := map[string]*model.RecurringTransaction{}
	for _, rt := range items {
		byID[rt.ID.String()] = rt
	}
	if len(items) != 2 || byID[expense] == nil || byID[transfer] == nil {
		t.Fatalf("want the member's expense and the transfer into it, got %d items", len(items))
	}
	if l := byID[expense].LabelIDs; len(l) != 1 || l[0].String() != label {
		t.Errorf("expense labels = %v, want [%s]", l, label)
	}

	for _, c := range []struct {
		user string
		want bool
	}{{owner, true}, {writer, true}, {guest, false}} {
		got, err := src.CanEdit(ctx, vo.MustParseId(c.user), vo.MustParseId(member))
		if err != nil {
			t.Fatalf("CanEdit(%s): %v", c.user, err)
		}
		if got != c.want {
			t.Errorf("CanEdit(%s) = %v, want %v", c.user, got, c.want)
		}
	}
	// A template whose account is gone is not editable, and must not fail the
	// list it appears in.
	got, err := src.CanEdit(ctx, vo.MustParseId(owner), vo.MustParseId("0197d000-0000-7000-8000-0000000000ff"))
	if err != nil || got {
		t.Errorf("CanEdit(missing account) = %v, %v; want false, nil", got, err)
	}
}
