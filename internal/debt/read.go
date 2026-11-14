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

// presenter names a debt's rules for its readers, reading each
// classification and owner once.
type presenter struct {
	s       *Service
	classes map[string]model.DebtClassification
	names   map[vo.Id]string
}

func (s *Service) presenter() *presenter {
	return &presenter{s: s, classes: map[string]model.DebtClassification{}, names: map[vo.Id]string{}}
}

// classification is the zero value for one that is gone: the rules are
// released before a classification is deleted, so this is only ever a race.
func (p *presenter) classification(ctx context.Context, kind string, id vo.Id) (model.DebtClassification, error) {
	key := kind + ":" + id.String()
	if c, ok := p.classes[key]; ok {
		return c, nil
	}
	c, err := p.s.class.Classification(ctx, kind, id)
	if err != nil && !isNotFound(err) {
		return c, err
	}
	p.classes[key] = c
	return c, nil
}

func (p *presenter) userName(ctx context.Context, id vo.Id) (string, error) {
	if n, ok := p.names[id]; ok {
		return n, nil
	}
	n, err := p.s.users.GetName(ctx, id)
	if err != nil && !isNotFound(err) {
		return "", err
	}
	p.names[id] = n
	return n, nil
}

// icon is every reader's icon of the debt: its own when set, else the first
// rule's category icon, else the default one.
func (p *presenter) icon(ctx context.Context, d *model.Debt) (string, error) {
	if d.Icon != "" {
		return d.Icon, nil
	}
	if len(d.Rules) == 0 {
		return model.DefaultIcon, nil
	}
	c, err := p.classification(ctx, KindCategory, d.Rules[0].CategoryID)
	if err != nil || c.Icon == "" {
		return model.DefaultIcon, err
	}
	return c.Icon, nil
}

func (p *presenter) rules(ctx context.Context, d *model.Debt) ([]model.DebtRuleResult, error) {
	out := make([]model.DebtRuleResult, 0, len(d.Rules))
	for _, r := range d.Rules {
		cat, err := p.classification(ctx, KindCategory, r.CategoryID)
		if err != nil {
			return nil, err
		}
		item := model.DebtRuleResult{Id: r.ID.String(), CategoryId: r.CategoryID.String(), CategoryName: cat.Name, CategoryIcon: cat.Icon,
			PayeeId: idString(r.PayeeID), TagId: idString(r.TagID), LabelId: idString(r.LabelID)}
		if !cat.OwnerID.IsZero() {
			item.OwnerUserId = cat.OwnerID.String()
			if item.OwnerName, err = p.userName(ctx, cat.OwnerID); err != nil {
				return nil, err
			}
		}
		for _, part := range []struct {
			kind string
			id   *vo.Id
			name *string
		}{{KindPayee, r.PayeeID, &item.PayeeName}, {KindTag, r.TagID, &item.TagName}, {KindLabel, r.LabelID, &item.LabelName}} {
			if part.id == nil {
				continue
			}
			c, err := p.classification(ctx, part.kind, *part.id)
			if err != nil {
				return nil, err
			}
			*part.name = c.Name
		}
		out = append(out, item)
	}
	return out, nil
}

func idString(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	v := id.String()
	return &v
}

// result presents a debt; one not set up has only its identity, rules,
// payment and schedule, with every term and figure empty.
func (s *Service) result(st *debtState, proj model.Projection, today time.Time, b model.DebtBudget, icon string, rules []model.DebtRuleResult) model.DebtResult {
	d, digits := st.debt, st.digits
	startDate := ""
	if d.StartDate != nil {
		startDate = dateString(*d.StartDate)
	}
	res := model.DebtResult{
		Id: d.ID.String(), BudgetId: d.BudgetID.String(), BudgetName: b.Name, Name: d.Name, Icon: icon,
		CurrencyId: d.CurrencyID.String(), Rules: rules, StartDate: startDate, Payment: d.Payment.String(), Schedule: string(d.Schedule),
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
	p := s.presenter()
	items := make([]model.DebtResult, 0, len(states))
	for _, st := range states {
		icon, err := p.icon(ctx, st.debt)
		if err != nil {
			return nil, err
		}
		rules, err := p.rules(ctx, st.debt)
		if err != nil {
			return nil, err
		}
		items = append(items, s.result(st, st.project(today, nil, nil), today, b, icon, rules))
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

// Views returns the rules, range and planning bounds of the budget's debts,
// in creation order, matching the payments on the scope's accounts. It writes
// nothing and takes no lock, so the budget can call it inside its own
// transactions.
func (s *Service) Views(ctx context.Context, scope model.DebtScope) ([]model.DebtView, error) {
	states, err := s.readStates(ctx, scope)
	if err != nil {
		return nil, err
	}
	today := s.today(ctx)
	p := s.presenter()
	views := make([]model.DebtView, 0, len(states))
	for _, st := range states {
		icon, err := p.icon(ctx, st.debt)
		if err != nil {
			return nil, err
		}
		proj := st.project(today, nil, nil)
		v := st.view
		v.Icon = icon
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
// later timezone change does not move it. A start that moves (finishing
// setup, a correction) moves the plans with it (moveHead). A debt not set up
// has no end: LastDay is zero.
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
