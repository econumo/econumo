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
	return s.projectWindow(ctx, userID, b, periodStart, periodStart.AddDate(0, 1, 0), now)
}

// projectWindow projects the recurring occurrences filed in [periodStart,
// periodEnd), or returns a nil read when the window ended before the caller's
// current month.
func (s *Service) projectWindow(ctx context.Context, userID vo.Id, b *budgetAggregate, periodStart, periodEnd, now time.Time) (*projectedRead, filters, error) {
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

// addPlanUpcoming fills the plan's upcoming fields the way addUpcoming does
// for get-budget: BuildBudgetPlan run again over the projected occurrences of
// the window. A window that ended before the current month keeps the zeros
// BuildBudgetPlan set.
func (s *Service) addPlanUpcoming(ctx context.Context, userID vo.Id, b *budgetAggregate, from time.Time, months int, result *model.BudgetPlanResult) error {
	pr, _, err := s.projectWindow(ctx, userID, b, from, from.AddDate(0, months, 0), s.clock.Now())
	if err != nil || pr == nil {
		return err
	}
	sp := *s
	sp.read = pr
	sp.upcoming = nil
	up, err := sp.BuildBudgetPlan(ctx, userID, b, from, months)
	if err != nil {
		return err
	}
	if mergePlanUpcoming(result, up) {
		reorderPlanElements(result.Structure.Elements, s.elementOptions(b))
	}
	return nil
}

// mergePlanUpcoming copies the projected plan's actuals into real's upcoming
// cells (rows by (id, type), cells by month index, children and savings rows
// by id) and takes its top-level arrays as the upcoming ones.
//
// A row or child only the projection has (a tag, an Uncategorized row, an
// archived row or child the builder shows on activity) is added with zero
// actuals, as projected money shows it the way real money would. Savings rows
// are never added: the only ones the builder hides are deleted accounts, whose
// booked closing balance the projection cannot supply. It reports whether
// anything was added, as positions then need recomputing.
func mergePlanUpcoming(real *model.BudgetPlanResult, up model.BudgetPlanResult) bool {
	real.UpcomingOpeningBalances = up.OpeningBalances
	real.UpcomingSavingsOpeningBalances = up.SavingsOpeningBalances
	real.UpcomingTransfers = up.Transfers
	real.UpcomingSavingsFlows = up.SavingsFlows
	real.UpcomingSavingsIncomeExpense = up.SavingsIncomeExpense

	type key struct {
		id  string
		typ int
	}
	elements := make(map[key]model.PlanElementResult, len(up.Structure.Elements))
	for _, e := range up.Structure.Elements {
		elements[key{e.Id, e.Type}] = e
	}
	added := false
	seen := map[key]bool{}
	for i := range real.Structure.Elements {
		el := &real.Structure.Elements[i]
		k := key{el.Id, el.Type}
		seen[k] = true
		u := elements[k]
		mergePlanCells(el.Cells, u.Cells)
		var childAdded bool
		el.Children, childAdded = mergePlanChildren(el.Children, u.Children)
		added = added || childAdded
	}
	for _, u := range up.Structure.Elements {
		if seen[key{u.Id, u.Type}] {
			continue
		}
		cells := make([]model.PlanCellResult, len(u.Cells))
		for i, c := range u.Cells {
			cells[i] = model.PlanCellResult{Actual: "0", Planned: c.Planned, Upcoming: c.Actual}
		}
		children, _ := mergePlanChildren([]model.PlanChildResult{}, u.Children)
		u.Cells, u.Children = cells, children
		real.Structure.Elements = append(real.Structure.Elements, u)
		added = true
	}

	savings := make(map[string]model.PlanSavingsElementResult, len(up.Structure.Savings))
	for _, sv := range up.Structure.Savings {
		savings[sv.Id] = sv
	}
	for i := range real.Structure.Savings {
		sv := &real.Structure.Savings[i]
		u := savings[sv.Id]
		for m := range sv.Cells {
			sv.Cells[m].Upcoming = "0"
			if m < len(u.Cells) {
				sv.Cells[m].Upcoming = u.Cells[m].Actual
			}
		}
	}
	return added
}

func mergePlanCells(real, up []model.PlanCellResult) {
	for i := range real {
		real[i].Upcoming = "0"
		if i < len(up) {
			real[i].Upcoming = up[i].Actual
		}
	}
}

// mergePlanChildren fills real's children from up's by id and appends the
// children only up has, with zero actuals, keeping the builder's id order.
func mergePlanChildren(real, up []model.PlanChildResult) ([]model.PlanChildResult, bool) {
	byID := make(map[string]model.PlanChildResult, len(up))
	for _, c := range up {
		byID[c.Id] = c
	}
	seen := map[string]bool{}
	for i := range real {
		c := &real[i]
		seen[c.Id] = true
		u := byID[c.Id]
		for m := range c.Cells {
			c.Cells[m].Upcoming = "0"
			if m < len(u.Cells) {
				c.Cells[m].Upcoming = u.Cells[m].Actual
			}
		}
	}
	added := false
	for _, u := range up {
		if seen[u.Id] {
			continue
		}
		cells := make([]model.PlanChildCellResult, len(u.Cells))
		for i, c := range u.Cells {
			cells[i] = model.PlanChildCellResult{Actual: "0", Upcoming: c.Actual}
		}
		u.Cells = cells
		real = append(real, u)
		added = true
	}
	if added {
		sort.Slice(real, func(i, j int) bool { return real[i].Id < real[j].Id })
	}
	return real, added
}

// reorderPlanElements is reorderElements over plan rows: the same keys
// emitPlanElements positions them with.
func reorderPlanElements(elements []model.PlanElementResult, options map[string]elementOption) {
	sortKey := func(e model.PlanElementResult) string {
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
	sortByPositionThenID(elements, func(p model.PlanElementResult) int { return p.Position }, func(p model.PlanElementResult) string { return p.Id })
}

// zeroBalances is balances with every amount "0": the upcoming opening
// balances of a window with nothing projected.
func zeroBalances(balances []model.OpeningBalanceResult) []model.OpeningBalanceResult {
	out := make([]model.OpeningBalanceResult, len(balances))
	for i, b := range balances {
		out[i] = model.OpeningBalanceResult{CurrencyId: b.CurrencyId, Amount: "0"}
	}
	return out
}

func emptyTransfers(months []model.PlanMonthTransfersResult) []model.PlanMonthTransfersResult {
	out := make([]model.PlanMonthTransfersResult, len(months))
	for i, m := range months {
		out[i] = model.PlanMonthTransfersResult{Period: m.Period, Items: []model.PlanTransferResult{}}
	}
	return out
}
