// Imports glue: adapters satisfying the ports the imports feature declares
// (internal/imports/ports.go). Features never import each other (archtest);
// the composition root bridges them here.
package server

import (
	"context"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type importsAccountSource interface {
	AccountOwner(ctx context.Context, id vo.Id) (vo.Id, error)
	AccountDeleted(ctx context.Context, id vo.Id) (bool, error)
	AccountCurrency(ctx context.Context, id vo.Id) (vo.Id, error)
}

type importsCurrencyByID interface {
	GetByID(ctx context.Context, id string) (currencyrepo.CurrencyView, error)
}

// ImportsAccountReader answers the pipeline's account questions from the
// account service, translating the currency id into the ISO code the
// Wallet payload speaks.
type ImportsAccountReader struct {
	accounts   importsAccountSource
	currencies importsCurrencyByID
}

func NewImportsAccountReader(accounts importsAccountSource, currencies importsCurrencyByID) *ImportsAccountReader {
	return &ImportsAccountReader{accounts: accounts, currencies: currencies}
}

func (r *ImportsAccountReader) AccountOwner(ctx context.Context, id vo.Id) (vo.Id, error) {
	return r.accounts.AccountOwner(ctx, id)
}

func (r *ImportsAccountReader) AccountDeleted(ctx context.Context, id vo.Id) (bool, error) {
	return r.accounts.AccountDeleted(ctx, id)
}

func (r *ImportsAccountReader) AccountCurrencyCode(ctx context.Context, id vo.Id) (string, error) {
	currencyID, err := r.accounts.AccountCurrency(ctx, id)
	if err != nil {
		return "", err
	}
	view, err := r.currencies.GetByID(ctx, currencyID.String())
	if err != nil {
		return "", err
	}
	return view.Code, nil
}

var _ imports.AccountReader = (*ImportsAccountReader)(nil)

type importsCurrencyCodeLookup interface {
	GetIDByCodeForUser(ctx context.Context, userID, code string) (string, error)
}

type importsRateSource interface {
	BaseCurrencyID(ctx context.Context) (vo.Id, error)
	AverageRates(ctx context.Context, start, end time.Time) ([]model.FullRate, error)
	SnappedRatePeriod(ctx context.Context, start, end time.Time) (time.Time, time.Time, error)
}

type importsConvertor interface {
	Convert(ctx context.Context, periodStart, periodEnd time.Time, from, to vo.Id, sum vo.DecimalNumber) (vo.DecimalNumber, error)
}

// ImportsCurrencyConverter wraps the shared convertor with the presence
// check it lacks: the convertor treats a missing rate as 1:1, which is fine
// for a budget total but would import a foreign tap at the wrong amount.
type ImportsCurrencyConverter struct {
	lookup    importsCurrencyCodeLookup
	rates     importsRateSource
	convertor importsConvertor
}

func NewImportsCurrencyConverter(lookup importsCurrencyCodeLookup, rates importsRateSource, convertor importsConvertor) *ImportsCurrencyConverter {
	return &ImportsCurrencyConverter{lookup: lookup, rates: rates, convertor: convertor}
}

func (c *ImportsCurrencyConverter) Convert(ctx context.Context, userID vo.Id, from, to, amount string, at time.Time) (string, bool, error) {
	fromID, ok, err := c.resolve(ctx, userID, from)
	if err != nil || !ok {
		return "", false, err
	}
	toID, ok, err := c.resolve(ctx, userID, to)
	if err != nil || !ok {
		return "", false, err
	}
	if fromID.Equal(toID) {
		return vo.NewDecimal(amount).String(), true, nil
	}
	start := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	baseID, err := c.rates.BaseCurrencyID(ctx)
	if err != nil {
		return "", false, err
	}
	// The provider snaps [start,end) to the month of the latest published
	// rate at or before end, which can be arbitrarily older than the tap — a
	// currency last rated three months ago must queue as no-rate rather than
	// silently convert at a stale month's average.
	realStart, _, err := c.rates.SnappedRatePeriod(ctx, start, end)
	if err != nil {
		return "", false, err
	}
	atUTC := at.UTC()
	if realStart.Year() != atUTC.Year() || realStart.Month() != atUTC.Month() {
		return "", false, nil
	}
	rates, err := c.rates.AverageRates(ctx, start, end)
	if err != nil {
		return "", false, err
	}
	for _, id := range []vo.Id{fromID, toID} {
		if !id.Equal(baseID) && !hasRate(rates, id) {
			return "", false, nil
		}
	}
	converted, err := c.convertor.Convert(ctx, start, end, fromID, toID, vo.NewDecimal(amount))
	if err != nil {
		return "", false, err
	}
	return converted.String(), true, nil
}

func (c *ImportsCurrencyConverter) resolve(ctx context.Context, userID vo.Id, code string) (vo.Id, bool, error) {
	raw, err := c.lookup.GetIDByCodeForUser(ctx, userID.String(), code)
	if err != nil {
		if _, nf := errs.AsNotFound(err); nf {
			return vo.Id{}, false, nil
		}
		return vo.Id{}, false, err
	}
	id, err := vo.ParseId(raw)
	if err != nil {
		return vo.Id{}, false, err
	}
	return id, true, nil
}

func hasRate(rates []model.FullRate, id vo.Id) bool {
	for _, r := range rates {
		if r.CurrencyID.Equal(id) {
			return true
		}
	}
	return false
}

var _ imports.CurrencyConverter = (*ImportsCurrencyConverter)(nil)

type importsTransactionSource interface {
	ListByAccountIDs(ctx context.Context, accountIDs []vo.Id, filter model.TransactionFilter) ([]*model.Transaction, error)
}

// ImportsTransactionLister narrows the transaction repo's account-list read
// to the matcher's window.
type ImportsTransactionLister struct{ txns importsTransactionSource }

func NewImportsTransactionLister(txns importsTransactionSource) *ImportsTransactionLister {
	return &ImportsTransactionLister{txns: txns}
}

func (l *ImportsTransactionLister) ListByAccount(ctx context.Context, accountID vo.Id, from, to time.Time) ([]*model.Transaction, error) {
	return l.txns.ListByAccountIDs(ctx, []vo.Id{accountID}, model.TransactionFilter{PeriodStart: from, PeriodEnd: to})
}

var _ imports.TransactionLister = (*ImportsTransactionLister)(nil)
