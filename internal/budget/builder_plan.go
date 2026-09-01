package budget

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// BuildBudgetPlan assembles the plan sheet: window months, opening balances,
// per-month rates, and the row structure. Unlike BuildBudget it never walks
// month-by-month over transactions — the by-month repo queries return the
// whole window in one pass each (the endpoint stays O(1) queries in the
// window size; only the rate lookups are per-month, and those read the small
// rates table).
func (s *Service) BuildBudgetPlan(ctx context.Context, userID vo.Id, b *budgetAggregate, from time.Time, months int) (model.BudgetPlanResult, error) {
	windowEnd := from.AddDate(0, months, 0)

	meta, err := s.buildMeta(ctx, b)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}
	f, err := s.buildFilters(ctx, userID, b, from, windowEnd)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}

	monthsList := make([]time.Time, months)
	monthStrs := make([]string, months)
	for i := range monthsList {
		m := from.AddDate(0, i, 0)
		monthsList[i] = m
		monthStrs[i] = m.Format(datetime.DateLayout)
	}

	opening, err := s.buildOpeningBalances(ctx, b.budget.CurrencyID, f, from)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}

	rates := make([]model.PlanMonthRatesResult, 0, months)
	for i, m := range monthsList {
		monthRates, rerr := s.buildAverageRates(ctx, m, m.AddDate(0, 1, 0))
		if rerr != nil {
			return model.BudgetPlanResult{}, rerr
		}
		rates = append(rates, model.PlanMonthRatesResult{Period: monthStrs[i], Rates: monthRates})
	}

	transfers, err := s.buildPlanTransfers(ctx, b.budget.CurrencyID, f, monthStrs, from, windowEnd)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}

	structure, err := s.buildPlanStructure(ctx, b, f, monthsList)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}

	return model.BudgetPlanResult{
		Meta:            meta,
		Months:          monthStrs,
		OpeningBalances: opening,
		CurrencyRates:   rates,
		Transfers:       transfers,
		Structure:       structure,
	}, nil
}

// buildPlanTransfers files TransfersByMonth's rows under their window month,
// one entry per month (empty Items when nothing crossed), items ordered
// budget currency first then by currency id.
func (s *Service) buildPlanTransfers(ctx context.Context, budgetCurrencyID vo.Id, f filters, monthStrs []string, from, windowEnd time.Time) ([]model.PlanMonthTransfersResult, error) {
	out := make([]model.PlanMonthTransfersResult, len(monthStrs))
	monthIdx := map[string]int{}
	for i, m := range monthStrs {
		out[i] = model.PlanMonthTransfersResult{Period: m, Items: []model.PlanTransferResult{}}
		monthIdx[m] = i
	}
	rows, err := s.read.TransfersByMonth(ctx, f.includedAccountIDs, from, windowEnd)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		i, ok := monthIdx[r.Month]
		if !ok {
			continue
		}
		out[i].Items = append(out[i].Items, model.PlanTransferResult{
			CurrencyId: r.CurrencyID,
			In:         vo.NewDecimal(r.In).String(),
			Out:        vo.NewDecimal(r.Out).String(),
		})
	}
	budgetCur := budgetCurrencyID.String()
	for i := range out {
		items := out[i].Items
		sort.SliceStable(items, func(a, b int) bool {
			if (items[a].CurrencyId == budgetCur) != (items[b].CurrencyId == budgetCur) {
				return items[a].CurrencyId == budgetCur
			}
			return items[a].CurrencyId < items[b].CurrencyId
		})
	}
	return out, nil
}

// buildOpeningBalances sums per-currency balances of the included accounts
// strictly before the window start, ordered budget currency first then
// discovery order — the same per-currency ordering rule as the budget page's
// balances block, but a different date bound: month 0's actual cells cover
// [from, from+1mo), so the seed must exclude spent_at == from exactly or the
// Balance row (seed + monthly nets) would double-count that instant.
func (s *Service) buildOpeningBalances(ctx context.Context, budgetCurrencyID vo.Id, f filters, from time.Time) ([]model.OpeningBalanceResult, error) {
	rows, err := s.read.AccountsBalancesBeforeDate(ctx, f.includedAccountIDs, from)
	if err != nil {
		return nil, err
	}
	ordered := make([]vo.Id, 0, len(f.currencyIDs))
	for _, c := range f.currencyIDs {
		if c.Equal(budgetCurrencyID) {
			ordered = append(ordered, c)
			break
		}
	}
	for _, c := range f.currencyIDs {
		if !c.Equal(budgetCurrencyID) {
			ordered = append(ordered, c)
		}
	}
	out := make([]model.OpeningBalanceResult, 0, len(ordered))
	for _, cid := range ordered {
		out = append(out, model.OpeningBalanceResult{
			CurrencyId: cid.String(),
			Amount:     sumBalances(rows, cid.String()).String(),
		})
	}
	return out, nil
}

// planElement / planChild are the in-progress plan rows accumulated before the
// single BulkConvert resolves every (element, month) actual.
type planElement struct {
	id         string
	typ        model.ElementType
	name       string
	icon       string
	ownerID    *string
	currencyID vo.Id
	isArchived bool
	folderID   *string
	sortKey    sortkey.Key
	sortLast   bool
	planned    []string // per month; "" = no limit row
	hasActual  bool     // any convert item accumulated in the window
	children   []planChild
}

type planChild struct {
	id         string
	typ        model.ElementType
	name       string
	icon       string
	ownerID    string
	isArchived bool
	hasActual  bool
}

// planKey / planChildKey are the per-month BulkConvert result keys.
func planKey(monthIdx int, index string) string {
	return fmt.Sprintf("plan%d_%s", monthIdx, index)
}
func planChildKey(monthIdx int, index, subIndex string) string {
	return fmt.Sprintf("plan%d_%s_%s", monthIdx, index, subIndex)
}

// buildPlanStructure emits all folders plus every plan row.
func (s *Service) buildPlanStructure(ctx context.Context, b *budgetAggregate, f filters, monthsList []time.Time) (model.PlanStructureResult, error) {
	nMonths := len(monthsList)
	monthIdx := map[string]int{}
	for i, m := range monthsList {
		monthIdx[m.Format(datetime.DateLayout)] = i
	}
	windowEnd := monthsList[0].AddDate(0, nMonths, 0)

	sorted := append([]*model.BudgetFolder(nil), b.folders...)
	sortBudgetFolders(sorted)
	folders := make([]model.BudgetFolderResult, 0, len(sorted))
	for i, fl := range sorted {
		folders = append(folders, model.BudgetFolderResult{Id: fl.ID.String(), Name: fl.Name, Position: i})
	}

	// --- window data: three whole-window queries ---
	expenseCategoryIDs := make([]vo.Id, 0, len(f.categories))
	for idStr := range f.categories {
		id, err := vo.ParseId(idStr)
		if err != nil {
			return model.PlanStructureResult{}, err
		}
		expenseCategoryIDs = append(expenseCategoryIDs, id)
	}
	spendRows, err := s.read.SpendingByMonth(ctx, expenseCategoryIDs, f.includedAccountIDs, monthsList[0], windowEnd)
	if err != nil {
		return model.PlanStructureResult{}, err
	}
	limitRows, err := s.read.LimitsByMonth(ctx, b.budget.ID, monthsList[0], windowEnd)
	if err != nil {
		return model.PlanStructureResult{}, err
	}
	incomeRows, err := s.read.IncomeByMonth(ctx, f.includedAccountIDs, monthsList[0], windowEnd)
	if err != nil {
		return model.PlanStructureResult{}, err
	}

	// planned lookup: elementKey -> per-month amount strings.
	limits := map[string][]string{}
	for _, l := range limitRows {
		i, ok := monthIdx[l.Month]
		if !ok {
			continue
		}
		key := elementKey(l.ExternalID, model.ElementType(l.Type))
		row := limits[key]
		if row == nil {
			row = make([]string, nMonths)
			limits[key] = row
		}
		row[i] = vo.NewDecimal(l.Amount).String()
	}
	plannedFor := func(index string) []string {
		if row, ok := limits[index]; ok {
			return row
		}
		return make([]string, nMonths)
	}

	options := s.elementOptions(b)
	budgetCurrencyID := b.budget.CurrencyID
	elementCurrency := func(index string) vo.Id {
		if opt, ok := options[index]; ok && opt.currencyID != nil {
			return *opt.currencyID
		}
		return budgetCurrencyID
	}

	// envelope membership + stored envelope side (default expense when no row).
	envelopeCats, err := s.envelopeCategories(ctx, b)
	if err != nil {
		return model.PlanStructureResult{}, err
	}
	envelopeType := map[string]model.ElementType{}
	for _, e := range b.elements {
		if e.Type == model.ElementEnvelope || e.Type == model.ElementIncomeEnvelope {
			envelopeType[e.ExternalID.String()] = e.Type
		}
	}
	// categoryID -> owning envelope external id, split by the ENVELOPE's side:
	// only side-matching membership hides a category from the top level (a
	// residual dirty cross-side link must not trap a row invisibly).
	// Ownership is deterministic regardless of ListEnvelopes' (unordered) row
	// order: the membership maps are built over an id-sorted copy, so the
	// lowest envelope id always wins a doubly-claimed category.
	envelopesByID := append([]*model.BudgetEnvelope(nil), b.envelopes...)
	sort.Slice(envelopesByID, func(i, j int) bool { return envelopesByID[i].ID.String() < envelopesByID[j].ID.String() })
	expenseChildOf := map[string]string{}
	incomeChildOf := map[string]string{}
	for _, env := range envelopesByID {
		income := envelopeType[env.ID.String()] == model.ElementIncomeEnvelope
		for _, catID := range envelopeCats[env.ID.String()] {
			if income {
				if _, ok := incomeChildOf[catID]; !ok {
					incomeChildOf[catID] = env.ID.String()
				}
			} else {
				if _, ok := expenseChildOf[catID]; !ok {
					expenseChildOf[catID] = env.ID.String()
				}
			}
		}
	}
	toConvert := map[string][]model.ConvertItem{}
	addActual := func(el *planElement, i int, index string, amount vo.DecimalNumber, currencyID vo.Id, to vo.Id) {
		item := model.ConvertItem{
			PeriodStart: monthsList[i], PeriodEnd: monthsList[i].AddDate(0, 1, 0),
			From: currencyID, To: to, Amount: amount,
		}
		toConvert[planKey(i, index)] = append(toConvert[planKey(i, index)], item)
		el.hasActual = true
	}

	var elements []*planElement

	// --- envelopes, both sides ---
	for _, env := range b.envelopes {
		typ := model.ElementEnvelope
		catSource := f.categories
		childTyp := model.ElementCategory
		childOf := expenseChildOf
		if envelopeType[env.ID.String()] == model.ElementIncomeEnvelope {
			typ = model.ElementIncomeEnvelope
			catSource = f.incomeCategories
			childTyp = model.ElementIncomeCategory
			childOf = incomeChildOf
		}
		index := elementKey(env.ID.String(), typ)
		el := &planElement{
			id: env.ID.String(), typ: typ, name: env.Name, icon: env.Icon,
			ownerID: nil, currencyID: elementCurrency(index), isArchived: env.IsArchived,
			folderID: optFolder(options[index]), sortKey: optSortKey(options[index]),
			planned: plannedFor(index),
		}
		for _, catID := range envelopeCats[env.ID.String()] {
			if childOf[catID] != env.ID.String() {
				continue // claimed by an earlier envelope, or wrong side: first-envelope-wins, matches where spending is filed
			}
			cat, ok := catSource[catID]
			if !ok {
				continue // wrong side or non-participant: renders on its own side
			}
			el.children = append(el.children, planChild{
				id: catID, typ: childTyp, name: cat.Name, icon: cat.Icon,
				ownerID: cat.OwnerID, isArchived: cat.IsArchived,
			})
		}
		elements = append(elements, el)
	}
	elementByID := map[string]*planElement{}
	for _, el := range elements {
		elementByID[el.id] = el
	}

	// --- tags (leaf rows) + standalone expense categories + expense Uncategorized ---
	tagEls := map[string]*planElement{}
	for tagIDStr, tag := range f.tags {
		index := elementKey(tagIDStr, model.ElementTag)
		el := &planElement{
			id: tagIDStr, typ: model.ElementTag, name: tag.Name, icon: "tag",
			ownerID: strPtr(tag.OwnerID), currencyID: elementCurrency(index), isArchived: tag.IsArchived,
			folderID: optFolder(options[index]), sortKey: optSortKey(options[index]),
			planned: plannedFor(index),
		}
		tagEls[tagIDStr] = el
		elements = append(elements, el)
	}
	catEls := map[string]*planElement{}
	for catIDStr, cat := range f.categories {
		if _, inEnvelope := expenseChildOf[catIDStr]; inEnvelope {
			continue
		}
		index := elementKey(catIDStr, model.ElementCategory)
		el := &planElement{
			id: catIDStr, typ: model.ElementCategory, name: cat.Name, icon: cat.Icon,
			ownerID: strPtr(cat.OwnerID), currencyID: elementCurrency(index), isArchived: cat.IsArchived,
			folderID: optFolder(options[index]), sortKey: optSortKey(options[index]),
			planned: plannedFor(index),
		}
		catEls[catIDStr] = el
		elements = append(elements, el)
	}
	expenseUncat := &planElement{
		id: model.UncategorizedID, typ: model.ElementCategory,
		name: model.UncategorizedName, icon: model.UncategorizedIcon,
		currencyID: budgetCurrencyID, sortLast: true, planned: make([]string, nMonths),
	}
	elements = append(elements, expenseUncat)

	incomeCatEls := map[string]*planElement{}
	for catIDStr, cat := range f.incomeCategories {
		if _, inEnvelope := incomeChildOf[catIDStr]; inEnvelope {
			continue
		}
		index := elementKey(catIDStr, model.ElementIncomeCategory)
		el := &planElement{
			id: catIDStr, typ: model.ElementIncomeCategory, name: cat.Name, icon: cat.Icon,
			ownerID: strPtr(cat.OwnerID), currencyID: elementCurrency(index), isArchived: cat.IsArchived,
			folderID: optFolder(options[index]), sortKey: optSortKey(options[index]),
			planned: plannedFor(index),
		}
		incomeCatEls[catIDStr] = el
		elements = append(elements, el)
	}
	incomeUncat := &planElement{
		id: model.UncategorizedID, typ: model.ElementIncomeCategory,
		name: model.UncategorizedName, icon: model.UncategorizedIcon,
		currencyID: budgetCurrencyID, sortLast: true, planned: make([]string, nMonths),
	}
	elements = append(elements, incomeUncat)

	// --- file the expense spending rows ---
	for _, row := range spendRows {
		i, ok := monthIdx[row.Month]
		if !ok {
			continue
		}
		cid, perr := vo.ParseId(row.CurrencyID)
		if perr != nil {
			return model.PlanStructureResult{}, perr
		}
		amount := vo.NewDecimal(row.Amount)
		switch {
		case row.TagID != nil && *row.TagID != "":
			// The tag wins over the category — a tagged expense belongs to its
			// tag row, exactly as on the budget page.
			if el, ok := tagEls[*row.TagID]; ok {
				addActual(el, i, elementKey(el.id, model.ElementTag), amount, cid, el.currencyID)
			}
		case row.CategoryID == nil:
			addActual(expenseUncat, i, elementKey(model.UncategorizedID, model.ElementCategory), amount, cid, budgetCurrencyID)
		default:
			catID := *row.CategoryID
			if envID, inEnvelope := expenseChildOf[catID]; inEnvelope {
				parent := elementByID[envID]
				parentIndex := elementKey(envID, model.ElementEnvelope)
				addActual(parent, i, parentIndex, amount, cid, parent.currencyID)
				subIndex := elementKey(catID, model.ElementCategory)
				toConvert[planChildKey(i, parentIndex, subIndex)] = append(toConvert[planChildKey(i, parentIndex, subIndex)], model.ConvertItem{
					PeriodStart: monthsList[i], PeriodEnd: monthsList[i].AddDate(0, 1, 0),
					From: cid, To: parent.currencyID, Amount: amount,
				})
				for ci := range parent.children {
					if parent.children[ci].id == catID {
						parent.children[ci].hasActual = true
					}
				}
			} else if el, ok := catEls[catID]; ok {
				addActual(el, i, elementKey(catID, model.ElementCategory), amount, cid, el.currencyID)
			}
		}
	}

	for _, row := range incomeRows {
		i, ok := monthIdx[row.Month]
		if !ok {
			continue
		}
		cid, perr := vo.ParseId(row.CurrencyID)
		if perr != nil {
			return model.PlanStructureResult{}, perr
		}
		amount := vo.NewDecimal(row.Amount)
		// Rows with no category — or a category that is not a participant
		// income category — land in the income Uncategorized row, so no income
		// ever vanishes from the sheet's Balance math.
		catID := ""
		if row.CategoryID != nil {
			catID = *row.CategoryID
		}
		if catID == "" || (f.incomeCategories[catID].ID == "" && incomeChildOf[catID] == "") {
			addActual(incomeUncat, i, elementKey(model.UncategorizedID, model.ElementIncomeCategory), amount, cid, budgetCurrencyID)
			continue
		}
		if envID, inEnvelope := incomeChildOf[catID]; inEnvelope {
			parent := elementByID[envID]
			parentIndex := elementKey(envID, model.ElementIncomeEnvelope)
			addActual(parent, i, parentIndex, amount, cid, parent.currencyID)
			subIndex := elementKey(catID, model.ElementIncomeCategory)
			toConvert[planChildKey(i, parentIndex, subIndex)] = append(toConvert[planChildKey(i, parentIndex, subIndex)], model.ConvertItem{
				PeriodStart: monthsList[i], PeriodEnd: monthsList[i].AddDate(0, 1, 0),
				From: cid, To: parent.currencyID, Amount: amount,
			})
			for ci := range parent.children {
				if parent.children[ci].id == catID {
					parent.children[ci].hasActual = true
				}
			}
		} else if el, ok := incomeCatEls[catID]; ok {
			addActual(el, i, elementKey(catID, model.ElementIncomeCategory), amount, cid, el.currencyID)
		}
	}

	// BulkConvert's top-level (periodStart, periodEnd) doubles as month 0's rate
	// range (its "currentKey" is monthKey(periodStart), so any item dated in
	// that same month reuses this range instead of getting its own entry — see
	// BulkConvert's needed-map construction). Passing the WHOLE window here
	// would make month 0 use the window-wide snapped average (typically the
	// LATEST month's rate) instead of its own month's average, while every
	// other month in the window still gets its own correctly-scoped range.
	// Bounding it to just month 0 keeps that implicit range consistent with
	// what every other month already gets explicitly.
	converted, err := s.convertor.BulkConvert(ctx, monthsList[0], monthsList[0].AddDate(0, 1, 0), toConvert)
	if err != nil {
		return model.PlanStructureResult{}, err
	}

	result := s.emitPlanElements(elements, converted, nMonths)
	return model.PlanStructureResult{Folders: folders, Elements: result}, nil
}

// emitPlanElements prunes, renders and positions the accumulated rows.
// Visibility: non-archived elements always render (the sheet is the place to
// START planning a dormant row; hiding empties is a client-side toggle) —
// except tags, which render only when they participate (any actual or planned
// in the window, the budget-page rule applied per-window) and the synthetic
// Uncategorized rows, which render only with actuals. Archived anything needs
// window activity.
func (s *Service) emitPlanElements(elements []*planElement, converted map[string]vo.DecimalNumber, nMonths int) []model.PlanElementResult {
	zero := vo.NewDecimal("0")
	get := func(key string) vo.DecimalNumber {
		if v, ok := converted[key]; ok {
			return v
		}
		return zero
	}
	hasPlanned := func(el *planElement) bool {
		for _, p := range el.planned {
			if p != "" {
				return true
			}
		}
		return false
	}

	var kept []*planElement
	result := []model.PlanElementResult{}
	for _, el := range elements {
		active := el.hasActual || hasPlanned(el)
		switch {
		case el.sortLast: // synthetic Uncategorized rows
			if !el.hasActual {
				continue
			}
		case el.typ == model.ElementTag:
			if !active {
				continue
			}
		default:
			if el.isArchived && !active {
				continue
			}
		}

		index := elementKey(el.id, el.typ)
		cells := make([]model.PlanCellResult, nMonths)
		for i := 0; i < nMonths; i++ {
			cells[i] = model.PlanCellResult{Actual: get(planKey(i, index)).String(), Planned: el.planned[i]}
		}

		children := []model.PlanChildResult{}
		for _, ch := range el.children {
			if ch.isArchived && !ch.hasActual {
				continue
			}
			subIndex := elementKey(ch.id, ch.typ)
			chCells := make([]model.PlanChildCellResult, nMonths)
			for i := 0; i < nMonths; i++ {
				chCells[i] = model.PlanChildCellResult{Actual: get(planChildKey(i, index, subIndex)).String()}
			}
			children = append(children, model.PlanChildResult{
				Id: ch.id, Type: int(ch.typ.Int16()), Name: ch.name, Icon: ch.icon,
				IsArchived: boolToInt(ch.isArchived), OwnerUserId: ch.ownerID, Cells: chCells,
			})
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Id < children[j].Id })

		kept = append(kept, el)
		result = append(result, model.PlanElementResult{
			Id: el.id, Type: int(el.typ.Int16()), Name: el.name, Icon: el.icon,
			CurrencyId: el.currencyID.String(), IsArchived: boolToInt(el.isArchived),
			FolderId: el.folderID, OwnerUserId: el.ownerID,
			Cells: cells, Children: children,
		})
	}

	// Dense 0-based position within the folder over sort-key order, synthetic
	// rows trailing — the same algorithm as assignElementPositions.
	order := make([]int, len(kept))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		x, y := kept[order[a]], kept[order[b]]
		if x.sortLast != y.sortLast {
			return !x.sortLast
		}
		if x.sortKey != y.sortKey {
			return x.sortKey < y.sortKey
		}
		return x.id < y.id
	})
	perFolder := map[string]int{}
	for _, i := range order {
		folder := ""
		if kept[i].folderID != nil {
			folder = *kept[i].folderID
		}
		result[i].Position = perFolder[folder]
		perFolder[folder]++
	}
	sortByPositionThenID(result,
		func(p model.PlanElementResult) int { return p.Position },
		func(p model.PlanElementResult) string { return p.Id })
	return result
}
