package debt

import (
	"context"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type iconCache map[vo.Id]string

func (s *Service) iconOf(ctx context.Context, cache iconCache, d *model.Debt) (string, error) {
	if icon, ok := cache[d.CategoryID]; ok {
		return icon, nil
	}
	_, icon, _, err := s.class.CategoryOf(ctx, d.OwnerID, d.CategoryID)
	if err != nil {
		return "", err
	}
	cache[d.CategoryID] = icon
	return icon, nil
}

func idString(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	v := id.String()
	return &v
}

// result presents a debt; one not set up has only its identity, payment and
// schedule, with every term and figure empty.
func (s *Service) result(st *debtState, proj model.Projection, icon string) model.DebtResult {
	d, digits := st.debt, st.digits
	startDate := ""
	if d.StartDate != nil {
		startDate = dateString(*d.StartDate)
	}
	res := model.DebtResult{
		Id: d.ID.String(), Name: d.Name, Icon: icon, CurrencyId: d.CurrencyID.String(), CategoryId: d.CategoryID.String(),
		PayeeId: idString(d.PayeeID), TagId: idString(d.TagID), LabelId: idString(d.LabelID), StartDate: startDate,
		Payment: d.Payment.String(), Schedule: string(d.Schedule),
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
	res.Balance, res.LastPaymentDate = d.Current.Balance.String(), dateString(d.Current.LastPaymentDate)
	res.CurrentBalance, res.PaymentsLeft, res.PeriodsPaid = amountString(proj.CurrentBalance, digits), proj.PaymentsLeft, proj.PeriodsPaid
	res.MoneyLeft, res.OverpaymentLeft = amountString(proj.MoneyLeft, digits), amountString(proj.OverpaymentLeft, digits)
	res.OverpaidSoFar, res.PayoffDate = amountString(proj.OverpaidSoFar, digits), dateString(payoff)
	res.NextDueDate = dateString(proj.NextDueDate)
	res.PeriodRate = strconv.FormatFloat(model.PeriodRate(termsFloats(*d.Current, digits)), 'f', 10, 64)
	return res
}

func (s *Service) presentAll(ctx context.Context, states []*debtState) ([]model.DebtResult, error) {
	today := s.today(ctx)
	icons := iconCache{}
	items := make([]model.DebtResult, 0, len(states))
	for _, st := range states {
		icon, err := s.iconOf(ctx, icons, st.debt)
		if err != nil {
			return nil, err
		}
		items = append(items, s.result(st, st.project(today, nil, nil), icon))
	}
	return items, nil
}

// stateOf syncs the debt's category and returns the debt's state.
func (s *Service) stateOf(ctx context.Context, d *model.Debt) (*debtState, error) {
	states, err := s.syncCategory(ctx, d.CategoryID)
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

func (s *Service) resultOf(ctx context.Context, d *model.Debt) (*model.DebtResult, error) {
	st, err := s.stateOf(ctx, d)
	if err != nil {
		return nil, err
	}
	items, err := s.presentAll(ctx, []*debtState{st})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (s *Service) GetDebtList(ctx context.Context, userID vo.Id) (*model.DebtListResult, error) {
	states, err := s.loadStates(ctx, []vo.Id{userID})
	if err != nil {
		return nil, err
	}
	items, err := s.presentAll(ctx, states)
	if err != nil {
		return nil, err
	}
	return &model.DebtListResult{Items: items}, nil
}

func (s *Service) GetDebt(ctx context.Context, userID vo.Id, req model.GetDebtRequest) (*model.DebtDetailsResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	d, err := s.ownedDebt(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	st, err := s.stateOf(ctx, d)
	if err != nil {
		return nil, err
	}
	items, err := s.presentAll(ctx, []*debtState{st})
	if err != nil {
		return nil, err
	}
	res := &model.DebtDetailsResult{Item: items[0], Snapshots: []model.DebtSnapshotResult{}, Payments: []model.DebtPaymentResult{}}
	for _, sn := range st.snaps {
		res.Snapshots = append(res.Snapshots, model.DebtSnapshotResult{AsOf: dateString(sn.AsOf), Balance: sn.Balance.String(),
			Payment: sn.Payment.String(), Schedule: string(sn.Schedule), LastPaymentDate: dateString(sn.LastPaymentDate)})
	}
	for i := len(st.pays) - 1; i >= 0; i-- {
		p := st.pays[i]
		res.Payments = append(res.Payments, model.DebtPaymentResult{TransactionId: p.txID.String(), Date: dateString(p.date),
			Amount: amountString(p.amount, st.digits), Interest: amountString(p.interest, st.digits),
			Principal: amountString(p.amount-p.interest, st.digits)})
	}
	return res, nil
}

// Views returns the matching key, range and planning bounds of the owners'
// debts, in creation order. It writes nothing and takes no lock, so the budget
// can call it inside its own transactions.
func (s *Service) Views(ctx context.Context, ownerIDs []vo.Id) ([]model.DebtView, error) {
	states, err := s.readStates(ctx, ownerIDs)
	if err != nil {
		return nil, err
	}
	today := s.today(ctx)
	icons := iconCache{}
	views := make([]model.DebtView, 0, len(states))
	for _, st := range states {
		icon, err := s.iconOf(ctx, icons, st.debt)
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

// planningBounds: a debt can be planned from its start date, else from the
// earliest of its first matched payment, its creation day and its first
// snapshot, to its last payment date, or its payoff once paid off; a budget
// plans it in the periods holding those days. The creation day is the stored
// one, taken in the owner's timezone at creation, so every viewer sees the
// same start and a later timezone change does not move it. Finishing setup
// never moves the start (its snapshot is dated today), so plans moved at
// conversion stay in the life; a later start date does, and the update hands
// the months before it back to the category (moveHeadToCategory). A debt not
// set up has no end: LastDay is zero.
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
