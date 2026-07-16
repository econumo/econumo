package transaction_test

import (
	"context"
	"testing"

	appaccount "github.com/econumo/econumo/internal/account"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	apptransaction "github.com/econumo/econumo/internal/transaction"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func newReadService(t *testing.T, db *dbtest.DB) *apptransaction.Service {
	t.Helper()
	txm := db.TX
	curLookup := currencyrepo.New(db.Engine, txm)
	accSvc := appaccount.NewService(
		accountrepo.NewRepo(db.Engine, txm), accountrepo.NewFolderRepo(db.Engine, txm),
		accountrepo.NewAccessRepo(db.Engine, txm),
		server.NewAccountCurrencyLookup(curLookup), server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm)),
		nil, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(),
	)
	txRepo := transactionrepo.NewRepo(db.Engine, txm)
	accessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, txm))
	return apptransaction.NewService(
		txRepo, accSvc, accessResolver, accSvc,
		server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm)),
		nil, nil, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(),
	)
}

// TestGetTransactionList_AccountAndPeriod: accountId combined with a full
// period window narrows the single account's list to [periodStart, periodEnd)
// while still excluding other accounts' transactions inside the window.
func TestGetTransactionList_AccountAndPeriod(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	acctA := f.Account(fixture.Account{UserID: userID})
	acctB := f.Account(fixture.Account{UserID: userID})
	catID := f.Category(fixture.Category{UserID: userID, Type: 0})

	inWindow := f.Transaction(fixture.Transaction{
		UserID: userID, AccountID: acctA, CategoryID: catID, SpentAt: "2024-04-01 15:30:00",
	})
	f.Transaction(fixture.Transaction{
		UserID: userID, AccountID: acctA, CategoryID: catID, SpentAt: "2024-04-03 10:00:00",
	})
	f.Transaction(fixture.Transaction{
		UserID: userID, AccountID: acctB, CategoryID: catID, SpentAt: "2024-04-01 12:00:00",
	})

	svc := newReadService(t, db)
	res, err := svc.GetTransactionList(context.Background(), mustID(t, userID), model.TransactionListRequest{
		AccountId:   acctA,
		PeriodStart: "2024-04-01 00:00:00",
		PeriodEnd:   "2024-04-02 00:00:00",
	})
	if err != nil {
		t.Fatalf("GetTransactionList: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %d, want 1: %#v", len(res.Items), res.Items)
	}
	if res.Items[0].Id != inWindow {
		t.Fatalf("item id = %q, want %q (the in-window transaction on account A)", res.Items[0].Id, inWindow)
	}
}

// TestGetTransactionList_AccountWithLonePeriodBound: a lone periodStart is
// ignored when accountId is set (REST-compatible), so the full account list
// comes back.
func TestGetTransactionList_AccountWithLonePeriodBound(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	acctA := f.Account(fixture.Account{UserID: userID})
	catID := f.Category(fixture.Category{UserID: userID, Type: 0})

	f.Transaction(fixture.Transaction{
		UserID: userID, AccountID: acctA, CategoryID: catID, SpentAt: "2024-04-01 15:30:00",
	})
	f.Transaction(fixture.Transaction{
		UserID: userID, AccountID: acctA, CategoryID: catID, SpentAt: "2024-04-03 10:00:00",
	})

	svc := newReadService(t, db)
	res, err := svc.GetTransactionList(context.Background(), mustID(t, userID), model.TransactionListRequest{
		AccountId:   acctA,
		PeriodStart: "2024-04-02 00:00:00",
	})
	if err != nil {
		t.Fatalf("GetTransactionList: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %d, want 2 (lone period bound must be ignored): %#v", len(res.Items), res.Items)
	}
}

func mustID(t *testing.T, s string) vo.Id {
	t.Helper()
	id, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("parse id %q: %v", s, err)
	}
	return id
}
