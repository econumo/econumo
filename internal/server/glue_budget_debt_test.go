package server

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type recordingBudgetElements struct {
	from, to, has model.ElementType
}

func (r *recordingBudgetElements) TransferElementData(_ context.Context, _ vo.Id, fromType model.ElementType, _ vo.Id, toType model.ElementType, _ vo.Id) error {
	r.from, r.to = fromType, toType
	return nil
}

func (r *recordingBudgetElements) ElementHasData(_ context.Context, _ vo.Id, typ model.ElementType) (bool, error) {
	r.has = typ
	return true, nil
}

func TestDebtBudgetElements_PassesTheStoredTypes(t *testing.T) {
	rec := &recordingBudgetElements{}
	a := NewDebtBudgetElements(rec)
	if err := a.TransferData(context.Background(), vo.NewId(), 1, vo.NewId(), 6, vo.NewId()); err != nil {
		t.Fatal(err)
	}
	if rec.from != model.ElementCategory || rec.to != model.ElementDebt {
		t.Errorf("transfer types = %d -> %d, want category -> debt", rec.from, rec.to)
	}
	if has, err := a.HasData(context.Background(), vo.NewId(), 6); err != nil || !has {
		t.Fatalf("HasData = %v, %v", has, err)
	}
	if rec.has != model.ElementDebt {
		t.Errorf("has-data type = %d, want debt", rec.has)
	}
}
