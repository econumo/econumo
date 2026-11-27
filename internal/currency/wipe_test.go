package currency_test

import (
	"context"
	"testing"
	"time"

	appcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// Explicit ids so "lowest" is unambiguous on both engines; the deleted user
// sorts first, so a query that forgot to exclude it would pick it.
const (
	moveU = "10000000-0000-7000-8000-000000000001"
	moveT = "30000000-0000-7000-8000-000000000003"
	moveP = "50000000-0000-7000-8000-000000000005"
)

type moveEnv struct {
	t   *testing.T
	db  *dbtest.DB
	f   *fixture.Builder
	svc *appcurrency.ManageService
}

func newMoveEnv(t *testing.T) *moveEnv {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: moveU, Name: "Wiped", Email: "wipe-cur-u@example.test"})
	f.User(fixture.User{ID: moveP, Name: "Partner", Email: "wipe-cur-p@example.test"})
	f.User(fixture.User{ID: moveT, Name: "Third", Email: "wipe-cur-t@example.test"})
	svc := appcurrency.NewManageService(currencyrepo.NewManageRepo(db.Engine, db.TX), db.TX, nil, clock.New(), nil,
		model.BaseCurrency{ID: fixture.USD, Code: "USD"})
	return &moveEnv{t: t, db: db, f: f, svc: svc}
}

func (e *moveEnv) custom(owner, code string) string {
	return e.f.Currency(fixture.Currency{Code: code, Symbol: "P", UserID: owner})
}

func (e *moveEnv) move() {
	e.t.Helper()
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		return e.svc.MoveSharedCustomCurrencies(ctx, vo.MustParseId(moveU))
	})
	if err != nil {
		e.t.Fatalf("MoveSharedCustomCurrencies: %v", err)
	}
}

func (e *moveEnv) row(id string) (owner string, deleted bool) {
	e.t.Helper()
	var o *string
	if err := e.db.Raw.QueryRow(e.db.Rebind(`SELECT user_id, is_deleted FROM currencies WHERE id = ?`), id).Scan(&o, &deleted); err != nil {
		e.t.Fatalf("read currency %s: %v", id, err)
	}
	if o != nil {
		owner = *o
	}
	return owner, deleted
}

func (e *moveEnv) requireRow(id, wantOwner string, wantDeleted bool) {
	e.t.Helper()
	owner, deleted := e.row(id)
	if owner != wantOwner || deleted != wantDeleted {
		e.t.Errorf("currency %s: owner %s deleted %v, want owner %s deleted %v", id, owner, deleted, wantOwner, wantDeleted)
	}
}

func (e *moveEnv) debt(budgetID, currencyID string) {
	e.t.Helper()
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	_, err := e.db.Raw.Exec(e.db.Rebind(`INSERT INTO debts (id, budget_id, element_external_id, element_type, currency_id, payment, schedule, created_day, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		vo.NewId().String(), budgetID, vo.NewId().String(), "category", currencyID, "0", "none", "2026-03-01", now, now)
	if err != nil {
		e.t.Fatalf("insert debt: %v", err)
	}
}

func TestMove_ToTheOnlyForeignOwner(t *testing.T) {
	e := newMoveEnv(t)
	pts := e.custom(moveU, "PTS")
	e.f.Budget(fixture.Budget{UserID: moveU, CurrencyID: pts})
	e.f.Budget(fixture.Budget{UserID: moveP, CurrencyID: pts})

	e.move()

	e.requireRow(pts, moveP, false)
}

func TestMove_ElementDebtAndAccountCount(t *testing.T) {
	cases := map[string]func(e *moveEnv, pts string){
		"element": func(e *moveEnv, pts string) {
			b := e.f.Budget(fixture.Budget{UserID: moveP})
			e.f.BudgetElement(fixture.BudgetElement{BudgetID: b, CurrencyID: pts, ExternalID: vo.NewId().String(), Type: 0})
		},
		"debt": func(e *moveEnv, pts string) {
			e.debt(e.f.Budget(fixture.Budget{UserID: moveP}), pts)
		},
		"account": func(e *moveEnv, pts string) {
			e.f.Account(fixture.Account{UserID: moveP, CurrencyID: pts})
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			e := newMoveEnv(t)
			pts := e.custom(moveU, "PTS")
			seed(e, pts)

			e.move()

			e.requireRow(pts, moveP, false)
		})
	}
}

func TestMove_LowestUserIDWins(t *testing.T) {
	e := newMoveEnv(t)
	pts := e.custom(moveU, "PTS")
	e.f.Budget(fixture.Budget{UserID: moveP, CurrencyID: pts})
	e.f.Budget(fixture.Budget{UserID: moveT, CurrencyID: pts})

	e.move()

	e.requireRow(pts, moveT, false)
}

func TestMove_CollisionArrivesSoftDeleted(t *testing.T) {
	e := newMoveEnv(t)
	pts := e.custom(moveU, "PTS")
	own := e.custom(moveP, "PTS")
	e.f.Budget(fixture.Budget{UserID: moveP, CurrencyID: pts})

	e.move()

	e.requireRow(pts, moveP, true)
	e.requireRow(own, moveP, false)
}

func TestMove_UnusedStays(t *testing.T) {
	e := newMoveEnv(t)
	pts := e.custom(moveU, "PTS")
	e.f.Budget(fixture.Budget{UserID: moveU, CurrencyID: pts})

	e.move()

	e.requireRow(pts, moveU, false)
}

func TestMove_RatesFollow(t *testing.T) {
	e := newMoveEnv(t)
	pts := e.custom(moveU, "PTS")
	e.f.Rate(fixture.Rate{CurrencyID: pts, Rate: "2.00000000", PublishedAt: "2026-03-01"})
	e.f.Budget(fixture.Budget{UserID: moveP, CurrencyID: pts})

	e.move()

	e.requireRow(pts, moveP, false)
	var n int
	if err := e.db.Raw.QueryRow(e.db.Rebind(`SELECT COUNT(*) FROM currencies_rates WHERE currency_id = ?`), pts).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rates of the moved currency = %d, want 1", n)
	}
}

func TestCurrencyList_OwnedReferences(t *testing.T) {
	cases := map[string]func(e *moveEnv, pts string){
		"budget": func(e *moveEnv, pts string) {
			e.f.Budget(fixture.Budget{UserID: moveT, CurrencyID: pts})
		},
		"element": func(e *moveEnv, pts string) {
			b := e.f.Budget(fixture.Budget{UserID: moveT})
			e.f.BudgetElement(fixture.BudgetElement{BudgetID: b, CurrencyID: pts, ExternalID: vo.NewId().String(), Type: 0})
		},
		"debt": func(e *moveEnv, pts string) {
			e.debt(e.f.Budget(fixture.Budget{UserID: moveT}), pts)
		},
		"account": func(e *moveEnv, pts string) {
			e.f.Account(fixture.Account{UserID: moveT, CurrencyID: pts})
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			e := newMoveEnv(t)
			// The partner's id is lower than the third user's here, so the
			// currency lands on the partner and the third user keeps only a
			// reference, with no access rows.
			partner := "20000000-0000-7000-8000-000000000002"
			e.f.User(fixture.User{ID: partner, Name: "Low", Email: "wipe-cur-low@example.test"})
			pts := e.custom(moveU, "PTS")
			e.f.Budget(fixture.Budget{UserID: partner, CurrencyID: pts})
			seed(e, pts)

			e.move()
			e.requireRow(pts, partner, false)

			read := appcurrency.NewReadService(currencyrepo.NewReadRepo(e.db.Engine, e.db.TX), model.BaseCurrency{ID: fixture.USD, Code: "USD"})
			res, err := read.GetCurrencyList(context.Background(), vo.MustParseId(moveT))
			if err != nil {
				t.Fatalf("GetCurrencyList: %v", err)
			}
			found := false
			for _, it := range res.Items {
				if it.Id == pts {
					found = true
					if it.Scope != appcurrency.ScopeShared {
						t.Errorf("scope = %q, want %q", it.Scope, appcurrency.ScopeShared)
					}
				}
			}
			if !found {
				t.Errorf("third user's currency list lacks the moved currency %s", pts)
			}
		})
	}
}
