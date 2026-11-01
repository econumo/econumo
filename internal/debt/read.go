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

func (s *Service) result(st *debtState, proj model.Projection, icon string) model.DebtResult {
	d, digits := st.debt, st.digits
	paidOff := 0
	if proj.PaidOff {
		paidOff = 1
	}
	startDate := ""
	if d.StartDate != nil {
		startDate = dateString(*d.StartDate)
	}
	rate := model.PeriodRate(termsFloats(d.Current))
	payoff := proj.PayoffDate
	if proj.PaidOff {
		payoff = time.Time{}
	}
	return model.DebtResult{
		Id: d.ID.String(), Name: d.Name, Icon: icon, CurrencyId: d.CurrencyID.String(), CategoryId: d.CategoryID.String(),
		PayeeId: idString(d.PayeeID), TagId: idString(d.TagID), StartDate: startDate,
		Balance: d.Current.Balance.String(), Payment: d.Current.Payment.String(),
		Schedule: string(d.Current.Schedule), LastPaymentDate: dateString(d.Current.LastPaymentDate),
		CurrentBalance: amountString(proj.CurrentBalance, digits), PaymentsLeft: proj.PaymentsLeft,
		MoneyLeft: amountString(proj.MoneyLeft, digits), OverpaymentLeft: amountString(proj.OverpaymentLeft, digits),
		OverpaidSoFar: amountString(proj.OverpaidSoFar, digits), PayoffDate: dateString(payoff),
		NextDueDate: dateString(proj.NextDueDate), PeriodRate: strconv.FormatFloat(rate, 'f', 10, 64), IsPaidOff: paidOff,
	}
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
// debts, in creation order.
func (s *Service) Views(ctx context.Context, ownerIDs []vo.Id) ([]model.DebtView, error) {
	states, err := s.loadStates(ctx, ownerIDs)
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
		v.StartMonth, v.EndMonth = planningBounds(st, proj)
		views = append(views, v)
	}
	return views, nil
}

// planningBounds: a debt can be planned from the month of its start date (else
// of its first matched payment, else of its first snapshot) to the month of
// its last payment date, or of its payoff once paid off.
func planningBounds(st *debtState, proj model.Projection) (start, end time.Time) {
	d := st.debt
	switch {
	case d.StartDate != nil:
		start = monthOf(*d.StartDate)
	case len(st.pays) > 0:
		start = monthOf(st.pays[0].date)
	case len(st.snaps) > 0:
		first := st.snaps[0].AsOf
		for _, sn := range st.snaps {
			if sn.AsOf.Before(first) {
				first = sn.AsOf
			}
		}
		start = monthOf(first)
	default:
		start = monthOf(d.Current.AsOf)
	}
	if proj.PaidOff {
		return start, proj.PayoffMonth
	}
	return start, monthOf(d.Current.LastPaymentDate)
}
