package server

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type recordingBudgetElements struct {
	from, to, has, locked, removed model.ElementType
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
}
