package budget

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// projectWindow projects the recurring occurrences filed in f's window, or
// returns a nil read when the window ended before the caller's current month
// or nothing is projected into it.
func (s *Service) projectWindow(ctx context.Context, f filters, now time.Time) (*projectedRead, error) {
	if !upcomingShown(ctx, f.periodEnd, now) {
		return nil, nil
	}
	return s.newProjectedRead(ctx, f, f.periodEnd)
}

// upcomingShown reports whether a window ending at periodEnd reaches the
// caller's current month: one that ended before it has no upcoming money.
func upcomingShown(ctx context.Context, periodEnd, now time.Time) bool {
	return periodEnd.After(localMonth(now, reqctx.Location(ctx)))
}

// addUpcoming fills result's upcoming fields: the same builder run a second
// time over the projected recurring occurrences instead of the stored
// transactions.
func (s *Service) addUpcoming(ctx context.Context, b *budgetAggregate, now time.Time, pr *projectedRead, f filters, result *model.BudgetResult) error {
	sp := *s
	sp.read = pr
	up, err := sp.buildBudget(ctx, b, f, now)
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
// fields; a row the projected build lacks gets "0".
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
			el.Upcoming, el.BudgetUpcoming = "0", "0"
			el.Children, _ = mergeChildrenUpcoming(el.Children, nil)
			continue
		}
		el.Upcoming, el.BudgetUpcoming = u.Spent, u.BudgetSpent
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
			Upcoming: u.Spent, BudgetUpcoming: u.BudgetSpent,
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
	reorderRows(elements, options,
		func(e model.ParentElementResult) (string, int, *string) { return e.Id, e.Type, e.FolderId },
		func(e *model.ParentElementResult) *int { return &e.Position })
}

// reorderRows orders rows by sort key with the Uncategorized rows last, then
// assigns dense per-folder positions and sorts by them. The two Uncategorized
// rows share an id and an empty key, so their type breaks the tie: expense
// before income, the order the builders append them in.
func reorderRows[T any](rows []T, options map[string]elementOption, key func(T) (id string, typ int, folder *string), position func(*T) *int) {
	sortKey := func(r T) string {
		id, typ, _ := key(r)
		return string(options[elementKey(id, model.ElementType(typ))].sortKey)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		xi, xt, _ := key(rows[i])
		yi, yt, _ := key(rows[j])
		if xl, yl := xi == model.UncategorizedID, yi == model.UncategorizedID; xl != yl {
			return !xl
		}
		if kx, ky := sortKey(rows[i]), sortKey(rows[j]); kx != ky {
			return kx < ky
		}
		if xi != yi {
			return xi < yi
		}
		return xt < yt
	})
	perFolder := map[string]int{}
	for i := range rows {
		_, _, folderID := key(rows[i])
		folder := ""
		if folderID != nil {
			folder = *folderID
		}
		*position(&rows[i]) = perFolder[folder]
		perFolder[folder]++
	}
	sortByPositionThenID(rows,
		func(r T) int { return *position(&r) },
		func(r T) string { id, _, _ := key(r); return id })
}

// addPlanUpcoming fills the plan's upcoming fields the way addUpcoming does
// for get-budget: BuildBudgetPlan run again over the projected occurrences of
// the window. A window that ended before the current month, or one nothing is
// projected into, keeps the zeros BuildBudgetPlan set.
func (s *Service) addPlanUpcoming(ctx context.Context, b *budgetAggregate, f filters, months int, result *model.BudgetPlanResult) error {
	pr, err := s.projectWindow(ctx, f, s.clock.Now())
	if err != nil || pr == nil {
		return err
	}
	sp := *s
	sp.read = pr
	up, err := sp.buildBudgetPlan(ctx, b, f, months)
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
// by id).
//
// A row or child only the projection has (a tag, an Uncategorized row, an
// archived row or child the builder shows on activity) is added with zero
// actuals, as projected money shows it the way real money would. Savings rows
// are never added: the only ones the builder hides are deleted accounts, whose
// booked closing balance the projection cannot supply. It reports whether
// anything was added, as positions then need recomputing.
func mergePlanUpcoming(real *model.BudgetPlanResult, up model.BudgetPlanResult) bool {
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
	reorderRows(elements, options,
		func(e model.PlanElementResult) (string, int, *string) { return e.Id, e.Type, e.FolderId },
		func(e *model.PlanElementResult) *int { return &e.Position })
}
