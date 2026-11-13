package debt_test

import (
	"context"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func TestDebtIcon_OwnIconOverridesTheCategorys(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	if res.Icon != "i" {
		t.Fatalf("a debt without an icon shows %q, want its category's", res.Icon)
	}

	req := e.updateReq(res)
	req.Icon = "savings"
	got, err := e.svc.UpdateDebt(ctx, e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Icon != "savings" {
		t.Fatalf("icon = %q, want savings", got.Icon)
	}
	if n := len(e.snapshots(t, res.Id)); n != 1 {
		t.Fatalf("an icon change wrote a snapshot: %d", n)
	}

	// an update without an icon keeps the stored one
	req.Icon, req.Name = "", "Renamed"
	if got, err = e.svc.UpdateDebt(ctx, e.uid(), req); err != nil || got.Icon != "savings" || got.Name != "Renamed" {
		t.Fatalf("update without icon = %+v, %v", got, err)
	}
	read, err := e.svc.GetDebt(ctx, e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil || read.Item.Icon != "savings" {
		t.Fatalf("get-debt icon = %+v, %v", read, err)
	}

	views, err := e.svc.Views(ctx, e.scope(e.budget))
	if err != nil || len(views) != 1 || views[0].Icon != "savings" {
		t.Fatalf("views = %+v, %v", views, err)
	}
}

func TestDebtIcon_CreateStoresIt(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, func(r *model.CreateDebtRequest) { r.Icon = " home " })
	if res.Icon != "home" {
		t.Fatalf("icon = %q, want home", res.Icon)
	}
}

func TestDebtIcon_TooLongIsRefused(t *testing.T) {
	long := strings.Repeat("i", 65)
	requireField(t, model.UpdateDebtRequest{Id: "0190f5a0-0000-7000-8000-000000000001", Name: "Loan", Icon: long}.Validate(), "icon", errs.CodeTooLong)
	requireField(t, model.CreateDebtRequest{Name: "Loan", Icon: long}.Validate(), "icon", errs.CodeTooLong)
}
