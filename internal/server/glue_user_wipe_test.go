package server

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

var (
	wipeUser = vo.MustParseId("01900000-0000-7000-8000-0000000000a1")
	wipeCat1 = vo.MustParseId("01900000-0000-7000-8000-0000000000c1")
	wipeCat2 = vo.MustParseId("01900000-0000-7000-8000-0000000000c2")
	wipeB1   = vo.MustParseId("01900000-0000-7000-8000-0000000000b1")
	wipeB2   = vo.MustParseId("01900000-0000-7000-8000-0000000000b2")
	wipeB3   = vo.MustParseId("01900000-0000-7000-8000-0000000000b3")
)

type wipeRecorder struct {
	calls    []string
	failOn   string
	held     map[string]bool
	budgets  []vo.Id
	debtSet  []vo.Id
	unlocked string
}

var errWipeFake = errors.New("fake failure")

func (r *wipeRecorder) rec(name string) error {
	r.calls = append(r.calls, name)
	if name == r.failOn {
		return errWipeFake
	}
	return nil
}

type fakeWipeDebt struct{ r *wipeRecorder }

func (f fakeWipeDebt) BudgetIDsOfCategories(_ context.Context, ids ...vo.Id) ([]vo.Id, error) {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = id.String()
	}
	if err := f.r.rec("debt.BudgetIDsOfCategories(" + strings.Join(names, ",") + ")"); err != nil {
		return nil, err
	}
	return f.r.debtSet, nil
}

func (f fakeWipeDebt) LockBudget(_ context.Context, id vo.Id) error {
	if err := f.r.rec("debt.LockBudget(" + id.String() + ")"); err != nil {
		return err
	}
	if id.String() != f.r.unlocked {
		f.r.held[id.String()] = true
	}
	return nil
}

func (f fakeWipeDebt) HoldsBudgetLock(_ context.Context, id vo.Id) bool {
	return f.r.held[id.String()]
}

func (f fakeWipeDebt) ReleaseCategory(_ context.Context, id vo.Id) error {
	return f.r.rec("debt.ReleaseCategory(" + id.String() + ")")
}

type fakeWipeCategories struct{ r *wipeRecorder }

func (f fakeWipeCategories) ListByOwner(context.Context, vo.Id) ([]*model.Category, error) {
	if err := f.r.rec("category.ListByOwner"); err != nil {
		return nil, err
	}
	return []*model.Category{{ID: wipeCat1}, {ID: wipeCat2}}, nil
}

type fakeWipeBudgets struct{ r *wipeRecorder }

func (f fakeWipeBudgets) WipeBudgetIDs(context.Context, vo.Id) ([]vo.Id, error) {
	if err := f.r.rec("budget.WipeBudgetIDs"); err != nil {
		return nil, err
	}
	return f.r.budgets, nil
}
func (f fakeWipeBudgets) UnwindUser(context.Context, vo.Id) error {
	return f.r.rec("budget.UnwindUser")
}
func (f fakeWipeBudgets) DeleteOwned(context.Context, vo.Id) error {
	return f.r.rec("budget.DeleteOwned")
}

type fakeWipeAccounts struct{ r *wipeRecorder }

func (f fakeWipeAccounts) LockOwnedForWipe(context.Context, vo.Id) error {
	return f.r.rec("account.LockOwnedForWipe")
}
func (f fakeWipeAccounts) UnwindGrantsOf(context.Context, vo.Id) error {
	return f.r.rec("account.UnwindGrantsOf")
}

type fakeWipeTransactions struct{ r *wipeRecorder }

func (f fakeWipeTransactions) RewriteForDeletedUser(context.Context, vo.Id) error {
	return f.r.rec("transaction.RewriteForDeletedUser")
}

type fakeWipeCurrencies struct{ r *wipeRecorder }

func (f fakeWipeCurrencies) MoveSharedCustomCurrencies(context.Context, vo.Id) error {
	return f.r.rec("currency.MoveSharedCustomCurrencies")
}

type fakeWipeApps struct{ r *wipeRecorder }

func (f fakeWipeApps) DeleteAppsOwnedBy(context.Context, vo.Id) error {
	return f.r.rec("authserver.DeleteAppsOwnedBy")
}

type fakeWipeImports struct{ r *wipeRecorder }

func (f fakeWipeImports) ClearAppliedRefsToUser(context.Context, vo.Id) error {
	return f.r.rec("imports.ClearAppliedRefsToUser")
}

type fakeWipeIdempotency struct{ r *wipeRecorder }

func (f fakeWipeIdempotency) DeleteByUser(context.Context, vo.Id) error {
	return f.r.rec("idempotency.DeleteByUser")
}

type fakeWipeOAuthStates struct{ r *wipeRecorder }

func (f fakeWipeOAuthStates) DeleteByLinkUser(context.Context, vo.Id) (int64, error) {
	return 0, f.r.rec("oauthStates.DeleteByLinkUser")
}

func newRecordingWiper() (*UserDataWiper, *wipeRecorder) {
	r := &wipeRecorder{
		held:    map[string]bool{},
		budgets: []vo.Id{wipeB3, wipeB1},
		debtSet: []vo.Id{wipeB1, wipeB2},
	}
	w := NewUserDataWiper(fakeWipeDebt{r}, fakeWipeCategories{r}, fakeWipeBudgets{r}, fakeWipeAccounts{r},
		fakeWipeTransactions{r}, fakeWipeCurrencies{r}, fakeWipeApps{r}, fakeWipeImports{r},
		fakeWipeIdempotency{r}, fakeWipeOAuthStates{r})
	return w, r
}

var (
	wipeSetReads = []string{
		"budget.WipeBudgetIDs",
		"category.ListByOwner",
		"debt.BudgetIDsOfCategories(" + wipeCat1.String() + "," + wipeCat2.String() + ")",
	}
	wipeLockCalls = []string{
		"debt.LockBudget(" + wipeB1.String() + ")",
		"debt.LockBudget(" + wipeB2.String() + ")",
		"debt.LockBudget(" + wipeB3.String() + ")",
	}
	wipeEraseChanges = []string{
		"budget.UnwindUser",
		"account.UnwindGrantsOf",
		"debt.ReleaseCategory(" + wipeCat1.String() + ")",
		"debt.ReleaseCategory(" + wipeCat2.String() + ")",
		"transaction.RewriteForDeletedUser",
		"currency.MoveSharedCustomCurrencies",
		"authserver.DeleteAppsOwnedBy",
		"imports.ClearAppliedRefsToUser",
		"idempotency.DeleteByUser",
		"oauthStates.DeleteByLinkUser",
		"budget.DeleteOwned",
	}
)

func TestUserDataWiper_Order(t *testing.T) {
	ctx := context.Background()

	t.Run("lock budgets: sorted distinct union, ascending", func(t *testing.T) {
		w, r := newRecordingWiper()
		if err := w.LockBudgets(ctx, wipeUser); err != nil {
			t.Fatalf("LockBudgets: %v", err)
		}
		want := slices.Concat(wipeSetReads, wipeLockCalls)
		if !slices.Equal(r.calls, want) {
			t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(r.calls, "\n"), strings.Join(want, "\n"))
		}
	})

	t.Run("erase order", func(t *testing.T) {
		w, r := newRecordingWiper()
		if err := w.LockBudgets(ctx, wipeUser); err != nil {
			t.Fatalf("LockBudgets: %v", err)
		}
		r.calls = nil
		if err := w.Erase(ctx, wipeUser); err != nil {
			t.Fatalf("Erase: %v", err)
		}
		want := slices.Concat([]string{"account.LockOwnedForWipe"}, wipeSetReads, wipeEraseChanges)
		if !slices.Equal(r.calls, want) {
			t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(r.calls, "\n"), strings.Join(want, "\n"))
		}
	})

	t.Run("an error stops the sequence and is returned unchanged", func(t *testing.T) {
		steps := slices.Concat(wipeSetReads, wipeLockCalls)
		for _, step := range steps {
			w, r := newRecordingWiper()
			r.failOn = step
			if err := w.LockBudgets(ctx, wipeUser); err != errWipeFake {
				t.Fatalf("LockBudgets failing at %s: err = %v", step, err)
			}
			if last := r.calls[len(r.calls)-1]; last != step {
				t.Fatalf("LockBudgets failing at %s: continued to %s", step, last)
			}
		}
		steps = slices.Concat([]string{"account.LockOwnedForWipe"}, wipeSetReads, wipeEraseChanges)
		for _, step := range steps {
			w, r := newRecordingWiper()
			if err := w.LockBudgets(ctx, wipeUser); err != nil {
				t.Fatalf("LockBudgets: %v", err)
			}
			r.calls = nil
			r.failOn = step
			if err := w.Erase(ctx, wipeUser); err != errWipeFake {
				t.Fatalf("Erase failing at %s: err = %v", step, err)
			}
			if last := r.calls[len(r.calls)-1]; last != step {
				t.Fatalf("Erase failing at %s: continued to %s", step, last)
			}
		}
	})

	t.Run("a budget not held after the account lock changes nothing", func(t *testing.T) {
		w, r := newRecordingWiper()
		if err := w.LockBudgets(ctx, wipeUser); err != nil {
			t.Fatalf("LockBudgets: %v", err)
		}
		r.calls = nil
		// A budget cloned or shared between reading the set and locking it.
		r.budgets = append(r.budgets, vo.MustParseId("01900000-0000-7000-8000-0000000000b4"))
		if err := w.Erase(ctx, wipeUser); !errors.Is(err, ErrWipeSetChanged) {
			t.Fatalf("Erase err = %v, want ErrWipeSetChanged", err)
		}
		want := slices.Concat([]string{"account.LockOwnedForWipe"}, wipeSetReads)
		if !slices.Equal(r.calls, want) {
			t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(r.calls, "\n"), strings.Join(want, "\n"))
		}
	})

	t.Run("a lock that did not register is a changed set", func(t *testing.T) {
		w, r := newRecordingWiper()
		r.unlocked = wipeB2.String()
		if err := w.LockBudgets(ctx, wipeUser); err != nil {
			t.Fatalf("LockBudgets: %v", err)
		}
		if err := w.Erase(ctx, wipeUser); !errors.Is(err, ErrWipeSetChanged) {
			t.Fatalf("Erase err = %v, want ErrWipeSetChanged", err)
		}
	})
}

func TestBuildWithServices_InstallsDeletion(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	u := f.User(fixture.User{Email: "wipe-build@example.test", Name: "W"})
	cfg := config.Config{
		DatabaseDriver:  db.Engine,
		CurrencyBase:    "USD",
		RateLimitWindow: 15 * time.Minute,
	}
	built, err := BuildWithServices(cfg, db.Raw, Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)})
	if err != nil {
		t.Fatalf("BuildWithServices: %v", err)
	}
	if built.API == nil || built.Admin == nil || built.Rates == nil || built.Users == nil {
		t.Fatalf("built = %+v, want every field set", built)
	}
	ctx := context.Background()
	if err := built.Users.AdminMarkDeleted(ctx, "wipe-build@example.test"); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}
	st, err := userrepo.NewDeletionRepo(db.Engine, db.TX).GetState(ctx, vo.MustParseId(u))
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.MarkedAt == nil {
		t.Fatal("the mark was not written")
	}
}
