package debt

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateDebt creates a debt in a budget the user plans in, matching payments
// through its rules. It is set up when the request carries the loan's facts
// (amount borrowed, start date, last payment date): its first snapshot, the
// origin, is as of the start date, and a balance now adds one as of today.
// Without them the debt collects its payments without terms until setup is
// finished. Each whole-category rule takes its category row's plans.
func (s *Service) CreateDebt(ctx context.Context, userID vo.Id, req model.CreateDebtRequest) (*model.DebtResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, err
	}
	rules, err := parseRules(req.Rules)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, fieldError("rules", "This value should not be blank.", errs.CodeIsBlank)
	}
	if len(rules) > maxRules {
		return nil, tooManyRules()
	}
	in, err := parseInput(debtBody{name: req.Name, currencyID: req.CurrencyId, start: req.StartDate, borrowed: req.AmountBorrowed,
		balance: req.Balance, balanceDate: req.BalanceDate, payment: req.Payment, schedule: req.Schedule, lastPayment: req.LastPaymentDate})
	if err != nil {
		return nil, err
	}
	if err := in.requirePaymentAndSchedule(); err != nil {
		return nil, err
	}
	if (in.hasOwed && !in.hasOrigin) || (in.hasOrigin && in.start == nil) {
		return nil, missingFacts(!in.hasOrigin, in.start == nil, !in.hasOrigin)
	}
	loc, err := s.userLocation(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	var d *model.Debt
	var budget model.DebtBudget
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		// a caller without access never holds the budget's lock
		if _, err := s.access(ctx, userID, budgetID); err != nil {
			return err
		}
		if err := s.repo.LockBudget(ctx, budgetID); err != nil {
			return err
		}
		b, err := s.planBudget(ctx, userID, budgetID)
		if err != nil {
			return err
		}
		budget = b
		currency, err := s.debtCurrency(ctx, userID, b, in.currency)
		if err != nil {
			return err
		}
		digits, err := s.rates.FractionDigits(ctx, currency)
		if err != nil {
			return err
		}
		d = &model.Debt{ID: s.repo.NextIdentity(), BudgetID: b.ID, CurrencyID: currency, Name: in.name, Icon: strings.TrimSpace(req.Icon),
			StartDate: in.start, CreatedDay: debtDate(now.In(loc)), CreatedAt: now, UpdatedAt: now}
		for _, r := range rules {
			d.Rules = append(d.Rules, s.mint(r, now))
		}
		sortRules(d.Rules)
		var snaps []model.DebtTerms
		if in.hasOrigin {
			if snaps, err = in.setupTerms(model.DebtTerms{Payment: in.payment, Schedule: in.schedule}, s.today(ctx), digits); err != nil {
				return err
			}
			d.AmountBorrowed = snaps[0].Balance
			d.SetTerms(snaps[len(snaps)-1])
		} else {
			d.Payment, d.Schedule = in.payment.Round(digits), in.schedule
			if err := validatePayment(d.Payment); err != nil {
				return err
			}
		}
		budgetDebts, err := s.repo.ListByBudget(ctx, b.ID)
		if err != nil {
			return err
		}
		if err := s.writeRules(ctx, b, d, d.Rules, budgetDebts); err != nil {
			return err
		}
		if err := s.repo.Save(ctx, d); err != nil {
			return err
		}
		for _, t := range snaps {
			if err := s.repo.AddSnapshot(ctx, s.snapshotOf(d.ID, t, now)); err != nil {
				return err
			}
		}
		// The debt's row exists from the start, under the budget lock held
		// here: a row first inserted by a later plan's self-heal would be
		// invisible to a concurrent delete-debt's row lock, and its plan
		// would outlive the debt without the delete's confirmation.
		if err := s.ensureRows(ctx, b.ID); err != nil {
			return err
		}
		for _, r := range d.Rules {
			if model.IsWholeCategoryRule(d, r, budgetDebts) {
				if err := s.moveToDebt(ctx, d, r.CategoryID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.resultOf(ctx, budget, d)
}

// debtCurrency is the currency a new or changed debt is in: none asked for
// takes the budget's. A currency the budget already uses (its own or a row's)
// is usable there whoever asks; any other must be usable by the user.
func (s *Service) debtCurrency(ctx context.Context, userID vo.Id, b model.DebtBudget, asked vo.Id) (vo.Id, error) {
	if asked.IsZero() {
		return b.CurrencyID, nil
	}
	if asked.Equal(b.CurrencyID) || slices.ContainsFunc(b.CurrencyIDs, asked.Equal) {
		return asked, nil
	}
	return asked, s.currencies.EnsureUsable(ctx, userID.String(), asked.String())
}

// setupTerms are the snapshots that set a debt up: the origin from the body's
// facts (base filling the payment and schedule it leaves out) and, with a
// statement balance, one as of its date under the same terms.
func (in debtInput) setupTerms(base model.DebtTerms, today time.Time, digits int) ([]model.DebtTerms, error) {
	origin := in.origin(base, digits)
	if err := validateOrigin(origin, today, digits); err != nil {
		return nil, err
	}
	out := []model.DebtTerms{origin}
	if in.hasOwed {
		owed := in.statement(origin, today, digits)
		if err := validateStatement(owed, origin, today, digits); err != nil {
			return nil, err
		}
		out = append(out, owed)
	}
	return out, nil
}
