package budget_test

import (
	"context"
	"slices"
	"testing"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appbudget "github.com/econumo/econumo/internal/budget"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	domcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

type clearRecordingUsers struct {
	*server.BudgetUserLookup
	cleared []string
}

func (u *clearRecordingUsers) ClearActiveBudget(ctx context.Context, userID, budgetID vo.Id) error {
	u.cleared = append(u.cleared, userID.String())
	return u.BudgetUserLookup.ClearActiveBudget(ctx, userID, budgetID)
}

// Each clear takes that user's row lock, so a deletion must take them in one
// global order (ascending id) or two deletions sharing members can deadlock.
func TestDeleteBudget_ClearsParticipantsInAscendingUserIDOrder(t *testing.T) {
	tdb := dbtest.New(t)
	txm := tdb.TX
	f := fixture.New(t, tdb)

	const (
		low   = "11111111-1111-7111-8111-111111111111"
		mid   = "55555555-5555-7555-8555-555555555555"
		owner = "99999999-9999-7999-8999-999999999999"
	)
	f.User(fixture.User{ID: owner, Name: "Owner", Email: "owner@delete-order.test"})
	f.User(fixture.User{ID: mid, Name: "Mid", Email: "mid@delete-order.test"})
	f.User(fixture.User{ID: low, Name: "Low", Email: "low@delete-order.test"})
	budgetID := f.Budget(fixture.Budget{UserID: owner})
	f.BudgetAccess(budgetID, mid, int(model.BudgetRoleUser), true)
	f.BudgetAccess(budgetID, low, int(model.BudgetRoleAdmin), true)

	engine := tdb.Engine
	clk := clock.New()
	currencyLookup := currencyrepo.New(engine, txm)
	rateProvider := currencyrepo.NewRateProvider(engine, txm, currencyLookup, fixture.USD)
	users := &clearRecordingUsers{BudgetUserLookup: server.NewBudgetUserLookup(userrepo.NewRepo(engine, txm), clk)}
	svc := appbudget.NewService(
		budgetrepo.NewRepo(engine, txm), budgetrepo.NewReadRepo(engine, txm),
		domcurrency.NewConvertor(rateProvider), rateProvider,
		users,
		server.NewBudgetAccountLookup(accountrepo.NewRepo(engine, txm)),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(
			server.NewBudgetCategoryMetadataLookup(categoryrepo.NewRepo(engine, txm)),
			server.NewBudgetTagMetadataLookup(tagrepo.NewRepo(engine, txm)),
			server.NewBudgetPayeeMetadataLookup(payeerepo.NewRepo(engine, txm))),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(engine, txm)),
		operationrepo.NewGuard(engine, txm),
		txm, clk,
	)

	if _, err := svc.DeleteBudget(context.Background(), vo.MustParseId(owner), model.DeleteBudgetRequest{Id: budgetID}); err != nil {
		t.Fatalf("DeleteBudget: %v", err)
	}
	if want := []string{low, mid, owner}; !slices.Equal(users.cleared, want) {
		t.Errorf("ClearActiveBudget calls = %v, want %v", users.cleared, want)
	}
}
