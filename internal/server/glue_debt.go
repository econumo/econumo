// Debt glue: adapters satisfying the ports the debt feature declares
// (internal/debt/ports.go).
package server

import (
	"context"
	"time"

	appcategory "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/debt"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type debtCategories interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Category, error)
}

// DebtCategories reads a debt's category from the category repository; a
// missing one is the repository's not-found.
type DebtCategories struct{ categories debtCategories }

func NewDebtCategories(categories debtCategories) *DebtCategories {
	return &DebtCategories{categories: categories}
}

func (c *DebtCategories) Category(ctx context.Context, id vo.Id) (model.DebtClassification, error) {
	cat, err := c.categories.GetByID(ctx, id)
	if err != nil {
		return model.DebtClassification{}, err
	}
	return model.DebtClassification{Name: cat.Name, Icon: cat.Icon}, nil
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
	GetTimezone(ctx context.Context, id vo.Id) (string, error)
}

// DebtUsers reads a user's stored timezone for the debt service.
type DebtUsers struct{ users debtUserRepo }

func NewDebtUsers(users debtUserRepo) *DebtUsers { return &DebtUsers{users: users} }

func (u *DebtUsers) GetTimezone(ctx context.Context, id vo.Id) (string, error) {
	return u.users.GetTimezone(ctx, id)
}

type debtCategoryUsage interface {
	LockForCategories(ctx context.Context, ids ...vo.Id) error
	ReleaseCategory(ctx context.Context, id vo.Id) error
	ReassignCategory(ctx context.Context, oldID, newID vo.Id) error
}

// DebtUsage adapts the debt service to the category service's DebtUsage port.
type DebtUsage struct{ debts debtCategoryUsage }

func NewDebtUsage(debts debtCategoryUsage) *DebtUsage { return &DebtUsage{debts: debts} }

func (u *DebtUsage) Prepare(ctx context.Context, ids ...vo.Id) error {
	return u.debts.LockForCategories(ctx, ids...)
}

func (u *DebtUsage) Release(ctx context.Context, id vo.Id) error {
	return u.debts.ReleaseCategory(ctx, id)
}

func (u *DebtUsage) Reassign(ctx context.Context, oldID, newID vo.Id) error {
	return u.debts.ReassignCategory(ctx, oldID, newID)
}

var (
	_ appcategory.DebtUsage = (*DebtUsage)(nil)
	_ debt.Categories       = (*DebtCategories)(nil)
	_ debt.Rates            = (*DebtRates)(nil)
	_ debt.Users            = (*DebtUsers)(nil)
)
