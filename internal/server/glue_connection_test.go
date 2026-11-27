package server_test

import (
	"context"
	"slices"
	"testing"
	"time"

	appaccount "github.com/econumo/econumo/internal/account"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appbudget "github.com/econumo/econumo/internal/budget"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

const (
	revokerUSDID   = "dffc2a06-6f29-4704-8575-31709adee926"
	revokerUserA   = "11111111-1111-1111-1111-111111111111"
	revokerUserB   = "22222222-2222-2222-2222-222222222222"
	revokerBudgetA = "b0d00000-0000-0000-0000-00000000b0a1"
)

var revokerFixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func seedRevokerUser(t *testing.T, f *fixture.Builder, id string) {
	t.Helper()
	f.User(fixture.User{ID: id, Name: "u"})
}

func TestConnectionBudgetRevoker_RevokeBetween(t *testing.T) {
	db := dbtest.NewSQLite(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	seedRevokerUser(t, f, revokerUserA)
	seedRevokerUser(t, f, revokerUserB)
	// userB owns a category with a seeded element in budgetA, so RemoveMember has
	// records to clean.
	const revokerCatB = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	f.Category(fixture.Category{ID: revokerCatB, UserID: revokerUserB, Name: "Groceries", Type: 0, Icon: "local_offer"})
	// userB's active budget points at budgetA; removal must clear it (else their
	// client keeps requesting a budget that now 403s).
	activeBudget := revokerBudgetA
	f.Option(revokerUserB, "budget", &activeBudget)

	budgets, budgetSvc := newRevokerBudgetSvc(db)
	revoker := server.NewConnectionBudgetRevoker(budgets, budgetSvc)

	// userA owns budgetA; userB has an access grant on it.
	b := &model.Budget{
		ID: vo.MustParseId(revokerBudgetA), UserID: vo.MustParseId(revokerUserA), Name: "Shared",
		CurrencyID: vo.MustParseId(revokerUSDID), StartedAt: revokerFixedTime, CreatedAt: revokerFixedTime, UpdatedAt: revokerFixedTime,
	}
	if err := budgets.Save(ctx, b); err != nil {
		t.Fatalf("Save budget: %v", err)
	}
	access := &model.BudgetAccess{
		ID: vo.MustParseId(revokerBudgetA), BudgetID: vo.MustParseId(revokerBudgetA), UserID: vo.MustParseId(revokerUserB),
		Role: model.BudgetRoleUser, IsAccepted: true, CreatedAt: revokerFixedTime, UpdatedAt: revokerFixedTime,
	}
	if err := budgets.SaveAccess(ctx, access); err != nil {
		t.Fatalf("SaveAccess: %v", err)
	}
	el := model.NewBudgetElement(budgets.NextIdentity(), vo.MustParseId(revokerBudgetA), vo.MustParseId(revokerCatB), model.ElementCategory, nil, nil, revokerFixedTime)
	if err := budgets.SaveElement(ctx, el); err != nil {
		t.Fatalf("SaveElement: %v", err)
	}

	// Sanity: the grant exists.
	if list, _ := budgets.ListAccess(ctx, vo.MustParseId(revokerBudgetA)); len(list) != 1 {
		t.Fatalf("precondition: want 1 access, got %d", len(list))
	}

	if err := revoker.RevokeBetween(ctx, vo.MustParseId(revokerUserA), vo.MustParseId(revokerUserB)); err != nil {
		t.Fatalf("RevokeBetween: %v", err)
	}

	list, err := budgets.ListAccess(ctx, vo.MustParseId(revokerBudgetA))
	if err != nil {
		t.Fatalf("ListAccess after revoke: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("want budget access revoked, still %d grants", len(list))
	}
	elements, err := budgets.ListElements(ctx, vo.MustParseId(revokerBudgetA))
	if err != nil {
		t.Fatalf("ListElements after revoke: %v", err)
	}
	if len(elements) != 0 {
		t.Errorf("want userB's elements removed, still %d", len(elements))
	}
	var optValue *string
	if err := db.Raw.QueryRow(`SELECT value FROM users_options WHERE user_id = ? AND name = 'budget'`, revokerUserB).Scan(&optValue); err != nil {
		t.Fatalf("read budget option: %v", err)
	}
	if optValue != nil {
		t.Errorf("want userB's active-budget option cleared, got %q", *optValue)
	}
}

func newRevokerBudgetSvc(db *dbtest.DB) (*budgetrepo.Repo, *appbudget.Service) {
	budgets := budgetrepo.NewRepo("sqlite", db.TX)
	budgetSvc := appbudget.NewService(
		budgets, nil, nil, nil,
		server.NewBudgetUserLookup(userrepo.NewRepo("sqlite", db.TX), clock.New()),
		nil, nil,
		budgetrepo.NewMetadataLookup(
			server.NewBudgetCategoryMetadataLookup(categoryrepo.NewRepo("sqlite", db.TX)),
			server.NewBudgetTagMetadataLookup(tagrepo.NewRepo("sqlite", db.TX)),
			server.NewBudgetPayeeMetadataLookup(payeerepo.NewRepo("sqlite", db.TX)),
		),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo("sqlite", db.TX)),
		operationrepo.NewGuard("sqlite", db.TX),
		db.TX, clock.New(),
	)
	return budgets, budgetSvc
}

const (
	revokerUserP2     = "33333333-3333-3333-3333-333333333333"
	revokerBudgetP2   = "b0d00000-0000-0000-0000-00000000b0c1"
	revokerBudgetLow  = "b0d00000-0000-0000-0000-00000000b0e1"
	revokerBudgetHigh = "b0d00000-0000-0000-0000-00000000b0f2"
	revokerCatP1      = "cccccccc-cccc-cccc-cccc-ccccccccccc1"
)

// TestConnectionBudgetRevoker_LeavesThirdPartyBudgets: U and P1 are both
// members of P2's budget; disconnecting U and P1 must not touch it.
func TestConnectionBudgetRevoker_LeavesThirdPartyBudgets(t *testing.T) {
	cases := []struct {
		name string
		a, b string
	}{
		{"U then P1", revokerUserA, revokerUserB},
		{"P1 then U", revokerUserB, revokerUserA},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := dbtest.NewSQLite(t)
			ctx := context.Background()
			f := fixture.New(t, db)
			seedRevokerUser(t, f, revokerUserA)
			seedRevokerUser(t, f, revokerUserB)
			seedRevokerUser(t, f, revokerUserP2)
			f.Category(fixture.Category{ID: revokerCatP1, UserID: revokerUserB, Name: "Groceries", Type: 0, Icon: "local_offer"})
			f.Budget(fixture.Budget{ID: revokerBudgetP2, UserID: revokerUserP2, CurrencyID: revokerUSDID, Name: "P2's"})
			f.BudgetAccess(revokerBudgetP2, revokerUserA, int(model.BudgetRoleUser), true)
			f.BudgetAccess(revokerBudgetP2, revokerUserB, int(model.BudgetRoleUser), true)
			f.BudgetElement(fixture.BudgetElement{BudgetID: revokerBudgetP2, ExternalID: revokerCatP1, Type: int(model.ElementCategory)})

			budgets, budgetSvc := newRevokerBudgetSvc(db)
			revoker := server.NewConnectionBudgetRevoker(budgets, budgetSvc)

			if err := revoker.RevokeBetween(ctx, vo.MustParseId(tc.a), vo.MustParseId(tc.b)); err != nil {
				t.Fatalf("RevokeBetween: %v", err)
			}

			list, err := budgets.ListAccess(ctx, vo.MustParseId(revokerBudgetP2))
			if err != nil {
				t.Fatalf("ListAccess: %v", err)
			}
			if got := len(list); got != 2 {
				t.Errorf("access rows on P2's budget = %d, want 2", got)
			}
			els, err := budgets.ListElements(ctx, vo.MustParseId(revokerBudgetP2))
			if err != nil {
				t.Fatalf("ListElements: %v", err)
			}
			if got := len(els); got != 1 {
				t.Errorf("P1's elements on P2's budget = %d, want 1", got)
			}
		})
	}
}

type recordingRemover struct {
	inner interface {
		RemoveMemberKeepPointer(ctx context.Context, budgetID, memberID vo.Id) error
		ClearActiveBudget(ctx context.Context, userID, budgetID vo.Id) error
	}
	budgetIDs []string
	calls     []string
}

func (r *recordingRemover) RemoveMemberKeepPointer(ctx context.Context, budgetID, memberID vo.Id) error {
	r.budgetIDs = append(r.budgetIDs, budgetID.String())
	r.calls = append(r.calls, "remove "+budgetID.String())
	return r.inner.RemoveMemberKeepPointer(ctx, budgetID, memberID)
}

func (r *recordingRemover) ClearActiveBudget(ctx context.Context, userID, budgetID vo.Id) error {
	r.calls = append(r.calls, "clear "+userID.String())
	return r.inner.ClearActiveBudget(ctx, userID, budgetID)
}

// TestConnectionBudgetRevoker_ClearsPointersAfterRemovalsInUserIDOrder: the
// pointer clears take user row locks, so they follow every budget-side removal
// and go in ascending user id order, whatever order the budget ids put the
// users in. Here A < B but A's budget has the lower id, so B's removal comes
// first.
func TestConnectionBudgetRevoker_ClearsPointersAfterRemovalsInUserIDOrder(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
	}{
		{"A then B", revokerUserA, revokerUserB},
		{"B then A", revokerUserB, revokerUserA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := dbtest.NewSQLite(t)
			ctx := context.Background()
			f := fixture.New(t, db)
			seedRevokerUser(t, f, revokerUserA)
			seedRevokerUser(t, f, revokerUserB)
			f.Budget(fixture.Budget{ID: revokerBudgetLow, UserID: revokerUserA, CurrencyID: revokerUSDID, Name: "A's"})
			f.BudgetAccess(revokerBudgetLow, revokerUserB, int(model.BudgetRoleUser), true)
			f.Budget(fixture.Budget{ID: revokerBudgetHigh, UserID: revokerUserB, CurrencyID: revokerUSDID, Name: "B's"})
			f.BudgetAccess(revokerBudgetHigh, revokerUserA, int(model.BudgetRoleUser), true)
			low, high := revokerBudgetLow, revokerBudgetHigh
			f.Option(revokerUserA, model.OptionBudget, &high)
			f.Option(revokerUserB, model.OptionBudget, &low)

			budgets, budgetSvc := newRevokerBudgetSvc(db)
			rec := &recordingRemover{inner: budgetSvc}
			revoker := server.NewConnectionBudgetRevoker(budgets, rec)

			if err := db.TX.WithTx(ctx, func(ctx context.Context) error {
				return revoker.RevokeBetween(ctx, vo.MustParseId(tc.a), vo.MustParseId(tc.b))
			}); err != nil {
				t.Fatalf("RevokeBetween: %v", err)
			}

			want := []string{
				"remove " + revokerBudgetLow,
				"remove " + revokerBudgetHigh,
				"clear " + revokerUserA,
				"clear " + revokerUserB,
			}
			if !slices.Equal(rec.calls, want) {
				t.Errorf("calls = %v, want %v", rec.calls, want)
			}
			for _, uid := range []string{revokerUserA, revokerUserB} {
				var v *string
				if err := db.Raw.QueryRow(`SELECT value FROM users_options WHERE user_id = ? AND name = 'budget'`, uid).Scan(&v); err != nil {
					t.Fatalf("read budget option: %v", err)
				}
				if v != nil {
					t.Errorf("user %s: active budget = %q, want NULL", uid, *v)
				}
			}
		})
	}
}

// TestConnectionBudgetRevoker_RemovesInBudgetIDOrder: removals of both
// directions run in one pass ordered by budget id, the order budget locks are
// taken everywhere.
func TestConnectionBudgetRevoker_RemovesInBudgetIDOrder(t *testing.T) {
	cases := []struct {
		name string
		a, b string
	}{
		{"A then B", revokerUserA, revokerUserB},
		{"B then A", revokerUserB, revokerUserA},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := dbtest.NewSQLite(t)
			ctx := context.Background()
			f := fixture.New(t, db)
			seedRevokerUser(t, f, revokerUserA)
			seedRevokerUser(t, f, revokerUserB)
			f.Budget(fixture.Budget{ID: revokerBudgetHigh, UserID: revokerUserA, CurrencyID: revokerUSDID, Name: "A's"})
			f.BudgetAccess(revokerBudgetHigh, revokerUserB, int(model.BudgetRoleUser), true)
			f.Budget(fixture.Budget{ID: revokerBudgetLow, UserID: revokerUserB, CurrencyID: revokerUSDID, Name: "B's"})
			f.BudgetAccess(revokerBudgetLow, revokerUserA, int(model.BudgetRoleUser), true)

			budgets, budgetSvc := newRevokerBudgetSvc(db)
			rec := &recordingRemover{inner: budgetSvc}
			revoker := server.NewConnectionBudgetRevoker(budgets, rec)

			if err := revoker.RevokeBetween(ctx, vo.MustParseId(tc.a), vo.MustParseId(tc.b)); err != nil {
				t.Fatalf("RevokeBetween: %v", err)
			}

			want := []string{revokerBudgetLow, revokerBudgetHigh}
			if !slices.Equal(rec.budgetIDs, want) {
				t.Errorf("RemoveMemberKeepPointer budget order = %v, want %v", rec.budgetIDs, want)
			}
			for _, id := range want {
				list, err := budgets.ListAccess(ctx, vo.MustParseId(id))
				if err != nil {
					t.Fatalf("ListAccess: %v", err)
				}
				if len(list) != 0 {
					t.Errorf("budget %s: %d access rows left, want 0", id, len(list))
				}
			}
		})
	}
}

const (
	revokerAccountA = "a0000000-0000-0000-0000-00000000a0a1" // owned by revokerUserA
	revokerAccountB = "b0000000-0000-0000-0000-00000000b0b1" // owned by revokerUserB
)

// isNotFoundErr reports whether err is (or wraps) a *errs.NotFoundError.
func isNotFoundErr(err error) bool {
	_, ok := errs.AsNotFound(err)
	return ok
}

// newRevokerAccountSvc wires a real account.Service over sqlite repos, mirroring
// how internal/server/server.go builds accountSvc.
func newRevokerAccountSvc(t *testing.T, db *dbtest.DB) *appaccount.Service {
	t.Helper()
	txm := db.TX
	repo := accountrepo.NewRepo(db.Engine, txm)
	folderRepo := accountrepo.NewFolderRepo(db.Engine, txm)
	accessRepo := accountrepo.NewAccessRepo(db.Engine, txm)
	curLookup := server.NewAccountCurrencyLookup(currencyrepo.New(db.Engine, txm))
	userLookup := server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm))
	opGuard := operationrepo.NewGuard(db.Engine, txm)
	connections := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, txm))
	return appaccount.NewService(repo, folderRepo, accessRepo, curLookup, userLookup, connections, txm, opGuard, clock.New())
}

// TestConnectionAccountAccessRevoker_RevokeAccessBetween exercises the REAL
// account service (the same wiring server.go uses) through the
// ConnectionAccountAccessRevoker adapter that backs delete-connection's
// account-access unwind. It covers the case that matters most: a PENDING
// grant and an ACCEPTED grant, one in EACH direction between the pair, must
// both be removed.
func TestConnectionAccountAccessRevoker_RevokeAccessBetween(t *testing.T) {
	db := dbtest.NewSQLite(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	f.User(fixture.User{ID: revokerUserA, Name: "A"})
	f.User(fixture.User{ID: revokerUserB, Name: "B"})
	f.Account(fixture.Account{ID: revokerAccountA, UserID: revokerUserA, Name: "Owned by A"})
	f.Account(fixture.Account{ID: revokerAccountB, UserID: revokerUserB, Name: "Owned by B"})

	// A owns accountA and grants B a PENDING share; B owns accountB and grants
	// A an ACCEPTED share — one grant each direction, one of each state.
	f.AccountAccessPending(revokerAccountA, revokerUserB, 1)
	f.AccountAccess(revokerAccountB, revokerUserA, 1)

	accountSvc := newRevokerAccountSvc(t, db)
	accessRepo := accountrepo.NewAccessRepo("sqlite", db.TX)
	revoker := server.NewConnectionAccountAccessRevoker(accountSvc)

	if err := revoker.RevokeAccessBetween(ctx, vo.MustParseId(revokerUserA), vo.MustParseId(revokerUserB)); err != nil {
		t.Fatalf("RevokeAccessBetween: %v", err)
	}

	if _, err := accessRepo.Get(ctx, vo.MustParseId(revokerAccountA), vo.MustParseId(revokerUserB)); !isNotFoundErr(err) {
		t.Errorf("pending grant (A's account -> B) not removed: err=%v", err)
	}
	if _, err := accessRepo.Get(ctx, vo.MustParseId(revokerAccountB), vo.MustParseId(revokerUserA)); !isNotFoundErr(err) {
		t.Errorf("accepted grant (B's account -> A) not removed: err=%v", err)
	}
}
