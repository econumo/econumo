package debt

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateDebt attaches a debt to an element of a budget the user plans in: a
// top-level expense category or an envelope that carries no active debt. Its
// currency is the element row's; its payments are the untagged expenses the
// element's Spent counts. A start date must follow the end of the element's
// previous debt. A body with the loan's facts creates it set up, its origin
// checked as update-debt checks a setup; without them it is not set up.
func (s *Service) CreateDebt(ctx context.Context, userID vo.Id, req model.CreateDebtRequest) (*model.DebtResult, error) {
	return inOneTx(ctx, s, func(ctx context.Context) (*model.DebtResult, error) { return s.createDebt(ctx, userID, req) })
}

func (s *Service) createDebt(ctx context.Context, userID vo.Id, req model.CreateDebtRequest) (*model.DebtResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, err
	}
	elementID, err := vo.ParseId(req.ElementId)
	if err != nil {
		return nil, err
	}
	in, err := parseInput(debtBody{start: req.StartDate, borrowed: req.AmountBorrowed, payment: req.Payment,
		schedule: req.Schedule, lastPayment: req.LastPaymentDate})
	if err != nil {
		return nil, err
	}
	if err := in.requirePaymentAndSchedule(); err != nil {
		return nil, err
	}
	if in.hasOrigin && in.start == nil {
		return nil, missingFacts(false, true, false)
	}
	loc, err := s.userLocation(ctx, userID)
	if err != nil {
		return nil, err
	}
	today, now := s.today(ctx), s.clock.Now()
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
		el, err := s.element(ctx, b.ID, elementID)
		if err != nil {
			return err
		}
		if !el.Eligible {
			return notEligible()
		}
		// a category delete committing meanwhile would leave the debt on
		// nothing: it waits for this write and then deletes the debt
		if el.Type == model.DebtElementCategory {
			if err := s.repo.LockCategories(ctx, []vo.Id{el.ID}); err != nil {
				return err
			}
		}
		budgetDebts, err := s.repo.ListByBudget(ctx, b.ID)
		if err != nil {
			return err
		}
		others := onElement(budgetDebts, el.ID, nil)
		for _, o := range others {
			if !o.IsPaidOff() {
				return elementHasDebt()
			}
		}
		if err := checkStart(in.start, previousEnd(others, nil)); err != nil {
			return err
		}
		digits, err := s.rates.FractionDigits(ctx, el.CurrencyID)
		if err != nil {
			return err
		}
		d = &model.Debt{ID: s.repo.NextIdentity(), BudgetID: b.ID, CurrencyID: el.CurrencyID, ElementID: el.ID, ElementType: el.Type,
			Status: model.DebtActive, StartDate: in.start, Payment: in.payment.Round(digits), Schedule: in.schedule,
			CreatedDay: debtDate(now.In(loc)), CreatedAt: now, UpdatedAt: now}
		if err := validatePayment(d.Payment, d.Schedule); err != nil {
			return err
		}
		var origin []model.DebtTerms
		if in.hasOrigin {
			if origin, err = in.setupTerms(model.DebtTerms{Payment: d.Payment, Schedule: d.Schedule}, today, digits); err != nil {
				return err
			}
			d.AmountBorrowed = origin[0].Balance
			d.SetTerms(origin[len(origin)-1])
		}
		if err := s.repo.Save(ctx, d); err != nil {
			return err
		}
		for _, t := range origin {
			if err := s.repo.AddSnapshot(ctx, s.snapshotOf(d.ID, t, now)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.resultOf(ctx, budget, d)
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
		if err := validateStatement(owed, origin, today, digits, nil); err != nil {
			return nil, err
		}
		out = append(out, owed)
	}
	return out, nil
}
