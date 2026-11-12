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
	owner := vo.NewId()
	cat := &model.Category{ID: vo.NewId(), UserID: owner, Name: "Loans", Icon: "bank", Type: model.TypeExpense}
	c := NewDebtClassifications(
		stubDebtCategories{cat: cat},
		stubDebtPayees{p: &model.Payee{ID: vo.NewId(), UserID: owner, Name: "Bank"}},
		stubDebtTags{t: &model.Tag{ID: vo.NewId(), UserID: owner, Name: "Home", Icon: "home"}},
		stubDebtLabels{l: &model.Label{ID: vo.NewId(), UserID: owner, Name: "Car", Icon: "car"}},
	)

	for kind, want := range map[string]model.DebtClassification{
		"category": {OwnerID: owner, Name: "Loans", Icon: "bank", IsExpense: true},
		"payee":    {OwnerID: owner, Name: "Bank"},
		"tag":      {OwnerID: owner, Name: "Home", Icon: "home"},
		"label":    {OwnerID: owner, Name: "Car", Icon: "car"},
	} {
		got, err := c.Classification(ctx, kind, vo.NewId())
		if err != nil || got != want {
			t.Errorf("%s = %+v, %v; want %+v", kind, got, err, want)
		}
	}
	cat.Type = model.TypeIncome
	if got, _ := c.Classification(ctx, "category", cat.ID); got.IsExpense {
		t.Error("an income category is not an expense")
	}
	if _, err := c.Classification(ctx, "account", vo.NewId()); err == nil {
		t.Error("an unknown kind went through")
	}

	missing := NewDebtClassifications(stubDebtCategories{}, stubDebtPayees{}, stubDebtTags{}, stubDebtLabels{})
	for _, kind := range []string{"category", "payee", "tag", "label"} {
		if _, err := missing.Classification(ctx, kind, vo.NewId()); !isNotFoundErr(err) {
			t.Errorf("missing %s: err = %v, want not found", kind, err)
		}
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

func (r *recordingDebtUsage) LockForClassification(_ context.Context, kind string, ids ...vo.Id) error {
	r.calls = append(r.calls, fmt.Sprintf("lock %s %d", kind, len(ids)))
	return nil
}

func (r *recordingDebtUsage) ReleaseClassification(_ context.Context, kind string, _ vo.Id) error {
	r.calls = append(r.calls, "release "+kind)
	return nil
}

func (r *recordingDebtUsage) ReassignClassification(_ context.Context, kind string, _, _ vo.Id) error {
	r.calls = append(r.calls, "reassign "+kind)
	return nil
}

func TestDebtUsage_PassesItsKind(t *testing.T) {
	rec := &recordingDebtUsage{}
	u := NewDebtUsage(rec, "tag")
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
	if want := []string{"lock tag 2", "release tag", "reassign tag"}; !slices.Equal(rec.calls, want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
}

type stubDebtUserRepo struct{ user *model.User }

func (s stubDebtUserRepo) GetByID(context.Context, vo.Id) (*model.User, error) {
	if s.user == nil {
		return nil, errs.NewNotFound("User not found")
	}
	return s.user, nil
}

func (s stubDebtUserRepo) GetTimezone(context.Context, vo.Id) (string, error) {
	return "Europe/Berlin", nil
}

func TestDebtUsers(t *testing.T) {
	ctx := context.Background()
	u := model.NewUser(vo.NewId(), "u@example.test", "Una", "face:sky", "", "", time.Now())
	users := NewDebtUsers(stubDebtUserRepo{user: u})
	if name, err := users.GetName(ctx, u.ID); err != nil || name != "Una" {
		t.Errorf("GetName = %q, %v", name, err)
	}
	if tz, err := users.GetTimezone(ctx, u.ID); err != nil || tz != "Europe/Berlin" {
		t.Errorf("GetTimezone = %q, %v", tz, err)
	}
	if _, err := NewDebtUsers(stubDebtUserRepo{}).GetName(ctx, vo.NewId()); !isNotFoundErr(err) {
		t.Errorf("missing user: err = %v, want not found", err)
	}
}
