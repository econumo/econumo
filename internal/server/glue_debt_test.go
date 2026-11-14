package server

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type stubDebtCategories struct{ cat *model.Category }

func (s stubDebtCategories) GetByID(context.Context, vo.Id) (*model.Category, error) {
	if s.cat == nil {
		return nil, errs.NewNotFound("Category not found")
	}
	return s.cat, nil
}

type recordingConvertor struct {
	start, end time.Time
	calls      int
}

func (c *recordingConvertor) Convert(_ context.Context, start, end time.Time, _, _ vo.Id, sum vo.DecimalNumber) (vo.DecimalNumber, error) {
	c.start, c.end, c.calls = start, end, c.calls+1
	return sum.Mul(vo.NewDecimal("2")), nil
}

type stubDigits struct{}

func (stubDigits) FractionDigits(context.Context, vo.Id) (int, error) { return 3, nil }

func TestDebtCategories(t *testing.T) {
	ctx := context.Background()
	owner := vo.NewId()
	cat := &model.Category{ID: vo.NewId(), UserID: owner, Name: "Loans", Icon: "bank", Type: model.TypeExpense}
	c := NewDebtCategories(stubDebtCategories{cat: cat})
	want := model.DebtClassification{OwnerID: owner, Name: "Loans", Icon: "bank", IsExpense: true}
	if got, err := c.Category(ctx, cat.ID); err != nil || got != want {
		t.Errorf("category = %+v, %v; want %+v", got, err, want)
	}
	cat.Type = model.TypeIncome
	if got, _ := c.Category(ctx, cat.ID); got.IsExpense {
		t.Error("an income category is not an expense")
	}
	if _, err := NewDebtCategories(stubDebtCategories{}).Category(ctx, vo.NewId()); !isNotFoundErr(err) {
		t.Errorf("missing category: err = %v, want not found", err)
	}
}

func isNotFoundErr(err error) bool {
	_, ok := errs.AsNotFound(err)
	return ok
}

func TestDebtRates_ConvertOnDay(t *testing.T) {
	ctx := context.Background()
	conv := &recordingConvertor{}
	r := NewDebtRates(stubDigits{}, conv)
	from, to := vo.NewId(), vo.NewId()

	got, err := r.ConvertOnDay(ctx, from, to, vo.NewDecimal("10"), time.Date(2026, 3, 31, 15, 30, 0, 0, time.UTC))
	if err != nil || got.String() != "20" {
		t.Fatalf("converted = %v, %v", got, err)
	}
	if want := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC); !conv.start.Equal(want) || !conv.end.Equal(want.AddDate(0, 0, 1)) {
		t.Fatalf("window = [%v, %v), want the day [2026-03-31, 2026-04-01)", conv.start, conv.end)
	}

	got, err = r.ConvertOnDay(ctx, from, from, vo.NewDecimal("10"), time.Now())
	if err != nil || got.String() != "10" || conv.calls != 1 {
		t.Fatalf("same currency = %v, %v after %d convertor calls", got, err, conv.calls)
	}
	if d, err := r.FractionDigits(ctx, from); err != nil || d != 3 {
		t.Fatalf("FractionDigits = %d, %v", d, err)
	}
}

type recordingDebtUsage struct{ calls []string }

func (r *recordingDebtUsage) LockForCategories(_ context.Context, ids ...vo.Id) error {
	r.calls = append(r.calls, fmt.Sprintf("lock %d", len(ids)))
	return nil
}

func (r *recordingDebtUsage) ReleaseCategory(context.Context, vo.Id) error {
	r.calls = append(r.calls, "release")
	return nil
}

func (r *recordingDebtUsage) ReassignCategory(context.Context, vo.Id, vo.Id) error {
	r.calls = append(r.calls, "reassign")
	return nil
}

func TestDebtUsage_PassesThrough(t *testing.T) {
	rec := &recordingDebtUsage{}
	u := NewDebtUsage(rec)
	ctx := context.Background()
	if err := u.Prepare(ctx, vo.NewId(), vo.NewId()); err != nil {
		t.Fatal(err)
	}
	if err := u.Release(ctx, vo.NewId()); err != nil {
		t.Fatal(err)
	}
	if err := u.Reassign(ctx, vo.NewId(), vo.NewId()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"lock 2", "release", "reassign"}; !slices.Equal(rec.calls, want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
}

type stubDebtUserRepo struct{}

func (stubDebtUserRepo) GetTimezone(context.Context, vo.Id) (string, error) {
	return "Europe/Berlin", nil
}

func TestDebtUsers(t *testing.T) {
	users := NewDebtUsers(stubDebtUserRepo{})
	if tz, err := users.GetTimezone(context.Background(), vo.NewId()); err != nil || tz != "Europe/Berlin" {
		t.Errorf("GetTimezone = %q, %v", tz, err)
	}
}
