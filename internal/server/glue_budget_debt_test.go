package server

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type recordingBudgetElements struct {
	from, to, has, locked, removed, currencyType model.ElementType
	currency                                     vo.Id
}

func (r *recordingBudgetElements) SetElementsCurrency(_ context.Context, _ vo.Id, typ model.ElementType, currencyID vo.Id) error {
	r.currencyType, r.currency = typ, currencyID
	return nil
}

func (r *recordingBudgetElements) TransferElementData(_ context.Context, t model.ElementDataTransfer) (bool, error) {
	r.from, r.to = t.FromType, t.ToType
	return true, nil
}

func (r *recordingBudgetElements) RemoveElements(_ context.Context, _ vo.Id, typ model.ElementType) error {
	r.removed = typ
	return nil
}

func (r *recordingBudgetElements) LockElements(_ context.Context, _ vo.Id, typ model.ElementType) error {
	r.locked = typ
	return nil
}

func (r *recordingBudgetElements) ElementHasData(_ context.Context, _ vo.Id, typ model.ElementType) (bool, error) {
	r.has = typ
	return true, nil
}

func TestDebtBudgetElements_PassesTheStoredTypes(t *testing.T) {
	rec := &recordingBudgetElements{}
	a := NewDebtBudgetElements(rec)
	left, err := a.TransferData(context.Background(), model.ElementDataTransfer{FromID: vo.NewId(), FromType: model.ElementCategory,
		ToID: vo.NewId(), ToType: model.ElementDebt, CurrencyID: vo.NewId()})
	if err != nil || !left {
		t.Fatalf("TransferData = %v, %v; want the budget's leftBehind passed through", left, err)
	}
	if rec.from != model.ElementCategory || rec.to != model.ElementDebt {
		t.Errorf("transfer types = %d -> %d, want category -> debt", rec.from, rec.to)
	}
	if has, err := a.HasData(context.Background(), vo.NewId(), model.ElementDebt); err != nil || !has {
		t.Fatalf("HasData = %v, %v", has, err)
	}
	if rec.has != model.ElementDebt {
		t.Errorf("has-data type = %d, want debt", rec.has)
	}
	if err := a.LockRows(context.Background(), vo.NewId(), model.ElementDebt); err != nil || rec.locked != model.ElementDebt {
		t.Errorf("LockRows = %v, locked type %d; want debt", err, rec.locked)
	}
	if err := a.RemoveRows(context.Background(), vo.NewId(), model.ElementDebt); err != nil || rec.removed != model.ElementDebt {
		t.Errorf("RemoveRows = %v, removed type %d; want debt", err, rec.removed)
	}
	cur := vo.NewId()
	if err := a.SetRowsCurrency(context.Background(), vo.NewId(), model.ElementDebt, cur); err != nil || rec.currencyType != model.ElementDebt || !rec.currency.Equal(cur) {
		t.Errorf("SetRowsCurrency = %v, type %d currency %s; want debt %s", err, rec.currencyType, rec.currency, cur)
	}
}

type stubDebtRowPlans struct{ asked vo.Id }

func (s *stubDebtRowPlans) DebtRowPlans(_ context.Context, _, budgetID, _ vo.Id) (model.DebtBudgetPlans, bool, error) {
	s.asked = budgetID
	return model.DebtBudgetPlans{Name: "Home"}, true, nil
}

type stubBudgetOwner struct{ user *model.User }

func (s stubBudgetOwner) GetByID(context.Context, vo.Id) (*model.User, error) { return s.user, nil }

func TestDebtBudgetPlans_ActiveBudgetIsTheUsersOption(t *testing.T) {
	ctx := context.Background()
	budget := vo.NewId()
	value := budget.String()
	u := model.NewUser(vo.NewId(), "u@example.test", "u", "face:sky", "", "", time.Now())
	u.Options = []model.UserOption{model.NewUserOption(vo.NewId(), model.OptionBudget, &value, time.Now())}
	rows := &stubDebtRowPlans{}
	a := NewDebtBudgetPlans(rows, stubBudgetOwner{user: u})

	got, err := a.ActiveBudget(ctx, u.ID)
	if err != nil || got == nil || !got.Equal(budget) {
		t.Fatalf("ActiveBudget = %v, %v; want %s", got, err, budget)
	}
	if p, ok, err := a.DebtPlans(ctx, u.ID, budget, vo.NewId()); err != nil || !ok || p.Name != "Home" || !rows.asked.Equal(budget) {
		t.Fatalf("DebtPlans = %+v, %v, %v", p, ok, err)
	}

	none := model.NewUser(vo.NewId(), "n@example.test", "n", "face:sky", "", "", time.Now())
	none.Options = []model.UserOption{model.NewUserOption(vo.NewId(), model.OptionBudget, nil, time.Now())}
	if got, err := NewDebtBudgetPlans(rows, stubBudgetOwner{user: none}).ActiveBudget(ctx, none.ID); err != nil || got != nil {
		t.Fatalf("no active budget: %v, %v", got, err)
	}
}
