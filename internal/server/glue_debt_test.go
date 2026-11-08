package server

import (
	"context"
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

type stubDebtPayees struct{ p *model.Payee }

func (s stubDebtPayees) GetByID(context.Context, vo.Id) (*model.Payee, error) {
	if s.p == nil {
		return nil, errs.NewNotFound("Payee not found")
	}
	return s.p, nil
}

type stubDebtTags struct{ t *model.Tag }

func (s stubDebtTags) GetByID(context.Context, vo.Id) (*model.Tag, error) {
	if s.t == nil {
		return nil, errs.NewNotFound("Tag not found")
	}
	return s.t, nil
}

type stubDebtLabels struct{ l *model.Label }

func (s stubDebtLabels) GetByID(context.Context, vo.Id) (*model.Label, error) {
	if s.l == nil {
		return nil, errs.NewNotFound("Label not found")
	}
	return s.l, nil
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

func TestDebtClassifications(t *testing.T) {
	ctx := context.Background()
	owner, other := vo.NewId(), vo.NewId()
	cat := &model.Category{ID: vo.NewId(), UserID: owner, Name: "Loans", Icon: "bank", Type: model.TypeExpense}
	c := NewDebtClassifications(
		stubDebtCategories{cat: cat},
		stubDebtPayees{p: &model.Payee{ID: vo.NewId(), UserID: owner}},
		stubDebtTags{t: &model.Tag{ID: vo.NewId(), UserID: owner}},
		stubDebtLabels{l: &model.Label{ID: vo.NewId(), UserID: owner}},
	)

	name, icon, isExpense, err := c.CategoryOf(ctx, owner, cat.ID)
	if err != nil || name != "Loans" || icon != "bank" || !isExpense {
		t.Fatalf("CategoryOf owner = %q %q %v %v", name, icon, isExpense, err)
	}
	if _, _, _, err := c.CategoryOf(ctx, other, cat.ID); err == nil {
		t.Fatal("a category of another user must be not found")
	} else if _, nf := errs.AsNotFound(err); !nf {
		t.Fatalf("err = %v, want not found", err)
	}
	cat.Type = model.TypeIncome
	if _, _, isExpense, _ := c.CategoryOf(ctx, owner, cat.ID); isExpense {
		t.Fatal("an income category is not an expense")
	}

	if ok, err := c.PayeeOwned(ctx, owner, vo.NewId()); err != nil || !ok {
		t.Fatalf("PayeeOwned owner = %v, %v", ok, err)
	}
	if ok, err := c.PayeeOwned(ctx, other, vo.NewId()); err != nil || ok {
		t.Fatalf("PayeeOwned other = %v, %v", ok, err)
	}
	if ok, err := c.TagOwned(ctx, owner, vo.NewId()); err != nil || !ok {
		t.Fatalf("TagOwned owner = %v, %v", ok, err)
	}
	if ok, err := c.TagOwned(ctx, other, vo.NewId()); err != nil || ok {
		t.Fatalf("TagOwned other = %v, %v", ok, err)
	}

	if ok, err := c.LabelOwned(ctx, owner, vo.NewId()); err != nil || !ok {
		t.Fatalf("LabelOwned owner = %v, %v", ok, err)
	}
	if ok, err := c.LabelOwned(ctx, other, vo.NewId()); err != nil || ok {
		t.Fatalf("LabelOwned other = %v, %v", ok, err)
	}

	missing := NewDebtClassifications(stubDebtCategories{}, stubDebtPayees{}, stubDebtTags{}, stubDebtLabels{})
	if ok, err := missing.PayeeOwned(ctx, owner, vo.NewId()); err != nil || ok {
		t.Fatalf("missing payee = %v, %v", ok, err)
	}
	if ok, err := missing.TagOwned(ctx, owner, vo.NewId()); err != nil || ok {
		t.Fatalf("missing tag = %v, %v", ok, err)
	}
	if ok, err := missing.LabelOwned(ctx, owner, vo.NewId()); err != nil || ok {
		t.Fatalf("missing label = %v, %v", ok, err)
	}
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

func (r *recordingDebtUsage) InUse(context.Context, string, vo.Id) (bool, error) { return false, nil }

func (r *recordingDebtUsage) ReassignClassification(_ context.Context, kind string, _, _ vo.Id) error {
	r.calls = append(r.calls, "reassign "+kind)
	return nil
}

func (r *recordingDebtUsage) LockForMerge(_ context.Context, kind string, _, _ vo.Id) error {
	r.calls = append(r.calls, "lock "+kind)
	return nil
}

func TestDebtUsage_PassesItsKind(t *testing.T) {
	rec := &recordingDebtUsage{}
	u := NewDebtUsage(rec, "tag")
	if err := u.PrepareMerge(context.Background(), vo.NewId(), vo.NewId()); err != nil {
		t.Fatal(err)
	}
	if err := u.Reassign(context.Background(), vo.NewId(), vo.NewId()); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0] != "lock tag" || rec.calls[1] != "reassign tag" {
		t.Fatalf("calls = %v", rec.calls)
	}
}
