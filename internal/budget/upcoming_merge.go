package budget

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// projectUpcoming projects the recurring occurrences for the period. A month
// that ended before the caller's current month has no upcoming money, so it
// returns a nil read and the zeros BuildBudget set stay.
func (s *Service) projectUpcoming(ctx context.Context, userID vo.Id, b *budgetAggregate, periodStart, now time.Time) (*projectedRead, filters, error) {
	periodEnd := periodStart.AddDate(0, 1, 0)
	if !periodEnd.After(localMonth(now, reqctx.Location(ctx))) {
		return nil, filters{}, nil
	}
	f, err := s.buildFilters(ctx, userID, b, periodStart, periodEnd)
	if err != nil {
		return nil, filters{}, err
	}
	pr, err := s.newProjectedRead(ctx, f, periodEnd)
	if err != nil {
		return nil, filters{}, err
	}
	return pr, f, nil
}

// addUpcoming fills result's upcoming fields: the same builder run a second
// time over the projected recurring occurrences instead of the stored
// transactions.
func (s *Service) addUpcoming(ctx context.Context, userID vo.Id, b *budgetAggregate, periodStart, now time.Time, pr *projectedRead, f filters, result *model.BudgetResult) error {
	sp := *s
	sp.read = pr
	sp.upcoming = nil
	up, err := sp.BuildBudget(ctx, userID, b, periodStart, now)
	if err != nil {
		return err
	}
	if mergeBudgetUpcoming(result, up) {
		reorderElements(result.Structure.Elements, s.elementOptions(b))
		sortBySortKeyThenID(result.Structure.Labels,
			func(l model.LabelSpendResult) string { return f.labels[l.Id].SortKey },
			func(l model.LabelSpendResult) string { return l.Id })
	}
	return nil
}

// mergeBudgetUpcoming copies the projected build's spend into real's upcoming
// fields; a row the projected build lacks gets "0". The projected build sums no
// limits, so its Available is -(spentBefore + spent) and
// upcomingBefore = -Available - Spent.
//
// A row only the projected build has (a tag, tag child or label carrying
// upcoming money but no real spend or limit) is added with zero real amounts:
// the builders show those rows on spend, so projected spend shows them the
// way real spend would. It reports whether anything was added, as positions
// and label order then need recomputing.
func mergeBudgetUpcoming(real *model.BudgetResult, up model.BudgetResult) bool {
	added := false
	type key struct {
		id  string
		typ int
	}
	elements := make(map[key]model.ParentElementResult, len(up.Structure.Elements))
	for _, e := range up.Structure.Elements {
		elements[key{e.Id, e.Type}] = e
	}
	seen := map[key]bool{}
	for i := range real.Structure.Elements {
		el := &real.Structure.Elements[i]
		k := key{el.Id, el.Type}
		seen[k] = true
		u, ok := elements[k]
		if !ok {
			el.Upcoming, el.BudgetUpcoming, el.UpcomingBefore = "0", "0", "0"
			el.Children, _ = mergeChildrenUpcoming(el.Children, nil)
			continue
		}
		el.Upcoming, el.BudgetUpcoming, el.UpcomingBefore = u.Spent, u.BudgetSpent, upcomingBefore(u)
		var childAdded bool
		el.Children, childAdded = mergeChildrenUpcoming(el.Children, u.Children)
		added = added || childAdded
	}
	for _, u := range up.Structure.Elements {
		if seen[key{u.Id, u.Type}] {
			continue
		}
		children, _ := mergeChildrenUpcoming([]model.ChildElementResult{}, u.Children)
		real.Structure.Elements = append(real.Structure.Elements, model.ParentElementResult{
			Id: u.Id, Type: u.Type, Name: u.Name, Icon: u.Icon, CurrencyId: u.CurrencyId,
			IsArchived: u.IsArchived, FolderId: u.FolderId, Position: u.Position, OwnerUserId: u.OwnerUserId,
			Budgeted: "0", Available: "0", Spent: "0", BudgetSpent: "0",
			Upcoming: u.Spent, BudgetUpcoming: u.BudgetSpent, UpcomingBefore: upcomingBefore(u),
			Children: children,
		})
		added = true
	}

	labels := make(map[string]model.LabelSpendResult, len(up.Structure.Labels))
	for _, l := range up.Structure.Labels {
		labels[l.Id] = l
	}
	seenLabel := map[string]bool{}
	for i := range real.Structure.Labels {
		l := &real.Structure.Labels[i]
		seenLabel[l.Id] = true
		u, ok := labels[l.Id]
		l.Upcoming = "0"
		if ok {
			l.Upcoming = u.Spent
		}
		var childAdded bool
		l.Children, childAdded = mergeChildrenUpcoming(l.Children, u.Children)
		added = added || childAdded
	}
	for _, u := range up.Structure.Labels {
		if seenLabel[u.Id] {
			continue
		}
		children, _ := mergeChildrenUpcoming([]model.ChildElementResult{}, u.Children)
		real.Structure.Labels = append(real.Structure.Labels, model.LabelSpendResult{
			Id: u.Id, Name: u.Name, Icon: u.Icon, IsArchived: u.IsArchived, OwnerUserId: u.OwnerUserId,
			Spent: "0", Upcoming: u.Spent, Children: children,
		})
		added = true
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
	return added
}

func upcomingBefore(u model.ParentElementResult) string {
	return vo.NewDecimal("0").Sub(vo.NewDecimal(u.Available)).Sub(vo.NewDecimal(u.Spent)).String()
}

// mergeChildrenUpcoming fills real's children from up's by id and appends the
// children only up has, with zero real amounts, keeping the builders' id order.
func mergeChildrenUpcoming(real, up []model.ChildElementResult) ([]model.ChildElementResult, bool) {
	byID := make(map[string]model.ChildElementResult, len(up))
	for _, c := range up {
		byID[c.Id] = c
	}
	seen := map[string]bool{}
	for i := range real {
		c := &real[i]
		seen[c.Id] = true
		c.Upcoming, c.BudgetUpcoming = "0", "0"
		if u, ok := byID[c.Id]; ok {
			c.Upcoming, c.BudgetUpcoming = u.Spent, u.BudgetSpent
		}
	}
	added := false
	for _, u := range up {
		if seen[u.Id] {
			continue
		}
		real = append(real, model.ChildElementResult{
			Id: u.Id, Type: u.Type, Name: u.Name, Icon: u.Icon, IsArchived: u.IsArchived, OwnerUserId: u.OwnerUserId,
			Spent: "0", BudgetSpent: "0", Upcoming: u.Spent, BudgetUpcoming: u.BudgetSpent,
		})
		added = true
	}
	if added {
		sort.Slice(real, func(i, j int) bool { return real[i].Id < real[j].Id })
	}
	return real, added
}

// reorderElements reassigns the dense per-folder positions and the wire order
// over the merged rows, with the same keys assignElementPositions uses.
func reorderElements(elements []model.ParentElementResult, options map[string]elementOption) {
	sortKey := func(e model.ParentElementResult) string {
		return string(options[elementKey(e.Id, model.ElementType(e.Type))].sortKey)
	}
	sort.SliceStable(elements, func(i, j int) bool {
		x, y := elements[i], elements[j]
		if xl, yl := x.Id == model.UncategorizedID, y.Id == model.UncategorizedID; xl != yl {
			return !xl
		}
		if kx, ky := sortKey(x), sortKey(y); kx != ky {
			return kx < ky
		}
		return x.Id < y.Id
	})
	perFolder := map[string]int{}
	for i := range elements {
		folder := ""
		if elements[i].FolderId != nil {
			folder = *elements[i].FolderId
		}
		elements[i].Position = perFolder[folder]
		perFolder[folder]++
	}
	sortByPositionThenID(elements, func(p model.ParentElementResult) int { return p.Position }, func(p model.ParentElementResult) string { return p.Id })
}
