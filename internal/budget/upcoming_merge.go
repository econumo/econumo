package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// addUpcoming fills result's upcoming fields: the same builder run a second
// time over the projected recurring occurrences instead of the stored
// transactions. A month that ended before the caller's current month has no
// upcoming money, so it skips the projection and keeps the zeros BuildBudget
// set.
func (s *Service) addUpcoming(ctx context.Context, userID vo.Id, b *budgetAggregate, periodStart, now time.Time, result *model.BudgetResult) error {
	periodEnd := periodStart.AddDate(0, 1, 0)
	if !periodEnd.After(localMonth(now, reqctx.Location(ctx))) {
		return nil
	}
	f, err := s.buildFilters(ctx, userID, b, periodStart, periodEnd)
	if err != nil {
		return err
	}
	pr, err := s.newProjectedRead(ctx, f, periodEnd)
	if err != nil {
		return err
	}
	sp := *s
	sp.read = pr
	up, err := sp.BuildBudget(ctx, userID, b, periodStart, now)
	if err != nil {
		return err
	}
	mergeBudgetUpcoming(result, up)
	return nil
}

// mergeBudgetUpcoming copies the projected build's spend into real's upcoming
// fields; an element the projected build lacks gets "0". The projected build
// sums no limits, so its Available is -(spentBefore + spent) and
// upcomingBefore = -Available - Spent.
func mergeBudgetUpcoming(real *model.BudgetResult, up model.BudgetResult) {
	type key struct {
		id  string
		typ int
	}
	elements := make(map[key]model.ParentElementResult, len(up.Structure.Elements))
	for _, e := range up.Structure.Elements {
		elements[key{e.Id, e.Type}] = e
	}
	for i := range real.Structure.Elements {
		el := &real.Structure.Elements[i]
		u, ok := elements[key{el.Id, el.Type}]
		if !ok {
			el.Upcoming, el.BudgetUpcoming, el.UpcomingBefore = "0", "0", "0"
			mergeChildrenUpcoming(el.Children, nil)
			continue
		}
		el.Upcoming, el.BudgetUpcoming = u.Spent, u.BudgetSpent
		el.UpcomingBefore = vo.NewDecimal("0").Sub(vo.NewDecimal(u.Available)).Sub(vo.NewDecimal(u.Spent)).String()
		mergeChildrenUpcoming(el.Children, u.Children)
	}

	labels := make(map[string]model.LabelSpendResult, len(up.Structure.Labels))
	for _, l := range up.Structure.Labels {
		labels[l.Id] = l
	}
	for i := range real.Structure.Labels {
		l := &real.Structure.Labels[i]
		u, ok := labels[l.Id]
		l.Upcoming = "0"
		if ok {
			l.Upcoming = u.Spent
		}
		mergeChildrenUpcoming(l.Children, u.Children)
	}

	savings := make(map[string]model.SavingsElementResult, len(up.Structure.Savings))
	for _, sv := range up.Structure.Savings {
		savings[sv.Id] = sv
	}
	for i := range real.Structure.Savings {
		sv := &real.Structure.Savings[i]
		sv.Upcoming = "0"
		if u, ok := savings[sv.Id]; ok {
			sv.Upcoming = u.Spent
		}
	}
}

func mergeChildrenUpcoming(real, up []model.ChildElementResult) {
	byID := make(map[string]model.ChildElementResult, len(up))
	for _, c := range up {
		byID[c.Id] = c
	}
	for i := range real {
		c := &real[i]
		c.Upcoming, c.BudgetUpcoming = "0", "0"
		if u, ok := byID[c.Id]; ok {
			c.Upcoming, c.BudgetUpcoming = u.Spent, u.BudgetSpent
		}
	}
}
