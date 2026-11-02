// Debt glue: adapters satisfying the ports the debt feature declares
// (internal/debt/ports.go).
package server

import (
	"context"
	"time"

	appcategory "github.com/econumo/econumo/internal/category"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/debt"
	"github.com/econumo/econumo/internal/model"
	apppayee "github.com/econumo/econumo/internal/payee"
	"github.com/econumo/econumo/internal/shared/errs"
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

// DebtClassifications answers ownership questions from the category, payee
// and tag repositories.
type DebtClassifications struct {
	categories debtCategories
	payees     debtPayees
	tags       debtTags
}

func NewDebtClassifications(categories debtCategories, payees debtPayees, tags debtTags) *DebtClassifications {
	return &DebtClassifications{categories: categories, payees: payees, tags: tags}
}

// CategoryOf masks a category of another user as not found.
func (c *DebtClassifications) CategoryOf(ctx context.Context, ownerID, id vo.Id) (string, string, bool, error) {
	cat, err := c.categories.GetByID(ctx, id)
	if err != nil {
		return "", "", false, err
	}
	if !cat.UserID.Equal(ownerID) {
		return "", "", false, errs.NewNotFound("Category not found")
	}
	return cat.Name, cat.Icon, cat.Type == model.TypeExpense, nil
}

func (c *DebtClassifications) PayeeOwned(ctx context.Context, ownerID, id vo.Id) (bool, error) {
	p, err := c.payees.GetByID(ctx, id)
	if err != nil {
		return false, ignoreNotFound(err)
	}
	return p.UserID.Equal(ownerID), nil
}

func (c *DebtClassifications) TagOwned(ctx context.Context, ownerID, id vo.Id) (bool, error) {
	t, err := c.tags.GetByID(ctx, id)
	if err != nil {
		return false, ignoreNotFound(err)
	}
	return t.UserID.Equal(ownerID), nil
}

func ignoreNotFound(err error) error {
	if _, nf := errs.AsNotFound(err); nf {
		return nil
	}
	return err
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

type debtClassificationUsage interface {
	InUse(ctx context.Context, kind string, id vo.Id) (bool, error)
	ReassignClassification(ctx context.Context, kind string, oldID, newID vo.Id) error
}

// DebtUsage adapts the debt service to the DebtUsage port the category, payee
// and tag services declare; one instance serves one classification kind.
type DebtUsage struct {
	debts debtClassificationUsage
	kind  string
}

func NewDebtUsage(debts debtClassificationUsage, kind string) *DebtUsage {
	return &DebtUsage{debts: debts, kind: kind}
}

func (u *DebtUsage) InUse(ctx context.Context, id vo.Id) (bool, error) {
	return u.debts.InUse(ctx, u.kind, id)
}

func (u *DebtUsage) Reassign(ctx context.Context, oldID, newID vo.Id) error {
	return u.debts.ReassignClassification(ctx, u.kind, oldID, newID)
}

var (
	_ appcategory.DebtUsage = (*DebtUsage)(nil)
	_ apppayee.DebtUsage    = (*DebtUsage)(nil)
	_ apptag.DebtUsage      = (*DebtUsage)(nil)
	_ debt.Classifications  = (*DebtClassifications)(nil)
	_ debt.Rates            = (*DebtRates)(nil)
	_ debt.Currencies       = (*currencyrepo.Lookup)(nil)
)
