package server_test

import (
	"context"
	"testing"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func TestOnboardingAccountReader(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := f.User(fixture.User{Name: "Me"})
	live := f.Account(fixture.Account{UserID: u, Name: "Wallet"})
	gone := f.Account(fixture.Account{UserID: u, Name: "Old", Deleted: true})
	r := server.NewOnboardingAccountReader(accountrepo.NewRepo(db.Engine, db.TX), server.NewAccountCurrencyLookup(currencyrepo.New(db.Engine, db.TX)))
	got, err := r.AccountsByIDs(context.Background(), []vo.Id{vo.MustParseId(live), vo.MustParseId(gone), vo.MustParseId("019a0000-0000-7000-8000-00000000dead")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Wallet" || got[0].CurrencyCode != "USD" || got[0].OwnerID != u || got[0].IsDeleted || !got[1].IsDeleted {
		t.Fatalf("got %+v", got)
	}
	facts := server.NewOnboardingUserFacts(userrepo.NewRepo(db.Engine, db.TX))
	tz, err := facts.StoredTimezone(context.Background(), vo.MustParseId(u))
	if err != nil || tz != "" {
		t.Fatalf("tz = %q, err = %v", tz, err)
	}
	if off, err := facts.AIOptedOut(context.Background(), vo.MustParseId(u)); err != nil || off {
		t.Fatalf("absent option: off = %v, err = %v", off, err)
	}
	no := "0"
	f.Option(u, model.OptionAI, &no)
	if off, err := facts.AIOptedOut(context.Background(), vo.MustParseId(u)); err != nil || !off {
		t.Fatalf("opted out: off = %v, err = %v", off, err)
	}
}
