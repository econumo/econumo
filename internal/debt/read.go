package debt

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// label is every reader's name and icon of the debt: its element's. A
// category no participant of the budget shows any more (its owner left) is
// read from the category itself.
func (s *Service) label(ctx context.Context, d *model.Debt, el model.DebtElementInfo) (name, icon string, err error) {
	if el.Type != "" {
		return el.Name, el.Icon, nil
	}
	if d.ElementType == model.DebtElementCategory && s.categories != nil {
		c, err := s.categories.Category(ctx, d.ElementID)
		if err != nil && !isNotFound(err) {
			return "", "", err
		}
		if c.Name != "" {
			return c.Name, c.Icon, nil
		}
	}
	return "", model.DefaultIcon, nil
}

// result presents a debt; one not set up has only its identity, element,
// payment and schedule, with every term and figure empty.
func (s *Service) result(st *debtState, proj model.Projection, today time.Time, b model.DebtBudget, name, icon string) model.DebtResult {
	d, digits := st.debt, st.digits
	startDate := ""
	if d.StartDate != nil {
		startDate = dateString(*d.StartDate)
	}
	endedAt := ""
	if d.EndedAt != nil {
		endedAt = dateString(*d.EndedAt)
	}
	status := d.Status
	if status == "" {
		status = model.DebtActive
	}
	res := model.DebtResult{
		Id: d.ID.String(), BudgetId: d.BudgetID.String(), BudgetName: b.Name, ElementId: d.ElementID.String(),
		ElementType: string(d.ElementType), Status: string(status), EndedAt: endedAt, Name: name, Icon: icon,
		CurrencyId: d.CurrencyID.String(), StartDate: startDate, Payment: d.Payment.String(), Schedule: string(d.Schedule),
	}
	if canEdit(b) {
		res.CanEdit = 1
	}
	if !d.IsSetUp() {
		return res
	}
	payoff := proj.PayoffDate
	if proj.PaidOff {
		payoff = time.Time{}
		res.IsPaidOff = 1
	}
	res.IsSetUp = 1
	res.AmountBorrowed = d.AmountBorrowed.String()
	res.Balance, res.LastPaymentDate = d.Current.Balance.String(), dateString(d.Current.LastPaymentDate)
	res.CurrentBalance, res.PaymentsLeft, res.PeriodsPaid = amountString(proj.CurrentBalance, digits), proj.PaymentsLeft, proj.PeriodsPaid
	res.MoneyLeft, res.OverpaymentLeft = amountString(proj.MoneyLeft, digits), amountString(proj.OverpaymentLeft, digits)
	res.OverpaidSoFar, res.PayoffDate = amountString(proj.OverpaidSoFar, digits), dateString(payoff)
	res.NextDueDate, res.ExtraPaid = dateString(proj.NextDueDate), amountString(proj.ExtraPaid, digits)
	snaps := st.mathSnaps()
	rate, schedule := model.PeriodRateOn(snaps, today), d.Current.Schedule
	res.PeriodRate = strconv.FormatFloat(rate, 'f', 10, 64)
	res.AnnualRate = rateString(model.PeriodToAnnualRate(rate, schedule))
	if len(snaps) > 0 {
		implied, _ := model.SnapshotRate(snaps[0])
		res.ImpliedAnnualRate = rateString(model.PeriodToAnnualRate(implied, schedule))
	}
	return res
}

func (s *Service) presentAll(ctx context.Context, b model.DebtBudget, states []*debtState) ([]model.DebtResult, error) {
	today := s.today(ctx)
	items := make([]model.DebtResult, 0, len(states))
	for _, st := range states {
		name, icon, err := s.label(ctx, st.debt, st.element)
		if err != nil {
			return nil, err
		}
		items = append(items, s.result(st, st.project(today, nil, nil), today, b, name, icon))
	}
	return items, nil
}

// stateOf syncs the debt's budget and returns the debt's state.
func (s *Service) stateOf(ctx context.Context, b model.DebtBudget, d *model.Debt) (*debtState, error) {
	states, err := s.syncBudget(ctx, scopeOf(b))
	if err != nil {
		return nil, err
	}
	for _, st := range states {
		if st.debt.ID.Equal(d.ID) {
			return st, nil
		}
	}
	return nil, errs.NewNotFound("Debt not found")
}

func (s *Service) resultOf(ctx context.Context, b model.DebtBudget, d *model.Debt) (*model.DebtResult, error) {
	st, err := s.stateOf(ctx, b, d)
	if err != nil {
		return nil, err
	}
	items, err := s.presentAll(ctx, b, []*debtState{st})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

// GetDebtList lists the debts of every budget the user can read, archived
// ones included, budget by budget in the budget list's order.
func (s *Service) GetDebtList(ctx context.Context, userID vo.Id) (*model.DebtListResult, error) {
	if s.budgets == nil {
		return nil, errNoBudgets
	}
	budgets, err := s.budgets.Readable(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]vo.Id, 0, len(budgets))
	for _, b := range budgets {
		ids = append(ids, b.ID)
	}
	debts, err := s.repo.ListByBudgets(ctx, ids)
	if err != nil {
		return nil, err
	}
	withDebts := map[vo.Id]bool{}
	for _, d := range debts {
		withDebts[d.BudgetID] = true
	}
	items := []model.DebtResult{}
	for _, b := range budgets {
		if !withDebts[b.ID] {
			continue
		}
		states, err := s.syncBudget(ctx, scopeOf(b))
		if err != nil {
			return nil, err
		}
		presented, err := s.presentAll(ctx, b, states)
		if err != nil {
			return nil, err
		}
		items = append(items, presented...)
	}
	return &model.DebtListResult{Items: items}, nil
}

func (s *Service) GetDebt(ctx context.Context, userID vo.Id, req model.GetDebtRequest) (*model.DebtDetailsResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	d, b, err := s.readableDebt(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	st, err := s.stateOf(ctx, b, d)
	if err != nil {
		return nil, err
	}
	items, err := s.presentAll(ctx, b, []*debtState{st})
	if err != nil {
		return nil, err
	}
	res := &model.DebtDetailsResult{Item: items[0], Snapshots: []model.DebtSnapshotResult{}, Payments: []model.DebtPaymentResult{},
		RateChanges: []model.DebtRateChangeResult{}}
	for _, c := range st.rates {
		res.RateChanges = append(res.RateChanges, model.DebtRateChangeResult{Id: c.ID.String(), Date: dateString(c.EffectiveDate),
			AnnualRate: c.AnnualRate.String(), Payment: c.Payment.String()})
	}
	computed := st.computedBalances()
	for i, sn := range st.snaps {
		item := model.DebtSnapshotResult{Id: sn.ID.String(), AsOf: dateString(sn.AsOf), Balance: sn.Balance.String(),
			Payment: sn.Payment.String(), Schedule: string(sn.Schedule), LastPaymentDate: dateString(sn.LastPaymentDate)}
		if i == 0 {
			item.IsOrigin = 1
		} else {
			item.ComputedBalance = amountString(computed[i], st.digits)
		}
		res.Snapshots = append(res.Snapshots, item)
	}
	balances := st.balancesAfter()
	for i := len(st.pays) - 1; i >= 0; i-- {
		p := st.pays[i]
		pay := model.DebtPaymentResult{TransactionId: p.txID.String(), Date: dateString(p.date),
			Amount: amountString(p.amount, st.digits), Interest: amountString(p.interest, st.digits),
			Principal: amountString(p.amount-p.interest, st.digits)}
		if balances != nil {
			pay.Balance = amountString(balances[i], st.digits)
		}
		res.Payments = append(res.Payments, pay)
	}
	return res, nil
}

// computedBalances is, for each snapshot after the origin, what the snapshots
// before it and the recorded payments gave for its date: what Econumo had
// worked out before the statement.
func (st *debtState) computedBalances() []float64 {
	snaps := st.mathSnaps()
	pays, interest := st.mathPays()
	out := make([]float64, len(snaps))
	for i := 1; i < len(snaps); i++ {
		out[i] = math.Max(model.BalanceAt(snaps[:i], pays, interest, snaps[i].AsOf), 0)
	}
	return out
}

// balancesAfter is what was owed after each payment (st.pays order), nil for
// a debt not set up. BalanceAt closes a whole day, so the payments of one day
// after this one are added back: they had not been made yet.
func (st *debtState) balancesAfter() []float64 {
	if !st.debt.IsSetUp() {
		return nil
	}
	snaps := st.mathSnaps()
	pays, interest := st.mathPays()
	out := make([]float64, len(st.pays))
	for i := len(st.pays) - 1; i >= 0; i-- {
		day := debtDate(st.pays[i].date)
		if i+1 < len(st.pays) && debtDate(st.pays[i+1].date).Equal(day) {
			next := st.pays[i+1]
			out[i] = out[i+1] + next.amount - next.interest
			continue
		}
		out[i] = model.BalanceAt(snaps, pays, interest, day)
	}
	for i := range out {
		out[i] = math.Max(out[i], 0)
	}
	return out
}

// Views returns the element, range and planning bounds of the budget's debts,
// in creation order, matching the payments on the scope's accounts. It writes
// nothing and takes no lock, so the budget can call it inside its own
// transactions.
func (s *Service) Views(ctx context.Context, scope model.DebtScope) ([]model.DebtView, error) {
	states, err := s.readStates(ctx, scope)
	if err != nil {
		return nil, err
	}
	today := s.today(ctx)
	views := make([]model.DebtView, 0, len(states))
	for _, st := range states {
		name, icon, err := s.label(ctx, st.debt, st.element)
		if err != nil {
			return nil, err
		}
		proj := st.project(today, nil, nil)
		v := st.view
		v.Name, v.Icon = name, icon
		v.PaidOff = proj.PaidOff
		v.FirstDay, v.LastDay = planningBounds(st, proj)
		v.StartMonth = monthOf(v.FirstDay)
		if !v.LastDay.IsZero() {
			v.EndMonth = monthOf(v.LastDay)
		}
		views = append(views, v)
	}
	return views, nil
}

// planningBounds: a debt can be planned from its start date (always there
// once set up: the day the loan began), else from the earliest of its first
// matched payment, its creation day and its first snapshot, to its last
// payment date, or its payoff once paid off; a budget plans it in the periods
// holding those days. The creation day is the stored one, taken in the
// creator's timezone at creation, so every viewer sees the same start and a
// later timezone change does not move it. A debt not set up has no end:
// LastDay is zero.
func planningBounds(st *debtState, proj model.Projection) (first, last time.Time) {
	d := st.debt
	if d.StartDate != nil {
		first = debtDate(*d.StartDate)
	} else {
		first = debtDate(d.CreatedDay)
		for _, sn := range st.snaps {
			if sn.AsOf.Before(first) {
				first = debtDate(sn.AsOf)
			}
		}
		if len(st.pays) > 0 && st.pays[0].date.Before(first) {
			first = debtDate(st.pays[0].date)
		}
	}
	switch {
	case !d.IsSetUp():
		return first, time.Time{}
	case proj.PaidOff:
		return first, proj.PaidOffAt
	}
	return first, debtDate(d.Current.LastPaymentDate)
}

// ElementIDs are the elements of the budget that carry a debt, active or
// paid off, each once.
func (s *Service) ElementIDs(ctx context.Context, budgetID vo.Id) ([]vo.Id, error) {
	debts, err := s.repo.ListByBudget(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	seen := map[vo.Id]bool{}
	var out []vo.Id
	for _, d := range debts {
		if !seen[d.ElementID] {
			seen[d.ElementID] = true
			out = append(out, d.ElementID)
		}
	}
	return out, nil
}
