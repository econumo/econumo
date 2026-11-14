package debt

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// rateDigits are the decimals a yearly rate keeps, stored and on the wire.
const rateDigits = 8

// AddDebtRateChange records a rate change on a set-up debt: from its date on
// the payments split at its rate and the forecast uses its rate and payment.
// A change on a date that has one replaces it. A change after the start while
// none is dated on the start also stores one there, with the rate the terms
// imply now and the origin's payment, so a later correction of the terms no
// longer moves the rate the loan began with.
func (s *Service) AddDebtRateChange(ctx context.Context, userID vo.Id, req model.AddDebtRateChangeRequest) (*model.DebtResult, error) {
	return inOneTx(ctx, s, func(ctx context.Context) (*model.DebtResult, error) { return s.addDebtRateChange(ctx, userID, req) })
}

func (s *Service) addDebtRateChange(ctx context.Context, userID vo.Id, req model.AddDebtRateChangeRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	date, err := time.Parse(datetime.DateLayout, strings.TrimSpace(req.Date))
	if err != nil {
		return nil, fieldError("date", "This value is not valid.", errs.CodeInvalidFormat)
	}
	annual, err := parseAnnualRate(req.AnnualRate.String())
	if err != nil {
		return nil, err
	}
	payment := vo.NewDecimal(req.Payment.String())
	if err := validatePayment(payment); err != nil {
		return nil, err
	}
	return s.withSetUpDebt(ctx, userID, id, func(ctx context.Context, cur *model.Debt, snaps []*model.DebtSnapshot, _ time.Time, digits int) error {
		origin := snapshotTerms(snaps[0])
		if date.Before(origin.AsOf) {
			return fieldError("date", "This value is out of range.", errs.CodeOutOfRange)
		}
		payment := payment.Round(digits)
		if err := validatePayment(payment); err != nil {
			return err
		}
		changes, err := s.repo.RateChanges(ctx, []vo.Id{cur.ID})
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if date.After(origin.AsOf) && !hasChangeOn(changes, origin.AsOf) {
			implied, err := model.SnapshotRate(mathTerms(nil, digits, origin)[0])
			if err != nil {
				return err
			}
			err = s.repo.SaveRateChange(ctx, &model.DebtRateChange{ID: s.repo.NextIdentity(), DebtID: cur.ID, EffectiveDate: origin.AsOf,
				AnnualRate: vo.NewDecimal(rateString(model.PeriodToAnnualRate(implied, origin.Schedule))), Payment: origin.Payment, CreatedAt: now})
			if err != nil {
				return err
			}
		}
		err = s.repo.SaveRateChange(ctx, &model.DebtRateChange{ID: s.repo.NextIdentity(), DebtID: cur.ID, EffectiveDate: date,
			AnnualRate: annual, Payment: payment, CreatedAt: now})
		if err != nil {
			return err
		}
		return s.afterRateChange(ctx, cur, date)
	})
}

// DeleteDebtRateChange removes one of a debt's rate changes; deleting the one
// on the start date brings back the rate the terms imply.
func (s *Service) DeleteDebtRateChange(ctx context.Context, userID vo.Id, req model.DeleteDebtRateChangeRequest) (*model.DebtResult, error) {
	return inOneTx(ctx, s, func(ctx context.Context) (*model.DebtResult, error) { return s.deleteDebtRateChange(ctx, userID, req) })
}

func (s *Service) deleteDebtRateChange(ctx context.Context, userID vo.Id, req model.DeleteDebtRateChangeRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	changeID, err := vo.ParseId(req.RateChangeId)
	if err != nil {
		return nil, err
	}
	return s.withSetUpDebt(ctx, userID, id, func(ctx context.Context, cur *model.Debt, _ []*model.DebtSnapshot, _ time.Time, _ int) error {
		changes, err := s.repo.RateChanges(ctx, []vo.Id{cur.ID})
		if err != nil {
			return err
		}
		for _, c := range changes {
			if !c.ID.Equal(changeID) {
				continue
			}
			deleted, err := s.repo.DeleteRateChange(ctx, cur.ID, changeID)
			if err != nil {
				return err
			}
			if !deleted {
				break
			}
			return s.afterRateChange(ctx, cur, c.EffectiveDate)
		}
		return errs.NewNotFound("Rate change not found")
	})
}

// afterRateChange drops the stored splits from the start of the due window
// holding day (a window straddling the change mixes both rates) so the next
// sync splits those payments again, then refuses a list under which the
// payments in force never repay the debt.
func (s *Service) afterRateChange(ctx context.Context, cur *model.Debt, day time.Time) error {
	from := debtDate(day)
	if start, _ := model.DueWindowContaining(cur.Current.Schedule, cur.Current.LastPaymentDate, from); !start.IsZero() && start.Before(from) {
		from = start
	}
	if err := s.dropSplitsFrom(ctx, cur, from); err != nil {
		return err
	}
	return s.checkRepayable(ctx, cur)
}

// checkRepayable refuses, inside the write's transaction, terms or rate
// changes under which what is owed is never repaid within the projection's
// horizon; payments not split yet are split in memory.
func (s *Service) checkRepayable(ctx context.Context, d *model.Debt) error {
	st, err := s.stateNow(ctx, d)
	if err != nil {
		return err
	}
	if proj := st.project(s.today(ctx), nil, nil); proj.PaymentsLeft > 0 && proj.PayoffDate.IsZero() {
		return paymentsDontCover()
	}
	return nil
}

func hasChangeOn(changes []*model.DebtRateChange, day time.Time) bool {
	for _, c := range changes {
		if c.EffectiveDate.Equal(day) {
			return true
		}
	}
	return false
}

// parseAnnualRate reads a yearly percent from 0 to 100.
func parseAnnualRate(raw string) (vo.DecimalNumber, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return vo.DecimalNumber{}, fieldError("annualRate", "This value should not be blank.", errs.CodeIsBlank)
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return vo.DecimalNumber{}, fieldError("annualRate", "This value is not valid.", errs.CodeInvalidFormat)
	}
	if f < 0 || f > 100 {
		return vo.DecimalNumber{}, fieldError("annualRate", "This value is out of range.", errs.CodeOutOfRange)
	}
	return vo.NewDecimal(rateString(f)), nil
}
