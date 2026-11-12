// Debt glue: adapters satisfying the ports the debt feature declares
// (internal/debt/ports.go).
package server

import (
	"context"
	"fmt"
	"time"

	appcategory "github.com/econumo/econumo/internal/category"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/debt"
	applabel "github.com/econumo/econumo/internal/label"
	"github.com/econumo/econumo/internal/model"
	apppayee "github.com/econumo/econumo/internal/payee"
	"github.com/econumo/econumo/internal/shared/vo"
	apptag "github.com/econumo/econumo/internal/tag"
)

type debtCategories interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Category, error)
}

type debtPayees interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Payee, error)
}

type debtTags interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Tag, error)
}

type debtLabels interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Label, error)
}

// DebtClassifications reads the category, payee, tag or label a debt rule
// names from its repository; a missing one is the repository's not-found.
type DebtClassifications struct {
	categories debtCategories
	payees     debtPayees
	tags       debtTags
	labels     debtLabels
}

func NewDebtClassifications(categories debtCategories, payees debtPayees, tags debtTags, labels debtLabels) *DebtClassifications {
	return &DebtClassifications{categories: categories, payees: payees, tags: tags, labels: labels}
}

func (c *DebtClassifications) Classification(ctx context.Context, kind string, id vo.Id) (model.DebtClassification, error) {
	switch kind {
	case debt.KindCategory:
		cat, err := c.categories.GetByID(ctx, id)
		if err != nil {
			return model.DebtClassification{}, err
		}
		return model.DebtClassification{OwnerID: cat.UserID, Name: cat.Name, Icon: cat.Icon, IsExpense: cat.Type == model.TypeExpense}, nil
	case debt.KindPayee:
		p, err := c.payees.GetByID(ctx, id)
		if err != nil {
			return model.DebtClassification{}, err
		}
		return model.DebtClassification{OwnerID: p.UserID, Name: p.Name}, nil
	case debt.KindTag:
		t, err := c.tags.GetByID(ctx, id)
		if err != nil {
			return model.DebtClassification{}, err
		}
		return model.DebtClassification{OwnerID: t.UserID, Name: t.Name, Icon: t.Icon}, nil
	case debt.KindLabel:
		l, err := c.labels.GetByID(ctx, id)
		if err != nil {
			return model.DebtClassification{}, err
		}
		return model.DebtClassification{OwnerID: l.UserID, Name: l.Name, Icon: l.Icon}, nil
	}
	return model.DebtClassification{}, fmt.Errorf("debt glue: unknown classification kind %q", kind)
}

type debtRateSource interface {
	FractionDigits(ctx context.Context, currencyID vo.Id) (int, error)
}

type debtConvertor interface {
	Convert(ctx context.Context, periodStart, periodEnd time.Time, from, to vo.Id, sum vo.DecimalNumber) (vo.DecimalNumber, error)
}

// DebtRates converts a payment at the average rate of its own day and reads
// a currency's fraction digits.
type DebtRates struct {
	rates     debtRateSource
	convertor debtConvertor
}

func NewDebtRates(rates debtRateSource, convertor debtConvertor) *DebtRates {
	return &DebtRates{rates: rates, convertor: convertor}
}

func (r *DebtRates) ConvertOnDay(ctx context.Context, from, to vo.Id, amount vo.DecimalNumber, day time.Time) (vo.DecimalNumber, error) {
	if from.Equal(to) {
		return amount, nil
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	return r.convertor.Convert(ctx, start, start.AddDate(0, 0, 1), from, to, amount)
}

func (r *DebtRates) FractionDigits(ctx context.Context, currencyID vo.Id) (int, error) {
	return r.rates.FractionDigits(ctx, currencyID)
}

type debtUserRepo interface {
	GetByID(ctx context.Context, id vo.Id) (*model.User, error)
	GetTimezone(ctx context.Context, id vo.Id) (string, error)
}

// DebtUsers reads a user's stored timezone and name for the debt service.
type DebtUsers struct{ users debtUserRepo }

func NewDebtUsers(users debtUserRepo) *DebtUsers { return &DebtUsers{users: users} }

func (u *DebtUsers) GetTimezone(ctx context.Context, id vo.Id) (string, error) {
	return u.users.GetTimezone(ctx, id)
}

func (u *DebtUsers) GetName(ctx context.Context, id vo.Id) (string, error) {
	user, err := u.users.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	return user.Name, nil
}

type debtClassificationUsage interface {
	LockForClassification(ctx context.Context, kind string, ids ...vo.Id) error
	ReleaseClassification(ctx context.Context, kind string, id vo.Id) error
	ReassignClassification(ctx context.Context, kind string, oldID, newID vo.Id) error
}

// DebtUsage adapts the debt service to the DebtUsage port the category, payee,
// tag and label services declare; one instance serves one classification kind.
type DebtUsage struct {
	debts debtClassificationUsage
	kind  string
}

func NewDebtUsage(debts debtClassificationUsage, kind string) *DebtUsage {
	return &DebtUsage{debts: debts, kind: kind}
}

func (u *DebtUsage) Prepare(ctx context.Context, ids ...vo.Id) error {
	return u.debts.LockForClassification(ctx, u.kind, ids...)
}

func (u *DebtUsage) Release(ctx context.Context, id vo.Id) error {
	return u.debts.ReleaseClassification(ctx, u.kind, id)
}

func (u *DebtUsage) Reassign(ctx context.Context, oldID, newID vo.Id) error {
	return u.debts.ReassignClassification(ctx, u.kind, oldID, newID)
}

var (
	_ appcategory.DebtUsage = (*DebtUsage)(nil)
	_ apppayee.DebtUsage    = (*DebtUsage)(nil)
	_ apptag.DebtUsage      = (*DebtUsage)(nil)
	_ applabel.DebtUsage    = (*DebtUsage)(nil)
	_ debt.Classifications  = (*DebtClassifications)(nil)
	_ debt.Rates            = (*DebtRates)(nil)
	_ debt.Users            = (*DebtUsers)(nil)
	_ debt.Currencies       = (*currencyrepo.Lookup)(nil)
)
